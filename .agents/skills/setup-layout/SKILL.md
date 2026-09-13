---
name: setup-layout
description: "Get a mimi user tiling, or change how they tile: put the example layouts on their machine even without a repo checkout, pick one that matches how they work, wire [tiling] in config.toml, and prove it with a preview before enabling. Also covers switching layouts, changing tiling options or float rules, refreshing the examples after an upgrade without losing edits, and writing or editing a custom layout. Use when a mimi user asks to set up or change tiling, choose or switch a layout, or write their own."
---

# Setting up a mimi tiling layout

mimi does not tile. It runs the program named in `[tiling].layout` on every
window event, hands it the windows as JSON on stdin, and applies the frames
the program prints. The guide is `docs/TILING.md` and the programs are
`examples/tiling/` in the repo. Neither ships with a Homebrew install.

## Get the examples onto the machine

Resolve in this order and stop at the first hit:

1. **A checkout in the working directory.** `examples/tiling/rules.py`
   exists. Copy the directory, do not point config at the checkout.
2. **A Nix install.** The package ships the directory at
   `share/mimi/examples/tiling` beside the binary. Find it with
   `dirname "$(dirname "$(readlink -f "$(command -v mimi)")")"`. The user
   can run layouts from the store or keep a copy, see the Tiling on Nix
   section of `docs/INSTALLATION.md`.
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

   Fetch at the tag, not `main`. A newer `rules.py` may read input fields
   the installed daemon does not send. When `$dest` already exists, the
   user may have edited what is in it. Fetch into a new directory instead
   and see Refreshing the examples below.

Whichever way, copy the whole directory. Every layout imports `rules.py`
from its own directory. The shipped layouts need only `python3`, which the
Xcode Command Line Tools provide. Check `python3 --version` runs. A layout
of the user's own can be in any language, see Writing a custom layout.

Fetch `docs/TILING.md` the same way for anything `man mimi-tiling` does
not answer. The man page covers the commands but not the contract.

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

   When the layout exits non-zero, preview prints `layout failed` and the
   layout's stderr. Empty `frames` with windows open means the float rules excluded
   every window. `mimi tiling preview --input` prints what the layout would
   receive, as an array with one entry per display. Feed one entry to the
   layout by hand to see its full output:

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
binds a key to `mimi tiling cmd <name> [args]`. A name the layout does not
handle runs a pass that changes nothing, so check it against the layout
file. Without a daemon, `mimi tiling cmd` runs the layout with a null state,
enough to try a command but it forgets the result. `mimi tiling state` shows
what the daemon holds per space, and `mimi tiling reset` starts the layout
over when its state is wrong.

## Changing an existing setup

**Switching layouts.** Change `layout` in `[tiling]` and save. The daemon
runs the new layout on the next pass without a restart. It hands the new
layout the state the old one left for each space. The shipped layouts
ignore state they did not write, so nothing else is needed between them. A
custom layout may not, so run `mimi tiling reset --all` after switching to
one, and `mimi tiling relayout` to lay the desktop out now.

**Changing options.** Gap, animation, drop zone, stackbar, drag behaviour,
and layout mode are keys under `[tiling]`, all reloadable on save. Edit
the key, run `mimi config validate`, and save. The `[tiling]` section of
`docs/CONFIGURATION.md` lists every key with its default.

**Changing the float rules.** Edit the list in the user's `rules.py`.
Every layout picks it up on its next run.

**Refreshing the examples.** After a mimi upgrade, the examples at the new
tag may read input fields the old ones did not. Never overwrite the user's copy,
since they may have edited `rules.py` or a layout. Fetch the new set into
a fresh directory with the recipe above, changing `dest`, then diff:

```bash
diff -r ~/.config/mimi/tiling /path/to/fresh/tiling
```

Replace the files the user never changed. For a file they changed, show
them the diff and apply only what they choose. Run `mimi tiling preview`
before enabling anything.

**Editing a custom layout.** Keep the state keys the layout already
writes, so the state a running daemon holds stays valid, and run the
layout by hand against `mimi tiling preview --input` after every change.
If the state shape had to change, run `mimi tiling reset --all`.

## Writing a custom layout

Ask which language they want first. A layout is any executable that reads
one JSON object on stdin and writes one on stdout, so Go, Rust, Swift,
Ruby, Lua, or shell with jq all work. `layout` is a command line, so a
compiled binary, a script, or an interpreter plus a file are all fine.
Two things decide the language. Startup time matters, since the program
runs once per display on every window event, and `layout_mode = "resident"`
removes it from every pass but the first for a program that reads a line at
a time and flushes. A JSON library matters, since `state` is how a layout
remembers anything.

In Python, start from `monocle.py` and keep `rules.py` beside it for the
float rules, the area, the command parsing, and the temporary maximise. In
any other language, implement the contract directly. The guide's Writing
your own layout section has the contract, the `rules.py` helpers, a
complete layout in under thirty lines, and a whole layout in shell and jq.
Fetch it, do not work from memory. Two things the guide leaves to the
reader:

- `mimi tiling preview --input` prints a real input per display, so a
  layout can be run by hand against the actual desktop, as in step 3 above.
- A Python layout built on `serve()` lays the desktop out once when run
  with no stdin, which is the fastest test loop while writing one. In any
  other language, pipe one entry of `--input` into it as in step 3 above.

Keep `enabled` off until the preview frames look right.
