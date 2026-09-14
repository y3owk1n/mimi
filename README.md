<div align="center">

# mimi

A macOS command line tool that switches native Spaces and moves, resizes and focuses windows, with SIP left on.

[![Latest Release](https://img.shields.io/github/v/release/y3owk1n/mimi?style=flat-square)](https://github.com/y3owk1n/mimi/releases)
[![License](https://img.shields.io/github/license/y3owk1n/mimi?style=flat-square)](LICENSE)
[![Sponsor](https://img.shields.io/badge/sponsor-%E2%9D%A4-30363D?style=flat-square)](https://github.com/sponsors/y3owk1n)

|  macOS 14+   | SIP                | Status                    |
| :----------: | :----------------: | :-----------------------: |
| Supported    | Leave it enabled   | Early development         |

<sub>Config keys, CLI flags and behaviour may still change between releases. See the [CHANGELOG](CHANGELOG.md).</sub>

[Install](#install) · [What mimi does](#what-mimi-does) · [Configuration](#configuration) · [Compare](#how-mimi-compares) · [Docs](#documentation)

</div>

---

https://github.com/user-attachments/assets/1b21b596-1578-4344-96d3-eaea8a5ab9c0

Every window and space move in mimi is a shell command, so you can bind it to a hotkey or call it from a script.

```bash
mimi action space 2                              # jump to space 2
mimi action move_window_to_space next --follow   # move the window and follow it
mimi action resize_window left-half --cycle      # half, two thirds, a third
mimi action focus_window --left                  # focus by direction
```

## Why mimi

- **Native Spaces.** mimi switches the Mission Control spaces you already have by sending the same dock swipe as the trackpad. It moves a window to another space with no animation.
- **No SIP changes.** mimi needs only the Accessibility permission and loads no scripting addition into the Dock.
- **The daemon is optional.** Every action runs directly from the CLI. Start the daemon for hooks, borders or tiling. While it runs, the CLI sends actions over its socket.
- **Tiling is a program you write.** mimi ships no built-in layout. The daemon runs your program when windows change, passes it the windows as JSON, and applies the frames it prints.
- **Read and change the desktop.** `mimi action` changes it, `mimi query` prints it as JSON, and hooks run your shell commands on app, window and space events.
- **Bad config fails early.** `mimi config validate` rejects unknown hook keys and invalid filters. A reload with errors keeps the previous config running.

---

## Install

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

<details>
<summary>Nix (nix-darwin, home-manager)</summary>

Add `github:y3owk1n/mimi` as a flake input, apply `mimi.overlays.default`, import `mimi.darwinModules.default` or `mimi.homeManagerModules.default`, then:

```nix
services.mimi.enable = true;
services.mimi.config = ''
  [systray]
  enabled = true
'';
```

`pkgs.mimi` uses the release zip and `pkgs.mimi-source` builds from source. Both packages install shell completions, and both modules add a launchd agent. Full examples are in the [Installation Guide](docs/INSTALLATION.md#method-2-nix-flake).

</details>

<details>
<summary>Prebuilt binaries</summary>

Download from [GitHub Releases](https://github.com/y3owk1n/mimi/releases/latest):

| Architecture  | File                    |
| :------------ | :---------------------- |
| Apple Silicon | `mimi-darwin-arm64.zip` |
| Intel         | `mimi-darwin-amd64.zip` |

Each archive has a `.sha256` checksum file.

</details>

<details>
<summary>From source</summary>

Needs Go, the Xcode Command Line Tools and [just](https://github.com/casey/just).

```bash
git clone https://github.com/y3owk1n/mimi.git && cd mimi
just bundle   # builds build/Mimi.app
```

</details>

### First run

Grant **Accessibility** in **System Settings > Privacy & Security > Accessibility**. That is all the CLI needs.

```bash
mimi action space next      # works right away, no daemon
mimi config init            # write ~/.config/mimi/config.toml
mimi services install       # run the daemon at login, for hooks, borders and tiling
mimi status                 # daemon state and permissions
```

### Set up with an agent

The repo ships three skills for coding agents such as Claude Code, Codex, and Cursor. `mimi-ask` answers what mimi can do and which command does it, `mimi-setup-config` writes and applies the config file, and `mimi-setup-layout` gets tiling working. They read the help and docs of the installed version, so a Homebrew install needs no checkout.

```bash
npx skills add y3owk1n/mimi --skill mimi-ask --skill mimi-setup-config --skill mimi-setup-layout
```

---

## What mimi does

Actions and queries work without the daemon. Everything else needs it.

| Layer          | Needs the daemon | What you get                                                                 | Configured in |
| :------------- | :--------------: | :--------------------------------------------------------------------------- | :------------ |
| **Actions**    | No               | Switch spaces, move windows across spaces and displays, resize, focus        | CLI flags     |
| **Queries**    | No               | The active space, windows and displays as JSON                               | CLI flags     |
| **Hooks**      | Yes              | Your shell command on 15 app, window and space events, with filters          | `[hooks]`     |
| **Menu bar**   | Yes              | The active space number, a reload item and the last reload outcome           | `[systray]`   |
| **Borders**    | Yes              | An outline around each window, coloured by focus                             | `[border]`    |
| **Tiling**     | Yes              | Your layout program, run when windows change, with optional animation        | `[tiling]`    |

### The commands

```bash
# Spaces
mimi action space <n|next|prev>
mimi action move_window_to_space <n|next|prev> [--follow]
mimi action move_window_to_display <n|next|prev>

# Focus
mimi action focus_window [--backward | --same-app | --left | --right | --up | --down | --number <id>]
mimi action focus_app Safari                  # switches to the app's space first
mimi action focus_display next                # the window in front on the next display

# Size and place
mimi action resize_window <preset> [--cycle]  # halves, quadrants, thirds, two thirds, center, fill
mimi action resize_window center --width-percent 80 --height-percent 90
mimi action resize_window --width 1024 --height 768 --anchor br
mimi action resize_window --dx -50 --dw 100   # move and grow from where it is
my-layout | mimi action apply_frames          # apply frames from any program

# Close, minimize, full screen
mimi action close_window | minimize_window | fullscreen_window [--number <id>]
mimi action unminimize_window --number <id>   # ids from: mimi query minimized

# Read the desktop
mimi query space | spaces | window | windows | minimized | displays | margins
```

`resize_window` honours the macOS tiled-window margins setting, so hand-placed and tiled windows line up. Every flag and preset is in the [CLI Reference](docs/CLI.md).

---

## Configuration

Config is one TOML file at `~/.config/mimi/config.toml`. Saving it reloads a running daemon, and so does `mimi config reload`.

**Bind to hotkeys.** Every action is a plain shell command, so any hotkey tool works: skhd, Raycast Script Commands, Alfred, Karabiner, Hammerspoon, BetterTouchTool.

```bash
# ~/.skhdrc
alt - n         : mimi action space next
alt - p         : mimi action space prev
shift + alt - n : mimi action move_window_to_space next --follow
shift + alt - h : mimi action resize_window left-half --cycle
shift + alt - l : mimi action resize_window right-half --cycle
alt - h         : mimi action focus_window --left
alt - l         : mimi action focus_window --right
```

**Hooks.** Filter a hook by app name or bundle ID glob, window title regex, or space number. A leading `!` negates a filter. mimi passes event details as environment variables and quotes each value for the shell.

```toml
[hooks]
on_workspace_changed = [
  { run = "sketchybar --trigger space_change INDEX=$mimi_SPACE_INDEX" },
  { run = "sketchybar --trigger work_mode", space = 2 },
]
on_app_launch = [
  { run = "echo launched $mimi_APP_NAME >> ~/apps.log", app = "!Finder", async = true },
]
```

**Borders.** The daemon draws an outline around each window, like JankyBorders, in one colour for the focused window and another for the rest.

```toml
[border]
enabled = true
width = 4
active_color = "#e2e2e3"     # #rrggbb or #aarrggbb, alpha first
inactive_color = "#414141"
```

**Tiling.** Copy the example layouts and set `layout` to one of them. The `mimi-setup-layout` skill fetches them for an install without a checkout.

```bash
cp -r examples/tiling ~/.config/mimi/tiling
```

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/bsp.py"
relayout_on_drag = true          # drag an edge to resize a split, drop on a window to swap

[tiling.animation]
enabled = true
```

Six layouts ship as starting points, all Python with the standard library only: `monocle`, `columns`, `master-stack`, a Hyprland-style dwindle `bsp`, a yabai-style `stacked`, and a niri-style scrollable `strip`. Each layout defines its own commands, and you bind them like any other action.

https://github.com/user-attachments/assets/9d0cbed9-c03c-4985-968f-dd78d7ca6f69

https://github.com/user-attachments/assets/0d6d3b5d-d15f-4305-8a64-cf86025e4929

```bash
mimi tiling preview | jq     # print the frames without applying them
mimi tiling cmd swap left    # your layout decides what swap means
mimi tiling cmd togglemax    # temporary maximise, in every shipped layout but monocle
```

[Configuration Reference](docs/CONFIGURATION.md) · [CLI Reference](docs/CLI.md) · [Tiling Guide](docs/TILING.md)

---

## How mimi compares

| Tool                                                  | Approach                                          | Spaces                 | Needs SIP changes | Open source |
| :---------------------------------------------------- | :------------------------------------------------ | :--------------------- | :---------------: | :---------: |
| **mimi**                                              | Commands, hooks, borders, and a layout you write  | Native                 | No                | Yes         |
| [yabai](https://github.com/koekeishiya/yabai)         | BSP, stack and float tiling with a query CLI      | Native                 | For space control | Yes         |
| [AeroSpace](https://github.com/nikitabobko/AeroSpace) | i3-style tree tiling                              | Its own workspaces     | No                | Yes         |
| [Amethyst](https://github.com/ianyh/Amethyst)         | xmonad-style automatic layouts                    | Native                 | No                | Yes         |
| [Rectangle](https://rectangleapp.com/)                | Snap to presets with shortcuts                    | Not managed            | No                | Yes         |
| [Hammerspoon](https://www.hammerspoon.org/)           | General macOS automation in Lua                   | Native, `hs.spaces`    | No                | Yes         |

mimi fits if you want to keep native Spaces and control them from the keyboard, or you want to write your tiling layout as a program.

---

## How it works

```
mimi action ... -> daemon socket if running, else in-process -> Accessibility + SkyLight

daemon -> app, window and space observers -> event bus -> your hooks
                                                       -> borders
                                                       -> your layout program -> frames -> Accessibility
                                                       -> menu bar
```

Space switching sends a synthetic dock swipe through `CGEvent`. Window-to-space moves use private SkyLight calls. Everything else is public Accessibility. The private paths are timing-sensitive and can break on a macOS update. [Architecture](docs/ARCHITECTURE.md)

---

## Documentation

| Using mimi                                       |                                                      |
| :----------------------------------------------- | :--------------------------------------------------- |
| [Installation](docs/INSTALLATION.md)             | Homebrew, Nix, source, permissions, completions      |
| [CLI Reference](docs/CLI.md)                     | Every command, flag and preset                       |
| [Configuration Reference](docs/CONFIGURATION.md) | Settings, hooks, environment variables, borders      |
| [Tiling Guide](docs/TILING.md)                   | From first run to writing a layout from scratch      |
| [Troubleshooting](docs/TROUBLESHOOTING.md)       | Common issues and fixes                              |

| Working on mimi                          |                                          |
| :--------------------------------------- | :--------------------------------------- |
| [Contributing](CONTRIBUTING.md)          | How to propose and land a change         |
| [Development Guide](docs/DEVELOPMENT.md) | Toolchain, building, testing             |
| [Architecture](docs/ARCHITECTURE.md)     | Execution paths and native bridges       |

---

## Contributing

mimi is written in Go, with Objective-C in `internal/native`, `internal/systray` and `internal/permissions`. `devbox shell` provisions the toolchain.

```bash
just fmt && just lint && just test && just build   # the pre-commit gate
```

Report bugs through the [issue form](https://github.com/y3owk1n/mimi/issues/new/choose). [Contributing Guide](CONTRIBUTING.md)

---

## Support the project

One person builds mimi in their spare time. If you use it, you can [sponsor it](https://github.com/sponsors/y3owk1n).

[neru](https://github.com/y3owk1n/neru) adds keyboard hints, grids and vim-style scrolling for clicking and scrolling inside apps. mimi's window and space code started in neru.

## License

MIT. See [LICENSE](LICENSE).

<div align="center">
<br/>

**Install and switch to the next space:**

```bash
brew install --cask y3owk1n/tap/mimi && mimi action space next
```

Made with ❤️ by <a href="https://github.com/y3owk1n">y3owk1n</a>

</div>
