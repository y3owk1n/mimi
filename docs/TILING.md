# Tiling

mimi does not tile. It runs a program you own whenever the desktop changes,
hands it every window and display as JSON, and applies the frames the program
prints back. The layout, the windows it leaves alone, what a hotkey means,
and what a drag does are all decided in your file, in any language. mimi
keeps the timing, the state, and the hard parts of driving macOS.

- [Five minutes to a tiled desktop](#five-minutes-to-a-tiled-desktop)
- [How it works](#how-it-works)
- [The contract](#the-contract)
- [Writing your own layout](#writing-your-own-layout)
- [Commands and hotkeys](#commands-and-hotkeys)
- [Drags and the temporary maximise](#drags-and-the-temporary-maximise)
- [More than one display](#more-than-one-display)
- [Trying a layout without turning it on](#trying-a-layout-without-turning-it-on)
- [When nothing happens](#when-nothing-happens)

---

## Five minutes to a tiled desktop

1. Copy the example layouts somewhere you own. They import `rules.py` from
   their own directory, so copy the directory as a whole.

   ```bash
   cp -r examples/tiling ~/.config/mimi/tiling
   ```

   On Nix the same directory ships at
   `${pkgs.mimi}/share/mimi/examples/tiling`. The
   [Installation Guide](INSTALLATION.md#tiling-on-nix) covers running it from
   the store or keeping a copy under Home Manager.

2. Name one in your config. `mimi config init` creates the file if you have
   none.

   ```toml
   [tiling]
   enabled = true
   layout = "~/.config/mimi/tiling/bsp.py"
   ```

3. Look before you leap. This runs the layout once against your desktop and
   prints what it would do, applying nothing.

   ```bash
   mimi tiling preview | jq
   ```

4. Start the daemon, or reload it. The whole `[tiling]` section is
   reloadable, so saving the file is enough when the daemon is running.

   ```bash
   mimi start            # or: mimi config reload
   ```

Your windows are laid out immediately, and again whenever one opens, closes,
or gains focus. Set `enabled = false` and save to stop. Every window stays
where it is.

The layouts shipped, all Python with the standard library only:

| Layout | Shape | Commands it answers |
| --- | --- | --- |
| `monocle.py` | Every window fills the display. Move between them with focus. No state, no commands. The one to copy when starting your own. | none |
| `columns.py` | Equal-width columns. | `togglemax` |
| `master-stack.py [ratio]` | One master on the left, the rest stacked on the right. Remembers the master and the ratio. | `swap`, `ratio <delta>`, `togglemax` |
| `bsp.py` | Dwindle BSP, as Hyprland tiles by default. A new window splits the focused one, closing hands the area back. | `swap <dir>`, `togglesplit`, `ratio <delta>`, `togglefloat`, `togglemax` |
| `strip.py` | Scrollable strip, as niri tiles: columns on a strip wider than the display, focus scrolls it, neighbours peek in at the edges. | `focus <dir>`, `move <dir>`, `consume`, `expel`, `width [fraction\|prev\|+d\|-d]`, `center`, `scroll <dir> [fraction]`, `togglefloat`, `togglemax` |

None of them takes a gap. The gap comes from mimi: `tiling.gap` when the
config sets it, otherwise the macOS tiled-window margin that
`mimi action resize_window` honours, so tiled and hand-placed windows line
up.

Windows that should never be tiled (System Settings, Finder, Activity
Monitor, 1Password, windows titled Preferences or Settings, and anything
smaller than 400 by 300 points) are listed in `rules.py`. Edit it to taste.

---

## How it works

```
window event ---> daemon settles the burst (debounce_ms, default 100)
                      |
                      v
               runs your program once per display:
                   stdin  = JSON: event, display, windows, state
                   stdout = JSON: frames, state, focus
                      |
                      v
               applies every frame in one write, keeps the state
```

- **Events that wake it.** A window created, closed, or focused. An
  application activated, hidden, unhidden, or quit. A space change. The
  daemon's Accessibility observer reaching an application it could not see
  at launch. The daemon starting with tiling on, and a reload that switches
  it on or names another layout. With `relayout_on_drag = true`, a window you
  moved or resized. A burst of events settles into one run.
- **One run per display.** The program runs once for each display that has a
  window on it, with that display's windows and that display's own state, so
  a layout only ever thinks about one display. The frames from every run are
  applied together.
- **State.** Whatever you print as `state` comes back on the next run for
  the same display and space. A layout remembers a tree or a ratio without
  touching a file. Each display keeps one state per Mission Control space.
- **The engine never fights you.** Its own frame writes never wake it. With
  `relayout_on_drag` it tells your drag from its own write by reading back
  where every window actually landed.
- **A stale pass is dropped.** If the space changed while your program ran,
  its frames are discarded, because the switch itself raises the event that
  lays the new space out.
- **Failure is contained.** A program that exits non-zero, runs past
  `timeout_secs` (default 5), or prints something that is not the output
  shape is logged and applies nothing. The next event tries again.

The `[tiling]` keys are `enabled`, `layout`, `debounce_ms`, `timeout_secs`,
`relayout_on_drag`, and `gap`. Every one is reloadable. The reference is in
[CONFIGURATION.md](CONFIGURATION.md#tiling).

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
  "gap": 8,
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
| `event.kind` | Why you were run. A hook name (`window_created`, `window_closed`, `window_focus`, `app_activate`, `app_hide`, `app_unhide`, `app_quit`, `workspace_changed`), `_ax_attached`, `startup`, `reload`, `preview`, `relayout`, `command`, `window_move`, or `window_resize`. |
| `event.app`, `event.bundleId`, `event.pid` | The application behind a hook event, when there is one. |
| `event.name`, `event.args` | For a `command`: what the user typed after `mimi tiling cmd`. |
| `event.windows` | For a `window_move` or `window_resize`: the numbers of the windows the user dragged. |
| `display` | The display this run is for. Fill its `visible`, never its `frame`. `visible` is what is left after the menu bar and the Dock. |
| `space` | The 1-based Mission Control space in front on that display. |
| `gap` | Points to leave between windows and at the display's edges, resolved by mimi: `tiling.gap` when set, else the macOS tiled-window margin, else 0. |
| `displays` | Every display, numbered as `move_window_to_display` counts them. |
| `focused` | Index into `windows` of the focused window, or -1 when the focused window is on another display. |
| `windows` | The focusable windows whose centres are on this display, in `focus_window` order. `number` is the window server's number, stable for the window's lifetime, and how you name a window in the output. |
| `state` | What you printed last time for this display and space, or `null`. |

`displays` and `windows` are exactly what `mimi query displays` and
`mimi query windows` print, so real data is one command away.

### Output

```json
{
  "frames": [{"number": 4242, "frame": {"x": 8, "y": 33, "width": 1424, "height": 859}}],
  "state": {"anything": "you like"},
  "focus": 4242
}
```

| Field | Meaning |
| --- | --- |
| `frames` | The windows to place. Leave a window out to leave it where it is. Print nothing, or an empty list, to change nothing. |
| `state` | Any JSON. Handed back next run. Omit the key and the previous state is kept. Print `null` to clear it. |
| `focus` | Optional. A window number to give keyboard focus once the frames land. For moving focus along a layout's own structure where spatial `focus_window` cannot, such as a strip's parked columns. |

### Coordinates

Everything is in window coordinates: the origin is the top-left corner of the
primary display and y grows downward. A display above the primary has a
negative y. This is the same system `mimi action resize_window` takes for
`--x` and `--y`.

---

## Writing your own layout

A layout is any executable that reads stdin and writes stdout. This is the
whole contract with the shared helpers spelled out:

```python
#!/usr/bin/env python3
import json, sys

inp = json.load(sys.stdin)
GAP = inp["gap"]

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

The shipped layouts import these from `rules.py`, in the order you will want
them:

- **`read_input()`**: the input with `windows` narrowed by `floating()` and
  `focused` re-pointed. Run with nothing on stdin, it lays the desktop out
  once by itself instead (see [Trying a layout](#trying-a-layout-without-turning-it-on)).
- **`gap(inp)`** and **`area(inp, gap)`**: the gap as mimi resolved it, and
  the display's visible frame inset by it.
- **`command(inp, "name")`**: the args when the event is that command, else
  `None`.
- **`maximised(inp, state, frames, area)`**: the temporary maximise, called
  last on the frames a layout computed.
- **`write_output(frames, state, focus=None)`**: rounds frames to whole
  points and prints the output.

**Remember something** by printing it in `state` and reading it back from
`inp["state"]`. `master-stack.py` keeps its master and ratio there.
`bsp.py` keeps its whole tree there.

**Use another language.** `layout` is a command line run through
`settings.hook_shell`, so arguments are fine and nothing requires Python.
Anything that reads stdin, speaks JSON, and writes stdout will do: Go, Rust,
Swift, Ruby, Lua, a shell script. Two things matter more than the language.
Startup time, since the program runs once per display on every window event
and a compiled binary starts in a few milliseconds where Node takes tens.
And a JSON codec, since `state` is how a layout remembers anything. The
examples are Python because it ships with the Xcode Command Line Tools and
needs no library. A whole layout in shell and jq, one column per window:

```sh
#!/bin/sh
jq -c '(.display.visible) as $v | (.windows | length) as $n
  | {frames: [.windows | to_entries[]
      | {number: .value.number,
         frame: {x: ($v.x + .key * ($v.width / $n)), y: $v.y,
                 width: ($v.width / $n), height: $v.height}}],
     state: null}'
```

---

## Commands and hotkeys

```bash
mimi tiling cmd <name> [args...]
```

sends `{"kind": "command", "name": "<name>", "args": [...]}` as the event.
mimi gives the name no meaning. Your file does, which is how a layout defines
its own hotkeys. Everything after the name goes to your layout verbatim, so
`mimi tiling cmd ratio -0.05` works without quoting. A name your layout does
not handle is a run that changes nothing.

```python
args = command(inp, "ratio")       # ["+0.05"] when the event is that command, else None
if args is not None:
    state["ratio"] = clamp(state.get("ratio", 0.6) + float(args[0]), 0.2, 0.8)
```

Two more that are not commands: `mimi tiling relayout` sends a `relayout`
event, a "put everything back" key, and `mimi tiling preview` sends `preview`
and applies nothing.

With the daemon running, a command reaches its engine and the state it holds,
and is refused with a message when tiling is disabled. Without a daemon the
command runs the layout in the CLI with a null state, whether or not tiling
is enabled, which is enough to try one.

Bind them in whatever you use for hotkeys. With skhd, the set `strip.py`
answers. Its `focus` walks the strip itself, since the parked columns all sit
at the edge where spatial focus cannot tell them apart:

```
alt - h         : mimi tiling cmd focus left
alt - l         : mimi tiling cmd focus right
alt - j         : mimi tiling cmd focus down
alt - k         : mimi tiling cmd focus up
alt - shift - h : mimi tiling cmd move left
alt - shift - l : mimi tiling cmd move right
alt - r         : mimi tiling cmd width
alt - c         : mimi tiling cmd center
alt - comma     : mimi tiling cmd consume
alt - period    : mimi tiling cmd expel
alt - f         : mimi tiling cmd togglefloat
alt - m         : mimi tiling cmd togglemax
```

And the set `bsp.py` answers:

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

---

## Drags and the temporary maximise

By default a window you drag stays where you dropped it until the next event,
then snaps back. Opt in to something better:

```toml
[tiling]
relayout_on_drag = true
```

Now a drag runs your layout with the event saying what happened and
`event.windows` naming the windows that moved:

- `window_resize` when the size changed at least as much as the position,
  which includes any dragged edge. `bsp.py` resizes the split that edge
  belongs to. `master-stack.py` reads the new ratio off either side.
  `strip.py` sets the column's width.
- `window_move` when the position changed more than the size. `bsp.py` swaps
  the window with the one it was dropped on. `master-stack.py` makes a stack
  window dropped on the master the master. `strip.py` moves the window into
  the column it was dropped on.

The engine decides which by comparing where the windows landed against where
it last placed them, so a terminal that snaps its width to the character grid
on every move still reads as a move. For a second after each of its own
writes it only reads frames back, so an application settling into a frame is
never mistaken for a drag.

**The temporary maximise** is `mimi tiling cmd togglemax` in every shipped
layout but monocle: the focused window fills the area over the layout, whose
frames and state underneath are untouched. It ends on a second `togglemax`,
when the window closes, or when you focus another tiled window. It is one
call from `rules.py`, `maximised(inp, state, frames, area)`, made last on the
frames a layout computed. Add it to your own layout with that one line.

---

## More than one display

Each display is tiled on its own. mimi runs your program once per display
that has a window, with `display` set to it, `windows` narrowed to the
windows whose centres are on it, and `space` the space in front on it. State
is kept per display and space, so a tree, a master, or a maximised window on
one monitor is never confused with the other's, and switching the space on
one monitor leaves the other's state where it was. A window that crosses to
the other monitor, by drag or by `mimi action move_window_to_display`,
leaves one run and joins the other next time.

`mimi query displays` lists the displays in the order
`move_window_to_display` counts them, in the shared coordinate system.
`mimi tiling preview` prints one entry per display that has a window.

---

## Trying a layout without turning it on

```bash
mimi tiling preview --input | jq     # what your program will be given, layout not run
mimi tiling preview | jq             # what it returns, applied to nothing
~/.config/mimi/tiling/columns.py     # apply once, no daemon, tiling off
```

`preview` works with `enabled = false`, so you can iterate on a layout while
the daemon leaves your windows alone, then flip it on. If your program prints
something malformed, `preview` shows the error the daemon would log.

A shipped layout run with nothing on stdin, from a terminal or a hotkey, lays
the desktop out once by itself. It builds the inputs the daemon would from
`mimi query`, runs itself once per display, and applies the frames with
`mimi action apply_frames`. Tiling stays off and no daemon is needed, which
makes any layout a one-shot command to bind to a key. The event is
`relayout`, the state is null, and the gap is the macOS tiled-window margin,
since no config is read. A layout of your own that uses `read_input()` gets
the same.

---

## When nothing happens

Work down this list.

1. **Is Accessibility granted?** Every window read and write needs it.
   Without it the daemon logs `accessibility permission not granted` with
   `tiling disabled` at startup and on each reload, and `mimi tiling cmd`
   reports that tiling is disabled.
2. **Is it enabled, and is the layout there?** `mimi config validate` rejects
   `enabled = true` with no `layout`. The path is a command line, so `~` is
   expanded but `$HOME` is not.
3. **Does the layout run by hand?** `mimi tiling preview` runs it the way the
   daemon would and shows its stderr. A missing `python3`, a syntax error, or
   a wrong shebang all show up here.
4. **Read the daemon log.** With `log_level = "debug"` every pass logs
   `tiling pass applied` with the event kind, display count, and frame count.
   A failing pass logs `tiling pass failed` with the reason. A slow layout
   logs `layout timed out`. Raise `timeout_secs`.
5. **Is the window one mimi tiles?** `mimi query windows` lists exactly what
   a layout is given: standard windows of regular applications on the current
   space. Sheets, popovers, minimized windows, and full-screen spaces are not
   there. If it is listed but not tiled, check `rules.py`.
6. **A window that will not take its frame.** Some applications enforce a
   minimum size or snap to a grid, and land a little off what was asked. The
   next input shows where things actually are, which is why the shipped
   layouts read ratios off actual frames rather than assuming.
7. **An app that opens windows but never tiles.** Applications slow to start
   refuse the daemon's observer for a moment after launch. The daemon retries
   for several seconds and runs your layout the moment it gets in. If it
   gives up it logs `AX observer install gave up` naming the app. Its windows
   are then tiled only when another event runs a pass, or by
   `mimi tiling relayout`.
8. **It tiles, then un-tiles, then tiles.** A layout that reads a drag and
   emits a different frame every time will loop with an application that
   refuses that frame. Read the actual frame back from the input and treat a
   difference of a few points as "already there".
