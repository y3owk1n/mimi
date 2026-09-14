# Configuration

mimi reads one TOML config file. The first of these that exists wins:

1. The path given with `--config` (or `-c`)
2. `$XDG_CONFIG_HOME/mimi/config.toml`
3. `~/.config/mimi/config.toml`
4. `mimi.toml` in the current directory

When none exists, commands use `$XDG_CONFIG_HOME/mimi/config.toml` if that
variable is set, and `~/.config/mimi/config.toml` otherwise.

```bash
mimi config init       # write the default config, overwriting any file already there
mimi config validate   # check for errors
mimi config dump       # print the loaded config, defaults filled in, as JSON
mimi config reload     # send SIGHUP to the running daemon
```

`mimi config dump` prints camelCase JSON keys, such as `hookTimeoutSecs`.

---

## Reloading

`mimi config reload`, the systray's reload item, and an edit to the config
file on disk all trigger the same reload. A bad config (for example an invalid
`title` regex) is rejected whole. The previous config stays in place and the
failure is logged.

**Reloadable**. Every reload picks these up:

- `settings.hook_timeout_secs`
- `settings.hook_shell`
- `settings.resize_debounce_ms`
- `[hooks]` (every hook kind)
- `[tiling]` (every field)
- `[border]` (every field)

**Restart-only**. The daemon reads these once at startup. Restart it to apply
them, with `mimi stop && mimi start`, or `mimi services restart` for the
installed service:

- `settings.log_file`
- `settings.log_level`
- `settings.log_format`
- `settings.max_hook_workers`
- `settings.pid_file`
- `settings.socket_file`
- `systray.enabled`
- `systray.show_workspace_number`

**Reinstall-only**. `mimi services install` writes this into the launchd
plist. Run it again to apply a change. A restart does not apply it:

- `settings.service_path`

A reload still applies everything reloadable. It then logs a warning that
names the settings it could not apply and what each needs:

```text
config reloaded; restart required for changed restart-only settings
  trigger=sighup restart_only=["settings.log_level","settings.max_hook_workers"]
config reloaded; run `mimi services install` for changed reinstall-only settings
  trigger=sighup reinstall_only=["settings.service_path"]
```

The daemon compares against the config it started with, so the warning
repeats on every reload until the file and the running daemon agree. With
`systray.enabled = true`, a line under **Reload Config** shows the last
outcome (`Reloaded 14:32`, `Reloaded 14:32 — restart required`,
`Reloaded 14:32 — run mimi services install`, `Reload failed 14:32`).

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
resize_debounce_ms = 250                     # on_window_resize and on_window_move debounce window
service_path = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin" # PATH for the installed service. reinstall-only
```

`log_format` selects the console encoder only. The `log_file` log is always
JSON, so you can pipe it through `jq`. An unrecognized `log_format` logs a
warning and falls back to `text`. An unrecognized `log_level` falls back to
`info`.

### socket_file

The Unix socket the daemon listens on. Every `mimi action` reads `socket_file`
from its config and checks it:

- When a daemon is listening, the action runs on the daemon.
- When nothing is listening, the CLI runs the action in its own process.

If the daemon's socket and the CLI's config disagree, actions run directly
without any message. See
[Troubleshooting](TROUBLESHOOTING.md#mimi-action-runs-but-seems-to-ignore-the-running-daemon).

The daemon also writes `minsizes.json` to the socket's directory. It holds the
minimum sizes tiling has learned per application, so a restart lays those
windows out right the first time. See [Tiling](TILING.md#when-nothing-happens), item 7.

### service_path

`mimi services install` writes this `PATH` into the launchd plist. The
installed daemon and every hook it runs inherit it. Set it when a hook works
from your shell but not under the service, because a login shell's `PATH`
never reaches a launchd agent.

```toml
[settings]
service_path = "/Users/me/.local/bin:/run/current-system/sw/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
```

- It replaces the whole `PATH`. Keep the directories you still need.
- Write absolute directories. `~` is not expanded here.
- Run `mimi services install` after changing it. A restart keeps the old `PATH`.
- `mimi start` run by hand ignores it and inherits the caller's `PATH`.
- The nix-darwin and home-manager modules render their own agent. Set
  `services.mimi.extraEnvironment.PATH` there instead.

### Debug logging

`log_level = "debug"` logs one `"event"` line per routed event and one
`"hook matched"` or `"hook skipped"` line per hook on that kind. These lines
carry only counts, IDs, kinds, PIDs, booleans, and the hook's `index` within
its kind. mimi never logs window titles or `run` commands.

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
command_timeout_secs = 1     # kill a before or after line the layout returned past this
relayout_on_drag = false     # a window the user moves or resizes runs a pass too
# gap = 12                   # points between windows; unset follows the macOS tiled-window margin

[[tiling.rules]]
app = "Finder"             # glob on the application name; ! in front negates
manage = false             # the layout never sees a window this rule names

[[tiling.rules]]
bundle_id = "com.apple.*"  # glob on the bundle identifier
title = "^Settings$"       # regular expression on the window title
manage = false

[[tiling.rules]]
narrower_than = 400        # points; both have to hold when both are set
shorter_than = 300
manage = false

[tiling.animation]
enabled = false       # move the frames into place over time instead of at once
duration_ms = 150     # how long the move takes, 1 to 1000
easing = "ease-out"   # linear, ease-in, ease-out or ease-in-out

[tiling.dropzone]
enabled = false            # while you drag a window, show where the layout would put it
color = "#30e2e2e3"        # the zone's fill, #rrggbb or #aarrggbb
outline_color = "#e2e2e3"  # its outline
outline_width = 2          # points, up to 32
radius = 12                # corner radius in points
```

