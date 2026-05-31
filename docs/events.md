# mimi Event Reference

## Application Events

| Event kind            | Trigger                              | Key env vars available      |
| --------------------- | ------------------------------------ | --------------------------- |
| `app_activate`        | An app comes to the foreground       | APP_NAME, BUNDLE_ID, PID    |
| `app_deactivate`      | An app loses the foreground          | APP_NAME, BUNDLE_ID, PID    |
| `app_launch`          | A new app process starts             | APP_NAME, BUNDLE_ID, PID    |
| `app_quit`            | An app process terminates            | APP_NAME, BUNDLE_ID, PID    |
| `app_hide`            | User hides app (Cmd+H)               | APP_NAME, BUNDLE_ID         |
| `app_unhide`          | Hidden app is shown                  | APP_NAME, BUNDLE_ID         |

## Window Events (requires Accessibility permission)

| Event kind            | Trigger                              | Key env vars available      |
| --------------------- | ------------------------------------ | --------------------------- |
| `window_focus`        | Focused window changes               | APP_NAME, WINDOW_TITLE, PID |
| `window_title_change` | Active window title changes          | APP_NAME, WINDOW_TITLE, PID |
| `window_created`      | New window opens                     | APP_NAME, PID               |
| `window_closed`       | Window closes                        | APP_NAME, PID               |

## System Events

| Event kind            | Trigger                              | Key env vars available      |
| --------------------- | ------------------------------------ | --------------------------- |
| `system_sleep`        | System/display going to sleep        | —                           |
| `system_wake`         | System wakes from sleep              | —                           |
| `screen_lock`         | Screen locked / session resigned     | —                           |
| `screen_unlock`       | Screen unlocked                      | —                           |
| `system_shutdown`     | Shutdown/restart imminent            | —                           |

## Storage Events

| Event kind            | Trigger                              | Key env vars available      |
| --------------------- | ------------------------------------ | --------------------------- |
| `volume_mount`        | Volume/USB drive mounted             | VOLUME_PATH, VOLUME_NAME    |
| `volume_unmount`      | Volume/USB drive unmounted           | VOLUME_PATH, VOLUME_NAME    |

## Environment Variables

| Variable            | Type    | Description                                 |
| ------------------- | ------- | ------------------------------------------- |
| `mimi_EVENT`        | string  | Event kind (e.g. `app_activate`)            |
| `mimi_EVENT_ID`     | UUID    | Unique identifier for this event occurrence |
| `mimi_APP_NAME`     | string  | Localised app display name                  |
| `mimi_BUNDLE_ID`    | string  | Bundle identifier (e.g. `com.apple.Safari`) |
| `mimi_PID`          | int     | App process ID                              |
| `mimi_WINDOW_TITLE` | string  | Focused window title (window events only)   |
| `mimi_VOLUME_PATH`  | path    | Mount point (volume events only)            |
| `mimi_VOLUME_NAME`  | string  | Volume display name                         |
| `mimi_TIMESTAMP`    | RFC3339 | Time the event was observed                 |
