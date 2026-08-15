# CLI Usage

mimi is a macOS window and space utility. Use `mimi action` for immediate commands, or `mimi start` to run the hook daemon.

---

## Table of Contents

- [Global Flags](#global-flags)
- [Window & Space Actions](#window--space-actions)
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

## Window & Space Actions

These commands run directly in the CLI process when the daemon is not running. When the daemon **is** running, mimi routes actions over its Unix socket (`settings.socket_file`, default `~/.local/share/mimi/mimi.sock`) so hotkeys feel instant. **Accessibility permission is required.**

```bash
mimi action focus_window
mimi action focus_window --backward
mimi action focus_window --left
mimi action focus_window --right
mimi action focus_window --up
mimi action focus_window --down
mimi action space 1
mimi action space next
mimi action space prev
mimi action move_window_to_space 2
mimi action move_window_to_space next
mimi action move_window_to_space prev
mimi action resize_window left-half
mimi action resize_window center --width-percent 80 --height-percent 90
mimi action resize_window --width 1024 --height 768 --anchor cc
```

### `mimi action focus_window`

Cycle keyboard focus through all focusable windows on the current space, or move focus spatially with direction flags.

| Flag         | Description                                                      |
| ------------ | ---------------------------------------------------------------- |
| `--backward` | Cycle to the previous window instead of the next                 |
| `--up`       | Move focus to the nearest window above the current one           |
| `--down`     | Move focus to the nearest window below the current one           |
| `--left`     | Move focus to the nearest window to the left of the current one  |
| `--right`    | Move focus to the nearest window to the right of the current one |

### `mimi action space <number|next|prev>`

Focus a Mission Control space by its 1-based index, or cycle to the next/previous space with wrapping. Uses a synthetic dock-swipe gesture (no public macOS API exists for direct space switching).

### `mimi action move_window_to_space <number|next|prev>`

Move the frontmost window to a space by its 1-based index, or cycle to the next/previous space with wrapping. Uses private SkyLight APIs; does not require disabling SIP.

### `mimi action resize_window [preset] [flags]`

Resize and reposition the frontmost window using presets or custom flags. Respects the macOS tiled window margins setting (`com.apple.WindowManager.EnableTiledWindowMargins`), applying full margins on screen-facing edges and half margins on internal (split) edges. Margins are skipped entirely when they would leave the window no width or height, so a window smaller than its margins is sized as you asked for it.

**Presets** provide quick tiling layouts:

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
| `center`       | Center window at 60% × 80% of screen |
| `fill`         | Fill entire screen                   |

**Custom sizing flags:**

| Flag                     | Description                            |
| ------------------------ | -------------------------------------- |
| `--width, -w <pixels>`   | Absolute window width in points        |
| `--height, -h <pixels>`  | Absolute window height in points       |
| `--width-percent <pct>`  | Width as percentage of screen (0–100)  |
| `--height-percent <pct>` | Height as percentage of screen (0–100) |

**Positioning flags** (use anchors to align the window):

| Flag           | Description              |
| -------------- | ------------------------ |
| `--x <pixels>` | Absolute X position      |
| `--y <pixels>` | Absolute Y position      |
| `--anchor, -a` | Anchor point (see below) |

**Anchor system** places the window's anchor point at the computed or specified position. Valid anchors (use 2 letters: vertical + horizontal):

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

Give one or neither: omitting both defers to the system tiled-window-margins setting, and giving both is rejected rather than resolved by picking one of them.

**Examples:**

```bash
# Presets
mimi action resize_window left-half
mimi action resize_window right-half
mimi action resize_window top-left
mimi action resize_window center

# Fixed dimensions, centered
mimi action resize_window --width 800 --height 600 --anchor cc

# Percentage of screen, top-left
mimi action resize_window --width-percent 50 --height-percent 75 --anchor tl

# Absolute position, top-left anchor
mimi action resize_window --width 1024 --height 768 --x 100 --y 50 --anchor tl

# Override margins for a preset
mimi action resize_window left-half --no-margin

# Mix preset with custom size
mimi action resize_window center --width-percent 80 --height-percent 90
```

---

## Hook Daemon

### `mimi start`

Start the background daemon that watches window and space events and runs your hooks.

```bash
mimi start
mimi start -c /path/to/config.toml
```

On first run without a config file, mimi offers to create one.

### `mimi stop`

Stop the running daemon via SIGTERM.

```bash
mimi stop
```

### `mimi status`

Show whether the daemon is running, whether Accessibility permission is granted, and whether the IPC socket is available.

```bash
mimi status
```

---

## Service Management

### `mimi services install`

Install mimi as a launchd user agent for automatic startup at login.

```bash
mimi services install
```

The generated plist captures the daemon's stdout and stderr beside
`settings.log_file`, creating that directory if missing, or in `/tmp` when
`log_file` is unset. See
[Troubleshooting](TROUBLESHOOTING.md#where-a-service-installed-daemons-console-output-lands).

The `PATH` the service runs its hooks with comes from
[`settings.service_path`](CONFIGURATION.md#service_path), defaulting to
`/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin`. Nothing else reads that
setting, so this command is what makes a change to it take effect.

The plist is a snapshot of the config taken at install time, so running install
again is how an installed service is brought back in line with a config that
has moved since. It is idempotent and reports which of three things it did:

| Output                                       | What happened                                                              |
| -------------------------------------------- | -------------------------------------------------------------------------- |
| `Service installed and loaded successfully`  | The service was not loaded; the plist was written and the service loaded.   |
| `Service plist updated and service reloaded` | The plist disagreed with the config; it was replaced and the service reloaded. |
| `Service already up to date`                 | The installed plist already matched. Nothing was written or reloaded.       |

Replacing the plist unloads the running service first, and waits for it to
actually be gone before handing launchd the new one: `launchctl bootout`
returns as soon as launchd accepts the request, not once the daemon has
finished exiting, and a load fired into that window fails. The wait is bounded
at five seconds, after which the install fails with the old plist untouched —
stop whatever is holding the service and run it again.

A load that fails for any other reason leaves the new plist on disk, so the
config change is already made and only the load is missing; that error says so,
and running install again retries just the load.

Install refuses rather than replaces when what it finds is not a plist it
wrote: a symlink at `~/Library/LaunchAgents/com.y3owk1n.mimi.plist` — the shape
home-manager installs — or a loaded `com.y3owk1n.mimi` with no plist of mimi's
behind it. Both errors name nix-darwin and home-manager, because the fix is in
their configuration.

### `mimi services uninstall`

Remove the launchd agent.

A service that is not loaded is uninstalled without complaint — there is
nothing to unload, and the leftover plist is removed. When a loaded service
cannot be unloaded, the plist is left in place and the command fails, because a
service that keeps running until logout with no plist behind it is one nothing
can uninstall any more. Fix what blocked the unload and run it again.

### `mimi services start` / `stop` / `restart` / `status`

Control the launchd service directly.

`status` separates a loaded service from a running one — the installed plist
sets `KeepAlive`, so a daemon that crashes at startup is relaunched forever
while staying loaded:

```
Service loaded and running (pid 1478)
Service loaded but not running (last exit status 1)
Service loaded
Service not loaded
```

The bare `Service loaded` is the fallback, printed whenever neither number is
available — the daemon has never run, it was killed by a signal rather than
exiting, or launchd's description of the job could not be read. That
description is undocumented text, so output mimi cannot parse costs the detail,
never the answer. See
[TROUBLESHOOTING.md](TROUBLESHOOTING.md#reading-mimi-services-status).

---

## Configuration Management

### `mimi config init`

Create a default config at `~/.config/mimi/config.toml`.

### `mimi config validate`

Parse and validate the config file. Exits 0 and prints the hook count when the
config is good, exits 1 and prints the problems to stderr when it is not.

A key under `[hooks]` that names no hook kind is a failure here — it is a hook
that would never fire. The daemon is more forgiving: it logs a warning and runs
with the hooks it did understand, so a typo does not stop mimi from starting.

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
