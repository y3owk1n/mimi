# Tiling with mimi

mimi does not tile. It gives your program the windows and applies the frames
the program returns. Everything else, which windows count, where they go,
what a hotkey means, is yours, in whatever language you like.

Copy a program from this directory, make it your own, and name it in
`config.toml`. Nothing here is loaded by mimi and nothing here is promised to
keep working across mimi versions except the JSON contract below.

## The contract

A layout is a program. It reads one JSON document on stdin and prints one on
stdout. The daemon runs it on every window event; `mimi tiling preview` runs
it once by hand; `standalone.sh` runs it without a daemon at all.

Input:

```json
{"version":1,
 "event":{"kind":"window_created","app":"Safari","bundleId":"com.apple.Safari","pid":501},
 "space":2,
 "displays":[{"index":1,"id":1,"frame":{"x":0,"y":0,"width":1440,"height":900},
              "visible":{"x":0,"y":25,"width":1440,"height":875}}],
 "focused":0,
 "windows":[{"number":4242,"pid":501,"app":"Safari","bundleId":"com.apple.Safari",
             "title":"Start Page","frame":{"x":0,"y":25,"width":1440,"height":875}}],
 "state":null}
```

- `event.kind` is a hook event name (`window_created`, `window_closed`,
  `window_focus`, `workspace_changed`, `app_hide`, `app_unhide`, `app_quit`),
  or `preview` from `mimi tiling preview`, or `relayout` from `standalone.sh`.
- `displays` and `windows` are exactly what `mimi query displays` and
  `mimi query windows` print. `focused` is the index into `windows` of the
  focused one, or -1.
- `state` is whatever your program printed as `state` last time for this
  space, or `null`. The daemon keeps it for you; that is how a layout
  remembers a master window or a split ratio without a file.

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

| File | What it does | Needs |
| --- | --- | --- |
| `columns.sh [gap]` | Equal-width columns across the display holding the focused window. | `jq` |
| `master-stack.sh [ratio] [gap]` | One master on the left, the rest stacked on the right. Remembers the master in `state`. | `jq` |
| `float-rules.jq` | jq definitions the two above include: `floating`, `tileable`, `display_for`. Edit the bundle ids and title patterns here. | `jq` |
| `standalone.sh <layout> [args]` | Run any layout once without the daemon. | `jq` |

## Wiring it up

Copy the directory somewhere, say `~/.config/mimi/tiling/`, then:

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/columns.sh 8"
```

Try it before switching it on:

```bash
mimi tiling preview --input      # what your program will be given
mimi tiling preview              # what it returns, applied to nothing
~/.config/mimi/tiling/standalone.sh ~/.config/mimi/tiling/columns.sh   # apply once
```

`enabled` and `layout` are reloadable, so editing the config while the daemon
runs is enough. A program that fails is logged and applies nothing, and the
next event tries again.

## Things macOS will do to you

- An application can refuse a size. Terminals snap to cell multiples and many
  windows have a minimum. The next input shows where things actually landed.
- A window on another space is not in `windows` and cannot be placed.
- Full-screen and native tiled spaces have no windows to tile.
- `windows` is what `focus_window` cycles: standard windows of regular
  applications, not sheets, popovers, or minimized windows. `float-rules.jq`
  is for what you want to skip beyond that.
- Resizes never wake the daemon's engine, so a window the user drags stays
  where it was dragged until the next event.
