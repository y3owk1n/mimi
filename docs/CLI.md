# CLI Usage

mimi is a macOS window and space utility. Use `mimi action` for immediate commands, or `mimi start` to run the hook daemon.

---

## Table of Contents

- [Global Flags](#global-flags)
- [Interrupting a Command](#interrupting-a-command)
- [Window & Space Actions](#window--space-actions)
- [Queries](#queries)
- [Tiling](#tiling)
- [Hook Daemon](#hook-daemon)
- [Service Management](#service-management)
- [Configuration Management](#configuration-management)

---

## Global Flags

| Flag            | Shorthand | Default | Description            |
| --------------- | --------- | ------- | ---------------------- |
| `--config, -c`  |           | auto    | Path to config file    |
| `--verbose, -v` |           | `false` | Verbose output         |
| `--version`     |           |         | Print version and exit |

---

## Interrupting a Command

A second Ctrl-C always ends the process immediately with exit status 130. What
the first one does depends on the command:

| Command                      | First Ctrl-C                                                                    |
| ---------------------------- | ------------------------------------------------------------------------------- |
| `mimi services install`      | Stops the work in progress and says where it stopped.                           |
| `mimi services uninstall`    | Stops, keeping the plist, exactly as a failed unload does.                      |
| `mimi services start`/`stop`/`restart` | Stops before or during the `launchctl` call, and says so.             |
| `mimi services status`       | Stops asking `launchctl` and prints the unknown state. Exits 0.                 |
| `mimi start`                 | Shuts the daemon down gracefully.                                               |
| `mimi action *`              | Does not reach the action; it finishes. Press Ctrl-C again to end the process.   |
| `mimi config *`              | Does not reach the command; it finishes. Each is one local file read or write.   |
| `mimi status`, `mimi stop`   | Does not reach the command; it finishes. Each is a file read and one syscall.    |
| `mimi query *`               | Does not reach the command; it finishes. Each is a few desktop reads and one line of output. |
| `mimi tiling preview`        | Kills the layout program if it is still running; nothing is applied either way. |
| `mimi tiling relayout`/`cmd` | With a daemon, as `mimi action *`. Without one, as `preview`, then the frames are applied. |

A command the first Ctrl-C did not reach still finishes and exits 0.

For `mimi start`, before the daemon installs its signal watch
(while the first-run config alert is up, say) the first Ctrl-C does nothing.
And a second Ctrl-C during a stalled shutdown skips the daemon's cleanup, so a
PID file may be left behind. `mimi status` reports it as stale and the next
`mimi start` overwrites it.

---

## Window & Space Actions

Actions run in the CLI process when no daemon is running. With a daemon,
the CLI sends them over its Unix socket (`settings.socket_file`), which is
faster than starting the action from scratch. **Accessibility permission is required.**

```bash
mimi action focus_window
mimi action focus_window --backward
mimi action focus_window --left
mimi action focus_window --right
mimi action focus_window --up
mimi action focus_window --down
mimi action focus_window --same-app
mimi action focus_window --number 4242
mimi action focus_app Safari
mimi action space 1
mimi action space next
mimi action space prev
mimi action move_window_to_space 2
mimi action move_window_to_space next
mimi action move_window_to_space prev
mimi action move_window_to_space next --follow
mimi action move_window_to_display next
mimi action resize_window left-half
mimi action resize_window center --width-percent 80 --height-percent 90
mimi action resize_window --width 1024 --height 768 --anchor cc
mimi action apply_frames < frames.json
```

### `mimi action focus_window`

Cycle keyboard focus through the focusable windows on the current space, or
move focus spatially.

| Flag         | Description                                                      |
| ------------ | ---------------------------------------------------------------- |
| `--backward` | Cycle to the previous window instead of the next                 |
| `--up`       | Move focus to the nearest window above the current one           |
| `--down`     | Move focus to the nearest window below the current one           |
| `--left`     | Move focus to the nearest window to the left of the current one  |
| `--right`    | Move focus to the nearest window to the right of the current one |
| `--same-app` | Stay within the focused window's application, cycling or directional |
| `--number <n>` | Focus the window with that window-server number |

`--same-app` is the keyboard's Cmd-backtick. It combines with `--backward` or
a direction flag, and needs a focused window to take the application from.

`--number` names one window instead of saying which way to move from the
focused one, so it combines with none of the other flags. The number is the
one `mimi query windows` reports, stable for the window's lifetime, and the
same number `mimi action apply_frames` and a layout's `frames` take:

```bash
mimi action focus_window --number 4242
```

This is the only way to focus a window without saying where it is on screen.
A layout's `before` and `after` command lines need that, and so does a hotkey
bound to a window you noted earlier. The window has to be on the current
space. `mimi action focus_app` is the one that switches space to reach a
window.

### `mimi action focus_app <name|bundle-id>`

Bring a running application's window to the front, switching space first with
the same instant gesture `mimi action space` uses. Name it by app name (case
does not matter) or bundle identifier. Pair with `open` for the not-running
case:

```bash
mimi action focus_app Safari || open -a Safari
```

- When the application is **not in front**, its most recently used window is
  chosen, wherever it is.
- When it **is in front**, each press moves to its next window, ordered by
  space then age, wrapping at the end.
- Minimized windows are skipped. A window assigned to every space is raised
  without a space switch. An application with no window is reopened as if
  its Dock icon were clicked.

### `mimi action space <number|next|prev>`

Focus a Mission Control space by 1-based index, or cycle with wrapping. Uses
a synthetic dock-swipe gesture, since no public macOS API switches spaces.
When the destination is on another display, the pointer is warped to that
display's center first and stays there. The same applies to
`move_window_to_space --follow`.

### `mimi action move_window_to_space <number|next|prev>`

Move the frontmost window to a space by 1-based index, or cycle with
wrapping. Uses private SkyLight APIs and does not require disabling SIP.

| Flag       | Description                                                      |
| ---------- | ---------------------------------------------------------------- |
| `--follow` | Switch to the destination space once the window is there         |

With `--follow`, the switch is the same dock-swipe gesture as `space`, and the
moved window is raised again once the switch lands. If the move lands but the
switch or raise fails, the error says so and the window stays on its new
space.

### `mimi action move_window_to_display <number|next|prev>`

Move the frontmost window to another display by 1-based index, or cycle with
wrapping. Displays are counted left to right, then top to bottom. The window
lands on the destination's active space and keeps the share of the display it
had. A window already on the destination does not move. **Accessibility permission
is required.**

### `mimi action resize_window [preset] [flags]`

Resize and reposition the frontmost window. Respects the macOS tiled window
margins setting, with full margins on screen edges and half margins on split
edges. Margins are skipped when they would leave the window no width or
height.

**Presets:**

| Preset         | Effect                               |
| -------------- | ------------------------------------ |
| `left-half`    | Fill the left half of the screen     |
| `right-half`   | Fill the right half of the screen    |
| `top-half`     | Fill the top half of the screen      |
| `bottom-half`  | Fill the bottom half of the screen   |
| `top-left`     | Fill the top-left quadrant           |
| `top-right`    | Fill the top-right quadrant          |
| `bottom-left`  | Fill the bottom-left quadrant        |
| `bottom-right` | Fill the bottom-right quadrant       |
| `left-third`   | Fill the left third of the screen    |
| `center-third` | Fill the middle third of the screen  |
| `right-third`  | Fill the right third of the screen   |
| `left-two-thirds`  | Fill the left two thirds of the screen  |
| `right-two-thirds` | Fill the right two thirds of the screen |
| `center`       | Center window at 60% × 80% of screen |
| `fill`         | Fill entire screen                   |

**Cycling:** `--cycle` makes `left-half` and `right-half` step through half,
two thirds, a third, then half again on repeated presses. The step is decided
from where the window is now, so it works with or without the daemon. It
takes no size, position or anchor flag, and only those two presets accept it.

**Custom sizing flags:**

| Flag                     | Description                            |
| ------------------------ | -------------------------------------- |
| `--width, -w <pixels>`   | Absolute window width in points        |
| `--height, -h <pixels>`  | Absolute window height in points       |
| `--width-percent <pct>`  | Width as percentage of screen (0–100)  |
| `--height-percent <pct>` | Height as percentage of screen (0–100) |

**Positioning flags:**

| Flag           | Description              |
| -------------- | ------------------------ |
| `--x <pixels>` | Absolute X position      |
| `--y <pixels>` | Absolute Y position      |
| `--anchor, -a` | Anchor point (see below) |

**Anchors** are two letters, vertical then horizontal, naming the point of
the window placed at the computed position:

```
tl  tc  tr       (top-left, top-center, top-right)
cl  cc  cr       (center-left, center-center, center-right)
bl  bc  br       (bottom-left, bottom-center, bottom-right)
```

**Margin control:**

| Flag          | Effect                                          |
| ------------- | ----------------------------------------------- |
| `--margin`    | Enable tiled margins (overrides system setting) |
| `--no-margin` | Disable tiled margins                           |

Give one or neither. Omitting both follows the system setting. Giving both is
rejected.

**Examples:**

```bash
mimi action resize_window left-half
mimi action resize_window center
mimi action resize_window --width 800 --height 600 --anchor cc
mimi action resize_window --width-percent 50 --height-percent 75 --anchor tl
mimi action resize_window --width 1024 --height 768 --x 100 --y 50 --anchor tl
mimi action resize_window left-half --no-margin
mimi action resize_window left-half --cycle
mimi action resize_window center --width-percent 80 --height-percent 90
```

### `mimi action apply_frames [--file path]`

Move and resize several windows on the active space in one action. Frames come
as a JSON array on stdin or from `--file`, each naming a window by the `number`
from `mimi query windows` and its frame in window coordinates.
**Accessibility permission is required.**

```
[{"number":4242,"frame":{"x":0,"y":25,"width":720,"height":875}},
 {"number":4243,"frame":{"x":720,"y":25,"width":720,"height":875}}]
```

The action attempts every frame in order. The action fails when any frame did not
land, naming each window it could not place and why. The payload is rejected
before anything moves when it is empty, names a window twice, names window 0,
or gives a frame without a positive width and height.

With `mimi query windows` and `mimi query displays`, this is all a tiling
script needs: list, decide, apply. `examples/tiling/` holds scripts to copy.

```bash
mimi action apply_frames < frames.json
mimi action apply_frames --file frames.json
my-layout | jq .frames | mimi action apply_frames
```

---

## Queries

Queries read the desktop and print one line of JSON on stdout. They never
move focus, a window, or a space, and always run in the CLI's own process. A
failed query prints nothing on stdout.

```bash
mimi query space
mimi query window
mimi query windows
mimi query displays
mimi query margins
```

Pipe through `jq` to pick one field:

```bash
mimi query space | jq .index
```

### `mimi query space`

The active space and the count, in the 1-based ordering `mimi action space`
takes. `index` is the space in front on the display holding the cursor. Needs
no Accessibility permission.

```
$ mimi query space
{"index":2,"count":5}
```

### `mimi query window`

The frontmost window, with its owner's PID and frame in window coordinates
(origin at the top-left of the primary display, y growing downward).
**Accessibility permission is required.**

```
$ mimi query window
{"pid":4242,"frame":{"x":100,"y":50,"width":1024,"height":768}}
```

### `mimi query windows`

Every focusable window on the active space, in `focus_window` cycle order.
`focused` is the index of the focused window, or -1. `number` is the window
server's number, stable for the window's lifetime, and what `apply_frames`
takes. A window whose frame cannot be read is left out. **Accessibility
permission is required.**

```
$ mimi query windows
{"focused":0,"windows":[{"number":4242,"pid":501,"app":"Safari","bundleId":"com.apple.Safari","title":"Start Page","frame":{"x":0,"y":25,"width":1440,"height":875}}]}
```

### `mimi query displays`

Every connected display, numbered as `move_window_to_display` counts them.
`frame` is the whole display and `visible` excludes the menu bar and Dock.
Both are in window coordinates. Needs no Accessibility permission.

```
$ mimi query displays
[{"index":1,"id":1,"frame":{"x":0,"y":0,"width":1440,"height":900},"visible":{"x":0,"y":25,"width":1440,"height":875}}]
```

### `mimi query margins`

The macOS tiled-window margins setting. `size` is in points. Needs no
Accessibility permission.

```
$ mimi query margins
{"enabled":true,"size":8}
```

---

## Tiling

The daemon runs the layout program named in `[tiling]` on window events. See
[TILING.md](TILING.md) for the guide and
[CONFIGURATION.md](CONFIGURATION.md#tiling) for the settings. mimi ships no
layout. `examples/tiling/` holds programs to copy.

Tiling needs Accessibility permission, and animating the moves
(`[tiling.animation]`) needs nothing more. See
[CONFIGURATION.md](CONFIGURATION.md#animation).

### `mimi tiling preview [--input]`

Run `tiling.layout` once per display with a `preview` event and a null
state, and print what it returned without applying it. Runs whether or not
`tiling.enabled` is set. `--input` prints the JSON each run would receive
instead of running the layout. **Accessibility permission is required.**

```
$ mimi tiling preview
[{"display":1,"space":2,"frames":[{"number":4242,"frame":{"x":8,"y":33,"width":1904,"height":1034}}],"state":null}]
$ mimi tiling preview --input | jq '.[].windows[].app'
```

### `mimi tiling relayout`

Run the layout once with a `relayout` event and apply the frames. With a
daemon, its engine runs it with the state it holds, and `tiling.enabled` must
be set. Without one, the layout runs in the CLI with a null state.
**Accessibility permission is required.**

### `mimi tiling cmd <name> [args...]`

Send a named command to the layout. mimi gives the name no meaning: the layout
reads `name` and `args` from a `command` event and decides, which is how a
layout defines its own hotkeys. The example master-stack layout answers `swap`
and `ratio +0.05`. Routed as `relayout` is. A blank name is rejected.
**Accessibility permission is required.**

```bash
mimi tiling cmd swap
mimi tiling cmd ratio +0.05
```

---

## Hook Daemon

### `mimi start`

Start the daemon that watches window and space events and runs your hooks.
On first run without a config file, mimi offers to create one.

```bash
mimi start
mimi start -c /path/to/config.toml
```

### `mimi stop`

Stop the running daemon via SIGTERM.

### `mimi status`

Show whether the daemon is running, whether Accessibility permission is
granted, and whether the IPC socket is available.

---

## Service Management

### `mimi services install`

Install mimi as a launchd user agent that starts at login.

```bash
mimi services install
```

The plist is a snapshot of the config at install time. Run install again to
bring the service in line with a changed config. It is idempotent:

| Output                                       | What happened                                                              |
| -------------------------------------------- | -------------------------------------------------------------------------- |
| `Service installed and loaded successfully`  | The plist was written and the service loaded.                              |
| `Service plist updated and service reloaded` | The plist was replaced and the service reloaded.                           |
| `Service already up to date`                 | Nothing was written or reloaded.                                           |

The plist also sets:

- The daemon's stdout and stderr, captured beside `settings.log_file` (or in
  `/tmp` when unset) and emptied at each start. See
  [Troubleshooting](TROUBLESHOOTING.md#where-a-service-installed-daemons-console-output-lands).
- The service `PATH`, from
  [`settings.service_path`](CONFIGURATION.md#service_path). Only this command
  applies a change to it.

Replacing the plist unloads the running service and waits up to five seconds
for it to be gone before loading the new one. If the wait runs out, or
`launchctl` stops answering, the install fails with the old plist untouched.
Ctrl-C leaves it in the same state. A load that fails after the plist is
written leaves the plist on disk, and running install again retries only the
load.

Install refuses when the plist at `~/Library/LaunchAgents/com.y3owk1n.mimi.plist`
is a symlink (a Nix-managed plist), when `com.y3owk1n.mimi` is loaded with no
plist of mimi's behind it, or when `launchctl` cannot be run. The nix-darwin
and home-manager modules register under their own labels. See
[INSTALLATION.md](INSTALLATION.md#post-installation).

### `mimi services uninstall`

Remove the launchd agent. A service that is not loaded is uninstalled without
complaint. When a loaded service cannot be unloaded, or `launchctl` could not
say whether it is loaded, the plist is kept and the command fails, so it can
be run again once the unload works.

### `mimi services start` / `stop` / `restart` / `status`

Control the launchd service directly.

`restart` uses one `launchctl kickstart -k`, and checks first that a job is
loaded:

```
$ mimi services restart
Error: [SERVICE_FAILED] there is no loaded service to restart; run `mimi services install` first
```

`status` separates a loaded service from a running one, since `KeepAlive`
relaunches a crashing daemon while it stays loaded:

```
Service loaded and running (pid 1478)
Service loaded but not running (last exit status 1)
Service loaded
Service not loaded
Service state unknown: launchctl could not be run
```

The bare `Service loaded` means neither a PID nor an exit status was
available. The last line means `launchctl` itself could not be run, so
nothing is known about the service. Under the state line come the captured
console streams and their sizes, one run's output each:

```
Captured stdout: /Users/me/.local/state/mimi/mimi.out.log (2.0 KB)
Captured stderr: /Users/me/.local/state/mimi/mimi.err.log (not created yet)
```

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md#reading-mimi-services-status).

---

## Configuration Management

### `mimi config init`

Create a default config at `~/.config/mimi/config.toml`.

### `mimi config validate`

Parse and validate the config. Exits 0 with the hook count when good, exits 1
with the problems on stderr when not. A key under `[hooks]` that names no hook
kind fails here. The daemon only warns about it and runs the rest.

```
$ mimi config validate
Config invalid:
  hooks.on_window_focussed: not a recognized hook kind

Recognized hook kinds:
  on_app_activate
  ...
```

### `mimi config dump`

Print the resolved config as JSON.

### `mimi config reload`

Send SIGHUP to a running daemon to reload config without restart.
