# Configuration Guide

Mimi uses TOML for configuration. On first `mimi start`, Mimi prompts you to create a sensible default config or quit. Only define the options you want to change.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Config File Location](#config-file-location)
- [Settings](#settings)
- [Systray](#systray)
- [Hooks](#hooks)
- [Environment Variables](#environment-variables)
- [Event Reference](#event-reference)

---

## Quick Start

```toml
[settings]
log_level = "info"

[hooks]
on_app_activate = ['echo "Switched to $mimi_APP_NAME"']
```

Generate a starter file:

```bash
mimi config init                              # Creates ~/.config/mimi/config.toml
mimi config init -c /path/to/config.toml      # Custom path
```

Validate your config:

```bash
mimi config validate
```

Inspect the resolved config or reload a running daemon:

```bash
mimi config dump
mimi config reload
```

---

## Config File Location

Config is resolved in this priority order (first match wins):

1. CLI `--config` / `-c` flag
2. `$XDG_CONFIG_HOME/mimi/config.toml` (if `$XDG_CONFIG_HOME` is set and file exists)
3. `~/.config/mimi/config.toml` (if file exists)
4. `./mimi.toml` (current directory)
5. Falls back to `$XDG_CONFIG_HOME/mimi/config.toml` (prompts to create if missing)
6. Falls back to `~/.config/mimi/config.toml` (prompts to create if missing)

---

## [settings]

| Key                 | Type   | Default                          | Description                                                   |
| ------------------- | ------ | -------------------------------- | ------------------------------------------------------------- |
| `log_file`          | string | `""`                             | Log file path (`~` expansion supported). Empty = stdout only. |
| `log_level`         | string | `"info"`                         | Log level: `debug`, `info`, `warn`, `error`                   |
| `log_format`        | string | `"text"`                         | Log format: `text` or `json`                                  |
| `hook_timeout_secs` | int    | `10`                             | Default timeout in seconds per hook command                   |
| `hook_shell`        | string | `"/bin/sh"`                      | Shell used to execute hook commands (`-c` flag)               |
| `max_hook_workers`  | int    | `4`                              | Maximum concurrent hook processes                             |
| `pid_file`          | string | `"~/.local/share/mimi/mimi.pid"` | PID file path (`~` expansion supported)                       |

```toml
[settings]
log_file = "~/.local/share/mimi/mimi.log"
log_level = "info"
log_format = "text"
hook_timeout_secs = 10
hook_shell = "/bin/sh"
max_hook_workers = 4
pid_file = "~/.local/share/mimi/mimi.pid"
```

---

## [systray]

| Key                    | Type | Default | Description                                                   |
| ---------------------- | ---- | ------- | ------------------------------------------------------------- |
| `enabled`              | bool | `true`  | Show the Mimi menu bar item on macOS                          |
| `show_workspace_number` | bool | `false` | Show the current macOS Space number in the menu bar (opt-in) |

```toml
[systray]
enabled = true
show_workspace_number = false
```

The tray menu includes links to Mimi docs/source, a config reload action, and Quit Mimi.

---

## [hooks]

Each hook key maps to an array of hook entries. An entry can be:

> [!NOTE]
> **Selective Event Listening**: Mimi only starts the underlying macOS system observers (e.g. power, audio, clipboard, USB, display, workspace) if there is at least one active hook configured for those events. If a hook array is empty (`[]`) or omitted, Mimi does not listen to that category of events on the OS level, keeping CPU and resource usage to an absolute minimum.

### Simplified String Form

```toml
[hooks]
on_app_activate = ['echo "Switched to $mimi_APP_NAME"']
```

### Full Table Form

```toml
[hooks]
on_app_activate = [
  { run = "echo 'hello'", app = "Slack", bundle_id = "com.tinyspeck.slack", title = ".*", timeout_secs = 30, async = true }
]
```

### Hook Entry Fields

| Field          | Type   | Required | Default             | Description                                   |
| -------------- | ------ | -------- | ------------------- | --------------------------------------------- |
| `run`          | string | **yes**  | —                   | Shell command to execute                      |
| `app`          | string | no       | `""` (all apps)     | Glob pattern on app name to filter            |
| `bundle_id`    | string | no       | `""` (all)          | Exact bundle identifier to filter             |
| `title`        | string | no       | `""` (all)          | Regex pattern on window title                 |
| `timeout_secs` | int    | no       | `hook_timeout_secs` | Per-hook timeout override                     |
| `async`        | bool   | no       | `false`             | If true, runs in a goroutine without blocking |

### App Name Filtering

The `app` field supports glob patterns:

```toml
[hooks]
on_app_activate = [
  # Match by exact name
  { run = "script.sh", app = "Slack" },

  # Match by glob
  { run = "script.sh", app = "Slack*" },

  # Match by bundle ID (exact only)
  { run = "script.sh", bundle_id = "com.tinyspeck.slack" },
]
```

### Title Filtering

The `title` field supports regex patterns:

```toml
[hooks]
on_window_focus = [
  # Match any window with "Slack" in the title
  { run = "script.sh", title = ".*Slack.*" },
]
```

### Async Hooks

Hooks with `async = true` run in a goroutine without blocking the event pipeline. The daemon does not wait for completion or capture output.

```toml
[hooks]
on_app_launch = [
  { run = "~/scripts/long-running-task.sh", async = true }
]
```

### Variables in Hook Commands

Hook commands support `$mimi_*` and `${mimi_*}` shell variable substitution:

```toml
[hooks]
on_app_activate = [
  'echo "$mimi_APP_NAME (PID: $mimi_PID) came to front" >> ~/app-log.txt',
]
```

### Example Config

```toml
[settings]
# log_file = "~/.local/share/mimi/mimi.log"
log_level = "info"

[hooks]
# Log every app change
on_app_activate = ['echo "Focused: $mimi_APP_NAME" >> ~/app-log.txt']
on_app_quit = ['echo "Quit: $mimi_APP_NAME" >> ~/app-log.txt']

# Sleep handling
on_system_sleep = ['pmset displaysleepnow']
on_system_wake = ['osascript -e "display notification \"System woke up\" with title \"mimi\""']

# Auto-backup on USB mount
on_volume_mount = [{ run = "~/scripts/backup.sh $mimi_VOLUME_PATH", async = true }]

# Charger notifications
on_power_adapter_connected = ['osascript -e "display notification \"Charger connected\" with title \"mimi\""']
on_power_adapter_disconnected = ['osascript -e "display notification \"Charger disconnected\" with title \"mimi\""']

# Network monitoring
on_network_up = ['echo "Network restored at $mimi_TIMESTAMP" >> ~/network-log.txt']
on_network_down = ['echo "Network lost at $mimi_TIMESTAMP" >> ~/network-log.txt']
```

---

## Environment Variables

Every hook receives these variables in the child process environment:

| Variable             | Type    | Description                                 | Available For                 |
| -------------------- | ------- | ------------------------------------------- | ----------------------------- |
| `mimi_EVENT`         | string  | Event kind (e.g. `app_activate`)            | All events                    |
| `mimi_EVENT_ID`      | UUID    | Unique ID for this event occurrence         | All events                    |
| `mimi_APP_NAME`      | string  | Localised app display name                  | App, window, workspace events |
| `mimi_BUNDLE_ID`     | string  | Bundle identifier (e.g. `com.apple.Safari`) | App, window events            |
| `mimi_PID`           | int     | App process ID                              | App, window events            |
| `mimi_WINDOW_TITLE`  | string  | Focused window title                        | Window, workspace events      |
| `mimi_VOLUME_PATH`   | path    | Mount point path                            | Volume mount/unmount events   |
| `mimi_VOLUME_NAME`   | string  | Volume display name                         | Volume mount/unmount events   |
| `mimi_TIMESTAMP`     | RFC3339 | Time the event was observed                 | All events                    |
| `mimi_WINDOWS_COUNT` | int     | On-screen window count                      | Workspace changed events      |
| `mimi_INFO`          | JSON    | Window details (see below)                  | Workspace changed events      |

### Workspace Event JSON (`mimi_INFO`)

For `on_workspace_changed` events, `mimi_INFO` contains a JSON string:

```json
{
	"total_count": 12,
	"real_count": 8,
	"windows": [
		{
			"app": "Safari",
			"title": "GitHub — mimi",
			"pid": 9876,
			"layer": 0,
			"x": 0,
			"y": 25,
			"w": 1440,
			"h": 900
		}
	]
}
```

| Field         | Type  | Description                                             |
| ------------- | ----- | ------------------------------------------------------- |
| `total_count` | int   | Total visible windows on the current Space              |
| `real_count`  | int   | Windows at layer 0 (normal app windows)                 |
| `windows`     | array | Per-window details — app name, title, pid, layer, frame |

---

## Event Reference

### Application Events

| Hook key            | Fires when…                    | Variables available      |
| ------------------- | ------------------------------ | ------------------------ |
| `on_app_activate`   | An app comes to the foreground | APP_NAME, BUNDLE_ID, PID |
| `on_app_deactivate` | An app loses focus             | APP_NAME, BUNDLE_ID, PID |
| `on_app_launch`     | A new app process starts       | APP_NAME, BUNDLE_ID, PID |
| `on_app_quit`       | An app process terminates      | APP_NAME, BUNDLE_ID, PID |
| `on_app_hide`       | User hides an app (⌘H)         | APP_NAME, BUNDLE_ID      |
| `on_app_unhide`     | A hidden app is shown again    | APP_NAME, BUNDLE_ID      |

### Window Events (requires Accessibility permission)

| Hook key                 | Fires when…                                   | Variables available                    |
| ------------------------ | --------------------------------------------- | -------------------------------------- |
| `on_window_focus`        | Focused window changes                        | APP_NAME, WINDOW_TITLE, BUNDLE_ID, PID |
| `on_window_title_change` | Active window title changes                   | APP_NAME, WINDOW_TITLE, BUNDLE_ID, PID |
| `on_window_created`      | A new window opens                            | APP_NAME, BUNDLE_ID, PID               |
| `on_window_closed`       | A window closes                               | APP_NAME, BUNDLE_ID, PID               |
| `on_window_resize`       | A window finishes resizing (debounced, 250ms) | APP_NAME, WINDOW_TITLE, BUNDLE_ID, PID |

### System Events

| Hook key              | Fires when…                     |
| --------------------- | ------------------------------- |
| `on_system_sleep`     | System or display goes to sleep |
| `on_system_wake`      | System wakes from sleep         |
| `on_screen_lock`      | Screen is locked                |
| `on_screen_unlock`    | Screen is unlocked              |
| `on_system_shutdown`  | Shutdown or restart is imminent |
| `on_user_session_end` | User session ends (logout)      |

### Storage Events

| Hook key            | Fires when…                    | Variables available      |
| ------------------- | ------------------------------ | ------------------------ |
| `on_volume_mount`   | A volume or USB drive mounts   | VOLUME_PATH, VOLUME_NAME |
| `on_volume_unmount` | A volume or USB drive unmounts | VOLUME_PATH, VOLUME_NAME |

### Display / Appearance Events

| Hook key                           | Fires when…                                 |
| ---------------------------------- | ------------------------------------------- |
| `on_external_display_connected`    | An external display is connected            |
| `on_external_display_disconnected` | An external display is disconnected         |
| `on_appearance_changed`            | System appearance changes (Dark/Light mode) |

### Power / Battery Events

| Hook key                        | Fires when…                                 |
| ------------------------------- | ------------------------------------------- |
| `on_power_adapter_connected`    | AC power adapter is plugged in              |
| `on_power_adapter_disconnected` | AC power adapter is unplugged               |
| `on_battery_low`                | Battery level drops to low (~20%)           |
| `on_battery_critical`           | Battery level drops to critically low (~5%) |

### Audio Events

| Hook key                  | Fires when…                                       |
| ------------------------- | ------------------------------------------------- |
| `on_audio_device_changed` | Audio device list or default input/output changes |

### Workspace / Desktop Events

| Hook key               | Fires when…                                      | Variables available                       |
| ---------------------- | ------------------------------------------------ | ----------------------------------------- |
| `on_workspace_changed` | Active Space / Desktop changes (Mission Control) | WINDOWS_COUNT, INFO (JSON window details) |

### USB / Peripheral Events

| Hook key                     | Fires when…                  |
| ---------------------------- | ---------------------------- |
| `on_usb_device_connected`    | A USB device is connected    |
| `on_usb_device_disconnected` | A USB device is disconnected |

### Network Events

| Hook key          | Fires when…                      |
| ----------------- | -------------------------------- |
| `on_network_up`   | Network connectivity is restored |
| `on_network_down` | Network connectivity is lost     |

### Clipboard Events

| Hook key               | Fires when…               |
| ---------------------- | ------------------------- |
| `on_clipboard_changed` | Clipboard content changes |
