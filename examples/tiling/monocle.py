#!/usr/bin/env python3
"""Monocle: every window fills the display.

Windows sit on top of one another and you move between them with focus
(mimi action focus_window, or an app switcher). The smallest layout there
is, and the one to copy when starting your own: no state, no commands.

Every window being in the same place is exactly what a stack is, so this
names one, and with [tiling.stackbar] enabled mimi marks it: one segment per
window with the focused one lit. That is the one thing monocle otherwise
cannot tell you, since a display with six windows on it looks like a display
with one.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: monocle.py     (the gap is tiling.gap, else the macOS tiled-window margin)
"""

from rules import area, gap, serve, unmanaged_of, write_output


def main(inp):
    box = area(inp, gap(inp))
    numbers = [w["number"] for w in inp["windows"]]
    focused = inp["windows"][inp["focused"]]["number"] if inp["focused"] >= 0 else None

    # Two or more windows in one place is a stack. mimi drops a stack of
    # one, so there is nothing to check here.
    stacks = [{"windows": numbers, "active": focused or numbers[0]}] if numbers else []

    write_output(
        [(number, box) for number in numbers],
        None,
        unmanaged=unmanaged_of(inp),
        stacks=stacks,
    )


if __name__ == "__main__":
    serve(main)
