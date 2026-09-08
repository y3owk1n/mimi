#!/usr/bin/env python3
"""Monocle: every window fills the display.

Windows sit on top of one another and you move between them with focus
(mimi action focus_window, or an app switcher). The smallest layout there
is, and the one to copy when starting your own: no state, no commands.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: monocle.py [gap]
"""

import sys

from rules import area, read_input, write_output

GAP = float(sys.argv[1]) if len(sys.argv) > 1 else 0.0


def main():
    inp = read_input()
    box = area(inp, GAP)
    write_output([(w["number"], box) for w in inp["windows"]], None)


if __name__ == "__main__":
    main()
