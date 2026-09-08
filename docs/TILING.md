# Tiling

mimi does not tile. It runs a program you own whenever the desktop changes,
hands it every window and display as JSON, and applies the frames the program
prints back. What a layout looks like, which windows it leaves alone, what a
hotkey means, and what a drag does are all decided in your file, in any
language. mimi keeps the timing, the state, and the hard parts of driving
macOS.

This page is the whole of what you need: get a layout running, understand
the contract, write your own, bind hotkeys, and fix it when it goes wrong.

## Table of Contents

- [Five minutes to a tiled desktop](#five-minutes-to-a-tiled-desktop)
- [How it works](#how-it-works)
- [The contract](#the-contract)
- [Writing your own layout](#writing-your-own-layout)
- [Commands and hotkeys](#commands-and-hotkeys)
- [Drags, moves, and the temporary maximise](#drags-moves-and-the-temporary-maximise)
- [More than one display](#more-than-one-display)
- [Trying a layout without turning it on](#trying-a-layout-without-turning-it-on)
- [When nothing happens](#when-nothing-happens)

---

## Five minutes to a tiled desktop

1. Copy the example layouts somewhere you own. They import each other, so
   copy the directory as a whole.

   ```bash
   cp -r examples/tiling ~/.config/mimi/tiling
   ```

   On Nix the same directory ships in the package at
   `${pkgs.mimi}/share/mimi/examples/tiling`, and the
   [Installation Guide](INSTALLATION.md#tiling-on-nix) shows both running
   them from the store and keeping your own copy under Home Manager.

2. Name one in your config. `mimi config init` creates the file if you have
   none.

   ```toml
   [tiling]
   enabled = true
   layout = "~/.config/mimi/tiling/bsp.py 10"
   ```

3. Look before you leap. This runs the layout once against your desktop and
   prints what it would do, applying nothing.

   ```bash
   mimi tiling preview | jq
   ```

4. Start the daemon, or reload it if it is already running. The whole
   `[tiling]` section is reloadable, so saving the file is enough.

   ```bash
   mimi start            # or: mimi config reload
   ```

Your windows are laid out immediately, and again whenever one opens, closes,
or gains focus. Set `enabled = false` and save to stop; every window stays
where it is.

The three layouts shipped:

| Layout | Shape | Commands it answers |
| --- | --- | --- |
| `columns.py [gap]` | Equal columns. The one to copy when starting your own. | `togglemax` |
| `master-stack.py [ratio] [gap]` | One master on the left, the rest stacked on the right. | `swap`, `ratio <delta>`, `togglemax` |
| `bsp.py [gap]` | Dwindle BSP, as Hyprland tiles by default. | `swap <dir>`, `togglesplit`, `ratio <delta>`, `togglefloat`, `togglemax` |

Windows that should never be tiled (System Settings, Finder, small dialogs)
are listed in `rules.py`. Edit it to taste.

---

## How it works

```
window event ──▶ daemon settles the burst (debounce_ms)
                     │
                     ▼
              runs your program:   stdin  = JSON: event, displays, windows, state
                                   stdout = JSON: frames, state
                     │
                     ▼
              applies the frames, keeps the state for next time
```

- **One run per display:** the program runs once for each display that has
  a window on it, with that display's windows and that display's own state,
  so a layout only ever thinks about one display.
- **Events that wake it:** a window created, closed, or focused; an
  application hidden, unhidden, or quit; a space change; the daemon starting
  with tiling on; a reload that switches it on or names another layout; and,
  when `relayout_on_drag = true`, a window the user moved or resized. Every
  burst of events becomes one run of your program.
- **State:** whatever your program prints as `state` is handed back to it on
  the next run for the same display and space, so a layout remembers a tree
  or a ratio without touching a file. Each display keeps one state per
  Mission Control space in front on it.
- **The engine never fights you:** its own frame writes never wake it, and
  with `relayout_on_drag` it tells a drag you made from a write it made by
  reading back where every window actually landed.
- **Failure is contained:** a program that exits non-zero, times out, or
  prints something that is not the output shape is logged and applies
  nothing. The next event tries again.

The full `[tiling]` reference is in [CONFIGURATION.md](CONFIGURATION.md#tiling).

---

## The contract

Your program reads one JSON document on stdin and prints one on stdout.

### Input

```json
{
  "version": 1,
  "event": {"kind": "window_created", "app": "Safari",
            "bundleId": "com.apple.Safari", "pid": 501},
  "display": {"index": 1, "id": 1,
              "frame":   {"x": 0, "y": 0,  "width": 1440, "height": 900},
              "visible": {"x": 0, "y": 25, "width": 1440, "height": 875}},
  "space": 2,
  "displays": [
    {"index": 1, "id": 1,
     "frame":   {"x": 0, "y": 0,  "width": 1440, "height": 900},
     "visible": {"x": 0, "y": 25, "width": 1440, "height": 875}}
  ],
  "focused": 0,
  "windows": [
    {"number": 4242, "pid": 501, "app": "Safari", "bundleId": "com.apple.Safari",
     "title": "Start Page", "frame": {"x": 0, "y": 25, "width": 1440, "height": 875}}
  ],
  "state": null
}
```

| Field | Meaning |
| --- | --- |
| `version` | Moves only when a field is renamed, removed, or changes meaning. Adding a field does not move it. |
| `event.kind` | Why you were run. A hook name (`window_created`, `window_closed`, `window_focus`, `workspace_changed`, `app_hide`, `app_unhide`, `app_quit`), `startup`, `reload`, `preview`, `relayout`, `command`, `window_move`, or `window_resize`. |
| `event.name`, `event.args` | For a `command`: what the user typed after `mimi tiling cmd`. |
| `event.windows` | For a `window_move` or `window_resize`: the numbers of the windows the user dragged. |
| `display` | The display this run is for. Fill its `visible`, never its `frame`: `visible` is what is left after the menu bar and the Dock. |
| `space` | The 1-based Mission Control space in front on that display. |
| `displays` | Every display, numbered as `move_window_to_display` counts them, for reference. |
| `focused` | Index into `windows` of the focused one, or -1 when the focused window is on another display. |
| `windows` | The focusable windows whose centres are on this display, in `focus_window` order. `number` is the window server's number, stable for the window's lifetime, and how you name a window in the output. |
| `state` | What you printed last time for this display and space, or `null`. |

`displays` and `windows` are exactly what `mimi query displays` and
`mimi query windows` print, so you can look at real data any time.

### Output

```json
{
  "frames": [{"number": 4242, "frame": {"x": 8, "y": 33, "width": 1424, "height": 859}}],
  "state": {"anything": "you like"}
}
```

`frames` is applied in order, every one attempted even if an earlier one
fails. Leave a window out to leave it where it is. Print nothing at all, or
an empty `frames`, to change nothing. `state` may be any JSON.

### Coordinates

Everything is in window coordinates: the origin is the top-left corner of the
primary display and y grows downward. A display above or taller than the
primary has a negative y. This is the same system `mimi action resize_window`
takes for `--x` and `--y`.

---

## Writing your own layout

Start from `columns.py`. It is the whole contract in thirty lines. Here it
is with the shared helpers spelled out, so you can see there is no magic:

```python
#!/usr/bin/env python3
import json, sys

GAP = 8

inp = json.load(sys.stdin)

# 1. Decide which windows you are laying out.
windows = [w for w in inp["windows"] if w["bundleId"] != "com.apple.finder"]
if not windows:
    print(json.dumps({"frames": [], "state": None}))
    sys.exit(0)

# 2. Decide the area to fill. visible, never frame.
v = inp["display"]["visible"]
area = {"x": v["x"] + GAP, "y": v["y"] + GAP,
        "width": v["width"] - 2 * GAP, "height": v["height"] - 2 * GAP}

# 3. Give every window a frame.
n = len(windows)
col = (area["width"] - GAP * (n - 1)) // n
frames = [
    {"number": w["number"],
     "frame": {"x": area["x"] + i * (col + GAP), "y": area["y"],
               "width": col, "height": area["height"]}}
    for i, w in enumerate(windows)
]

# 4. Print it.
print(json.dumps({"frames": frames, "state": None}))
```

Save it, make it executable, point `layout` at it, and run
`mimi tiling preview`. That is a working tiler.

From there, the pieces you will want, in the order you will want them:

**Skip the right windows.** `rules.py` has `floating()` with a set of bundle
identifiers, a title pattern, and a size floor. Use `read_input()` from it and
`windows` arrives already filtered, with `focused` re-pointed.

**Fill the right area.** `area(inp, gap)` from `rules.py` is the display's
visible frame inset by the gap. There is nothing more to multi-display than
that: mimi runs your program once per display, and a window moved to another
monitor simply shows up in that monitor's run next time.

**Remember something.** Print it in `state`, read it back from
`inp["state"]` next time. `master-stack.py` remembers its master and ratio in
two keys. `bsp.py` keeps its whole tree there. There is no file to manage and
the state is per space.

**Answer a command.** See the next section. `command(inp, "name")` from
`rules.py` returns the args when the event is that command, else `None`.

**React to a drag.** See the section after. The engine tells you which window
moved and whether it was a move or a resize.

**Use another language.** Nothing here requires Python. A layout is any
executable that reads stdin and writes stdout. `examples/tiling/README.md`
has a complete one in shell and jq in ten lines. `layout` is a command line
run through `settings.hook_shell`, so arguments are fine.

---

## Commands and hotkeys

```bash
mimi tiling cmd <name> [args...]
```

sends `{"kind": "command", "name": "<name>", "args": [...]}` as the event.
mimi gives the name no meaning. Your file does, which is how a layout defines
its own hotkeys. The examples chose `swap`, `ratio`, `togglesplit`,
`togglefloat`, and `togglemax`; rename them, drop them, or invent your own
and nothing in mimi changes.

In a layout:

```python
args = command(inp, "ratio")       # ["+0.05"] when the event is that command, else None
if args:
    state["ratio"] = clamp(state.get("ratio", 0.6) + float(args[0]), 0.2, 0.8)
```

A name your layout does not handle is a run that changes nothing.

Two more that are not commands: `mimi tiling relayout` sends a `relayout`
event, useful as a "put everything back" key, and `mimi tiling preview` sends
`preview` and applies nothing.

Bind them in whatever you use for hotkeys. With skhd, the set `bsp.py`
answers:

```
alt - h      : mimi tiling cmd swap left
alt - l      : mimi tiling cmd swap right
alt - k      : mimi tiling cmd swap up
alt - j      : mimi tiling cmd swap down
alt - t      : mimi tiling cmd togglesplit
alt - f      : mimi tiling cmd togglefloat
alt - m      : mimi tiling cmd togglemax
alt - r      : mimi tiling relayout
```

Everything after the command name goes to your layout verbatim, so
`mimi tiling cmd ratio -0.05` works without quoting.

With the daemon running, a command reaches its engine and the state it holds.
Without a daemon, the command runs the layout in the CLI with a null state,
which is enough to try one.

---

## Drags, moves, and the temporary maximise

By default a window you drag stays where you dropped it until the next event,
then snaps back. Opt in to something better:

```toml
[tiling]
relayout_on_drag = true
```

Now a drag you make runs your layout, with the event telling it what
happened:

- `window_resize`, with `windows` naming the dragged one, when the size
  changed at least as much as the position. `bsp.py` resizes the split that
  edge belongs to. `master-stack.py` reads the new ratio off either side.
- `window_move`, when the position changed more than the size. `bsp.py` swaps
  the window with the one it was dropped on and snaps it back if dropped on
  nothing. `master-stack.py` makes a stack window dropped on the master the
  master.

The engine decides which by reading back where the windows actually landed,
so a terminal that snaps its width to the character grid on every move still
reads as a move, and the engine's own writes never come back as drags.

**The temporary maximise** is `mimi tiling cmd togglemax` in every shipped
layout: the focused window fills the area over the layout, and the layout's
frames and state underneath are untouched. It ends on a second `togglemax`,
when the window closes, or when you focus another tiled window, so the layout
is back the moment you leave. It is one call from `rules.py`,
`maximised(inp, state, frames, area)`, made last on the frames a layout
computed. Add it to your own layout with that one line.

---

## More than one display

Each display is tiled on its own: mimi runs your program once per display
that has a window, with `display` set to it, `windows` narrowed to the windows
whose centres are on it, and `space` the space in front on it. State is kept
per display and space, so a tree, a master, or a maximised window on one
monitor is never confused with the other's, and switching the space on one
monitor leaves the other monitor's state exactly where it was. A window that
crosses to the other monitor, by drag or by `mimi action
move_window_to_display`, leaves one run and joins the other next time.

`mimi query displays` lists the displays in the order
`move_window_to_display` counts them, with frames in the shared window
coordinate system, so a display below the primary has a y past the primary's
height and one above it has a negative y. `mimi tiling preview` prints one
entry per display.

---

## Trying a layout without turning it on

```bash
mimi tiling preview --input | jq     # what your program will be given
mimi tiling preview | jq             # what it returns, applied to nothing
~/.config/mimi/tiling/standalone.sh ~/.config/mimi/tiling/columns.py   # apply once, no daemon
```

`preview` works with `enabled = false`, so you can iterate on a layout while
the daemon leaves your windows alone, then flip it on. If your program prints
something malformed, `preview` shows you the error the daemon would log.

---

## When nothing happens

Work down this list.

1. **Is Accessibility granted?** Every window read and write needs it. Without
   it the daemon logs `accessibility permission not granted — tiling disabled`
   at startup and on each reload, and `mimi tiling cmd` reports that tiling is
   disabled.
2. **Is it enabled, and is the layout there?** `mimi config validate` rejects
   `enabled = true` with no `layout`. The path is a command line, so `~` is
   expanded but `$HOME` is not.
3. **Does the layout run by hand?** `mimi tiling preview` runs it the way the
   daemon would and shows its stderr. A missing `python3`, a syntax error, or
   a wrong shebang all show up here.
4. **Read the daemon log.** With `log_level = "debug"` every pass logs
   `tiling pass applied` with the event kind and frame count, and a failing
   one logs `tiling pass failed` with the reason. A layout that is slow logs
   `layout timed out`; raise `timeout_secs`.
5. **Is the window one mimi tiles?** `mimi query windows` lists exactly what a
   layout is given: standard windows of regular applications on the current
   space. Sheets, popovers, minimized windows, and full-screen spaces are not
   there. If it is listed but not tiled, check `rules.py`.
6. **A window that will not take its frame.** Some applications enforce a
   minimum size or snap to a grid, and land a little off what was asked. The
   next input shows where things actually are. This is why the shipped
   layouts read ratios off actual frames rather than assuming.
7. **An app that opens windows but never tiles.** Applications that are slow
   to start refuse the daemon's observer for a moment after launch. The daemon
   retries for several seconds and logs a warning naming the app if it gives
   up; a `relayout` will pick the windows up.
8. **It tiles, then un-tiles, then tiles.** A layout that reads a drag and
   emits a different frame every time will loop with an application that
   refuses that frame. Read the actual frame back from the input and treat a
   difference of a few points as "already there".
