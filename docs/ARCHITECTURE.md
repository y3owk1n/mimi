# Architecture

mimi is a macOS window and space utility with three execution paths:

1. **CLI actions (direct)** — immediate one-shot commands (`mimi action …`)
2. **CLI actions (via daemon IPC)** — same commands routed over a Unix socket when the daemon is running
3. **Hook daemon** — background process that fires shell hooks on app, window, and space events

Both paths use native macOS APIs via CGO. No SIP disable is required.

---

## CLI Actions

```
mimi action <subcommand>
  → internal/action        (argument parsing and the branch logic of each action)
  → action.Desktop         (the seam: one interface, values only)
  → internal/geometry      (pure window geometry, no macOS)
  → internal/native        (Objective-C + SkyLight, and the CGO window/space wrappers)
```

### The Desktop seam

`internal/action` never reaches macOS directly. Every action runs on an
`action.Executor`, which holds one `action.Desktop` — the interface describing
everything an action needs from the machine: the accessibility permission, the
focusable windows on the active space, window frames, screens, and Mission
Control spaces. The package-level `action.Execute` runs on an Executor bound to
the real desktop, so `internal/ipc` and `cmd` call it exactly as before.

Two properties keep the seam honest:

- **Only data crosses it.** A window enumerates as an opaque `action.WindowID`
  plus its PID, and frames cross as `geometry.Rect`. The native adapter (also in
  `internal/action`, the one file there that imports `internal/native`) owns the
  `AXUIElement` references behind those ids, and releases each generation when
  the next lookup replaces it. No action above the seam holds — or releases — a
  native reference.
- **Interfaces belong to the consumer.** The interface, its value types and the
  adapter all live in `internal/action`; `internal/native` stays a pure CGO
  module that knows nothing about the abstraction above it.

The payoff is that the branch logic — focus cycling and its wrap-around,
directional focus over windows whose frames cannot be read, the Mission Control
guards, the space range check and the `next`/`prev` arithmetic — is exercised in
unit tests against a fake desktop, on any machine, with no Accessibility grant.
`internal/baseline`'s integration tier still drives `internal/native` directly:
it is what checks that the real desktop behaves the way the fake pretends to.

| Action | API |
| ------ | --- |
| `focus_window` | Accessibility (`AXUIElement`) |
| `focus_app` | Private SkyLight for the window list and their spaces (`SLSCopyWindowsWithOptionsAndTags`, `SLSCopySpacesForWindows`), the `space` gesture, then Accessibility to raise |
| `space` | Synthetic dock-swipe gesture via `CGEvent` |
| `move_window_to_space` | Private SkyLight (`SLSMoveWindowsToManagedSpace`) |
| `move_window_to_display` | Accessibility (`AXUIElement`), `NSScreen` for the display list |
| `resize_window` | Accessibility (`AXUIElement`), `NSScreen` for the visible frame |
| `apply_frames` | Accessibility (`AXUIElement`), one frame write per window named by window number |

CLI actions pump the run loop briefly after posting events so gestures complete before the process exits.

When the daemon is running, `mimi action` first tries the Unix socket at `settings.socket_file`. The daemon executes the action on a dedicated OS thread and returns the result. If the socket is unavailable, the CLI falls back to direct execution. It falls back the same way when a daemon is listening but rejects the request because it accepts only the version its own build speaks, printing a warning naming the mismatch and the fix — the action still does what it was asked.

Each action builds its command through the constructor `internal/action` gives it (`NewFocusWindowCommand`, `NewSpaceCommand`, `NewMoveWindowToSpaceCommand`, `NewResizeWindowCommand`), and those constructors validate as they build. A malformed argument is therefore rejected before either path is chosen — no socket is opened, and the message reads the same whether or not a daemon is listening.

### Queries

`mimi query` reads the desktop through the same seam and prints one line of
JSON. A query runs on the direct path only: it has no side effect to serialize
with the daemon's actions, so routing it over the socket would cost a wire
change and buy nothing.

### Tiling is the user's program

mimi ships no layout. `query windows` and `query displays` give a program
everything on the active space with frames in window coordinates, and
`apply_frames` writes a whole layout back in one action. `internal/tiling`
is the engine that runs such a program for the daemon: it subscribes to the
bus for window, application and space events, settles each burst into one
pass, hands the program the same JSON the queries print plus the event and
the state the program returned last time for that space, and applies the
frames it prints. The program is a pure function of its input; the engine
owns the timing, the state, and the desktop.

Two rules keep it from fighting itself. Resizes never wake it, because every
frame it writes is one. And its desktop work runs on the IPC server's action
worker (`ipc.Server.Serialize`), so a pass never interleaves with an action
arriving over the socket and releases the window references that action is
holding.

`mimi tiling relayout` and `mimi tiling cmd <name>` reach the engine over
the same socket as the actions, as the `tiling` action; the server hands
that one action to the engine on the connection's goroutine
(`ipc.Server.HandleDirect`) rather than the worker, because the engine will
queue its own work on the worker. With no daemon the CLI runs an engine of
its own with no state.

`examples/tiling/` holds programs to copy; they are not loaded by mimi and
carry no promise beyond the JSON contract (`tiling.Input`, `tiling.Output`,
versioned by `tiling.InputVersion`).

---

## Hook Daemon

```
NSWorkspace + AX observers (workspace.m, axobserver.m)
  → internal/native (Go exports)
  → internal/observe (event router)
  → events.Bus
  → hooks.Executor
  → shell commands
```

### Observers

- **App lifecycle** — subscribes to `NSWorkspace` app notifications (activate, deactivate, launch, quit, hide, unhide) for both app hooks and AX observer management
- **AX window events** — focus, title change, create, close, resize (debounced)
- **Workspace polling** — detects Mission Control space changes when `on_workspace_changed` hooks are configured

### Event Bus

Non-blocking pub-sub fan-out. Subscribers: hook executor and optional event log writer.

### Hook Executor

Matches events against configured hooks, applies filters (`app`, `bundle_id`, `title`), runs shell commands with `mimi_*` environment variables.

---

## Package Layout

```
cmd/mimi/           CLI entry point and commands
internal/
  action/           Action dispatch (focus_window, space, move_window_to_space,
                    move_window_to_display, resize_window, apply_frames),
                    the queries, the
                    Desktop seam and its native adapter
  native/           All Objective-C + CGO: AX window wrappers, Mission Control
                    space operations, screen queries, and the observer bridge
  observe/          Hook daemon event routing
  hooks/            Hook registry and executor
  tiling/           The engine that runs the user's layout program on events
  border/           The engine that keeps a border under every window on events
  config/           TOML config loading
  daemon/           Daemon lifecycle
  permissions/      Accessibility permission checks
  systray/          Optional menu bar UI
```

---

## Permissions

**Accessibility** is required for:

- All `mimi action` commands
- `mimi query window`
- Window hooks (`on_window_*`)

App lifecycle hooks (`on_app_*`), workspace hooks (`on_workspace_changed`) and `mimi query space` do not require Accessibility.

---

## Platform Notes

Space switching and window-to-space moves use undocumented private APIs that may break on macOS updates. They are provided as-is for personal automation workflows, not as guaranteed-stable APIs.