mimi ships no layout. `layout` is a command line that mimi runs through
`settings.hook_shell`. The layout reads the tiling input as JSON on stdin and
prints the frames to apply as JSON on stdout. `examples/tiling/` holds layouts
to copy, and [TILING.md](TILING.md) explains how to write one.

`enabled = true` requires `layout` and Accessibility permission. Without the
permission, tiling stays off and the daemon logs a warning. `mimi tiling
preview` runs the layout once and prints what it would apply, whether or not
tiling is enabled.

### When a pass runs

The daemon runs the layout after `debounce_ms` of quiet following any of these
events: a window is created, closed, focused, minimized or unminimized, an
application activates, hides, unhides or quits, or the space changes. It runs
the layout once per display that has a window on it.

Moves and resizes run a pass only with `relayout_on_drag = true`, because the
engine's own writes are moves too. The engine remembers where it placed each
window. It treats a move or resize as the user's only when a placed window is
somewhere else, so a layout can read a dragged edge as a new split ratio or a
drop as a swap. A move within a second of the engine's write counts as that
write settling.

`mimi tiling cmd` runs one pass. While that pass runs, one more command of the
same name may wait, and the daemon drops any further copies. A held key
therefore does not queue up passes.

### layout_mode

- `oneshot` (default): every pass starts a new process.
- `resident`: one process, started once. Each pass writes one line of input
  to stdin and reads one line from stdout, so an interpreted layout pays its
  startup cost once. The layout must flush after every line and exit when
  stdin closes. The daemon restarts it if it exits, backing off up to five
  seconds after repeated failures. The daemon stops it when tiling is
  disabled, when a reload changes `layout`, `timeout_secs` or
  `settings.hook_shell`, and when mimi quits.

The layouts in `examples/tiling/` work in both modes.

### Input and output

The input holds the JSON that `mimi query windows` and `mimi query displays`
print, narrowed to one display. It adds the display itself, the event that
woke the daemon, the space in front, the `gap` to leave, and the `state` the
layout returned last time for that display and space (`null` the first time).

`event.kind` is a hook event name, or one of `startup`, `reload`, `preview`,
`relayout` and `command`. A `command` event carries `name` and `args`, and the
layout decides what they mean. A `window_resize` or `window_move` event
carries `windows`, the numbers of the windows the user dragged.

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

