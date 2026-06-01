<div align="center">

# mimi

**A macOS event daemon that runs your shell commands when things happen.**

[![Go Version](https://img.shields.io/github/go-mod/go-version/y3owk1n/mimi?style=flat-square&logo=go)](https://github.com/y3owk1n/mimi)
[![License](https://img.shields.io/github/license/y3owk1n/mimi?style=flat-square)](LICENSE)

</div>

---

**mimi** watches macOS system events — app focus changes, sleep/wake cycles, USB mounts, network flips, screen locks, clipboard changes — and fires the shell hooks you define in a single TOML config file. Think of it as a lightweight, scriptable automation layer that reacts to what your machine is doing.

No GUI. No cloud. Just your shell.

> [!CAUTION]
> **Early development.** mimi is in an early, experimental stage. APIs, config format, and behaviour may change without notice. Not yet recommended for production use.

---

## 🚀 Quick Start

```toml
# ~/.config/mimi/config.toml
[hooks]
on_app_activate    = ['echo "Focused: $mimi_APP_NAME" >> ~/app-log.txt']
on_system_sleep    = ['pmset displaysleepnow']
on_volume_mount    = ['rsync -a ~/Documents "$mimi_VOLUME_PATH/backup"']
on_battery_low     = ['osascript -e "display notification \"Plug me in!\" with title \"mimi\""']
```

```bash
mimi start          # Launch the daemon
mimi status         # Show status and recent events
```

---

## 📥 Installation

### Homebrew (Recommended)

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

### Nix Flake

```nix
# flake.nix
{ inputs.mimi.url = "github:y3owk1n/mimi"; }
```

### From Source

```bash
git clone https://github.com/y3owk1n/mimi.git
cd mimi && just build
```

For auto-start at login, launchd agent setup, Nix modules, and troubleshooting — see [Installation Guide](docs/INSTALLATION.md).

---

## ⌨️ Events

Mimi observes **30 macOS system events** across 10 categories. Every hook receives rich context via `mimi_*` environment variables.

| Category             | Events                                                 |
| :------------------- | :----------------------------------------------------- |
| App Lifecycle        | activate, deactivate, launch, quit, hide, unhide       |
| Window (AX)          | focus, title change, created, closed                   |
| System Power         | sleep, wake, screen lock/unlock, shutdown, session end |
| Storage              | volume mount, unmount                                  |
| Display / Appearance | external display connect/disconnect, dark/light mode   |
| Power / Battery      | AC adapter on/off, battery low/critical                |
| Audio                | device changed                                         |
| Workspace / Desktop  | Space changed (Mission Control)                        |
| USB / Peripheral     | device connect, disconnect                             |
| Network              | up, down                                               |
| Clipboard            | content changed                                        |

> Full event reference, all `mimi_*` variables, and workspace JSON schema → [Configuration Guide](docs/CONFIGURATION.md#environment-variables)

---

## ⚙️ Configuration

Your config lives at `~/.config/mimi/config.toml`. Human-readable, dotfile-friendly.

```bash
mimi init               # Create a starter config
mimi config validate    # Validate your TOML
```

Each hook can filter by `app`, `bundle_id`, and `title`, supports timeouts and async execution — see the [Configuration Guide](docs/CONFIGURATION.md).

---

## 🛠️ Commands

| Command                  | Description                          |
| :----------------------- | :----------------------------------- |
| `mimi start`             | Start the daemon                     |
| `mimi stop`              | Stop the daemon                      |
| `mimi status`            | Show daemon status and recent events |
| `mimi events`            | Tail the live event stream           |
| `mimi test <event-kind>` | Fire a synthetic event to test hooks |
| `mimi install`           | Install as a launchd agent           |
| `mimi uninstall`         | Remove the launchd agent             |
| `mimi config validate`   | Validate your config file            |
| `mimi init`              | Create a default config file         |

> Full CLI reference → [CLI.md](docs/CLI.md)

---

## 🏗️ Architecture

```
macOS Event (NSWorkspace, IOKit, CoreAudio, AX, etc.)
  → Obj-C Observer → CGo Bridge → Event Bus → Hook Executor → Your Shell Command
```

Mimi uses a pub-sub event bus with non-blocking fan-out. Events are produced by native Objective-C observers and consumed by the hook executor and event log writer. See [Architecture Guide](docs/ARCHITECTURE.md).

---

## 🤝 Contributing

Mimi is written in Go and Objective-C with a clean package layout.

```bash
just build && just lint && just test
# Open a pull request!
```

Refer to the [Development Guide](docs/DEVELOPMENT.md).

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE).

<div align="center">

**Made with ❤️ by [y3owk1n](https://github.com/y3owk1n)**

</div>
