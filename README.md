# mimi

> A macOS event daemon that runs your shell commands when things happen.

**mimi** watches macOS system events — app focus changes, sleep/wake cycles, USB mounts, screen locks — and fires the shell hooks you define in a single TOML config file. Think of it as a lightweight, scriptable automation layer that reacts to what your machine is doing.

> ⚠️ **Early development.** Not yet ready for production use.

---

## How it works

1. You write hooks in `~/.config/mimi/config.toml`
2. `mimi start` launches a background daemon
3. When macOS fires a matching event, mimi runs your commands with context injected as environment variables

No GUI. No cloud. Just your shell.

---

## Installation

```sh
# Build from source (Homebrew tap coming soon)
git clone https://github.com/y3owk1n/mimi
cd mimi
just dev
./bin/mimi start
```

---

## Configuration

Edit `~/.config/mimi/config.toml`:

```toml
[settings]
# log_file = "~/.local/share/mimi/mimi.log"  # omit for console-only
log_level = "info"

[hooks]
# Log every app you focus
on_app_activate = [
    "echo 'Switched to: $mimi_APP_NAME' >> ~/app-log.txt"
]

# Turn off the display when the system sleeps
on_system_sleep = ["pmset displaysleepnow"]

# Run a script when a USB drive mounts
on_volume_mount = [
    "~/scripts/backup.sh $mimi_VOLUME_PATH"
]
```

Each hook receives rich context through environment variables — see [Environment Variables](#environment-variables) below.

Validate your config before starting:

```sh
mimi config validate
```

---

## Commands

| Command                | Description                          |
| ---------------------- | ------------------------------------ |
| `mimi start`           | Start the daemon                     |
| `mimi stop`            | Stop the daemon                      |
| `mimi status`          | Show daemon status and recent events |
| `mimi install`         | Install as a launchd user agent      |
| `mimi uninstall`       | Remove the launchd agent             |
| `mimi events`          | Tail the live event stream           |
| `mimi test <event>`    | Fire a synthetic event to test hooks |
| `mimi config validate` | Parse and validate the config file   |

---

## Events

### Application Events

| Hook key            | Fires when…                    | Variables available            |
| ------------------- | ------------------------------ | ------------------------------ |
| `on_app_activate`   | An app comes to the foreground | `APP_NAME`, `BUNDLE_ID`, `PID` |
| `on_app_deactivate` | An app loses focus             | `APP_NAME`, `BUNDLE_ID`, `PID` |
| `on_app_launch`     | A new app process starts       | `APP_NAME`, `BUNDLE_ID`, `PID` |
| `on_app_quit`       | An app process terminates      | `APP_NAME`, `BUNDLE_ID`, `PID` |
| `on_app_hide`       | User hides an app (⌘H)         | `APP_NAME`, `BUNDLE_ID`        |
| `on_app_unhide`     | A hidden app is shown again    | `APP_NAME`, `BUNDLE_ID`        |

### Window Events _(requires Accessibility permission)_

| Hook key                 | Fires when…                 | Variables available               |
| ------------------------ | --------------------------- | --------------------------------- |
| `on_window_focus`        | Focused window changes      | `APP_NAME`, `WINDOW_TITLE`, `PID` |
| `on_window_title_change` | Active window title changes | `APP_NAME`, `WINDOW_TITLE`, `PID` |
| `on_window_created`      | A new window opens          | `APP_NAME`, `PID`                 |
| `on_window_closed`       | A window closes             | `APP_NAME`, `PID`                 |

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

| Hook key            | Fires when…                    | Variables available          |
| ------------------- | ------------------------------ | ---------------------------- |
| `on_volume_mount`   | A volume or USB drive mounts   | `VOLUME_PATH`, `VOLUME_NAME` |
| `on_volume_unmount` | A volume or USB drive unmounts | `VOLUME_PATH`, `VOLUME_NAME` |

### Display / Appearance Events

| Hook key                           | Fires when…                                 |
| ---------------------------------- | ------------------------------------------- |
| `on_external_display_connected`    | An external display is connected            |
| `on_external_display_disconnected` | An external display is disconnected         |
| `on_appearance_changed`            | System appearance changes (Dark/Light mode) |

### Power / Battery Events

| Hook key                        | Fires when…                           |
| ------------------------------- | ------------------------------------- |
| `on_power_adapter_connected`    | AC power adapter is plugged in        |
| `on_power_adapter_disconnected` | AC power adapter is unplugged         |
| `on_battery_low`                | Battery level drops to low            |
| `on_battery_critical`           | Battery level drops to critically low |

### Audio Events

| Hook key                  | Fires when…                                       |
| ------------------------- | ------------------------------------------------- |
| `on_audio_device_changed` | Audio device list or default input/output changes |

### Workspace / Desktop Events

| Hook key               | Fires when…                                      | Variables available                                  |
| ---------------------- | ------------------------------------------------ | ---------------------------------------------------- |
| `on_workspace_changed` | Active Space / Desktop changes (Mission Control) | `WINDOWS_COUNT`, `INFO` (JSON — see below) |

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

---

## Environment Variables

Every hook receives these variables, prefixed with `mimi_`:

| Variable            | Type    | Description                                 |
| ------------------- | ------- | ------------------------------------------- |
| `mimi_EVENT`        | string  | Event kind (e.g. `app_activate`)            |
| `mimi_EVENT_ID`     | UUID    | Unique ID for this event occurrence         |
| `mimi_APP_NAME`     | string  | Localised app display name                  |
| `mimi_BUNDLE_ID`    | string  | Bundle identifier (e.g. `com.apple.Safari`) |
| `mimi_PID`          | int     | App process ID                              |
| `mimi_WINDOW_TITLE` | string  | Focused window title (window events only)   |
| `mimi_VOLUME_PATH`  | path    | Mount point (volume events only)            |
| `mimi_VOLUME_NAME`  | string  | Volume display name (volume events only)    |
| `mimi_TIMESTAMP`    | RFC3339 | Time the event was observed                 |
| `mimi_WINDOWS_COUNT` | int    | On-screen window count (workspace events)   |
| `mimi_INFO`         | JSON    | Window details (workspace events)           |

The `mimi_INFO` variable is a JSON string with:
- `total_count` — all visible windows
- `real_count` — windows at layer 0 (normal app windows, excluding floating/utility)
- `windows` — array of `{app, title, pid, layer, x, y, w, h}` for each window

---

## Tech Stack

- **Go** — core daemon and CLI (`github.com/spf13/cobra`)
- **Objective-C / C** — native macOS event observation via `NSWorkspace`, `NSDistributedNotificationCenter`, `CGWindowListCopyWindowInfo`, and Accessibility APIs
- **TOML** — configuration (`github.com/BurntSushi/toml`)
- **launchd** — optional persistent agent installation

---

## Development

This project uses [just](https://github.com/casey/just) and [devbox](https://www.jetpack.io/devbox) for the dev environment.

```sh
# Enter the dev environment
devbox shell

# Build and run
just dev

# Lint
just lint
```

---

## License

[MIT](./LICENSE)
