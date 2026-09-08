#!/usr/bin/env python3
"""Monocle: every window fills the display.

Windows sit on top of one another and you move between them with focus
(mimi action focus_window, or an app switcher). The smallest layout there
is, and the one to copy when starting your own: no state, no commands.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: monocle.py     (the gap is tiling.gap, else the macOS tiled-window margin)
"""

from rules import area, gap, read_input, write_output


def main():
    inp = read_input()
    box = area(inp, gap(inp))
    write_output([(w["number"], box) for w in inp["windows"]], None)


if __name__ == "__main__":
    main()
