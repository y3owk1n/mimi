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

`mimi config reload`, the systray's reload item, and an edit to the config
file on disk all trigger the same reload. A bad config (for example an invalid
`title` regex) is rejected whole: the previous config stays in place and the
failure is logged.

**Reloadable**. Every reload picks these up:

- `settings.hook_timeout_secs`
- `settings.hook_shell`
- `settings.resize_debounce_ms`
- `[hooks]` (every hook kind)
- `[tiling]` (every field)

**Restart-only**. The daemon reads these once at startup. Restart it to apply
(`mimi daemon stop && mimi daemon start`):

- `settings.log_file`
- `settings.log_level`
- `settings.log_format`
- `settings.max_hook_workers`
- `settings.pid_file`
- `settings.socket_file`
- `systray.enabled`
- `systray.show_workspace_number`

**Reinstall-only**. `mimi services install` writes this into the launchd
plist. Run it again to apply. A restart does not:

- `settings.service_path`

A reload still applies everything reloadable, then logs a warning naming the
settings it could not apply and what each needs:

```text
config reloaded; restart required for changed restart-only settings
  trigger=sighup restart_only=["settings.log_level","settings.max_hook_workers"]
config reloaded; run `mimi services install` for changed reinstall-only settings
  trigger=sighup reinstall_only=["settings.service_path"]
```

The comparison is against the config the daemon started with, so the warning
repeats on every reload until the file and the running daemon agree. With
`systray.enabled = true`, a line under **Reload Config** shows the last
outcome (`Reloaded 14:32`, `Reloaded 14:32 — restart required`,
`Reloaded 14:32 — run mimi services install`, `Reload failed 14:32`).

A test checks these lists against the classification on the config type.

---

## Settings

```toml
[settings]
log_file = "~/.local/share/mimi/mimi.log"   # optional; omit for console-only. restart-only
log_level = "info"                           # debug | info | warn | error. restart-only
log_format = "text"                          # text | json, console output only. restart-only
hook_timeout_secs = 10
hook_shell = "/bin/sh"
max_hook_workers = 4                         # restart-only
pid_file = "~/.local/share/mimi/mimi.pid"    # restart-only
socket_file = "~/.local/share/mimi/mimi.sock" # restart-only
resize_debounce_ms = 250                     # on_window_resize debounce window
service_path = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin" # PATH for the installed service. reinstall-only
```

`log_format` selects the console encoder only. The `log_file` log is always
JSON, so piping it through `jq` keeps working. An unrecognized value warns and
falls back to `text`.

### socket_file

The Unix socket the daemon listens on. Every `mimi action` reads `socket_file`
from its config and checks it:

- When a daemon is listening, the action runs on the daemon.
- When nothing is listening, the CLI runs the action in its own process.

