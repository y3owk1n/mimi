# mimi — macOS Event Daemon

> [!WARNING]
> This project is still in early development. It is not yet ready for production use.

mimi listens to macOS system events (app focus, sleep/wake, volume mount, etc.)
and executes shell commands you define in `~/.config/mimi/config.toml`.

## Quick Start

```bash
# Install via Homebrew (once available)
# brew install mimi

# Or build from source
just dev
./bin/mimi start
```

## Configuration

Edit `~/.config/mimi/config.toml`:

```toml
[settings]
log_file  = "~/.local/share/mimi/mimi.log"
log_level = "info"

[hooks]
on_app_activate = [
    "echo 'activated: $mimi_APP_NAME'"
]
on_system_sleep = ["pmset displaysleepnow"]
```

Validate with:

```bash
mimi config validate
```

## Commands

| Command                | Description                          |
| ---------------------- | ------------------------------------ |
| `mimi start`           | Start the daemon                     |
| `mimi stop`            | Stop the daemon                      |
| `mimi status`          | Show daemon status and recent events |
| `mimi install`         | Install as launchd user agent        |
| `mimi uninstall`       | Remove launchd agent                 |
| `mimi events`          | Tail the live event stream           |
| `mimi test <event>`    | Fire a synthetic event to test hooks |
| `mimi config validate` | Parse and validate the config file   |

## Events

See [docs/events.md](docs/events.md) for the full event reference.

## License

MIT
