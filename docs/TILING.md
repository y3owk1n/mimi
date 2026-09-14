# Tiling

mimi does not tile. It runs a program you own whenever the desktop changes,
hands it every window and display as JSON, and applies the frames the program
prints back. Your file decides the layout, which windows to leave alone, what
a hotkey means, and what a drag does, and you can write it in any language.
mimi decides when the program runs, keeps its state between runs, and writes
the frames to the windows.

- [Five minutes to a tiled desktop](#five-minutes-to-a-tiled-desktop)
- [How it works](#how-it-works)
- [The contract](#the-contract)
- [Writing your own layout](#writing-your-own-layout)
- [Commands and hotkeys](#commands-and-hotkeys)
- [Drags and the temporary maximise](#drags-and-the-temporary-maximise)
- [Stacking windows in one place](#stacking-windows-in-one-place)
- [More than one display](#more-than-one-display)
- [Trying a layout without turning it on](#trying-a-layout-without-turning-it-on)
- [When nothing happens](#when-nothing-happens)

---

## Five minutes to a tiled desktop

1. Copy the example layouts somewhere you own. They import `rules.py` from
   their own directory, so copy the whole directory.

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

3. Check what it would do first. This runs the layout once against your
   desktop and prints its output without applying any of it.

   ```bash
   mimi tiling preview | jq
   ```

4. Start the daemon, or reload it. The whole `[tiling]` section is
   reloadable, so saving the file is enough when the daemon is running.

   ```bash
   mimi start            # or: mimi config reload
   ```

mimi lays your windows out at once, and again whenever one opens, closes,
or gains focus. To stop, set `enabled = false` and save. Every window stays
where it is.

The shipped layouts are all Python with the standard library only:

| Layout | Shape | Commands it answers |
| --- | --- | --- |
| `monocle.py` | Every window fills the display. Move between them with focus. No state, no commands. Names them all as one stack, so `[tiling.stackbar]` shows how many there are. The one to copy when starting your own. | none |
| `columns.py` | Equal-width columns. | `togglemax` |
| `master-stack.py [ratio]` | One master on the left, the rest stacked on the right. Remembers the master and the ratio. | `swap`, `ratio <delta>`, `togglemax` |
| `bsp.py` | Dwindle BSP, as Hyprland tiles by default. A new window splits the focused one, and closing a window hands its area back. | `swap <dir>`, `focus <dir>`, `togglesplit`, `ratio <delta>`, `togglefloat`, `togglemax`, `stack <dir>`, `unstack`, `next`, `prev` |
| `stacked.py` | Equal columns, where a column holds one window or several in one place with only the focused one seen, as yabai stacks and niri tabs. | `stack`, `unstack`, `next`, `prev`, `focus <left\|right>`, `togglemax` |
| `strip.py` | Scrollable strip, as niri tiles. Columns sit on a strip wider than the display, focus scrolls it, and neighbours peek in at the edges. | `focus <dir>`, `move <dir>`, `consume`, `expel`, `width [fraction\|prev\|+d\|-d]`, `center`, `scroll <dir> [fraction]`, `togglefloat`, `togglemax`, `togglestack` |

None of them takes a gap argument. mimi supplies the gap. It is `tiling.gap`
when the config sets it, otherwise the macOS tiled-window margin that
`mimi action resize_window` honours, so tiled and hand-placed windows line
up.

`[[tiling.rules]]` in config.toml says which windows never tile, by
application, title or size. The default config shows the shape. Its
commented entries float System Settings, Finder, Activity Monitor,
1Password and any window under 400 by 300 points.

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

- **Events that run a pass.** A window created, closed, focused, minimized,
  or restored from the Dock. An application activated, hidden, unhidden, or
  quit. A space change, which entering or leaving full screen also fires.
  The daemon's Accessibility observer reaching an application it could not
  see at launch. The daemon starting with tiling on, and a reload that
  switches it on or names another layout. With `relayout_on_drag = true`, a
  window you moved or resized, and a display plugged in, unplugged, or
  rearranged, which runs a `relayout` pass. A burst of events settles into
  one run.
- **New windows.** A run for a new window waits up to half a second for the
  window server to list it, because Accessibility reports the window a little
  before it is on screen.
- **One run per display.** The program runs once for each display that has a
  window on it, with that display's windows and that display's own state, so
  a layout only handles one display at a time. A display showing a
  full-screen space gets no run, and the other displays lay out as usual.
  mimi applies the frames from every run together.
- **State.** Whatever you print as `state` comes back on the next run for
  the same display and space, so a layout can remember a tree or a ratio
  without writing a file. mimi files each state under the space's window
  server identifier rather than its place in Mission Control. Adding or
  removing a space therefore never hands a layout the state it built for
  another space.
- **Its own writes.** A frame write by mimi does not run a pass. With
  `relayout_on_drag`, mimi tells your drag from its own write by reading back
  where every window actually landed.
- **A stale pass is dropped.** If the space changed while your program ran,
  mimi discards its frames, because the switch itself raises the event that
  lays out the new space.
- **Failures.** mimi logs a program that exits non-zero, runs past
  `timeout_secs` (default 5), or prints something that is not the output
  shape, and applies nothing. The next event tries again.

The `[tiling]` keys are `enabled`, `layout`, `layout_mode`, `debounce_ms`,
`timeout_secs`, `command_timeout_secs`, `relayout_on_drag`, and `gap`. The
section also has a `[tiling.animation]` table with `enabled`, `duration_ms`,
and `easing`, a `[tiling.dropzone]` table that shows where a drag would land,
and a `[tiling.stackbar]` table that draws the windows a layout stacked.
Every key is reloadable. The reference is in
[CONFIGURATION.md](CONFIGURATION.md#tiling).

**Animation is off by default.** With `[tiling.animation]` enabled, windows
move to their frames over `duration_ms` instead of jumping. The daemon moves
the real windows through their applications a step at a time, which needs
nothing beyond the Accessibility permission tiling already has.
[CONFIGURATION.md](CONFIGURATION.md#animation) has the details.

---

## The contract

Your program reads one JSON document on stdin and prints one on stdout.
With `layout_mode = "resident"` in the config, the program keeps running. It
reads one document per line on stdin for as long as stdin stays open, and
prints one line of output per document, flushed after each. mimi stops a
resident program that exits, times out, or prints something that is not an
output, and starts it again on the next pass, waiting longer after each
failure in a row. The shipped layouts handle both modes through `serve()` in
`rules.py`.

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
     "title": "Start Page", "order": 0, "space": 2, "display": 1,
     "frame": {"x": 0, "y": 25, "width": 1440, "height": 875}}
  ],
  "state": null
}
```

| Field | Meaning |
| --- | --- |
| `version` | Moves only when a field is renamed, removed, or changes meaning. Adding a field does not move it. |
| `event.kind` | Why you were run. A hook name (`window_created`, `window_closed`, `window_focus`, `window_minimize`, `window_unminimize`, `app_activate`, `app_hide`, `app_unhide`, `app_quit`, `workspace_changed`), `_ax_attached`, `startup`, `reload`, `preview`, `relayout`, `command`, `window_move`, or `window_resize`. |
| `event.app`, `event.bundleId`, `event.pid` | The application behind a hook event, when there is one. Absent otherwise. |
| `event.name`, `event.args` | For a `command`: what the user typed after `mimi tiling cmd`. `args` is absent when the user typed none. |
| `event.windows` | For a `window_move` or `window_resize`: the numbers of the windows the user dragged. |
| `display` | The display this run is for. Fill its `visible`, never its `frame`. `visible` is what is left after the menu bar and the Dock. |
| `space` | The 1-based Mission Control space in front on that display. |
| `gap` | Points to leave between windows and at the display's edges, resolved by mimi: `tiling.gap` when set, else the macOS tiled-window margin, else 0. |
| `displays` | Every display, numbered as `move_window_to_display` counts them. |
| `focused` | Index into `windows` of the focused window, or -1 when no window on this display has focus. |
| `windows` | The focusable windows whose centres are on this display, or nearest to it when a centre is off every display, in `focus_window` order. `number` is the window server's number, stable for the window's lifetime, and how you name a window in the output. `order` is where the window sits in the stacking order, 0 for the one in front. `space` is the space the window is on, 0 for one assigned to every space, and `display` is the display holding its centre, which here is always this display's `index`. `minSize` is present on a window mimi has asked for a smaller size and watched refuse: the `width` and `height` it kept instead, 0 on an axis it took as asked. Give the window at least that and share the rest out. See [When nothing happens](#when-nothing-happens), item 7. |
| `state` | What you printed last time for this display and space, or `null`. |
| `unmanaged` | The windows on this display you last said you were not managing, by number. Absent when there are none. mimi hands the set back so that a layout that keeps its floats outside `state`, or one restarted mid-session, can pick it up again. |
| `stacks` | The stacks you last named on this display, handed back for the same reason. Absent when there are none. |

`displays` and `windows` hold the same entries that `mimi query displays` and
`mimi query windows` print, so you can get real data with one command.

The `windows` list is ordered by position, which is the order `focus_window`
cycles through them. `order` answers a different question, which window is on
top, and the two rarely agree. Read `order` when the layout cares which of two
overlapping windows the user can see, and ignore it otherwise. mimi ranks the
whole desktop's windows and then narrows them to this display's, so the
numbers compare correctly but need not start at 0 or run without gaps.

### Output

```json
{
  "frames": [{"number": 4242, "frame": {"x": 8, "y": 33, "width": 1424, "height": 859}}],
  "state": {"anything": "you like"},
  "focus": 4242,
  "unmanaged": [4243],
  "stacks": [{"windows": [4242, 4244], "active": 4242}],
  "before": ["osascript -e 'beep'"],
  "after": ["~/.config/mimi/warp.py 720 462"]
}
```

| Field | Meaning |
| --- | --- |
| `frames` | The windows to place. Leave a window out to leave it where it is. Print nothing, or an empty list, to change nothing. With `[tiling.animation]` on, a frame may carry `"animate": false` to move that one window at once while the rest animate, for a window with no sensible starting position. mimi also moves a window the user just dragged at once. |
| `state` | Any JSON. Handed back next run. Omit the key and the previous state is kept. Print `null` to clear it. |
| `focus` | Optional. A window number to give keyboard focus, before the frames move. Use it to move focus along a layout's own structure where spatial `focus_window` cannot, such as a strip's parked columns. |
| `unmanaged` | Optional. The windows this run was given that you are leaving alone, by number, such as the ones you float. mimi then leaves them alone too, so dragging one raises no pass and shows no drop zone. A window stays unmanaged until a later run for the same display leaves it out of this list. |
| `stacks` | Optional. The sets of windows you put in one place, as `[{"windows": [n, ...], "active": n}]`. With `[tiling.stackbar]` enabled, mimi draws the ones behind `active` as a deck of cards and takes the room for them out of the window in front. Every member needs its own frame in `frames`. mimi drops a stack that names a window without a frame, or names fewer than two windows, and the frames still apply. |
| `before` | Optional. Command lines mimi runs through `settings.hook_shell` before the focus and the frames, all at once. mimi waits for every one and kills one past `tiling.command_timeout_secs`. A failure logs at debug and the frames still apply. See [Running commands around the frames](#running-commands-around-the-frames). |
| `after` | Optional. Command lines mimi runs through `settings.hook_shell` once the frames have been applied, and once the animation has ended when one runs. They run in order, detached. mimi kills one past `tiling.command_timeout_secs`, drops their output, and logs a failure at debug. Use it to act on the frames the layout returned, since a hook runs before they are applied. See [Running commands around the frames](#running-commands-around-the-frames). |

**Leaving a window out of `frames` is not the same as naming it in
`unmanaged`.** Omitting a frame says only that the window does not move this
pass. A temporary maximise does exactly that to the windows under the
maximised one, and mimi keeps watching those windows so a drag of one still
reaches you. Naming a window in `unmanaged` says you have no opinion about
where it goes at all, and mimi stops watching it until you claim it again.

A window you never want to see at all goes in `[[tiling.rules]]` in
config.toml with `manage = false`. It is then left out of `windows` for
every layout, with no edit to the layout. [CONFIGURATION.md](CONFIGURATION.md#rules)
has the shape.

`rules.py` handles the rest for you. `unmanaged_of(inp, state)` joins the
windows mimi handed back as `unmanaged` with any the layout floated itself
with `togglefloat`, and `write_output(..., unmanaged=...)` prints the
result.

### Coordinates

Everything is in window coordinates. The origin is the top-left corner of the
primary display and y grows downward. A display above the primary has a
negative y. This is the same system `mimi action resize_window` takes for
`--x` and `--y`.

---

## Writing your own layout

A layout is any executable that reads stdin and writes stdout. This example
implements the whole contract without the shared helpers:

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

The shipped layouts import these from `rules.py`:

- **`serve(main)`** runs `main(inp)` on every input mimi sends, in either
  layout mode. Run from a terminal, with no input piped in, it lays the
  desktop out once by itself instead (see
  [Trying a layout](#trying-a-layout-without-turning-it-on)).
- **`gap(inp)`** returns the gap as mimi resolved it, and **`area(inp, gap)`**
  returns the display's visible frame inset by it.
- **`command(inp, "name")`** returns the args when the event is that command,
  else `None`.
- **`maximised(inp, state, frames, area)`** applies the temporary maximise.
  Call it last, on the frames the layout computed.
- **`write_output(frames, state, focus=None, unmanaged=None, stacks=None)`**
  takes frames as `(number, frame)` pairs, rounds them to whole points, and
  prints the output.

**Remember something** by printing it in `state` and reading it back from
`inp["state"]`. `master-stack.py` keeps its master and ratio there.
`bsp.py` keeps its whole tree there.

**Use another language.** `layout` is a command line run through
`settings.hook_shell`, so arguments are fine and nothing requires Python.
Any program that reads JSON on stdin and writes JSON on stdout will do, in
Go, Rust, Swift, Ruby, Lua, or shell. Two things matter more than the
language. The first is startup time, since the program runs once per display
on every window event, and a compiled binary starts in a few milliseconds
where Node takes tens. `layout_mode = "resident"` removes startup from every
pass but the first, for a program that reads a line at a time and flushes its
answers. The second is a JSON library, since `state` is how a layout
remembers anything. The examples are Python because it ships with the Xcode
Command Line Tools and needs no extra library. A whole layout in shell and
jq, one column per window:

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
pass instead. `before` lines run first, and the pass waits for all of them
to finish before it writes the frames. `after` lines run once the frames
have been applied, and the pass does not wait for them. The layout reads
`event.kind`, so it can decide on every run whether to print either key.
Nothing runs on a pass where a key is absent or empty, or on a pass the
engine skips because the space changed under it.

This example warps the cursor to the window that gained focus, on the two
events that mean focus moved, with the centre taken from the frame the layout
is about to return:

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
command never holds up a pass. mimi drops its output and logs a non-zero exit
at debug. With `[tiling.animation]` on, the lines start once the animation
has ended, so a command that reads a window's frame reads the final one.
The lines run in order, one after another, and mimi kills one past
`tiling.command_timeout_secs`.

Use a `before` line for something that must have happened by the time the
frames are written, such as telling an application to leave a window alone
or hiding a border for the move. mimi starts every `before` line at once and
waits for all of them, so the pass waits as long as the slowest line. Do not
make one line depend on another. mimi kills a line past
`tiling.command_timeout_secs` (default 1). This bound is tighter than the
layout's because the frames wait on it. A `before` line that fails does
not stop the frames.

---

## Commands and hotkeys

```bash
mimi tiling cmd <name> [args...]
```

This sends `{"kind": "command", "name": "<name>", "args": [...]}` as the
event. mimi gives the name no meaning. Your file does, which is how a layout
defines its own hotkeys. Everything after the name goes to your layout
verbatim, so `mimi tiling cmd ratio -0.05` works without quoting. A name your
layout does not handle runs a pass that changes nothing.

```python
args = command(inp, "ratio")       # ["+0.05"] when the event is that command, else None
if args is not None:
    state["ratio"] = clamp(state.get("ratio", 0.6) + float(args[0]), 0.2, 0.8)
```

Two related subcommands are not commands. `mimi tiling relayout` sends a
`relayout` event, which puts every window back where the layout wants it.
`mimi tiling preview` sends `preview` and applies nothing.

With the daemon running, a command reaches the daemon's engine and the state
it holds, and the daemon refuses it with a message when tiling is disabled.
Without a daemon, the CLI runs the layout itself with a null state, whether or
not tiling is enabled, which is enough to try one. It still needs
`tiling.layout` set.

Bind them in whatever you use for hotkeys. With skhd, this is the set
`strip.py` answers. Its `focus` walks the strip itself, because the parked
columns all sit at the edge where spatial focus cannot tell them apart:

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
alt - t         : mimi tiling cmd togglestack
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
alt - s      : mimi tiling cmd stack right
alt + shift - s : mimi tiling cmd unstack
alt - n      : mimi tiling cmd next
```

---

## Drags and the temporary maximise

By default a window you drag stays where you dropped it until the next pass
moves it back. To have a drag run your layout, turn on:

```toml
[tiling]
relayout_on_drag = true
```

Now a drag runs your layout, with the event saying what happened and
`event.windows` naming the windows that moved. mimi waits until you release
the mouse button, however long you pause mid-drag.

- `window_resize` when the size changed at least as much as the position,
  which includes any dragged edge. `bsp.py` resizes the split that edge
  belongs to. `master-stack.py` reads the new ratio off either side.
  `strip.py` sets the column's width.
- `window_move` when the position changed more than the size. `bsp.py` swaps
  the window with the one it was dropped on. `master-stack.py` makes a stack
  window dropped on the master's side the master. `strip.py` moves the window
  into the column it was dropped on. A stacked window dropped on empty strip
  or in the outer quarter of its own column gets a column of its own. Dropped
  higher or lower in its column, it takes that row.

With `[tiling.dropzone]` enabled as well, mimi shows where the window would
land before you let go, by running your layout with the same event while you
drag. These runs apply nothing, keep no state, and run no `before` or `after`
lines. See [CONFIGURATION.md](CONFIGURATION.md#drop-zone).

The engine tells a move from a resize by comparing where the windows landed
against where it last placed them, so a terminal that snaps its width to the
character grid on every move still reads as a move. For a second after each
of its own writes, the engine only reads frames back, so an application
settling into a frame does not count as a drag.

**The temporary maximise** is `mimi tiling cmd togglemax` in every shipped
layout except monocle. The focused window fills the area over the layout, and
the layout's frames and state underneath stay untouched. It ends on a second
`togglemax`, when the window closes, or when you focus another tiled window.
To add it to your own layout, call `maximised(inp, state, frames, area)` from
`rules.py` last, on the frames the layout computed.

---

## Stacking windows in one place

Give two windows the same frame and only one of them is seen. Every layout
can already do this. The `stacks` key tells mimi that the shared frame is a
stack on purpose, so it draws the windows behind as a deck of cards. Without
that, a column of four windows looks exactly like a column of one.

The windows before the one in front in the stack show above it, and the ones
after it show below. Each card is a little narrower than the one in front of
it. The position of the front window between the cards matches its position
in the stack, so moving focus through a stack moves that window from one end
of the frame to the other.

```toml
[tiling.stackbar]
enabled = true
```

**The window seen is the one with keyboard focus.** mimi changes no z-order of
its own, because macOS offers no way to raise one application's window above
another's without also focusing it. Every private call that claims to,
`SLSOrderWindow` included, refuses a window belonging to another application
unless mimi runs with the scripting addition, which needs SIP disabled. A
layout therefore moves between the windows in a stack with the `focus` key,
as it moves focus anywhere else.

`active` is the member you mean to be seen, which is the card the deck opens
at. It records what the layout intends rather than what the desktop shows,
and the two can differ. Cmd-Tab puts a buried member in front without telling
your layout. The next input says which window really is in front, through
`focused` and each window's `order`, so a layout that cares can reconcile.
`rules.py` does that in `shown()`, which every shipped layout that names
stacks uses.

mimi draws the deck inside the frame you set aside, never around it. It takes
the room it needs out of the window in front, so a stack uses no more space
than one window and never covers a neighbour. The cards take at most a
quarter of the frame's height and width, so a small area shows fewer cards
than a deep stack holds.

Four shipped layouts name stacks. `stacked.py` is the clearest example. It
lays out equal columns where a column holds one window or several, and answers
`stack`, `unstack`, `next`, `prev`, and `focus <left|right>`. `strip.py`
answers `togglestack`, which puts the focused column's rows in one place, the
way niri tabs a column. `monocle.py` names every window as one stack, since
all its windows already share one frame, so the deck shows how many are on
the display. `bsp.py` lets a leaf of its tree hold several windows, as yabai
stacks, and answers `stack <dir>`, `unstack`, `next`, and `prev`.

The bindings for `stacked.py`:

```
alt - s         : mimi tiling cmd stack
alt + shift - s : mimi tiling cmd unstack
alt - n         : mimi tiling cmd next
alt - p         : mimi tiling cmd prev
```

---

## More than one display

mimi tiles each display on its own. It runs your program once per display
that has a window, with `display` set to it, `windows` narrowed to the
windows whose centres are on it, and `space` the space in front on it. State
is kept per display and space, so a tree, a master, or a maximised window on
one monitor never mixes with the other's. Switching the space on one monitor
leaves the other's state where it was. A window that crosses to the other
monitor, by drag or by `mimi action move_window_to_display`, leaves one run
and joins the other on the next pass.

Each display or space can run a different program. `[[tiling.layouts]]` in
config.toml names one for a display, a space, or a space on a display, and
`layout` covers the rest. [CONFIGURATION.md](CONFIGURATION.md#a-layout-per-display-or-space)
has the shape.

`mimi query displays` lists the displays in the order
`move_window_to_display` counts them, in the shared coordinate system.
`mimi tiling preview` prints one entry per display that has a window, with
the display id, the space, and the layout's `frames`, `state`, and
`unmanaged`.

---

## Trying a layout without turning it on

```bash
mimi tiling preview --input | jq     # what your program will be given, layout not run
mimi tiling preview | jq             # what it returns, applied to nothing
~/.config/mimi/tiling/columns.py     # apply once, no daemon, tiling off
```

`preview` works with `enabled = false`, so you can iterate on a layout while
the daemon leaves your windows alone, then turn it on. If your program prints
something malformed, `preview` shows the error the daemon would log.

A shipped layout run from a terminal, with nothing piped to stdin, lays the
desktop out once by itself. `serve()` checks whether stdin is a terminal. If
it is, the layout builds the inputs the daemon would from `mimi query`, runs
itself once per display, and applies the frames with
`mimi action apply_frames`. Tiling stays off and no daemon is needed. The
event is `relayout`, the state is null, and the gap is the macOS
tiled-window margin, since no config is read. A layout of your own that calls
`serve()` gets the same. Started with a stdin that is not a terminal, such
as `/dev/null`, the layout reads no input and does nothing.

---

## When nothing happens

Work down this list.

1. **Is Accessibility granted?** Every window read and write needs it.
   Without it the daemon logs `accessibility permission not granted` with
   `tiling disabled` at startup and treats tiling as disabled, so a
   `mimi tiling cmd` sent to the daemon reports that tiling is disabled.
2. **Is it enabled, and is the layout there?** `mimi config validate` rejects
   `enabled = true` with no `layout`. The layout is a command line run through
   `settings.hook_shell`, so the shell expands `~` and `$HOME`, and a path
   with spaces needs quoting.
3. **Does the layout run by hand?** `mimi tiling preview` runs it the way the
   daemon would and shows its stderr when it fails. A missing `python3`, a
   syntax error, or a wrong shebang all show up here.
4. **Ask the daemon what it is holding.** `mimi tiling state` prints the
   state the engine kept for each display and space, and the windows your
   layout said it is not managing. When a tree no longer matches the windows,
   `mimi tiling reset` forgets the state of the space in front on each
   display, and the next pass starts your layout over. Other spaces keep
   their state, which a daemon restart would lose. `mimi tiling reset --all`
   forgets every space.
5. **Read the daemon log.** With `log_level = "debug"` every pass logs
   `tiling pass applied` with the event kind, display count, and frame count.
   A failing pass logs `tiling pass failed` with the reason. A slow layout
   fails with `layout timed out`. Raise `timeout_secs`.
6. **Is the window one mimi tiles?** `mimi query windows` lists the windows a
   layout can be given: windows of regular, unhidden applications on the
   current space. Sheets, popovers, and minimized windows are not there. The
   pass skips a display showing a full-screen space even though its window is
   listed. If a window is listed but not tiled, check `rules.py`.
7. **A window that will not take its frame.** Some applications enforce a
   minimum size or snap to a grid, and land a little off the requested frame.
   The next input shows where windows actually are, which is why the shipped
   layouts read ratios off actual frames rather than assuming. A window that
   lands larger than asked gets a `minSize` on its entry in every later
   input, and mimi runs one more `relayout` pass as soon as it learns one,
   so a layout that reads it can make room at once. `bsp.py` and `strip.py`
   do, through `min_sizes` and `fit` in `rules.py`. A layout that ignores it
   leaves the window over its neighbour, as before, and no extra pass runs.
   When the minimums on a display add up to more than it has, no layout can
   satisfy them. `strip.py` lets the strip grow and scrolls. `bsp.py`
   overlaps the two sides of the split toward its middle, so every window
   stays inside the display, where focus can raise it. Float one of them if
   that is not what you want.
   `mimi tiling state` prints what mimi has learned as `minSizes`, and
   `mimi tiling reset` forgets it. A preview runs its own engine and has
   learned nothing, so its input never carries one.

   mimi also keeps the largest minimum any of an application's windows has
   shown, by bundle identifier, in `minsizes.json` beside the socket file,
   and hands it to every later window of that application before it has
   refused anything, across restarts. The pass that overlaps and the pass
   that corrects it happen once per application, not once per window or
   daemon. `mimi tiling state` prints this as `appMinSizes`. `mimi tiling
   reset` empties it, which is the fix when an update has lowered an
   application's minimum.
8. **An app that opens windows but never tiles.** Applications slow to start
   refuse the daemon's observer for a moment after launch. The daemon retries
   for several seconds and runs your layout as soon as the observer attaches.
   If it gives up, it logs `AX observer install gave up` naming the app. Its
   windows are then tiled only when another event runs a pass, or by
   `mimi tiling relayout`.
9. **It tiles, then un-tiles, then tiles.** A layout that reads a drag and
   emits a different frame every time will loop with an application that
   refuses that frame. Read the actual frame back from the input and treat a
   difference of a few points as "already there".