A mismatch between the daemon's socket and the CLI's config means actions
silently run directly. See
[Troubleshooting](TROUBLESHOOTING.md#mimi-action-runs-but-seems-to-ignore-the-running-daemon).

### service_path

The `PATH` written into the launchd plist by `mimi services install`, and so
the `PATH` the installed daemon and every hook inherit. Set it when a hook
works from your shell but not under the service: a login shell's `PATH` never
reaches a launchd agent.

```toml
[settings]
service_path = "/Users/me/.local/bin:/run/current-system/sw/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
```

- It is the whole `PATH`, not an addition. Keep the directories you still need.
- Write absolute directories. `~` is not expanded here.
- Run `mimi services install` after changing it. A restart keeps the old `PATH`.
- `mimi start` by hand ignores it and inherits the caller's `PATH`.
- The nix-darwin and home-manager modules render their own agent. Set
  `services.mimi.extraEnvironment.PATH` there instead.

### Debug logging

`log_level = "debug"` logs one `"event"` line per routed event and one
`"hook matched"` / `"hook skipped"` line per hook on that kind. These carry
only counts, IDs, kinds, PIDs, booleans, and the hook's `index` within its
kind. Window titles and `run` commands are never logged.

One exception: at `debug`, the `"hook ok"` line includes the hook's captured
stdout and stderr as `output` (trimmed, capped at 64 KiB). At `info` and above
no hook output reaches the log.

---

## Systray

```toml
[systray]
enabled = true                 # restart-only
show_workspace_number = true   # show active space number in menu bar. restart-only
```

---

## Tiling

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/columns.py"
layout_mode = "oneshot"   # or "resident", one process kept running between passes
debounce_ms = 100     # settle a burst of window events into one pass
timeout_secs = 5      # kill the layout past this
relayout_on_drag = false     # a window the user moves or resizes runs a pass too
# gap = 12                   # points between windows; unset follows the macOS tiled-window margin

[tiling.animation]
enabled = false       # move the frames into place over time instead of at once
duration_ms = 150     # how long the move takes
easing = "ease-out"   # linear, ease-in, ease-out or ease-in-out
```

mimi ships no layout. `layout` is a command line, run through
`settings.hook_shell`, that reads the tiling input as JSON on stdin and prints
the frames to apply as JSON on stdout. `examples/tiling/` holds layouts to
copy, and [TILING.md](TILING.md) is the guide to writing one.

`enabled = true` requires `layout` and Accessibility permission. Without the
permission tiling stays off and a warning says so. `mimi tiling preview` runs
the layout once and prints what it would apply, whether or not tiling is
enabled.

### When a pass runs

The daemon runs the layout after `debounce_ms` of quiet following a window
create, close or focus, an application hide, unhide or quit, or a space
change. It runs once per display that has a window on it.

Moves and resizes run a pass only with `relayout_on_drag = true`, since the
engine's own writes are moves too. The engine remembers where it placed each
window and treats a move or resize as the user's only when a placed window is
elsewhere, so a layout can read a dragged edge as a new split ratio or a drop
as a swap. A move within a second of the engine's write counts as that write
settling.

`mimi tiling cmd` runs one pass. While it runs, one more command of the same
name may wait and the daemon drops further copies, so a held key does not
queue up passes.

### layout_mode

- `oneshot` (default): every pass starts a new process.
- `resident`: one process, started once. Each pass writes one line of input
  to stdin and reads one line from stdout, so an interpreted layout starts
  up only once. The layout must flush after every line and exit when
  stdin closes. The daemon restarts it if it exits, backing off up to five
  seconds after repeated failures, and stops it on disable, on a reload that
  changes the command, and on quit.

The layouts in `examples/tiling/` work in both modes.

### Input and output

Input is the JSON `mimi query windows` and `mimi query displays` print,
narrowed to one display, plus the display, the waking event, the space in
front, the `gap` to leave, and the `state` the layout returned last time for
that display and space (`null` the first time).

`event.kind` is a hook event name, or `startup`, `reload`, `preview`,
`relayout` or `command`. A `command` carries `name` and `args`, whose meaning
is the layout's to decide. A `window_resize` or `window_move` carries
`windows`, the numbers the user dragged.

```json
{"version":1,
 "event":{"kind":"window_created","app":"Safari","bundleId":"com.apple.Safari","pid":501},
 "display":{"index":1,"id":1,"frame":{...},"visible":{...}},
 "space":2,
 "gap":8,
 "displays":[{"index":1,"id":1,"frame":{...},"visible":{...}}],
 "focused":0,
 "windows":[{"number":4242,"pid":501,"app":"Safari","bundleId":"com.apple.Safari","title":"...","frame":{...}}],
 "state":null}
```

Output is the frames in the shape `mimi action apply_frames` takes, the
`state` to hand back next time, and optionally `focus`, a window number to
focus once applied. Printing nothing changes nothing:

```json
{"frames":[{"number":4242,"frame":{"x":0,"y":25,"width":720,"height":875}}],
 "state":{"master":4242}}
```

`version` changes when a field is renamed, removed or changes meaning. The
daemon logs a layout that exits non-zero, times out, or prints the wrong
shape, and applies nothing.

### Animation

With `[tiling.animation]` enabled, windows move to their frames over
`duration_ms` along the `easing` curve. A pass that lands mid-animation
continues from where the windows are. A window the user just dragged moves at
once, and a layout can opt a window out with `animate: false` on its frame.

**Animation requires Screen Recording permission.** macOS lets a process
move another application's window, but not fade, transform or reorder it.
So mimi takes a picture of each window and of the rest of the screen, moves
the real windows to their frames at once behind that still, and has the
window server slide the pictures into place. No application does any work
while the animation runs. Taking those pictures is a screen capture, and
macOS gates every screen capture behind Screen Recording.

- The daemon asks for the permission once, at startup and on reload, and only
  while the animation is enabled. With it disabled, the default, mimi
  captures nothing and never asks.
- Until it is granted, windows move at once and a warning says so.
- Grant it in System Settings > Privacy & Security > Screen & System Audio
  Recording, then restart mimi. macOS applies the grant on the next start.

---

## Hooks

The hook kinds below are the complete set. `mimi config validate` rejects any
other key under `[hooks]`, and the daemon warns about it on startup and reload.

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
| `on_window_move` | Window move completes (debounced, same window as resize) |

### Workspace events

| Hook | Fires when |
| ---- | ---------- |
| `on_workspace_changed` | Active Mission Control space changes |

### Hook entry format

```toml
[hooks]
on_window_focus = ["echo focus: $mimi_APP_NAME"]

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
| `space` | Filter by the space now in front, as a 1-based number. Workspace hooks only |
| `timeout_secs` | Override global timeout |
| `async` | Run in background (default: false) |

**Negating a filter:** a filter that begins with `!` matches everything the
pattern does not. `app = "!Safari"` fires for every app but Safari. A filter
that is only `!` is rejected. Write `space` as a bare number, or as a string
when negated:

```toml
[hooks]
on_workspace_changed = [
  { run = "sketchybar --trigger work_mode", space = 2 },
  { run = "sketchybar --trigger normal_mode", space = "!2" },
]
on_window_focus = [
  { run = "echo focus", app = "!Terminal" }
]
```

A workspace event whose space could not be resolved fails every `space`
filter and passes every negated one.

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
| `mimi_SPACE_INDEX` | 1-based index of the space now in front (workspace events only) |
| `mimi_SPACE_COUNT` | How many Mission Control spaces there are (workspace events only) |

Write references **without** your own quotes. Each value is substituted as a
single, self-quoted shell token, so a crafted window title cannot break out of
the command. A reference inside your own double quotes shows the wrapping
quotes literally.

```toml
# Correct, the value quotes itself:
on_window_title_change = [{ run = "notify-send $mimi_WINDOW_TITLE" }]
```

---

## Example

```toml
[hooks]
on_app_activate = [
  { run = "echo active: $mimi_APP_NAME", async = true }
]

on_window_focus = [
  { run = "echo focus >> ~/window.log", app = "Code", async = true }
]

on_workspace_changed = [
  "echo switched to space $mimi_SPACE_INDEX >> ~/space.log"
]
```
