# Configuration

mimi reads a TOML config from `~/.config/mimi/config.toml` (or `$XDG_CONFIG_HOME/mimi/config.toml`).

```bash
mimi config init       # create default config
mimi config validate   # check for errors
mimi config dump       # print resolved config as JSON
mimi config reload     # reload running daemon (SIGHUP)
```

---

## Reloading

`mimi config reload` (SIGHUP) and the systray's reload menu item both trigger
the same reload, and the daemon also picks up an edit to the config file on
disk automatically (fsnotify). All three routes reload `[hooks]` and the
subset of `[settings]` listed below; a bad config (e.g. an invalid `title`
regex) leaves the previous, working config in place and logs the failure
rather than applying it partially.

**Restart-only** — these fields are read once at daemon startup and are not
picked up by a reload; changing them requires restarting the daemon
(`mimi daemon stop && mimi daemon start`, or equivalent):

- `[systray]` (both `enabled` and `show_workspace_number`)
- `settings.log_file`
- `settings.log_level`
- `settings.pid_file`
- `settings.socket_file`

Everything else in `[settings]` (`log_format`, `hook_timeout_secs`,
`hook_shell`, `max_hook_workers`, `resize_debounce_ms`) and all of `[hooks]`
take effect on the next reload.

---

## Settings

```toml
[settings]
log_file = "~/.local/share/mimi/mimi.log"   # optional; omit for console-only — restart-only
log_level = "info"                           # debug | info | warn | error — restart-only
log_format = "text"                          # text | json — console output only
hook_timeout_secs = 10
hook_shell = "/bin/sh"
max_hook_workers = 4
pid_file = "~/.local/share/mimi/mimi.pid"    # restart-only
socket_file = "~/.local/share/mimi/mimi.sock" # restart-only
resize_debounce_ms = 250                     # on_window_resize debounce window
```

`log_format` selects the **console** encoder only: `text` is the human-readable
line (colorized when the console is a terminal), `json` is one JSON object per
line with no color. The `log_file` log is always JSON, whatever `log_format`
says, so anything already piping that file through `jq` keeps working. An
unrecognized value logs a warning and falls back to `text`.

### socket_file

`socket_file` is the Unix socket the daemon listens on and `mimi action …`
looks for it on. Every `mimi action` invocation resolves its config path
first (the default path when `-c`/`--config` is not given) and reads
`socket_file` from it, then checks that socket:

- **A daemon is listening** — the action is sent over the socket and runs on
  the daemon's dedicated OS thread.
- **Nothing is listening** — the CLI falls back to running the action
  directly, in the CLI's own process.

Both paths produce the same result, but not with the same timing: the daemon
path is a socket round trip; the direct path runs in-process with no daemon
involved at all. If you run the daemon with a non-default `socket_file`, that
value is what routes `mimi action` to it — a mismatch between the two (or a
daemon that hasn't picked up a changed `socket_file`, since it is
restart-only) means actions silently execute directly instead of reaching the
daemon. See [Troubleshooting](TROUBLESHOOTING.md#mimi-action-runs-but-seems-to-ignore-the-running-daemon)
for how to tell which path an action actually took.

### Debug logging

`log_level = "debug"` adds two extra lines per routed event: a router
`"event"` line (one per event reaching the bus) and, for each hook
registered on that event's kind, an executor `"hook matched"` or
`"hook skipped"` line.

Both record only counts, IDs, kinds, PIDs, and booleans: `kind`, `app`,
`bundle`, `pid`, `event_id`, the hook's `index` within its kind (0-based,
scoped to hooks of the same kind — enough to tell two hooks apart in the
log), the `reason` a hook was skipped (a fixed string, e.g.
`"app filter mismatch"`), and `title_present` (whether the window has a
non-empty title). They never record the window title itself or a hook's
`run` command text, even at `debug`.

The `"hook ok"` / `"hook timed out"` / `"hook failed"` lines emitted after a
hook finishes running follow the same rule: they identify the hook by the
same `index`, and none of them records the `run` command text.

One gap remains. The `"hook ok"` line still logs the hook's own captured
stdout and stderr as `output` (trimmed, and capped at 64 KiB). That is hook
output, i.e. a user payload, and it is an open exception to AGENTS.md's
"never log ... other user payloads" rule. It is emitted only at
`log_level = "debug"`; at `info` and above no hook output reaches the log,
and the `"hook failed"` line reports the failure through `exit` alone.

---

## Systray

```toml
[systray]
enabled = true                 # restart-only
show_workspace_number = true   # show active space number in menu bar — restart-only
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
