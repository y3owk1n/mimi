# Tiling with mimi

mimi does not tile. It gives your script the windows and applies the frames
the script returns. Everything else, which windows count, where they go, what
a hotkey means, is yours, in whatever language you like.

Copy a script from this directory, make it your own, and wire it to a hook or
a hotkey. Nothing here is loaded by mimi and nothing here is promised to keep
working across mimi versions except the three commands it is built on.

## The contract

```bash
mimi query windows      # every focusable window on the active space
mimi query displays     # every display, frames in window coordinates
mimi action apply_frames < frames.json
```

`query windows` prints one line of JSON:

```json
{"focused":0,"windows":[
  {"number":4242,"pid":501,"app":"Safari","bundleId":"com.apple.Safari",
   "title":"Start Page","frame":{"x":0,"y":25,"width":1440,"height":875}}
]}
```

`query displays` prints an array, numbered the way `move_window_to_display`
counts displays. `visible` is the area a window may occupy:

```json
[{"index":1,"id":1,"frame":{"x":0,"y":0,"width":1440,"height":900},
  "visible":{"x":0,"y":25,"width":1440,"height":875}}]
```

`apply_frames` reads an array of `{number, frame}` and applies each in order.
It attempts every frame even after one fails, then reports the failures.

All frames are in window coordinates: the origin is the top left of the
primary display and y grows downward. A display above or taller than the
primary has a negative y. Use `visible` from `query displays`, never `frame`,
as the area to fill.

## Scripts

| File | What it does | Needs |
| --- | --- | --- |
| `columns.sh` | Equal-width columns across the display holding the focused window, with a gap. | `jq` |
| `master-stack.sh` | The focused window takes the left share, the rest stack on the right. | `jq` |
| `float-rules.jq` | A filter that drops windows a layout should leave alone. Pipe `query windows` through it first. | `jq` |

## Wiring it up

Run a script by hand or from a hotkey (skhd, Hammerspoon, Karabiner):

```
alt - t : ~/.config/mimi/tiling/columns.sh
```

Or re-tile whenever a window appears or goes away, from `config.toml`:

```toml
[hooks]
on_window_created = ["~/.config/mimi/tiling/columns.sh"]
on_window_closed  = ["~/.config/mimi/tiling/columns.sh"]
```

A hook runs after every event of that kind, including the ones your own
`apply_frames` causes. `on_window_resize` in particular fires for every frame
the script writes, so do not re-tile from it without a guard.

## Things macOS will do to you

- An application can refuse a size. Terminals snap to cell multiples and many
  windows have a minimum. Read frames back with `query windows` if you need to
  know where things actually landed.
- A window on another space is not on the list and cannot be placed.
- Full-screen and native tiled spaces have no windows to tile.
- `query windows` lists what `focus_window` cycles: standard windows of
  regular applications, not sheets, popovers, or minimized windows. Rules in
  `float-rules.jq` are for what you want to skip beyond that.
