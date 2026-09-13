# Architecture

mimi is a macOS window and space utility with three execution paths:

1. CLI actions, direct: one-shot commands (`mimi action ...`) run in the CLI process.
2. CLI actions over daemon IPC: the same commands, sent over a Unix socket when the daemon is running.
3. Hook daemon: a background process that fires shell hooks on app, window, and space events.

Every path calls native macOS APIs through CGO. None of them requires disabling SIP.

---

## CLI actions

```
mimi action <subcommand>
  -> internal/action        (argument parsing and the branch logic of each action)
  -> action.Desktop         (the seam: one interface, values only)
  -> internal/geometry      (pure window geometry, no macOS)
  -> internal/native        (Objective-C + SkyLight, and the CGO window/space wrappers)
```

### The Desktop seam

`internal/action` never calls macOS directly. Every action runs on an
`action.Executor`, which holds one `action.Desktop`. That interface lists what
an action needs from the machine: the Accessibility permission, the focusable
windows on the active space, window frames, screens, and Mission Control
spaces. The package-level `action.ExecuteCommand` runs on an Executor bound to
the real desktop. `cmd`, `internal/ipc` and `internal/tiling` call it.

Two rules hold the seam in place:

- Only data crosses it. A window enumerates as an opaque `action.WindowID`
  plus its PID and window number, and frames cross as `geometry.Rect`. The
  native adapter (`native_desktop.go`, the one file in `internal/action` that
  imports `internal/native`) owns the `AXUIElement` references behind those
  ids. It releases each generation when the next lookup replaces it. No action
  above the seam holds or releases a native reference.
- The consumer owns the interface. The interface, its value types and the
  adapter all live in `internal/action`. `internal/native` is a CGO package
  that knows nothing about the abstraction above it.

As a result, unit tests run the branch logic against a fake desktop, on any
machine, with no Accessibility grant. That logic covers focus cycling and its
wrap-around, directional focus over windows whose frames cannot be read, the
Mission Control guards, the space range check, and the `next`/`prev`
arithmetic. The integration tier in `internal/baseline` still drives
`internal/native` directly, to check that the real desktop behaves the way the
fake assumes.

| Action | API |
| ------ | --- |
| `focus_window` | Accessibility (`AXUIElement`) |
| `focus_app` | `NSRunningApplication` to find the app, private SkyLight for its real windows and their spaces (`SLSCopyWindowsWithOptionsAndTags`, `SLSCopySpacesForWindows`), the `space` gesture, then Accessibility to raise. An app with no window is reopened through `NSWorkspace` |
| `space` | Synthetic dock-swipe gesture via `CGEvent` |
| `move_window_to_space` | Private SkyLight: `SLSBridgedMoveWindowsToManagedSpaceOperation` run through `SLSPerformAsynchronousBridgedWindowManagementOperation`, falling back to `SLSMoveWindowsToManagedSpace`. With `--follow`, the `space` gesture and an Accessibility raise |
| `move_window_to_display` | Accessibility (`AXUIElement`), `NSScreen` for the display list, `SLSSetActiveMenuBarDisplayIdentifier` to activate the target display |
| `resize_window` | Accessibility (`AXUIElement`), `NSScreen` for the visible frame |
| `apply_frames` | Accessibility (`AXUIElement`), one frame write per window named by window number |

After posting a gesture or a move, the native code pumps the run loop briefly so the event completes before the process exits.

When the daemon is running, `mimi action` first tries the Unix socket at `settings.socket_file`. The request is one line of JSON carrying `ipc.ProtocolVersion` and the typed command. The daemon runs the action on a worker goroutine locked to one OS thread and returns the result. If the socket is unavailable, the CLI falls back to direct execution. A daemon from a different build rejects a request whose version does not match its own. The CLI then prints a warning that names the mismatch and the fix, and runs the action on the direct path.

Each action builds its command through a constructor in `internal/action` (`NewFocusWindowCommand`, `NewFocusAppCommand`, `NewSpaceCommand`, `NewMoveWindowToSpaceCommand`, `NewMoveWindowToDisplayCommand`, `NewResizeWindowCommand`, `NewApplyFramesCommand`), and the constructor validates the arguments. A malformed argument is rejected before either path is chosen. No socket is opened, and the message is the same whether or not a daemon is listening.

### Queries

`mimi query` reads the desktop through the same seam and prints one line of
JSON. Queries run on the direct path only. A read has no side effect that
needs ordering against the daemon's actions, so sending it over the socket
would need a wire change for no gain.

### Tiling runs the user's program

mimi ships no layout. `query windows` and `query displays` give a program
every window on the active space, with frames in window coordinates, and
`apply_frames` writes a whole layout back in one action. `internal/tiling` is
the engine that runs such a program for the daemon. It subscribes to the bus
for window, application and space events and settles each burst into one
pass. On each pass it hands the program the JSON the queries print, the event,
and the state the program returned last time for that space. Then it applies
the frames the program prints. The program is a pure function of its input.
The engine owns the timing, the state, and the desktop.

