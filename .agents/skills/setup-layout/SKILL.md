---
name: setup-layout
description: "Get a mimi user tiling: put the example layouts on their machine even without a repo checkout, pick one that matches how they work, wire [tiling] in config.toml, and prove it with a preview before enabling. Also guides writing a custom layout against the stdin/stdout contract. Use when a mimi user asks to set up tiling, choose a layout, or write their own layout."
---

# Setting up a mimi tiling layout

mimi does not tile. It runs the program named in `[tiling].layout` on every
window event, hands it the windows as JSON on stdin, and applies the frames
the program prints. So setting up tiling is two decisions, which program
and where it lives, plus one config section and one preview. The guide is
`docs/TILING.md` and the programs are `examples/tiling/` in the repo.
Neither ships with a Homebrew install, which is the case this skill exists
for.

## Get the examples onto the machine

Resolve in this order and stop at the first hit:

1. **A checkout in the working directory.** `examples/tiling/rules.py`
   exists. Copy the directory, do not point config at the checkout.
2. **A Nix install.** The package ships the directory at
   `share/mimi/examples/tiling` beside the binary. Find it with
   `dirname "$(dirname "$(readlink -f "$(command -v mimi)")")"`. The user
   can run layouts from the store or keep a copy, see the Tiling on Nix
   section of `docs/INSTALLATION.md`. Running from the store names both
   the interpreter and the layout by store path, so neither depends on the
   daemon's PATH.
3. **Homebrew or a bare binary.** Fetch the files at the installed version:

   ```bash
   tag=$(mimi --version | sed -n '1s/^Mimi version //p')
   case $tag in v*) ;; *) tag=main ;; esac
   dest=~/.config/mimi/tiling
   mkdir -p "$dest"
   for f in rules.py monocle.py columns.py master-stack.py bsp.py stacked.py strip.py README.md; do
     curl -fsSL -o "$dest/$f" "https://raw.githubusercontent.com/y3owk1n/mimi/$tag/examples/tiling/$f"
   done
   chmod +x "$dest"/*.py
   ```

   Matching the tag matters. The JSON contract is the only promised
   interface, and `rules.py` from a newer mimi may use input fields the
   installed daemon does not send.

Whichever way, copy the whole directory. Every layout imports `rules.py`
from its own directory. The layouts need only `python3`, which the Xcode
Command Line Tools provide. Check `python3 --version` runs.

Fetch `docs/TILING.md` the same way when a question outruns
`man mimi-tiling`, which covers the commands but not the contract.

## Pick a layout

Ask how they work, then map it. The README in the examples directory and
the table in the guide describe each in full. In short:

| They want | Layout |
| --- | --- |
| One window at a time, full screen, switch by focus | `monocle.py` |
| Equal columns, nothing to learn | `columns.py` |
| One big window plus a side stack, as dwm or yabai's default | `master-stack.py` |
| Splits that follow where focus is, as Hyprland | `bsp.py` |
| Columns where several windows can share one slot, as yabai stacks | `stacked.py` |
| A strip wider than the screen that scrolls with focus, as niri | `strip.py` |

`stacked.py` needs `[tiling.stackbar]` enabled to show the windows behind.
`monocle.py` is the one to copy when they want to write their own.

`rules.py` holds the float rules: System Settings, Finder, Activity
Monitor, 1Password, and small windows. Ask which apps they never want
tiled and edit the list there rather than in the layout.

## Wire it

1. Add to `config.toml`, or create it with `mimi config init` first:

   ```toml
   [tiling]
   enabled = false
   layout = "~/.config/mimi/tiling/bsp.py"
   ```

   Leave `enabled` off until the preview looks right. `layout` is a
   command line run through `settings.hook_shell`, so arguments work:
   `master-stack.py 0.6`. Do not pass a gap, mimi supplies it from
   `tiling.gap` or the macOS tiled-window margin.

2. `mimi config validate`.

3. **Preview before enabling.** This runs the layout once against the
   real desktop and prints what it would apply, touching nothing:

   ```bash
   mimi tiling preview | jq
   ```

   Empty `frames` with windows open means the float rules ate everything
   or the layout crashed. `mimi tiling preview --input` prints what the
   layout would receive, as an array with one entry per display. Feed one
   entry to the layout by hand for a traceback:

   ```bash
   mimi tiling preview --input | jq -c '.[0]' | ~/.config/mimi/tiling/bsp.py
   ```

4. Set `enabled = true` and save. The whole `[tiling]` section reloads on
   save when the daemon runs, so nothing to restart. Without a daemon,
   `mimi start`. Tiling needs Accessibility, check `mimi status`.

5. Confirm windows moved. Open one more and confirm it was placed. To back
   out, `enabled = false` and save. Windows stay where they are.

## Hotkeys

Layouts answer named commands, and a hotkey tool such as skhd or Hammerspoon
binds a key to `mimi tiling cmd <name> [args]`. Each layout's commands are in
the table. mimi gives the name no meaning, so a name the layout does not
handle runs a pass that changes nothing. Check it against the layout file.
Without a daemon, `mimi tiling cmd` runs the layout itself with a null
state, which is enough to try a command but forgets it. `mimi tiling state`
shows what the daemon holds per space, and `mimi tiling reset` starts the
layout over when its state goes odd.

## Writing a custom layout

Start from `monocle.py` and keep `rules.py` beside it. The guide's Writing
your own layout section carries the full contract and a complete layout in
under thirty lines. The parts that matter:

- Input is one JSON object: `event`, `display`, `space`, `gap`,
  `displays`, `focused`, `windows`, and `state`. Fill `display.visible`,
  never `display.frame`. `mimi tiling preview --input` prints a real one
  per display.
- Output is `{"frames": [...], "state": ...}`. A frame is a window `number`
  and an `x`, `y`, `width`, `height`. Whatever goes in `state` comes back
  on the next pass, which is the only memory a layout has.
- `serve(main)` from `rules.py` handles both layout modes, applies the
  float rules, and lays the desktop out once when run with no stdin. That
  self-run is the fastest test loop.
- Any language works. Startup time is what matters, since the program runs
  once per display per event. `layout_mode = "resident"` removes startup
  from every pass but the first for a program that reads a line at a time
  and flushes.

Test with `mimi tiling preview` after every change, and keep `enabled`
off until the frames look right.
