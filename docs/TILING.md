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
Monitor, 1Password, and anything smaller than 400 by 300 points) are
listed in `rules.py`. Edit it to taste.

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

- **Events that wake it.** A window created, closed, focused, minimized, or
  restored from the Dock. An application activated, hidden, unhidden, or
  quit. A space change, which entering or leaving full screen also fires.
  A display showing a full-screen space gets no run, and the rest lay out
  as usual. The daemon's Accessibility observer reaching an application it
  could not see at launch. The daemon starting with tiling on, and a reload that switches
  it on or names another layout. A run for a new window waits up to half a
  second for the window server to list it, since Accessibility reports the
  window a little before it is on screen. With `relayout_on_drag = true`, a window you
  moved or resized. A burst of events settles into one run.
- **One run per display.** The program runs once for each display that has a
  window on it, with that display's windows and that display's own state, so
  a layout only ever thinks about one display. The frames from every run are
  applied together.
- **State.** Whatever you print as `state` comes back on the next run for
  the same display and space. A layout remembers a tree or a ratio without
  touching a file. Each display keeps one state per space, filed under the
  space rather than its place in Mission Control, so adding or removing a
  space never hands a layout the state it built for another one.
- **The engine never fights you.** Its own frame writes never wake it. With
  `relayout_on_drag` it tells your drag from its own write by reading back
  where every window actually landed.
- **A stale pass is dropped.** If the space changed while your program ran,
  its frames are discarded, because the switch itself raises the event that
  lays the new space out.
- **Failure is contained.** A program that exits non-zero, runs past
  `timeout_secs` (default 5), or prints something that is not the output
  shape is logged and applies nothing. The next event tries again.

The `[tiling]` keys are `enabled`, `layout`, `layout_mode`, `debounce_ms`,
`timeout_secs`, `command_timeout_secs`, `relayout_on_drag`, and `gap`, plus a `[tiling.animation]`
table with `enabled`, `duration_ms`, and `easing`, and a `[tiling.dropzone]`
table that shows where a drag would land. Every one is reloadable.
The reference is in [CONFIGURATION.md](CONFIGURATION.md#tiling).

**Animation is off by default.** With `[tiling.animation]` enabled, windows
move to their frames over `duration_ms` instead of jumping. The daemon moves
the real windows through their applications a step at a time, which needs
nothing beyond the Accessibility permission tiling already has.
[CONFIGURATION.md](CONFIGURATION.md#animation) has the details.

---

## The contract

Your program reads one JSON document on stdin and prints one on stdout.
With `layout_mode = "resident"` in the config, it keeps going. It reads one
document per line on stdin for as long as stdin stays open, and prints one
line of output per document, flushed after each. The shipped layouts do
both, through `serve()` in `rules.py`.

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
     "title": "Start Page", "order": 0,
     "frame": {"x": 0, "y": 25, "width": 1440, "height": 875}}
  ],
  "state": null
}
```

| Field | Meaning |
| --- | --- |
| `version` | Moves only when a field is renamed, removed, or changes meaning. Adding a field does not move it. |
| `event.kind` | Why you were run. A hook name (`window_created`, `window_closed`, `window_focus`, `window_minimize`, `window_unminimize`, `app_activate`, `app_hide`, `app_unhide`, `app_quit`, `workspace_changed`), `_ax_attached`, `startup`, `reload`, `preview`, `relayout`, `command`, `window_move`, or `window_resize`. |
| `event.app`, `event.bundleId`, `event.pid` | The application behind a hook event, when there is one. |
| `event.name`, `event.args` | For a `command`: what the user typed after `mimi tiling cmd`. |
| `event.windows` | For a `window_move` or `window_resize`: the numbers of the windows the user dragged. |
| `display` | The display this run is for. Fill its `visible`, never its `frame`. `visible` is what is left after the menu bar and the Dock. |
| `space` | The 1-based Mission Control space in front on that display. |
| `gap` | Points to leave between windows and at the display's edges, resolved by mimi: `tiling.gap` when set, else the macOS tiled-window margin, else 0. |
| `displays` | Every display, numbered as `move_window_to_display` counts them. |
| `focused` | Index into `windows` of the focused window, or -1 when the focused window is on another display. |
| `windows` | The focusable windows whose centres are on this display, in `focus_window` order. `number` is the window server's number, stable for the window's lifetime, and how you name a window in the output. `order` is where the window sits in the stacking order, 0 for the one in front. |
| `state` | What you printed last time for this display and space, or `null`. |
| `unmanaged` | The windows on this display you last said you were not managing, by number. Absent when there are none. Handed back so a layout that keeps its floats outside `state`, or one restarted mid-session, can pick the set up again. |

`displays` and `windows` are exactly what `mimi query displays` and
`mimi query windows` print, so real data is one command away.

The `windows` list is ordered by position, which is the order `focus_window`
cycles through them. `order` answers a different question: which window is on
top. The two rarely agree. Read `order` when the layout cares which of two
overlapping windows the user can see, and ignore it otherwise. The numbers
rank the whole desktop's windows and are then narrowed to this display's, so
they compare correctly but need not start at 0 or run without gaps.

### Output

```json
{
  "frames": [{"number": 4242, "frame": {"x": 8, "y": 33, "width": 1424, "height": 859}}],
  "state": {"anything": "you like"},
  "focus": 4242,
  "unmanaged": [4243],
  "before": ["osascript -e 'beep'"],
  "after": ["~/.config/mimi/warp.py 720 462"]
}
```

| Field | Meaning |
| --- | --- |
| `frames` | The windows to place. Leave a window out to leave it where it is. Print nothing, or an empty list, to change nothing. With `[tiling.animation]` on, a frame may carry `"animate": false` to move that one window at once while the rest animate, for a window with no sensible starting position. |
| `state` | Any JSON. Handed back next run. Omit the key and the previous state is kept. Print `null` to clear it. |
| `focus` | Optional. A window number to give keyboard focus, before the frames move. For moving focus along a layout's own structure where spatial `focus_window` cannot, such as a strip's parked columns. |
| `unmanaged` | Optional. The windows this run was given that you are leaving alone, by number, such as the ones you float. mimi then leaves them alone too, so dragging one raises no pass and shows no drop zone. A window stays unmanaged until a later run for the same display leaves it out of this list. |
| `before` | Optional. Command lines mimi runs through `settings.hook_shell` before the focus and the frames, all at once. mimi waits for every one and kills one past `tiling.command_timeout_secs`. A failure logs at debug and the frames still apply. See [Running commands around the frames](#running-commands-around-the-frames). |
| `after` | Optional. Command lines mimi runs through `settings.hook_shell` once the frames have been applied, and once the animation has ended when one runs. They run in order, detached. mimi kills one past `tiling.command_timeout_secs`, drops their output and logs a failure at debug. Use it to act on the frames the layout returned, where a hook would run before them. See [Running commands around the frames](#running-commands-around-the-frames). |

**Leaving a window out of `frames` is not the same as naming it in
`unmanaged`.** Omitting a frame says only that the window does not move this
pass, which is exactly what a temporary maximise does to the windows under the
maximised one, and mimi keeps watching those so a drag of one still reaches
you. Naming a window in `unmanaged` says you have no opinion about where it
goes at all, and mimi stops watching it until you claim it again.

`rules.py` does this for you. `narrow()` records the windows the float rules
filtered out, `unmanaged_of(inp, state)` adds any the layout floated itself
with `togglefloat`, and `write_output(..., unmanaged=...)` prints the result.

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

- **`serve(main)`**: runs `main(inp)` on every input mimi sends, oneshot or
  resident, with `windows` narrowed by `floating()` and `focused` re-pointed.
  Run with nothing on stdin, it lays the desktop out once by itself instead
  (see [Trying a layout](#trying-a-layout-without-turning-it-on)).
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
`layout_mode = "resident"` takes startup out of every pass but the first for
a program that reads a line at a time and flushes its answers. And a JSON
codec, since `state` is how a layout remembers anything. The
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

### Running commands around the frames

A hook fires on the raw event, before the engine has settled the burst and
written the frames, so a hook that reads the focused window's frame may read
its old one. The `before` and `after` keys run command lines from inside the
pass instead. `before` lines run first, and the pass waits for each of them
to finish before it writes the frames. `after` lines run once the frames
have been applied, and the pass does not wait for them. The layout decides
on every run whether to print either key, since it reads `event.kind`.
Nothing runs on a pass where a key is absent or empty, and nothing runs on
a pass the engine skips because the space changed under it.

Warping the cursor to the window that gained focus, on the two events that
mean focus moved, with the centre taken from the frame the layout is about
to return:

```python
import os, sys

