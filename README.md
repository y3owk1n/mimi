<div align="center">

# mimi

**macOS windows and spaces. From the terminal.**

[![Go Version](https://img.shields.io/github/go-mod/go-version/y3owk1n/mimi?style=flat-square&logo=go)](https://github.com/y3owk1n/mimi)
[![License](https://img.shields.io/github/license/y3owk1n/mimi?style=flat-square)](LICENSE)
[![Early Development](https://img.shields.io/badge/status-early%20dev-orange?style=flat-square)](#)

</div>

---

https://github.com/user-attachments/assets/1b21b596-1578-4344-96d3-eaea8a5ab9c0

---

You already know your way around a terminal. Why are you still reaching for the trackpad just to move a window?

**mimi** gives you one-shot commands to jump spaces, move windows, cycle focus, and resize — bind them to hotkeys, drop them in dotfiles, wire them to shell hooks. No SIP disable. No tiling paradigm imposed: if you want your windows laid out for you, the layout is a small program you write, and mimi runs it. Just commands that do what they say.

```bash
mimi action space 2                      # jump to space 2
mimi action move_window_to_space next    # throw window forward
mimi action resize_window left-half      # tile left
mimi action focus_window                 # cycle focus
```

> **Early development** — config format, CLI, and behavior may change between releases.

---

## Install

```bash
brew tap y3owk1n/tap
brew install --cask y3owk1n/tap/mimi
```

Grant **Accessibility** in **System Settings → Privacy & Security → Accessibility**, then start using it immediately. No daemon required.

Other options (Nix flake, build from source) → [Installation Guide](docs/INSTALLATION.md)

---

## What mimi does

| You want to…                     | Command                                                                   |
| :------------------------------- | :------------------------------------------------------------------------ |
| Jump to a specific space         | `mimi action space <n>`                                                   |
| Jump to next / previous space    | `mimi action space next` / `prev`                                         |
| Move frontmost window to a space | `mimi action move_window_to_space <n\|next\|prev>`                        |
| Move a window and go with it     | `mimi action move_window_to_space next --follow`                          |
| Move a window to another display | `mimi action move_window_to_display <n\|next\|prev>`                      |
| Cycle focus between windows      | `mimi action focus_window`                                                |
| Cycle focus backward             | `mimi action focus_window --backward`                                     |
| Cycle within the frontmost app   | `mimi action focus_window --same-app`                                     |
| Jump to an app, switching space  | `mimi action focus_app Safari`                                            |
| Focus window to the left         | `mimi action focus_window --left`                                         |
| Focus window to the right        | `mimi action focus_window --right`                                        |
| Focus window above               | `mimi action focus_window --up`                                           |
| Focus window below               | `mimi action focus_window --down`                                         |
| Tile window to a preset          | `mimi action resize_window <left-half\|right-half\|center\|fill>`         |
| Tile to a third                  | `mimi action resize_window <left-third\|center-third\|right-third>`      |
| Cycle a half through its thirds  | `mimi action resize_window left-half --cycle`                             |
| Center at specific size          | `mimi action resize_window center --width-percent 80 --height-percent 90` |
| Resize to exact pixels           | `mimi action resize_window --width 1024 --height 768`                     |
| Resize anchored to a corner      | `mimi action resize_window --width 1024 --height 768 --anchor br`         |
| Read the active space as JSON    | `mimi query space`                                                        |
| Read the frontmost window's frame| `mimi query window`                                                       |
| List every window on the space   | `mimi query windows`                                                      |
| List every display               | `mimi query displays`                                                     |
| Apply a layout from a script     | `my-layout \| mimi action apply_frames` (see `examples/tiling/`)           |
| Tile automatically, your way     | `[tiling]` in the config, see `docs/TILING.md`                            |

Full reference → [CLI Guide](docs/CLI.md)

---

## Bind to hotkeys

Every action is a plain shell command. Drop it into whatever hotkey tool you already use.

**[skhd](https://github.com/koekeishiya/skhd)** — the natural pairing if you're in the yabai ecosystem:

```bash
# ~/.skhdrc
alt - 2         : mimi action space 2
alt - n         : mimi action space next
alt - p         : mimi action space prev
shift + alt - l : mimi action resize_window right-half
shift + alt - h : mimi action resize_window left-half
shift + alt - m : mimi action move_window_to_space next
shift + alt - f : mimi action focus_window
```

**[Raycast](https://www.raycast.com/)** — create a Script Command pointing to any `mimi action …` line.

**[Alfred](https://www.alfredapp.com/)** — wire up a Shell Script workflow step, same idea.

**Karabiner, Hammerspoon, BetterTouchTool** — if it can run a shell command on a keypress, mimi works with it.

---

## Fits where you are

mimi has no layout of its own, enforces no window rules, and doesn't replace Mission Control. It's not trying to.

[yabai](https://github.com/koekeishiya/yabai) and [AeroSpace](https://github.com/nikitabobko/AeroSpace) are excellent — and a significant commitment. If you've tried them and found it was more than you needed, or if you just want to stay on native macOS Spaces and drive them faster, mimi is for you. And if you do want tiling, mimi will run yours: see [Optional: tiling, your way](#optional-tiling-your-way).

---

## Optional: daemon + hooks

Start the daemon and mimi can react to what's happening on screen — fire a shell command whenever a window focuses, a space changes, or an app launches.

```bash
mimi config init   # creates ~/.config/mimi/config.toml
mimi start
mimi status        # verify everything's running
```

Edit `~/.config/mimi/config.toml`:

```toml
[systray]
enabled = true
show_workspace_number = true   # current space number in your menu bar

[hooks]
on_window_focus      = ['echo "$mimi_APP_NAME — $mimi_WINDOW_TITLE"']
on_workspace_changed = ['sketchybar --trigger space_change INDEX=$mimi_SPACE_INDEX']
on_app_launch        = ['osascript -e "display notification \"$mimi_APP_NAME launched\""']
```

The `[systray]` block shows the active space number in your menu bar while the daemon runs — no extra setup.

### Available hooks

| Event                  | Hook key                                 | Needs Accessibility |
| :--------------------- | :--------------------------------------- | :------------------ |
| App activated          | `on_app_activate`                        | Yes                 |
| App deactivated        | `on_app_deactivate`                      | Yes                 |
| App launched           | `on_app_launch`                          | No                  |
| App quit               | `on_app_quit`                            | No                  |
| App hidden / unhidden  | `on_app_hide` / `on_app_unhide`          | Yes                 |
| Window focused         | `on_window_focus`                        | Yes                 |
| Window title changed   | `on_window_title_change`                 | Yes                 |
| Window opened / closed | `on_window_created` / `on_window_closed` | Yes                 |
| Window resized         | `on_window_resize`                       | Yes                 |
| Window moved           | `on_window_move`                         | Yes                 |
| Active space changed   | `on_workspace_changed`                   | No                  |

Hooks support app, bundle, title and space filters, each negatable with a leading `!`, plus async execution and per-hook timeouts.
Full details → [Configuration Guide](docs/CONFIGURATION.md)

### Daemon commands

```bash
mimi start                  # start the hook daemon
mimi stop                   # stop it
mimi status                 # check daemon state and permissions
mimi config validate        # validate config before reloading
mimi config reload          # hot-reload config (no restart needed)
mimi services install       # auto-start at login via launchd
mimi services uninstall     # remove the launchd agent
```

Auto-start setup → [Installation Guide — launchd](docs/INSTALLATION.md#auto-start-launchd)

---

## Optional: tiling, your way

mimi ships no layout. With `[tiling]` on, the daemon runs a program you own whenever a window opens, closes, or gains focus — or when you drag one — hands it every window and display as JSON, and applies the frames it prints back. What the layout looks like, which windows it leaves alone, what a hotkey does, and what a drag means are all decided in your file, in any language. State is kept per display and space, so a layout remembers its tree or its ratio without touching disk.

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/bsp.py 10"   # copied from examples/tiling/
relayout_on_drag = true                      # drag an edge to resize a split, drop on a window to swap
```

Three layouts ship as starting points to copy and edit — equal columns, master and stack, and a Hyprland-style dwindle BSP — plus the commands they answer, which you bind like any other:

```bash
mimi tiling preview          # what your layout would do, applied to nothing
mimi tiling cmd swap left    # a name your layout gives meaning to
mimi tiling cmd togglemax    # temporary maximise, in every shipped layout
```

Everything from first run to writing a layout from scratch → [Tiling Guide](docs/TILING.md)

---

## How it works

Space switching uses a synthetic dock-swipe gesture — the same path Mission Control uses, no hacks. Window-to-space moves use the private SkyLight API for instant, animation-free relocation. Everything else goes through public Accessibility APIs.

```
CLI actions  →  action handler  →  AX API + SkyLight

daemon  →  observe events  →  event bus  →  your shell hooks
                                    ↓
                             your layout program (optional)  →  frames  →  AX API
                                    ↓
                             menu bar (optional)
```

→ [Architecture Guide](docs/ARCHITECTURE.md)

---

## Documentation

| Guide                                      | What's in it                                |
| :----------------------------------------- | :------------------------------------------ |
| [Installation](docs/INSTALLATION.md)       | Homebrew, Nix, source, permissions, launchd |
| [CLI](docs/CLI.md)                         | Every command and flag                      |
| [Configuration](docs/CONFIGURATION.md)     | Hooks, env vars, systray, all settings      |
| [Tiling](docs/TILING.md)                   | Running and writing your own layout         |
| [Architecture](docs/ARCHITECTURE.md)       | How the pieces fit                          |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common issues and fixes                     |
| [Contributing](CONTRIBUTING.md)            | PRs and bug reports                         |

---

## Contributing

```bash
just build && just lint && just test
```

→ [Development Guide](docs/DEVELOPMENT.md)

---

## From the same workshop

mimi's window management and space-switching code was part of **[neru](https://github.com/y3owk1n/neru)** — a broader tool for navigating your entire screen without touching the mouse.

Where mimi is focused on moving and resizing windows, neru covers the rest: labels on every clickable element, recursive grid navigation, vim-style scrolling — the kind of thing Vimium does in a browser, but system-wide.

If you find yourself still reaching for the mouse _inside_ apps, neru is the natural next step.

```bash
brew install --cask y3owk1n/tap/neru
```

---

## License

MIT — see [LICENSE](LICENSE).

<div align="center">
<br/>

**Try it. Two commands and you're running.**

```bash
brew install --cask y3owk1n/tap/mimi && mimi action space next
```

<br/>
Made with ❤️ by <a href="https://github.com/y3owk1n">y3owk1n</a>
</div>
