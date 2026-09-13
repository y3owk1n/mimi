# CLI usage

mimi is a macOS window and space utility. Use `mimi action` for immediate commands, or `mimi start` to run the hook daemon.

---

## Table of contents

- [Global flags](#global-flags)
- [Interrupting a command](#interrupting-a-command)
- [Window and space actions](#window-and-space-actions)
- [Queries](#queries)
- [Tiling](#tiling)
- [Hook daemon](#hook-daemon)
- [Service management](#service-management)
- [Configuration management](#configuration-management)
- [Shell completion](#shell-completion)

---

## Global flags

| Flag        | Shorthand | Default | Description            |
| ----------- | --------- | ------- | ---------------------- |
| `--config`  | `-c`      | auto    | Path to config file    |
| `--verbose` | `-v`      | `false` | Verbose output         |
| `--version` |           |         | Print version and exit |

With no `--config`, mimi uses the first config that exists out of
`$XDG_CONFIG_HOME/mimi/config.toml`, `~/.config/mimi/config.toml` and
`./mimi.toml`. When none exists, it uses `$XDG_CONFIG_HOME/mimi/config.toml`
if `XDG_CONFIG_HOME` is set, else `~/.config/mimi/config.toml`.

`mimi --version` prints the version, git commit and build date.

---

## Interrupting a command

A second Ctrl-C always ends the process immediately with exit status 130. What
the first one does depends on the command:

| Command                      | First Ctrl-C                                                                    |
| ---------------------------- | ------------------------------------------------------------------------------- |
| `mimi services install`      | Stops the work in progress and says where it stopped.                           |
| `mimi services uninstall`    | Stops, keeping the plist, exactly as a failed unload does.                      |
| `mimi services start`/`stop`/`restart` | Stops before or during the `launchctl` call, and says so.             |
| `mimi services status`       | Stops asking `launchctl` and prints the unknown state. Exits 0.                 |
| `mimi start`                 | Shuts the daemon down gracefully.                                               |
| `mimi action *`              | Does not reach the action, which finishes. Press Ctrl-C again to end the process. |
| `mimi config *`              | Does not reach the command, which finishes. Each is local file work, plus one signal for `reload`. |
| `mimi status`, `mimi stop`   | Does not reach the command, which finishes. Each is a few file reads and quick system calls. |
| `mimi query *`               | Does not reach the command, which finishes. Each is a few desktop reads and one line of output. |
| `mimi tiling preview`        | Kills the layout program if it is still running. Nothing is applied either way. |
| `mimi tiling relayout`/`cmd` | With a daemon, as `mimi action *`. Without one, as `preview`, then the frames are applied. |
| `mimi tiling state`/`reset`  | Does not reach the command, which finishes. Each is one request to the daemon.  |

A command the first Ctrl-C did not reach still finishes and exits 0.

For `mimi start`, the first Ctrl-C does nothing until the daemon installs its
signal watch, for example while the first-run config alert is up. A second
Ctrl-C during a stalled shutdown skips the daemon's cleanup, so a PID file may
be left behind. `mimi status` reports it as stale and the next `mimi start`
overwrites it.

---

## Window and space actions

Actions run in the CLI process when no daemon is running. With a daemon,
the CLI sends them over its Unix socket (`settings.socket_file`), which is
faster than starting the action from scratch. Accessibility permission is required.

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

Give at most one direction flag, and do not combine `--backward` with a
direction flag.

`--same-app` works like the keyboard's Cmd-backtick. It combines with
`--backward` or a direction flag, and needs a focused window to take the
application from.

`--number` names one window instead of saying which way to move from the
focused one, so it combines with none of the other flags. The number is the
one `mimi query windows` reports. It stays the same for the window's lifetime,
and `mimi action apply_frames` and a layout's `frames` take the same number:

```bash
mimi action focus_window --number 4242
```

This is the only way to focus a window without saying where it is on screen.
A layout's `before` and `after` command lines need that, and so does a hotkey
bound to a window you noted earlier. The window has to be on the current
space. Use `mimi action focus_app` to switch space and reach a window.

### `mimi action focus_app <name|bundle-id>`

Bring a running application's window to the front. When the window is on
another space, mimi first switches space with the same instant gesture
`mimi action space` uses. Name the application by its app name (case does not
matter) or bundle identifier. Pair it with `open` for the case where the
application is not running:

```bash
mimi action focus_app Safari || open -a Safari
```

- When the application is not in front, mimi picks its most recently used
  window, wherever it is.
- When it is in front, each press moves to its next window, ordered by
  space and then by age, wrapping at the end.
- Minimized windows are skipped. A window assigned to every space is raised
  without a space switch. An application with no window is reopened as if
  its Dock icon were clicked.

### `mimi action space <number|next|prev>`

Focus a Mission Control space by 1-based index, or cycle with wrapping. mimi
synthesizes a dock-swipe gesture, since no public macOS API switches spaces.
When the destination is on another display, mimi first moves the pointer to
that display's center, and the pointer stays there. The same applies to
`move_window_to_space --follow`.

### `mimi action move_window_to_space <number|next|prev>`

Move the frontmost window to a space by 1-based index, or cycle with
wrapping. This uses private SkyLight APIs and does not require disabling SIP.

| Flag       | Description                                                      |
| ---------- | ---------------------------------------------------------------- |
| `--follow` | Switch to the destination space once the window is there         |

With `--follow`, the switch uses the same dock-swipe gesture as `space`, and
mimi raises the moved window again once the switch lands. If the move lands
but the switch or raise fails, the error says so and the window stays on its
new space.

### `mimi action move_window_to_display <number|next|prev>`

Move the frontmost window to another display by 1-based index, or cycle with
wrapping. Displays are counted left to right, then top to bottom. The window
lands on the destination's active space and keeps the share of the display it
had. A window already on the destination does not move. Accessibility
permission is required.

### `mimi action resize_window [preset] [flags]`

Resize and reposition the frontmost window. mimi follows the macOS tiled
window margins setting, with full margins on screen edges and half margins on
split edges. A window too small to give up its margins gets the requested size
with no margins.

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
| `center`       | Center window at 60% x 80% of screen |
| `fill`         | Fill entire screen                   |

**Cycling.** With `--cycle`, repeated presses of `left-half` or `right-half`
step through half, two thirds, a third, then half again. mimi picks the step
from the window's current size, so cycling works with or without the daemon.
A window at none of those sizes starts at the half. `--cycle` takes no size,
position or anchor flag, and only those two presets accept it.

**Custom sizing flags:**

| Flag                     | Description                            |
| ------------------------ | -------------------------------------- |
| `--width, -w <points>`   | Absolute window width in points        |
| `--height <points>`      | Absolute window height in points       |
| `--width-percent <pct>`  | Width as percentage of screen (0-100)  |
| `--height-percent <pct>` | Height as percentage of screen (0-100) |

**Positioning flags:**

| Flag           | Description              |
| -------------- | ------------------------ |
| `--x <points>` | Absolute X position      |
| `--y <points>` | Absolute Y position      |
| `--anchor, -a` | Anchor point (see below) |

An anchor is two letters, vertical then horizontal. It names the point of the
window that mimi places at the computed position:

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

Give one or neither. Omitting both follows the system setting. mimi rejects
both together.

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

Move and resize several windows on the active space in one action. The frames
come as a JSON array on stdin or from `--file`. Each entry names a window by
the `number` from `mimi query windows` and gives its frame in window
coordinates. Accessibility permission is required.

```
[{"number":4242,"frame":{"x":0,"y":25,"width":720,"height":875}},
 {"number":4243,"frame":{"x":720,"y":25,"width":720,"height":875}}]
```

The action attempts every frame in order. It fails when any frame did not
land, and names each window it could not place and why. mimi rejects the
payload before anything moves when it is empty, is followed by a second JSON
document, names a window twice, names window 0, or gives a frame without a
positive width and height.

A tiling script needs only this action, `mimi query windows` and
`mimi query displays`. It lists the windows, decides where each goes, and
applies the frames. `examples/tiling/` holds scripts to copy.

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

The active space and the space count, in the 1-based ordering `mimi action space`
takes. `index` is the space in front on the display holding the cursor. Needs
no Accessibility permission.

```
$ mimi query space
{"index":2,"count":5}
```

### `mimi query window`

The frontmost window, with its owner's PID and frame in window coordinates.
The origin is the top-left of the primary display, and y grows downward.
Accessibility permission is required.

```
$ mimi query window
{"pid":4242,"frame":{"x":100,"y":50,"width":1024,"height":768}}
```

### `mimi query windows`

Every focusable window on the active space, in `focus_window` cycle order.
`focused` is the index of the focused window, or -1. `number` is the window
server's number for the window. It stays the same for the window's lifetime,
and `apply_frames` takes it. `order` is the window's place in the stacking
order, 0 for the frontmost. This differs from the cycle order of the list. A
window whose frame cannot be read is left out. Accessibility permission is
required.

```
$ mimi query windows
{"focused":0,"windows":[{"number":4242,"pid":501,"app":"Safari","bundleId":"com.apple.Safari","title":"Start Page","frame":{"x":0,"y":25,"width":1440,"height":875},"order":0}]}
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

Tiling needs Accessibility permission. Animating the moves
(`[tiling.animation]`) needs no further permission. See
[CONFIGURATION.md](CONFIGURATION.md#animation).

### `mimi tiling preview [--input]`

Run `tiling.layout` once for each display that has a window, with a `preview`
event and a null state, and print what it returned without applying it. This
runs whether or not `tiling.enabled` is set. With `--input`, mimi prints the
JSON each run would receive and does not run the layout. Accessibility
permission is required.

```
$ mimi tiling preview
[{"display":1,"space":2,"frames":[{"number":4242,"frame":{"x":8,"y":33,"width":1904,"height":1034}}],"state":null}]
$ mimi tiling preview --input | jq '.[].windows[].app'
```

### `mimi tiling relayout`

Run the layout once with a `relayout` event and apply the frames. With a
daemon, its engine runs the layout with the state it holds, and
`tiling.enabled` must be set. Without one, the layout runs in the CLI with a
null state. Accessibility permission is required.

### `mimi tiling cmd <name> [args...]`

Send a named command to the layout. mimi gives the name no meaning. The layout
reads `name` and `args` from a `command` event and decides what to do, which
is how a layout defines its own hotkeys. The example master-stack layout
answers `swap` and `ratio +0.05`. Everything after the name goes to the layout,
including arguments that start with `-`. The command is routed the same way
as `relayout`. mimi rejects a blank name. Accessibility permission is required.

```bash
mimi tiling cmd swap
mimi tiling cmd ratio +0.05
```

### `mimi tiling state`

Print, as one line of JSON, the state the running daemon's engine holds. There
is one entry per display and space it has run the layout for, with the state
the layout last returned there. `unmanaged` lists the windows the layout said
it is not managing.

```
$ mimi tiling state | jq -c
{"spaces":[{"display":1,"spaceId":5,"space":2,"state":{"ratio":0.6}}],"unmanaged":[4243]}
```

Each space is named twice. `spaceId` is the window server's own identifier for
the space. The engine files state under it, and it never changes. `space` is
the space's current position in Mission Control, or `0` when it is not in
front on any display.

This reads the daemon's memory, not the desktop, so it needs a running daemon.
With none, it prints an empty state and says so on stderr, because a layout
run from the CLI gets a null state and keeps none. Needs no Accessibility
permission of its own.

### `mimi tiling reset [--all]`

Forget what the layout returned for the space in front on each display, so
the next pass there starts it from a null state. With `--all`, forget every
space the daemon remembers. Prints how many spaces it forgot, as
`{"dropped":N}`.

```bash
mimi tiling reset
mimi tiling reset --all
```

Use this when a layout's state has gone wrong, for example a tree that no
longer matches the windows. Restarting the daemon clears the state for every
display at once. This command lays nothing out. The next event runs the
layout, or `mimi tiling relayout` runs it now. Needs a running daemon.

---

## Hook daemon

### `mimi start`

Start the daemon that watches window and space events and runs your hooks.
On first run without a config file, mimi offers to create one.

```bash
mimi start
mimi start -c /path/to/config.toml
```

### `mimi stop`

Stop the running daemon via SIGTERM. With no PID file, it prints that mimi is
not running and exits 0.

### `mimi status`

Show whether the daemon is running, whether Accessibility permission is
granted, and whether the IPC socket is available.

---

## Service management

### `mimi services install`

Install mimi as a launchd user agent that starts at login.

```bash
mimi services install
```

The plist is a snapshot of the config at install time. Run install again to
bring the service in line with a changed config. Install is idempotent, and
prints which of these happened:

| Output                                       | What happened                                                              |
| -------------------------------------------- | -------------------------------------------------------------------------- |
| `Service installed and loaded successfully`  | The plist was written and the service loaded.                              |
| `Service plist updated and service reloaded` | The plist was replaced and the service reloaded.                           |
| `Service already up to date`                 | Nothing was written or reloaded.                                           |

The plist also sets:

- The daemon's stdout and stderr, captured beside `settings.log_file` and
  emptied at each start. When `log_file` is unset or not an absolute path,
  they go to `/tmp/mimi.log` and `/tmp/mimi.err.log`. See
  [Troubleshooting](TROUBLESHOOTING.md#where-a-service-installed-daemons-console-output-lands).
- The service `PATH`, from
  [`settings.service_path`](CONFIGURATION.md#service_path). Only this command
  applies a change to it.

Before loading a replacement plist, install unloads the running service and
waits up to five seconds for it to be gone. If the wait runs out, or
`launchctl` stops answering, the install fails with the old plist untouched.
Ctrl-C leaves it in the same state. A load that fails after the plist is
written leaves the plist on disk, and running install again retries only the
load.

Install refuses in three cases: the path
`~/Library/LaunchAgents/com.y3owk1n.mimi.plist` holds something other than a
regular file (such as a Nix-managed symlink), `com.y3owk1n.mimi` is loaded
with no plist of mimi's behind it, or `launchctl` cannot be run. The
nix-darwin and home-manager modules register under their own labels. See
[INSTALLATION.md](INSTALLATION.md#post-installation).

### `mimi services uninstall`

Remove the launchd agent. A service that is not loaded is uninstalled without
complaint. When a loaded service cannot be unloaded, or `launchctl` could not
say whether it is loaded, the command keeps the plist and fails. Run it again
once the unload works.

### `mimi services start` / `stop` / `restart` / `status`

Control the launchd service directly.

`restart` checks that a job is loaded, then restarts it with one
`launchctl kickstart -k`:

```
$ mimi services restart
Error: [SERVICE_FAILED] there is no loaded service to restart; run `mimi services install` first
```

`status` reports whether the service is loaded and whether it is running as
separate facts, because `KeepAlive` relaunches a crashing daemon and keeps it
loaded:

```
Service loaded and running (pid 1478)
Service loaded but not running (last exit status 1)
Service loaded
Service not loaded
Service state unknown: launchctl could not be run
```

The bare `Service loaded` means neither a PID nor an exit status was
available. The last line means `launchctl` itself could not be run, so
nothing is known about the service. Below the state line, `status` lists the
captured console streams and their sizes. Each holds one run's output:

```
Captured stdout: /Users/me/.local/state/mimi/mimi.out.log (2.0 KB)
Captured stderr: /Users/me/.local/state/mimi/mimi.err.log (not created yet)
```

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md#reading-mimi-services-status).

---

## Configuration management

### `mimi config init`

Write the default config to the config path (`~/.config/mimi/config.toml`
unless `--config` or `XDG_CONFIG_HOME` says otherwise). It overwrites an
existing config.

### `mimi config validate`

Parse and validate the config. When the config is valid, it prints the hook
count and exits 0. When it is not, it prints the problems on stderr and exits 1.
A key under `[hooks]` that names no hook kind fails here. The daemon only
warns about such a key and runs the rest.

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

Send SIGHUP to the running daemon, found through `settings.pid_file`, so it
reloads the config without a restart.

---

## Shell completion

### `mimi completion <bash|zsh|fish|powershell>`

Print a shell completion script. See
[INSTALLATION.md](INSTALLATION.md#shell-completions) for where to put it.
