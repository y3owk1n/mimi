# Configuration

mimi reads a TOML config from `~/.config/mimi/config.toml` (or `$XDG_CONFIG_HOME/mimi/config.toml`).

```bash
mimi config init       # create default config
mimi config validate   # check for errors
mimi config dump       # print resolved config as JSON
mimi config reload     # reload running daemon (SIGHUP)
```

---

## Settings

```toml
[settings]
log_file = "~/.local/share/mimi/mimi.log"   # optional; omit for console-only
log_level = "info"                           # debug | info | warn | error
log_format = "text"                          # text | json
hook_timeout_secs = 10
hook_shell = "/bin/sh"
max_hook_workers = 4
pid_file = "~/.local/share/mimi/mimi.pid"
resize_debounce_ms = 250                     # on_window_resize debounce window
```

---

## Systray

```toml
[systray]
enabled = true
show_workspace_number = true   # show active space number in menu bar
```

---

## Hooks

### Application Lifecycle

| Hook | Fires when |
| ---- | ---------- |
| `on_app_activate` | App comes to foreground |
| `on_app_deactivate` | App loses foreground |
| `on_app_launch` | App process starts |
| `on_app_quit` | App process terminates |
| `on_app_hide` | App hidden (⌘H) |
| `on_app_unhide` | Hidden app shown again |

### Window events (requires Accessibility)

| Hook | Fires when |
| ---- | ---------- |
| `on_window_focus` | Focused window changes |
| `on_window_title_change` | Active window title changes |
| `on_window_created` | New window opens |
| `on_window_closed` | Window closes |
| `on_window_resize` | Window resize completes (debounced) |

### Workspace events

| Hook | Fires when |
| ---- | ---------- |
| `on_workspace_changed` | Active Mission Control space changes |

### Hook entry format

```toml
[hooks]
on_window_focus = ["echo 'focus: $mimi_APP_NAME'"]

on_window_focus = [
  { run = "notify-send focus", app = "Slack", async = true }
]
```

| Field | Description |
| ----- | ----------- |
| `run` | Shell command (required) |
| `app` | Filter by app name (glob) |
| `bundle_id` | Filter by bundle ID (exact) |
| `title` | Filter by window title (regex) |
| `timeout_secs` | Override global timeout |
| `async` | Run in background (default: false) |

---

## Environment Variables

Every hook receives:

| Variable | Description |
| -------- | ----------- |
| `mimi_EVENT` | Event kind (e.g. `app_activate`, `window_focus`, `workspace_changed`) |
| `mimi_EVENT_ID` | Unique event UUID |
| `mimi_APP_NAME` | App display name |
| `mimi_BUNDLE_ID` | Bundle identifier |
| `mimi_PID` | Process ID |
| `mimi_WINDOW_TITLE` | Window title (window events only) |
| `mimi_TIMESTAMP` | RFC3339 timestamp |
| `mimi_WINDOWS_COUNT` | Window count (workspace events only) |
| `mimi_INFO` | JSON workspace info (workspace events only) |

Use `$mimi_APP_NAME` or `${mimi_WINDOW_TITLE}` in hook commands.

---

## Example

```toml
[hooks]
on_app_activate = [
  { run = "echo 'active: $mimi_APP_NAME'", async = true }
]

on_window_focus = [
  { run = "echo focus >> ~/window.log", app = "Code", async = true }
]

on_workspace_changed = [
  "echo 'switched space' >> ~/space.log"
]
```
