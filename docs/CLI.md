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

| Flag            | Shorthand | Default | Description           |
| --------------- | --------- | ------- | --------------------- |
| `--config, -c`  |           | auto    | Path to config file   |
| `--verbose, -v` |           | `false` | Verbose output        |
| `--version`     |           |         | Print version and exit |

---

## Window & Space Actions

These commands run directly in the CLI process when the daemon is not running. When the daemon **is** running, mimi routes actions over its Unix socket (`settings.socket_file`, default `~/.local/share/mimi/mimi.sock`) so hotkeys feel instant. **Accessibility permission is required.**

```bash
mimi action focus_window
mimi action focus_window --backward
mimi action space 1
mimi action move_window_to_space 2
```

### `mimi action focus_window`

Cycle keyboard focus through all focusable windows on the current space.

| Flag         | Description                                      |
| ------------ | ------------------------------------------------ |
| `--backward` | Cycle to the previous window instead of the next |

### `mimi action space <number>`

Focus a Mission Control space by its 1-based index. Uses a synthetic dock-swipe gesture (no public macOS API exists for direct space switching).

### `mimi action move_window_to_space <number>`

Move the frontmost window to a space by its 1-based index. Uses private SkyLight APIs; does not require disabling SIP.

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

### `mimi services uninstall`

Remove the launchd agent.

### `mimi services start` / `stop` / `restart` / `status`

Control the launchd service directly.

---

## Configuration Management

### `mimi config init`

Create a default config at `~/.config/mimi/config.toml`.

### `mimi config validate`

Parse and validate the config file.

### `mimi config dump`

Print the resolved config as JSON.

### `mimi config reload`

Send SIGHUP to a running daemon to reload config without restart.
