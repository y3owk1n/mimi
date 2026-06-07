<div align="center">

# mimi

**A macOS window and space utility — fast actions, shell hooks, and a menu bar companion.**

[![Go Version](https://img.shields.io/github/go-mod/go-version/y3owk1n/mimi?style=flat-square&logo=go)](https://github.com/y3owk1n/mimi)
[![License](https://img.shields.io/github/license/y3owk1n/mimi?style=flat-square)](LICENSE)

[Why mimi](#why-mimi) · [Quick Start](#quick-start) · [Installation](#installation) · [Documentation](#documentation) · [Contributing](CONTRIBUTING.md)

</div>

---

**mimi** helps you move around macOS with less friction — switch Mission Control spaces, move windows between desktops, cycle focus on the active space, and optionally react to those changes with shell hooks when the daemon is running.

No SIP disable. No scripting additions. Public Accessibility APIs where possible; private SkyLight for instant window-to-space moves.

---

## Why mimi

This project started from a simple goal: **use macOS Spaces the way Apple intended**, without bolting on a tiling window manager.

Tools like [yabai](https://github.com/koekeishiya/yabai) and [AeroSpace](https://github.com/nikitabobko/AeroSpace) are excellent if you want a full window-management layer — tiling layouts, custom rules, and deep control over every frame. But they come with trade-offs: another long-running dependency, more configuration surface, and often SIP changes or a more invasive setup.

**mimi is none of that.** It is not a tiling window manager. It is not a window manager. It does not replace Mission Control, Stage Manager, or the native window chrome. It is a thin enhancement on top of what macOS already gives you:

- Jump to a space by number from the CLI or a hotkey
- Move the frontmost window to another space instantly
- Cycle focus among windows on the current space
- Optionally run shell hooks when windows or spaces change
- A small menu bar companion when the daemon is running

If you like native Spaces and want to stay close to stock macOS — or you are trying to **reduce your reliance on a TWM** rather than add another one — mimi is built for that workflow. Think of it as glue: keyboard shortcuts, scripts, and automation around Apple's own space model, not a replacement for it.

> [!CAUTION]
> **Early development.** Config format, CLI, and behaviour may change between releases.

---

## What you get

| Mode               | When to use it                                                                                 |
| :----------------- | :--------------------------------------------------------------------------------------------- |
| **CLI actions**    | One-shot commands — bind to hotkeys, scripts, or Alfred/Raycast. Work without the daemon; route over IPC when it is running for lower latency. |
| **Daemon + hooks** | React to window and space changes with shell commands (`on_window_*`, `on_workspace_changed`). |
| **Menu bar**       | See the active space number, reload config, or quit — enabled by default when the daemon runs. |

---

## Quick Start

### 1. Install

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

Other options → [Installation Guide](docs/INSTALLATION.md)

### 2. Run actions immediately

Grant **Accessibility** to `mimi` in **System Settings → Privacy & Security → Accessibility**, then:

```bash
mimi action focus_window              # cycle focus on the active space
mimi action focus_window --backward   # cycle backward
mimi action space 2                   # jump to space 2
mimi action move_window_to_space 3    # move frontmost window to space 3
```

Full command reference → [CLI Guide](docs/CLI.md)

### 3. Optional — run the daemon with hooks

```bash
mimi config init
mimi start
```

Edit `~/.config/mimi/config.toml`:

```toml
[systray]
enabled = true
show_workspace_number = true

[hooks]
on_window_focus = ['echo "Focused: $mimi_APP_NAME — $mimi_WINDOW_TITLE"']
on_workspace_changed = ['echo "Space changed"']
```

Check status anytime:

```bash
mimi status
```

Hook options, filters, and `mimi_*` env vars → [Configuration Guide](docs/CONFIGURATION.md)

Auto-start at login → [Installation Guide — launchd](docs/INSTALLATION.md#auto-start-launchd)

---

## Features

### Window & space actions

| Action                           | Command                                |
| :------------------------------- | :------------------------------------- |
| Cycle window focus               | `mimi action focus_window`             |
| Switch to a space (1-based)      | `mimi action space <n>`                |
| Move frontmost window to a space | `mimi action move_window_to_space <n>` |

Space switching uses a synthetic dock-swipe gesture. Window moves use SkyLight for instant relocation without animation.

### Hooks (daemon)

| Event                      | Hook                     | Accessibility required |
| :------------------------- | :----------------------- | :--------------------- |
| Window focused             | `on_window_focus`        | Yes                    |
| Window title changed       | `on_window_title_change` | Yes                    |
| Window opened              | `on_window_created`      | Yes                    |
| Window closed              | `on_window_closed`       | Yes                    |
| Window resized (debounced) | `on_window_resize`       | Yes                    |
| Active space changed       | `on_workspace_changed`   | No                     |

Hooks support app/title filters, async execution, and per-hook timeouts. See [Configuration Guide](docs/CONFIGURATION.md).

### Menu bar

When `[systray] enabled = true` (the default), the daemon shows a menu bar icon with the current space number, config reload, and quit. Disable it in config if you prefer a headless daemon.

---

## Commands at a glance

| Command                    | Description                        |
| :------------------------- | :--------------------------------- |
| `mimi action …`            | Immediate window/space actions     |
| `mimi start` / `mimi stop` | Start or stop the hook daemon      |
| `mimi status`              | Daemon state and permission checks |
| `mimi config init`         | Create default config              |
| `mimi config validate`     | Validate config                    |
| `mimi config reload`       | Reload running daemon (SIGHUP)     |
| `mimi services install`    | Install launchd agent              |

→ [CLI Guide](docs/CLI.md)

---

## Installation

| Method          | Command / link                            |
| :-------------- | :---------------------------------------- |
| **Homebrew**    | `brew install --cask y3owk1n/tap/mimi`    |
| **Nix flake**   | `inputs.mimi.url = "github:y3owk1n/mimi"` |
| **From source** | `git clone … && just build`               |

Details, Nix modules, permissions, and launchd setup → [Installation Guide](docs/INSTALLATION.md)

---

## Documentation

| Guide                                      | Contents                                          |
| :----------------------------------------- | :------------------------------------------------ |
| [Installation](docs/INSTALLATION.md)       | Homebrew, Nix, source build, permissions, launchd |
| [CLI](docs/CLI.md)                         | All commands, flags, and examples                 |
| [Configuration](docs/CONFIGURATION.md)     | Hooks, systray, settings, env vars                |
| [Architecture](docs/ARCHITECTURE.md)       | How actions, daemon, and native code fit together |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common issues and fixes                           |
| [Development](docs/DEVELOPMENT.md)         | Build, test, and project layout                   |
| [Contributing](CONTRIBUTING.md)            | How to send PRs and report bugs                   |
| [Security](SECURITY.md)                    | Reporting vulnerabilities                         |

Developer references: [Go conventions](docs/go/CONVENTIONS.md) · [Objective-C guidelines](docs/go/OBJECTIVE_C.md) · [Coding standards](docs/CODING_STANDARDS.md)

---

## Permissions

| Capability                           | Accessibility required |
| :----------------------------------- | :--------------------- |
| `mimi action …`                      | Yes                    |
| Window hooks (`on_window_*`)         | Yes                    |
| Space hooks (`on_workspace_changed`) | No                     |
| Menu bar / daemon lifecycle          | No                     |

---

## How it works

```
CLI actions     →  action  →  native (AX + SkyLight)

Hook daemon     →  observe →  event bus  →  hooks  →  your shell
                      ↓
                  systray (optional menu bar)
```

→ [Architecture Guide](docs/ARCHITECTURE.md)

---

## Contributing

```bash
just build && just lint && just test
```

See [Development Guide](docs/DEVELOPMENT.md) and [Contributing](CONTRIBUTING.md).

---

## License

MIT — see [LICENSE](LICENSE).

<div align="center">

**Made with ❤️ by [y3owk1n](https://github.com/y3owk1n)**

</div>