The output holds the frames in the shape `mimi action apply_frames` takes and
the `state` to hand back next time. It can also carry `focus`, a window number
to focus, plus `before`, `after`, `stacks` and `unmanaged`, which
[TILING.md](TILING.md#output) describes. Printing nothing changes nothing:

```json
{"frames":[{"number":4242,"frame":{"x":0,"y":25,"width":720,"height":875}}],
 "state":{"master":4242}}
```

`version` changes when a field is renamed, removed or changes meaning. When a
layout exits non-zero, times out, or prints the wrong shape, the daemon logs
it and applies nothing.

### Animation

With `[tiling.animation]` enabled, windows move to their frames over
`duration_ms` along the `easing` curve. When a pass lands mid-animation, the
windows continue from where they are. A pass that asks for the frames already
in flight lets the animation finish. A window arriving from off screen slides
in. A window the user just dragged moves at once, and a layout can opt a
window out with `animate: false` on its frame. The focus a layout asks for
lands before its frames move.

**How the windows move.** The daemon sets each window's frame through its
application every ten milliseconds along the curve, with one thread per
application. It turns off each application's enhanced accessibility interface
for the duration, so that applications do not animate each step themselves.
It needs only the Accessibility permission tiling already has, and captures
nothing on screen. A pass returns as soon as the windows start moving. The
layout's `after` lines still wait for the animation to end, so a command that
reads a window's frame reads the final one.

How smooth the motion looks depends on each application. A window whose
application is slow to answer moves in fewer, larger steps, and a resize costs
an application several times what a move does. Each animation logs a
`Mimi: animation:` line through NSLog with how many windows and frames it
stepped, how long it took, and its slowest write. Look there when one looks
rough.

### Drop zone

With `[tiling.dropzone]` enabled and `relayout_on_drag` on, dragging a tiled
window shows where the layout would put it if you let go. The zone is a
translucent rounded frame that moves ahead of the drag as the answer changes.
While the button is down, mimi runs the layout the same way the drop will,
with a `window_move` or `window_resize` event naming the window, at most once
every 40 ms. It draws the frame the layout returns for that window. Nothing is
applied and no state is kept until the drop. The zone disappears when you
release the button, and the settled drag then runs the layout for real.

A layout that ignores drags shows the window's own frame. A layout whose
answer depends on where the window is dropped, like the shipped `strip.py`,
shows the column the window would join. The zone needs Accessibility and
tiling enabled, and every key is reloadable.

### Stacked windows

```toml
[tiling.stackbar]
enabled = true
step = 10
taper = 6
radius = -1
color = "#b0636366"
far_color = "#30636366"
```

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | Draw the stacks a layout names |
| `step` | `10` | How much of each window behind shows above the one in front, in points, up to 40 |
| `taper` | `6` | How much narrower each window behind is drawn, on either side, in points, up to 40 |
| `radius` | `-1` | The corner radius the cards follow, or `-1` to follow each window's own |
| `color` | `#b0636366` | The card nearest the window in front |
| `far_color` | `#30636366` | The furthest card, so a deep stack fades away |

A layout can put several windows in one frame, so that only the one with
keyboard focus is visible. Nothing on screen shows that the others are there,
so this setting draws them as a deck of cards. The windows before the one in
front show above it and the ones after it show below. Each is a little
narrower than the one in front of it, so the position of the front window
within the deck is its position in the stack.

The deck stays inside the frame the layout set aside. mimi takes the room it
needs out of the window in front, so a stack uses no more space than one
window and never covers a neighbour. The cards take at most a quarter of the
frame, so a small frame shows fewer cards than a deep stack holds.

mimi draws only the stacks a layout names in its output's `stacks` key. A
layout that names none draws nothing. `examples/tiling/stacked.py`,
`strip.py`, `bsp.py` and `monocle.py` all name stacks, and
[TILING.md](TILING.md) has the contract. Colours are `#rrggbb` or
`#aarrggbb`, alpha first. Stacks need Accessibility and tiling enabled, like
the drop zone, and every key is reloadable.

---

### Rules

`[[tiling.rules]]` names windows the layout never sees. Each entry sets
`manage` and one or more conditions. `app` is a glob on the application
name and `bundle_id` a glob on the bundle identifier. `title` is a regular
expression on the window title. `narrower_than` and `shorter_than` are
sizes in points the window has to be under. Every condition set has to
hold. `*` is the only glob wildcard, and a pattern that starts with `!`
matches everything the rest does not, as a hook filter does. An entry that
sets no condition is rejected.

A window matching a rule with `manage = false` is left out of the layout's
input, so the layout cannot frame it, a drag of it runs no pass, and it gets
no drop zone. Rules are read in order and the last one that matches decides,
so a rule with `manage = true` after a broader one takes those windows back.
The shipped layouts carry no float rules of their own.

## Borders

```toml
[border]
enabled = true
width = 4                   # points, 1 to 32
# radius = 12               # force one corner radius; unset follows each window's own, 0 is square
active_color = "#e2e2e3"    # the focused window, #rrggbb or #aarrggbb
inactive_color = "#414141"  # every other window
```

With `[border]` enabled, the daemon draws a ring around every window on the
spaces in front, as JankyBorders does. The focused window gets `active_color`
and every other window gets `inactive_color`. A border follows its window as
it moves or resizes, and changes colour when focus moves. It disappears when
the window closes, leaves the space, or its application hides. A full-screen
window gets no border.

Each border is a separate mimi window, ordered directly under the window it
belongs to. The border never covers another application's content, and a
window that overlaps a bordered window covers its border too. A colour with an
alpha channel draws a translucent border.

Windows have different corner radii. With `radius` unset, each border uses the
radius the window server reports for its window, which macOS 26 added. Earlier
releases report none, and every border uses 12 points there. Set `radius` to
draw every border with one radius.

Borders need Accessibility, as window hooks do, because focus changes and
moves come from the same observers. Without it, borders stay off and the
daemon logs a warning. When the tiling animation moves a window, its border
moves with it. A border also follows a window you drag at every step, since
the window server reports each move.

---

## Hooks

The hook kinds below are the complete set. `mimi config validate` rejects any
other key under `[hooks]`, and the daemon logs a warning about it on startup
and reload.

### Application Lifecycle

| Hook | Fires when |
| ---- | ---------- |
| `on_app_activate` | App comes to foreground |
| `on_app_deactivate` | App loses foreground |
| `on_app_launch` | App process starts |
| `on_app_quit` | App process terminates |
| `on_app_hide` | App hidden (Cmd+H) |
| `on_app_unhide` | Hidden app shown again |

### Window events (requires Accessibility)

| Hook | Fires when |
| ---- | ---------- |
| `on_window_focus` | Focused window changes |
| `on_window_title_change` | Active window title changes |
| `on_window_created` | New window opens |
| `on_window_closed` | Window closes |
| `on_window_resize` | Window resize completes (debounced by `resize_debounce_ms`) |
| `on_window_move` | Window move completes (debounced the same way as resize) |
| `on_window_minimize` | Window minimized to the Dock |
| `on_window_unminimize` | Minimized window restored |

### Workspace events

| Hook | Fires when |
| ---- | ---------- |
| `on_workspace_changed` | Active Mission Control space changes |

### Hook entry format

An entry is either a plain command string or an inline table with filters:

```toml
[hooks]
on_app_activate = ["echo active: $mimi_APP_NAME"]

on_window_focus = [
  { run = "echo focus", app = "Slack", async = true }
]
```

| Field | Description |
| ----- | ----------- |
| `run` | Shell command (required) |
| `app` | Filter by app name (glob, `*` is the only wildcard) |
| `bundle_id` | Filter by bundle ID (glob, `*` is the only wildcard) |
| `title` | Filter by window title (regex) |
| `space` | Filter by the space now in front, as a 1-based number. Workspace hooks only |
| `timeout_secs` | Override `settings.hook_timeout_secs` |
| `async` | Run in background (default: false) |

**Negating a filter.** A filter that begins with `!` matches everything the
pattern does not. `app = "!Safari"` fires for every app except Safari. A
filter that is only `!` is rejected. Write `space` as a bare number, or as a
string when negated:

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

If mimi could not resolve the space for a workspace event, that event fails
every `space` filter and passes every negated one.

---

## Environment variables

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

Write references without your own quotes. mimi substitutes each value as a
single shell token wrapped in single quotes, so a crafted window title cannot
break out of the command. A reference inside your own double quotes shows the
wrapping quotes literally.

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
