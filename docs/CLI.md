# CLI Usage

Mimi provides a command-line interface for controlling the daemon, testing hooks, and managing configuration. All commands communicate with a running daemon except where noted.

---

## Table of Contents

- [Global Flags](#global-flags)
- [Daemon Control](#daemon-control)
- [Service Management](#service-management)
- [Configuration Management](#configuration-management)
- [Status & Diagnostics](#status--diagnostics)
- [Testing & Development](#testing--development)
- [Event Streaming](#event-streaming)

---

## Global Flags

| Flag            | Shorthand | Type   | Default | Description                 |
| --------------- | --------- | ------ | ------- | --------------------------- |
| `--config, -c`  |           | string | `""`    | Path to config file         |
| `--verbose, -v` |           | bool   | `false` | Enable verbose output       |
| `--version`     |           | bool   | `false` | Print version info and exit |

---

## Daemon Control

### `mimi start`

Start the mimi daemon. If no config file exists, prompts to create one with sensible defaults or quit.

```bash
mimi start                             # Start with auto-resolved config
mimi start -c /path/to/config.toml     # Start with custom config
```

The daemon:

1. Checks macOS accessibility permissions (warns if not granted)
2. Initialises all macOS event observers (NSWorkspace, IOKit, CoreAudio, etc.)
3. Starts the hook executor and event log writer
4. Installs signal handlers for graceful shutdown

### `mimi stop`

Stop a running daemon by sending SIGTERM to its PID.

```bash
mimi stop
```

### `mimi status`

Show daemon status and the last 10 events from the event log.

```bash
mimi status

# Example output:
# mimi: running (pid 12345)
#
# Recent events:
#   14:32:01 | app_activate | Slack (com.tinyspeck.slack)
#   14:31:55 | audio_device_changed
```

---

## Service Management

### `mimi install`

Install mimi as a launchd user agent for automatic startup at login.

```bash
mimi install
```

Writes `~/Library/LaunchAgents/com.y3owk1n.mimi.plist` and runs `launchctl load -w`. The agent is configured with `KeepAlive = true` and `RunAtLoad = true`.

### `mimi uninstall`

Remove the launchd agent.

```bash
mimi uninstall
```

Runs `launchctl unload -w` and deletes the plist file.

---

## Configuration Management

### `mimi init`

Create a default config file at the resolved path.

```bash
mimi init                               # Create at standard location
mimi init -c /path/to/config.toml       # Create at custom path
```

### `mimi config validate`

Parse and validate the config file, reporting any errors.

```bash
mimi config validate                    # Validate at standard location
mimi config validate -c /path/to/config.toml  # Validate a specific file
```

Returns exit code 0 on success, 1 on error.

---

## Status & Diagnostics

### `mimi status`

See [Daemon Control > mimi status](#mimi-start-1).

---

## Testing & Development

### `mimi test <event-kind>`

Fire a synthetic event to test your hooks without needing the actual macOS event. Runs synchronously in the foreground — no daemon required.

```bash
mimi test app_activate                         # Minimal event
mimi test app_activate --app "Slack"            # With app name
mimi test window_focus --app "Safari" --title "GitHub"  # With app + title
```

| Flag       | Type   | Description                       |
| ---------- | ------ | --------------------------------- |
| `--app`    | string | App name for the synthetic event  |
| `--bundle` | string | Bundle ID for the synthetic event |
| `--title`  | string | Window title for the event        |

All events include a synthetic `mimi_*` environment variable set matching the real event.

**Hook matching is active during test:** only hooks that match the event kind, app name, and bundle ID will fire.

---

## Event Streaming

### `mimi events`

Tail the live event stream from the running daemon's event log. Reads from `~/.local/share/mimi/mimi.log.events.jsonl`.

```bash
mimi events                          # Live tail (follow mode)
mimi events --json                   # Raw JSON lines
mimi events --kind app_activate      # Filter by event kind
mimi events --app "Slack"            # Filter by app name (glob)
```

| Flag     | Shorthand | Type   | Description                                |
| -------- | --------- | ------ | ------------------------------------------ |
| `--json` |           | bool   | Output raw JSON lines                      |
| `--kind` |           | string | Filter by event kind (e.g. `app_activate`) |
| `--app`  |           | string | Filter by app name (glob pattern)          |

Without flags, shows a human-readable stream:

```
14:32:01 | app_activate | Slack (com.tinyspeck.slack)
14:31:55 | audio_device_changed
14:31:50 | app_quit | Terminal (com.apple.Terminal)
```
