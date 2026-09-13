# Tiling with mimi

mimi does not tile. It gives your program the windows and applies the frames
the program returns. Your program decides which windows count, where they go,
and what a hotkey means, in whatever language you like.

Copy a program from this directory, change it to suit you, and name it in
`config.toml`. mimi loads nothing from here, and only the JSON contract is
promised to keep working across mimi versions.

The [Tiling Guide](https://github.com/y3owk1n/mimi/blob/main/docs/TILING.md)
covers the first run, the contract, hotkeys, drags, and what to do when
nothing happens.

## Programs

All are Python with the standard library only, so `python3` from the Xcode
Command Line Tools is enough. Each reads stdin, prints stdout, and imports
`rules.py` from its own directory, so copy the whole directory. They run in
both of mimi's layout modes. The default runs one process per pass. With
`layout_mode = "resident"`, one process keeps running and answers one line
per pass.

| File | What it does |
| --- | --- |
| `monocle.py` | Every window fills the display, and you move between them with focus. No state, no commands. Names them all as one stack, so `[tiling.stackbar]` shows how many there are. The smallest layout here, and the one to copy when starting your own. |
| `columns.py` | Equal-width columns. Its only command is `togglemax`, and its only state is the maximised window. |
| `master-stack.py [ratio]` | One master on the left, the rest stacked on the right. Remembers the master and ratio in `state` and answers `swap`, `ratio +0.05`, and `togglemax`. With `relayout_on_drag`, it reads a drag of the split from either side, and makes a stack window dropped on the master's side the master. |
| `strip.py` | Scrollable strip, as niri tiles. Columns sit on a strip wider than the display, focus scrolls it, and neighbours peek in at the edges. Answers `focus <dir>`, `move <dir>`, `consume`, `expel`, `width [fraction\|prev\|+d\|-d]`, `center`, `scroll <dir> [fraction]`, `togglefloat`, `togglemax`, and `togglestack`. A dragged edge sets a column's width, and a window dropped on a column joins it. A window sharing a column gets a column of its own when dropped on empty strip or in the outer quarter of its column, and takes a row when dropped higher or lower in it. `PRIORITY` fixes where listed apps open. |
| `bsp.py` | Dwindle BSP, as Hyprland tiles by default. A new window splits the focused one, closing a window hands its area back, a dragged edge resizes its split, and a window dropped on another swaps with it. A leaf can hold several windows sharing its whole area, as yabai stacks. Answers `swap <dir>`, `focus <dir>`, `togglesplit`, `ratio <delta>`, `togglefloat`, `togglemax`, `stack <dir>`, `unstack`, `next`, and `prev`. |
| `stacked.py` | Equal columns where a column holds one window or several in one place, as yabai stacks and niri tabs. Only the focused window is seen, and mimi draws the rest as cards behind it. Answers `stack`, `unstack`, `next`, `prev`, `focus <left\|right>`, and `togglemax`. Needs `[tiling.stackbar]` enabled to draw the cards. |
| `rules.py` | The code the others share: the float rules (edit the bundle ids and the minimum size here), the area to fill, `serve()`, which reads the input and runs the layout in either mode, `write_output`, and the temporary maximise that every layout except `monocle.py` answers as `togglemax`. |

Run any of them from a terminal, with nothing piped to stdin, and it lays the
desktop out once by itself. It builds the inputs the daemon would, runs once
per display, and applies the frames, with tiling off and no daemon running.
`serve()` in `rules.py` does this, so a layout of your own that uses it gets
the same.

## Wiring it up

Copy the directory somewhere, such as `~/.config/mimi/tiling/`, then:

```toml
[tiling]
enabled = true
layout = "~/.config/mimi/tiling/columns.py"
```

`mimi tiling preview` shows what your program returns before you switch it
on, and `mimi tiling preview --input` shows what it is given. The guide covers
commands, hotkeys, and the rest.
