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
| `space` | Synthetic dock-swipe gesture via `CGEvent` |
| `move_window_to_space` | Private SkyLight (`SLSMoveWindowsToManagedSpace`) |

CLI actions pump the run loop briefly after posting events so gestures complete before the process exits.

When the daemon is running, `mimi action` first tries the Unix socket at `settings.socket_file`. The daemon executes the action on a dedicated OS thread and returns the result. If the socket is unavailable, the CLI falls back to direct execution.

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
                    resize_window), the Desktop seam and its native adapter
  native/           All Objective-C + CGO: AX window wrappers, Mission Control
                    space operations, screen queries, and the observer bridge
  observe/          Hook daemon event routing
  hooks/            Hook registry and executor
  config/           TOML config loading
  daemon/           Daemon lifecycle
  permissions/      Accessibility permission checks
  systray/          Optional menu bar UI
```

---

## Permissions

**Accessibility** is required for:

- All `mimi action` commands
- Window hooks (`on_window_*`)

App lifecycle hooks (`on_app_*`) and workspace hooks (`on_workspace_changed`) do not require Accessibility.

---

## Platform Notes

Space switching and window-to-space moves use undocumented private APIs that may break on macOS updates. They are provided as-is for personal automation workflows, not as guaranteed-stable APIs.