Two rules stop the engine from reacting to its own writes. Resize and move
events do not wake it, because every frame it writes produces one. The
exception is `relayout_on_drag`, and even then a resize within a grace period
after an apply counts as the engine's own. Its desktop work also runs on the
IPC server's action worker (`ipc.Server.Serialize`). A pass therefore never
interleaves with an action arriving over the socket and never releases window
references that action holds.

`mimi tiling relayout`, `mimi tiling cmd <name>`, `mimi tiling state` and
`mimi tiling reset` reach the engine over the same socket as the actions, as
the `tiling` action. The server runs that action on the connection's
goroutine (`ipc.Server.HandleDirect`) rather than the worker, because the
engine queues its own work on the worker. With no daemon, `relayout` and `cmd`
run an engine inside the CLI with no state, and `state` and `reset` print an
empty answer. `mimi tiling preview` always runs in the CLI.

Two helpers hang off the engine. `internal/dropzone` shows where a dragged
window would land by asking the engine for a drop preview while the mouse
button is down. `internal/stackbar` draws the windows a layout stacked in one
frame, from the stacks the engine reports after each pass.

`examples/tiling/` holds programs to copy. mimi does not load them, and they
carry no promise beyond the JSON contract (`tiling.Input`, `tiling.Output`,
versioned by `tiling.InputVersion`).

---

## Hook daemon

```
NSWorkspace + AX observers (workspace.m, axobserver.m)
  -> internal/native (Go exports)
  -> internal/observe (event router)
  -> events.Bus
  -> hooks.Executor -> shell commands
  -> tiling.Engine, border.Engine, event log writer
```

### Observers

- App lifecycle: `NSWorkspace` app notifications (activate, deactivate, launch, quit, hide, unhide). They feed app hooks and attach or detach the per-process AX observers, so they run when app or window hooks are configured.
- AX window events: focus, title change, create, close, move, resize, minimize, unminimize. The router debounces moves and resizes.
- Space changes: `NSWorkspaceActiveSpaceDidChangeNotification`, observed when `on_workspace_changed` hooks are configured or tiling is enabled.

### Event bus

A pub-sub bus that fans each event out to subscribers without blocking. A full subscriber buffer drops the event and increments a drop counter. Subscribers are the hook executor, the tiling engine, the border engine, and the event log writer when `settings.log_file` is set. Each subscriber can pass a kind filter so the bus skips events it does not want.

### Hook executor

The executor matches events against configured hooks, applies the `app`, `bundle_id` and `title` filters, and runs shell commands with `mimi_*` environment variables.

### Border

`internal/border` keeps a border under every window on the spaces in front, in one colour for the focused window and another for the rest. The native side draws each border as an `NSWindow` ordered below its window, and follows drags through SkyLight window-server notifications (`SLSRegisterConnectionNotifyProc`, `SLSRequestNotificationsForWindows`).

### Config reload

`internal/config` watches the config file, and the daemon also reloads on `SIGHUP`. A reload updates the hook registry, the observers, the tiling engine, the border, the drop zone and the stack bars in place.

---

## Package layout

```
cmd/mimi/           CLI entry point and commands
cmd/genman/         Man page generator (just genman)
internal/
  action/           Action dispatch (focus_window, focus_app, space,
                    move_window_to_space, move_window_to_display,
                    resize_window, apply_frames, tiling), the queries, the
                    Desktop seam and its native adapter
  geometry/         Pure window geometry: rects, resize presets, nearest window
  native/           Objective-C + CGO: AX window wrappers, Mission Control
                    space operations, screen queries, the observer bridge, and
                    the border, drop zone and stack bar overlays
  ipc/              Unix socket client and server, versioned request envelope
  observe/          Hook daemon event routing
  events/           Event kinds and the event bus
  hooks/            Hook registry and executor
  tiling/           The engine that runs the user's layout program on events
  border/           The engine that keeps a border under every window on events
  dropzone/         Drop preview while dragging a tiled window
  stackbar/         Stack indicator for windows a layout stacked
  config/           TOML config loading, validation and file watching
  daemon/           Daemon lifecycle and config reload
  service/          launchd service install, start, stop and status
  logging/          Structured logger and event log writer
  errors/           Coded errors (derrors)
  paths/            Path helpers
  permissions/      Accessibility permission checks
  systray/          Optional menu bar UI
  baseline/         Recorded window behaviour used as a test oracle
```

---

## Permissions

Accessibility is required for:

- All `mimi action` commands
- `mimi query window` and `mimi query windows`
- `mimi tiling preview`, `relayout` and `cmd`
- Tiling, borders, the drop zone and stack bars in the daemon
- Window hooks (`on_window_*`)

App lifecycle hooks (`on_app_*`), workspace hooks (`on_workspace_changed`), `mimi query space`, `mimi query displays` and `mimi query margins` do not require Accessibility.

---

## Platform notes

Space switching and window-to-space moves use undocumented private APIs that may break on macOS updates. They are provided as-is for personal automation, with no stability guarantee.
