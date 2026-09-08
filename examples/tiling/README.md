# Tiling with mimi

The guide from first run to writing your own layout is `docs/TILING.md` in
the repository. This file is the short version that travels with the scripts.

mimi does not tile. It gives your program the windows and applies the frames
the program returns. Everything else, which windows count, where they go,
what a hotkey means, is yours, in whatever language you like.

Copy a program from this directory, make it your own, and name it in
`config.toml`. Nothing here is loaded by mimi and nothing here is promised to
keep working across mimi versions except the JSON contract below.

## The contract

A layout is a program. It reads one JSON document on stdin and prints one on
stdout. The daemon runs it on every window event; `mimi tiling preview` runs
it once by hand; run with nothing on stdin, a layout here lays the desktop
out once by itself, with tiling off and no daemon at all.

Input:

```json
{"version":1,
 "event":{"kind":"window_created","app":"Safari","bundleId":"com.apple.Safari","pid":501},
 "display":{"index":1,"id":1,"frame":{"x":0,"y":0,"width":1440,"height":900},
            "visible":{"x":0,"y":25,"width":1440,"height":875}},
 "space":2,
 "gap":8,
 "displays":[{"index":1,"id":1,"frame":{"x":0,"y":0,"width":1440,"height":900},
              "visible":{"x":0,"y":25,"width":1440,"height":875}}],
 "focused":0,
 "windows":[{"number":4242,"pid":501,"app":"Safari","bundleId":"com.apple.Safari",
             "title":"Start Page","frame":{"x":0,"y":25,"width":1440,"height":875}}],
 "state":null}
```

- `event.kind` is a hook event name (`window_created`, `window_closed`,
  `window_focus`, `workspace_changed`, `app_hide`, `app_unhide`, `app_quit`,
  and `window_move` or `window_resize` when `tiling.relayout_on_drag` is
  set, only for a drag the user made, which add `"windows"`, the numbers
  of the windows dragged),
  or `startup` when the daemon starts with tiling on, or `reload` when a
  reload switches it on or changes the layout,
  or `preview` from `mimi tiling preview`, or `relayout` from
  `mimi tiling relayout` and a layout run by itself, or `command` from
  `mimi tiling cmd <name> [args...]`, which adds `"name"` and `"args"`.
  mimi gives a command name no meaning. Your program does, which is how it
  defines its own hotkeys.
- mimi runs the program once per display. `display` is the one this run
  fills and `windows` are the windows on it, as `mimi query windows` prints
  them; `displays` lists every display for reference. `focused` is the
  index into `windows` of the focused one, or -1.
- `gap` is the space to leave between windows and at the edges, resolved
  by mimi: `tiling.gap` from the config when set, else the macOS
  tiled-window margin that `mimi action resize_window` honours. Use it as
  given and tiled windows line up with hand-placed ones.
- `state` is whatever your program printed as `state` last time for this
  display and space, or `null`. The daemon keeps it for you; that is how a
  layout remembers a master window or a split ratio without a file.

Output:

```json
{"frames":[{"number":4242,"frame":{"x":8,"y":33,"width":1424,"height":859}}],
 "state":{"master":4242}}
```

`frames` is what `mimi action apply_frames` takes. Every frame is attempted
even after one fails. Print nothing, or an empty `frames`, to change nothing.

All frames are in window coordinates: the origin is the top left of the
primary display and y grows downward. A display above or taller than the
primary has a negative y. Fill `visible` from a display, never `frame`.

## Programs

All in Python with the standard library only, so `python3` from the Xcode
Command Line Tools is enough. Each reads stdin, prints stdout, and imports
`rules.py` from its own directory, so copy the directory as a whole.

| File | What it does |
| --- | --- |
| `monocle.py` | Every window fills the display; move between them with focus. The smallest layout there is, and the one to copy when starting your own. |
| `columns.py` | Equal-width columns. No state, no commands. |
| `master-stack.py [ratio]` | One master on the left, the rest stacked on the right. Remembers the master and ratio in `state`, answers `swap` and `ratio +0.05`, reads a drag of the split from either side, and makes a stack window dropped on the master the master. |
| `bsp.py` | Dwindle BSP, as Hyprland tiles by default: a new window splits the focused one, closing hands the area back, any dragged edge resizes its split, a window dropped on another swaps with it. Answers `swap <dir>`, `togglesplit`, `ratio <delta>`, `togglefloat`. |
| `rules.py` | What the three share: the float rules (edit the bundle ids and title patterns here), the area to fill, reading the input and writing the output, and the temporary maximise every layout answers as `togglemax`. |

Run any of them with nothing on stdin and it lays the desktop out once by
itself: the inputs the daemon would build, one run per display, frames
applied. Tiling off, no daemon. That is `rules.py` at work, so a layout of
your own that uses it gets the same.

The contract needs no particular language. This is a whole layout in
shell and jq, one column per window:

```sh
#!/bin/sh
jq -c '(.display.visible) as $v | (.windows | length) as $n
  | {frames: [.windows | to_entries[]
      | {number: .value.number,
         frame: {x: ($v.x + .key * ($v.width / $n)), y: $v.y,
                 width: ($v.width / $n), height: $v.height}}],
     state: null}'
```

## Wiring it up

Copy the directory somewhere, say `~/.config/mimi/tiling/`, then:

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/columns.py"
```

Try it before switching it on:

```bash
mimi tiling preview --input      # what your program will be given
mimi tiling preview              # what it returns, applied to nothing
~/.config/mimi/tiling/columns.py   # apply once, tiling off, no daemon
```

Bind the commands to hotkeys (skhd, Hammerspoon, Karabiner):

```
alt - return : mimi tiling cmd swap
alt - l      : mimi tiling cmd ratio +0.05
alt - h      : mimi tiling cmd ratio -0.05
alt - r      : mimi tiling relayout
```

And for `bsp.py`, the Hyprland-shaped set:

```
alt - h : mimi tiling cmd swap left
alt - l : mimi tiling cmd swap right
alt - k : mimi tiling cmd swap up
alt - j : mimi tiling cmd swap down
alt - t : mimi tiling cmd togglesplit
alt - f : mimi tiling cmd togglefloat
alt - m : mimi tiling cmd togglemax
```

`togglemax` works in every layout here: the focused window fills the area
until you toggle again, close it, or focus another tiled window, and the
layout underneath is untouched the whole time.

With the daemon running these reach its engine, and the state it holds. With
no daemon they run in the CLI with a null state, which is enough for
`relayout` and for trying a command.

`enabled` and `layout` are reloadable, so editing the config while the daemon
runs is enough. A program that fails is logged and applies nothing, and the
next event tries again.

## Things macOS will do to you

- An application can refuse a size. Terminals snap to cell multiples and many
  windows have a minimum. The next input shows where things actually landed.
- A window on another space is not in `windows` and cannot be placed.
- Full-screen and native tiled spaces have no windows to tile.
- `windows` is what `focus_window` cycles: standard windows of regular
  applications, not sheets, popovers, or minimized windows. `rules.py` is
  for what you want to skip beyond that.
- Moves and resizes wake the daemon's engine only with
  `relayout_on_drag = true`. Without it a window the user drags stays where
  it was dragged until the next event. With it the examples read a dragged
  edge as a new ratio and a window dropped on another as a swap.
