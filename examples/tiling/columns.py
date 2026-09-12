#!/usr/bin/env python3
"""Equal-width columns across the display holding the focused window.

The simplest layout there is, and the one to copy when starting your own:
every window gets the same share. Its only state and only command are the
shared temporary maximise (mimi tiling cmd togglemax).

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: columns.py     (the gap is tiling.gap, else the macOS tiled-window margin)
"""

from rules import area, gap, maximised, serve, unmanaged_of, write_output


def main(inp):
    GAP = gap(inp)
    state = inp.get("state") or {}
    windows = inp["windows"]
    if not windows:
        write_output([], None, unmanaged=unmanaged_of(inp))
        return

    box = area(inp, GAP)
    n = len(windows)
    col = (box["width"] - GAP * (n - 1)) // n
    frames = [
        (
            w["number"],
            {
                "x": box["x"] + i * (col + GAP),
                "y": box["y"],
                "width": col,
                "height": box["height"],
            },
        )
        for i, w in enumerate(windows)
    ]
    write_output(
        maximised(inp, state, frames, box), state, unmanaged=unmanaged_of(inp, state)
    )


if __name__ == "__main__":
    serve(main)
