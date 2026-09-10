# Tiling with mimi

mimi does not tile. It gives your program the windows and applies the frames
the program returns. Everything else, which windows count, where they go,
what a hotkey means, is yours, in whatever language you like.

Copy a program from this directory, make it your own, and name it in
`config.toml`. Nothing here is loaded by mimi and nothing here is promised to
keep working across mimi versions except the JSON contract.

The guide, from first run through the contract, hotkeys, drags, and what to
do when nothing happens, is the
[Tiling Guide](https://github.com/y3owk1n/mimi/blob/main/docs/TILING.md).

## Programs

All in Python with the standard library only, so `python3` from the Xcode
Command Line Tools is enough. Each reads stdin, prints stdout, and imports
`rules.py` from its own directory, so copy the directory as a whole. They
run in both of mimi's layout modes: one process per pass, or, with
`layout_mode = "resident"`, one process kept running that answers a line
per pass.

| File | What it does |
| --- | --- |
| `monocle.py` | Every window fills the display; move between them with focus. The smallest layout there is, and the one to copy when starting your own. |
| `columns.py` | Equal-width columns. No state, no commands. |
| `master-stack.py [ratio]` | One master on the left, the rest stacked on the right. Remembers the master and ratio in `state`, answers `swap` and `ratio +0.05`, reads a drag of the split from either side, and makes a stack window dropped on the master the master. |
| `strip.py` | Scrollable strip, as niri tiles: columns on a strip wider than the display, focus scrolls it, neighbours peek in at the edges. Answers `focus <dir>`, `move <dir>`, `consume`, `expel`, `width [fraction|prev|+d|-d]`, `center`, `scroll <dir> [fraction]`, `togglefloat`, `togglemax`; a dragged edge sets a column's width and a window dropped on a column joins it, a stacked window dropped on empty strip or a column's outer quarter gets a column of its own, and one dropped higher or lower in its column takes that row. `PRIORITY` fixes where listed apps open. |
| `bsp.py` | Dwindle BSP, as Hyprland tiles by default: a new window splits the focused one, closing hands the area back, any dragged edge resizes its split, a window dropped on another swaps with it. Answers `swap <dir>`, `togglesplit`, `ratio <delta>`, `togglefloat`. |
| `rules.py` | What the others share: the float rules (edit the bundle ids and title patterns here), the area to fill, `serve()`, which reads the input and runs the layout in either mode, `write_output`, and the temporary maximise every layout answers as `togglemax`. |

Run any of them with nothing on stdin and it lays the desktop out once by
itself: the inputs the daemon would build, one run per display, frames
applied. Tiling off, no daemon. That is `rules.py` at work, so a layout of
your own that uses it gets the same.

## Wiring it up

Copy the directory somewhere, say `~/.config/mimi/tiling/`, then:

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/columns.py"
```

`mimi tiling preview` shows what your program is given and what it returns
before you switch it on. Commands, hotkeys, and everything after are in the
guide.
