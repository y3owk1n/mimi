# Installation Guide

Mimi is a macOS-only daemon. This guide covers building from source and installing as a launchd agent.

---

## Table of Contents

- [Requirements](#requirements)
- [Build from Source](#build-from-source)
- [Install as Launchd Agent](#install-as-launchd-agent)
- [Post-Installation](#post-installation)
- [Uninstallation](#uninstallation)

---

## Requirements

- macOS 14.0 or later
- Go 1.26+
- Xcode Command Line Tools (`xcode-select --install`)
- Just command runner (`brew install just`)

---

## Build from Source

```bash
git clone https://github.com/y3owk1n/mimi.git
cd mimi

# Install just (if you don't have it)
brew install just

# Development build
just build

# Run directly
./bin/mimi start

# Or install globally
cp ./bin/mimi /usr/local/bin/mimi
```

### Build Commands

| Command        | Description                |
| -------------- | -------------------------- |
| `just build`   | Development build          |
| `just release` | Optimised build (stripped) |
| `just bundle`  | Build Mimi.app bundle      |
| `just clean`   | Remove build artifacts     |

---

## Install as Launchd Agent

For automatic startup at login:

```bash
# Build first, then install
mimi install
```

This creates `~/Library/LaunchAgents/com.y3owk1n.mimi.plist` and loads it with `launchctl load -w`. The agent runs with:

- `KeepAlive = true` (auto-restart if crashed)
- `RunAtLoad = true` (start at login)
- `StandardOutPath` and `StandardErrorPath` point to `~/Library/Logs/mimi/`

### Verify Installation

```bash
launchctl list | grep mimi
mimi status
```

### Remove Agent

```bash
mimi uninstall
```

---

## Post-Installation

### Accessibility Permission

Window events (`window_focus`, `window_title_change`, etc.) require Accessibility permission:

**System Settings → Privacy & Security → Accessibility → enable "mimi"** (or your terminal emulator if running from CLI).

Mimi will warn on startup if this permission is missing. All other events (app lifecycle, power, USB, network, clipboard, etc.) work without Accessibility permission.

### Verify

```bash
mimi status              # Should show "running" or "not running"
mimi config validate     # Should show no errors
```

### Default Config

Mimi creates a default config on first `mimi start` if none exists. See [CONFIGURATION.md](CONFIGURATION.md) for all options.

---

## Uninstallation

```bash
# Stop and remove launchd agent
mimi uninstall

# Remove binary
rm /usr/local/bin/mimi

# Remove config and data
rm -rf ~/.config/mimi
rm -rf ~/.local/share/mimi

# Remove logs
rm -rf ~/Library/Logs/mimi
```
