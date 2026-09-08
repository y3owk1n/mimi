#!/usr/bin/env python3
"""Equal-width columns across the display holding the focused window.

The simplest layout there is, and the one to copy when starting your own:
no state, no commands, every window gets the same share.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: columns.py [gap]
"""

import sys

from rules import read_input, visible_area, write_output

GAP = float(sys.argv[1]) if len(sys.argv) > 1 else 8.0


def main():
    inp = read_input()
    windows = inp["windows"]
    if not windows:
        write_output([], None)
        return

    area = visible_area(inp, GAP)
    n = len(windows)
    col = (area["width"] - GAP * (n - 1)) // n
    frames = [
        (
            w["number"],
            {
                "x": area["x"] + i * (col + GAP),
                "y": area["y"],
                "width": col,
                "height": area["height"],
            },
        )
        for i, w in enumerate(windows)
    ]
    write_output(frames, None)


if __name__ == "__main__":
    main()
