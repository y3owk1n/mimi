# Mimi System Architecture

Mimi is a macOS event daemon that runs shell commands in response to system events. It is built with Go and Objective-C, using native macOS APIs for event observation and a Go-based pub-sub bus for internal event routing.

---

## Table of Contents

- [System Overview](#system-overview)
- [Event Flow](#event-flow)
- [Component Architecture](#component-architecture)
- [CGO Bridge Architecture](#cgo-bridge-architecture)
- [Configuration Hot-Reload](#configuration-hot-reload)
- [Daemon Lifecycle](#daemon-lifecycle)
- [Technology Stack](#technology-stack)

---

## System Overview

Mimi operates as a background daemon (`NSApplicationActivationPolicyAccessory` — no Dock icon) that:

1. Listens to macOS system notifications via native Objective-C observers
2. Translates them into typed `events.Event` structs
3. Fans each event out through a pub-sub bus to subscribers
4. Executes matching shell hooks with rich `mimi_*` environment variables

---

## Event Flow

```
┌──────────────────────────────────────────────────────────────────────┐
│                        macOS System Events                            │
│  (NSWorkspace, AXObserver, IOKit, CoreAudio, SystemConfiguration...)  │
└──────────────────────────────────────────────────────────────────────┘
         │
         │ Objective-C callbacks fire
         ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    cgo_bridge (CGo / Objective-C)                     │
│  workspace.m  axobserver.m  system_events.m                          │
│                                                                      │
│  Callbacks call //export Go functions:                               │
│    goWorkspaceEvent(), goAXEvent(), goSystemEvent(),                 │
│    goWorkspaceChangeEvent()                                          │
│                                                                      │
│  Each creates events.Event{ID, Kind, At, Extra} and pushes to       │
│  a buffered channel: eventCh <- evt                                  │
└──────────────────────────────────────────────────────────────────────┘
         │
         │ WorkspaceObserver.Run() goroutine consumes eventCh
         ▼
┌──────────────────────────────────────────────────────────────────────┐
│              internal/observers/workspace.go                          │
│  Loops over cgo_bridge.EventCh()                                     │
│  AppActivate/AppLaunch → installs AXObserver on that PID             │
│  AppQuit → removes AXObserver                                        │
│  Publishes → o.bus.Publish(evt)                                      │
└──────────────────────────────────────────────────────────────────────┘
         │
         │ Event bus (pub-sub) fans out
         ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    internal/events/bus.go                             │
│  Bus.Publish(evt) — non-blocking fan-out to all subscribers          │
│  (drops if subscriber channel is full)                               │
└──────────┬───────────────────────────────────────────────────────────┘
           │
           ├─────────────────────────────────────────────────────────┐
           │                                                         │
           ▼                                                         ▼
┌──────────────────────────┐            ┌──────────────────────────────┐
│   Hook Executor           │            │   Event Log Writer           │
│   internal/hooks/         │            │   internal/logging/          │
│   executor.go             │            │   logger.go                  │
│                           │            │                              │
│   Run():                  │            │   WriteEventLog():           │
│     Consumes hookSub      │            │     JSON-encodes each event  │
│     For each event:       │            │     appends to events.jsonl  │
│       ex.Handle(evt)      │            │                              │
│         ↓                 │            └──────────────────────────────┘
│       Registry            │
│       .HooksFor(evt)      │
│         ↓                 │
│       For each match:     │
│         run(hook, evt)    │
│           ↓               │
│         Build env vars    │
│         (mimi_* set)      │
│           ↓               │
│         Shell exec with   │
│         timeout + sem     │
└──────────────────────────┘
```

---

## Component Architecture

```
cmd/mimi/                          # Entry point
└── cmd/                           # Cobra CLI commands
    ├── start.go                   #   mimi start
    ├── stop.go                    #   mimi stop
    ├── status.go                  #   mimi status
    ├── services.go                #   mimi services <subcommand>
    ├── services_darwin.go         #   launchd service implementation
    ├── events.go                  #   mimi events (live tail)
    ├── test.go                    #   mimi test <kind>
    └── config_cmd.go              #   mimi config <subcommand>

internal/
├── events/                        # Event types + pub-sub bus
│   ├── types.go                   #   EventKind, Event, Publisher
│   └── bus.go                     #   Bus (Subscribe, Publish, Unsubscribe)
│
├── config/                        # TOML config loading + validation
│   ├── loader.go                  #   Load(), WriteDefault(), validate()
│   ├── watcher.go                 #   fsnotify-based hot-reload
│   └── types.go                   #   Config, Settings, HookEntry structs
│
├── hooks/                         # Hook registry + executor
│   ├── registry.go                #   Registry: hooks → event kind matching
│   └── executor.go                #   Executor: shell exec with env vars
│
├── observers/                     # Go-side event observer bridge
│   ├── workspace.go               #   WorkspaceObserver: consumes CGO events
│   ├── a11y_manager.go            #   AXObserver lifecycle manager
│   └── cgo_bridge/                #   CGo / Objective-C native layer
│       ├── bridge.go              #     //export Go functions, eventCh
│       ├── workspace.h / .m       #     NSWorkspace + space polling
│       ├── axobserver.h / .m     #     AXObserver per-app callbacks
│       └── system_events.h / .m   #     Power, Audio, USB, Network, Display, Clipboard
│
├── daemon/                        # Daemon orchestration
│   └── daemon.go                  #   Run(): wiring, signal handling
│
├── permissions/                   # macOS accessibility permission check
│   └── check.go                   #   Check() → CheckResult
│
├── logging/                       # Structured logging + event log
│   └── logger.go                  #   New(), WriteEventLog()
│
└── errors/                        # Structured error codes
    └── errors.go                  #   Code*, Error type

configs/                           # Embedded default config
└── embed.go                       #   DefaultConfig []byte
```

### Layer Responsibilities

| Directory                        | Role                                       |
| -------------------------------- | ------------------------------------------ |
| `cmd/mimi/`                      | Application entry point                    |
| `cmd/mimi/cmd/`                  | CLI command definitions (Cobra)            |
| `internal/events/`               | Event kinds, Event struct, pub-sub bus     |
| `internal/config/`               | TOML parsing, validation, defaults, reload |
| `internal/hooks/`                | Hook registry and shell executor           |
| `internal/observers/`            | Go-side observer orchestration             |
| `internal/observers/cgo_bridge/` | CGo exports and Objective-C observers      |
| `internal/daemon/`               | Daemon lifecycle (wiring, signals)         |
| `internal/permissions/`          | macOS accessibility permission checks      |
| `internal/logging/`              | Structured logging and event log writer    |
| `internal/errors/`               | Structured error types with codes          |
| `configs/`                       | Embedded default configuration             |

---

## CGO Bridge Architecture

The bridge has two layers:

### Go Layer (`bridge.go`)

- Exports functions callable from C via `//export`: `goWorkspaceEvent`, `goAXEvent`, `goSystemEvent`, `goWorkspaceChangeEvent`
- Maps integer kind codes to `events.EventKind` via `kindFromInt()`
- Pushes events to a buffered channel (`eventCh <- evt`, buf=256)
- Provides `EventCh()` for Go consumers to read from

### Objective-C Layer

| File                 | Observers / Functions                                                       |
| -------------------- | --------------------------------------------------------------------------- |
| `workspace.h / .m`   | `InitCocoaApp()` — NSApp with `NSApplicationActivationPolicyAccessory`      |
|                      | `WorkspaceObserverStart/Stop()` — NSWorkspace notifications + space polling |
|                      | `GetRunLoop()` — main CFRunLoop for AXObserver scheduling                   |
| `axobserver.h / .m`  | `AXInstallObserver(pid)` — per-app AXObserver for window events             |
|                      | `AXRemoveObserver(pid)` / `AXRemoveAllObservers()`                          |
| `system_events.h/.m` | `PowerObserverStart/Stop()` — IOKit power sources                           |
|                      | `AudioObserverStart/Stop()` — CoreAudio device listeners                    |
|                      | `ClipboardObserverStart/Stop()` — NSPasteboard polling (500ms)              |
|                      | `USBObserverStart/Stop()` — IOKit IOUSBDevice notifications                 |
|                      | `NetworkObserverStart/Stop()` — SCNetworkReachability                       |
|                      | `DisplayObserverStart/Stop()` — CGDisplayRegisterReconfigurationCallback    |

### Observers Summary

| Observer            | Mechanism                                  | Events Produced                                            |
| ------------------- | ------------------------------------------ | ---------------------------------------------------------- |
| **Workspace**       | NSWorkspace NotificationCenter             | app_activate, deactivate, launch, quit, hide, unhide       |
|                     |                                            | system_sleep, wake, screen_lock, unlock, shutdown          |
|                     |                                            | volume_mount, unmount                                      |
| **Appearance**      | NSDistributedNotificationCenter            | appearance_changed                                         |
| **Space Poll**      | CGWindowListCopyWindowInfo polling (200ms) | workspace_changed (with window info JSON)                  |
| **AXObserver**      | Accessibility API (per-app)                | window_focus, title_change, created, closed                |
| **Power / Battery** | IOKit (IOPowerSources)                     | power_adapter_connected/disconnected, battery_low/critical |
| **Audio**           | CoreAudio property listeners               | audio_device_changed                                       |
| **Clipboard**       | NSPasteboard polling (500ms)               | clipboard_changed                                          |
| **USB**             | IOKit (IOUSBDevice)                        | usb_device_connected/disconnected                          |
| **Network**         | SCNetworkReachability                      | network_up/down                                            |
| **Display**         | CGDisplayRegisterReconfigurationCallback   | external_display_connected/disconnected                    |

---

## Configuration Hot-Reload

```
Config file write
  → fsnotify.Watcher (debounced 300ms)
    → config.Load() → Registry.Reload(newCfg) → Executor.UpdateSettings()
```

SIGHUP also triggers a config reload without restart.

---

## Daemon Lifecycle

```
mimi start
  → writePID()
  → permissions.Check() (warn if no accessibility)
  → cgo_bridge.Start() (init Cocoa app, start all observers)
  → events.NewBus()
  → WorkspaceObserver.Run() (goroutine)
  → Executor.Run() (goroutine, consuming hookSub)
  → WriteEventLog() (goroutine, consuming logSub)
  → config.Watcher.Run() (goroutine, hot-reload)
  → signal handling (SIGTERM → graceful shutdown)
  → removePID()
```

### Signal Handling

| Signal  | Behaviour                     |
| ------- | ----------------------------- |
| SIGTERM | Graceful shutdown (default)   |
| SIGINT  | Graceful shutdown             |
| SIGHUP  | Reload config without restart |
| SIGQUIT | Force quit                    |

---

## Technology Stack

- **Core Language**: Go 1.26+
- **Native Integration**: CGo + Objective-C (macOS bridge)
- **CLI Framework**: Cobra (`github.com/spf13/cobra`)
- **Configuration**: TOML (`github.com/BurntSushi/toml`)
- **Logging**: Zap (`go.uber.org/zap`) with Lumberjack rotation
- **Config Watching**: fsnotify (`github.com/fsnotify/fsnotify`)
- **Build System**: Just
- **macOS Frameworks**: Cocoa, ApplicationServices, IOKit, CoreAudio, SystemConfiguration
