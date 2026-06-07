# Architecture

mimi is a macOS window and space utility with three execution paths:

1. **CLI actions (direct)** — immediate one-shot commands (`mimi action …`)
2. **CLI actions (via daemon IPC)** — same commands routed over a Unix socket when the daemon is running
3. **Hook daemon** — background process that fires shell hooks on window/space events

Both paths use native macOS APIs via CGO. No SIP disable is required.

---

## CLI Actions

```
mimi action <subcommand>
  → internal/action
  → internal/window / internal/space
  → internal/native (Objective-C + SkyLight)
```

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

Only window and workspace observers are active:

- **App lifecycle** (internal) — installs per-app AX observers when window hooks are configured
- **AX window events** — focus, title change, create, close, resize (debounced)
- **Workspace polling** — detects Mission Control space changes when `on_workspace_changed` hooks are configured

System observers (power, USB, network, clipboard, audio, etc.) have been removed.

### Event Bus

Non-blocking pub-sub fan-out. Subscribers: hook executor and optional event log writer.

### Hook Executor

Matches events against configured hooks, applies filters (`app`, `bundle_id`, `title`), runs shell commands with `mimi_*` environment variables.

---

## Package Layout

```
cmd/mimi/           CLI entry point and commands
internal/
  action/           Action dispatch (focus_window, space, move_window_to_space)
  window/           Go wrappers for AX window APIs
  space/            Mission Control space operations
  native/           All Objective-C + CGO (actions and observers)
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

Workspace hooks (`on_workspace_changed`) do not require Accessibility.

---

## Platform Notes

Space switching and window-to-space moves use undocumented private APIs that may break on macOS updates. They are provided as-is for personal automation workflows, not as guaranteed-stable APIs.