def cursor_run(inp, frames):
    if inp["event"]["kind"] not in ("window_focus", "app_activate") or inp["focused"] < 0:
        return []
    focused = inp["windows"][inp["focused"]]["number"]
    for number, f in frames:
        if number == focused:
            x, y = f["x"] + f["width"] / 2, f["y"] + f["height"] / 2
            return [f"{sys.executable} ~/.config/mimi/warp.py {x:.0f} {y:.0f}"]
    return []

out = {"frames": ..., "state": state}
after = cursor_run(inp, frames)
if after:
    out["after"] = after
```

And `warp.py`, standard library only, taking the point in the same
window coordinates the frames use:

```python
import ctypes, sys

class CGPoint(ctypes.Structure):
    _fields_ = [("x", ctypes.c_double), ("y", ctypes.c_double)]

cg = ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
cg.CGWarpMouseCursorPosition.argtypes = [CGPoint]
x, y = (float(v) for v in sys.argv[1:3])
cg.CGWarpMouseCursorPosition(CGPoint(x, y))
```

Each `after` line runs through `settings.hook_shell`, detached, so a slow
command never holds a pass. mimi drops its output and logs a non-zero exit
at debug. With `[tiling.animation]` on, the lines start once the animation
has ended, so a command that reads a window's frame reads the final one.
The lines run in order, one after another, and mimi kills one past
`tiling.command_timeout_secs`.

A `before` line is for something that must have happened by the time the
frames are written. An application told to leave a window alone, or a
border hidden for the move. mimi starts every `before` line at once and
waits for all of them, so the pass waits as long as the slowest line.
Do not make one depend on another. mimi kills a line past
`tiling.command_timeout_secs` (default 1). The bound is tighter than the
layout's because the frames wait on it. A `before` line that fails does
not stop the frames.

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
alt + shift - h : mimi tiling cmd move left
alt + shift - l : mimi tiling cmd move right
alt + shift - j : mimi tiling cmd move down
alt + shift - k : mimi tiling cmd move up
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
  the column it was dropped on. A stacked window dropped on empty strip or
  its own column's outer quarter gets a column of its own. Dropped higher or
  lower in its column, it takes that row.

With `[tiling.dropzone]` enabled as well, the drag shows where the window
would land before you let go, by asking your layout the same question as
you drag. See [CONFIGURATION.md](CONFIGURATION.md#drop-zone).

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
since no config is read. A layout of your own that calls `serve()` gets
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
   space. Sheets, popovers, and minimized windows are not there. The pass
   skips a display showing a full-screen space even though its window is
   listed. If it is listed but not tiled, check `rules.py`.
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
