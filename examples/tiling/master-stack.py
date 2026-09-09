#!/usr/bin/env python3
"""Master and stack: one window fills the left share of the display, every
other window stacks top to bottom on the right.

The master and the ratio live in the state, so focusing another window does
not reshuffle the layout, and the layout answers two commands of its own:

  mimi tiling cmd swap           make the focused window the master
  mimi tiling cmd ratio +0.05    widen the master (or -0.05 to narrow it)
  mimi tiling cmd togglemax      fill the area with the focused window, for now

mimi gives those names no meaning; this file does. Add your own. With
tiling.relayout_on_drag set, dragging the edge between the master and the
stack sets the ratio too, from whichever side was dragged, and dropping a
stack window onto the master makes it the master. Dragging a stack window
taller has nowhere to go in this layout and snaps back; see bsp.py for a
layout where every edge is a split.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: master-stack.py [ratio]     (the gap is tiling.gap, else the macOS tiled-window margin)
"""

import sys

from rules import area, clamp, command, gap, maximised, serve, write_output

RATIO = float(sys.argv[1]) if len(sys.argv) > 1 else 0.6


def main(inp):
    GAP = gap(inp)
    state = inp.get("state") or {}
    windows = inp["windows"]
    if not windows:
        write_output([], None)
        return

    box = area(inp, GAP)
    numbers = [w["number"] for w in windows]
    by_number = {w["number"]: w for w in windows}
    focused = numbers[inp["focused"]] if inp["focused"] >= 0 else None
    n = len(windows)

    # The master: "swap" makes the focused window the master; otherwise the
    # remembered one while it is still here, else the focused, else the first.
    master = state.get("master")
    if command(inp, "swap") is not None and focused is not None:
        master = focused
    elif master not in numbers:
        master = focused if focused is not None else numbers[0]

    # A stack window dropped on the master's side becomes the master.
    if inp["event"]["kind"] == "window_move" and n > 1:
        boundary = box["x"] + (box["width"] - GAP) * state.get("ratio", RATIO)
        for number in inp["event"].get("windows", []):
            if number != master and number in by_number:
                f = by_number[number]["frame"]
                if f["x"] + f["width"] / 2 < boundary:
                    master = number

    # The ratio: remembered, read off a dragged edge from either side of the
    # split, nudged by "ratio +0.05", clamped.
    ratio = state.get("ratio", RATIO)
    inner = box["width"] - GAP  # the width shared between master and stack
    if inp["event"]["kind"] == "window_resize" and n > 1:
        dragged = inp["event"].get("windows", [])
        stack_dragged = [d for d in dragged if d != master and d in by_number]
        if stack_dragged:
            ratio = 1 - by_number[stack_dragged[0]]["frame"]["width"] / inner
        elif master in dragged:
            ratio = by_number[master]["frame"]["width"] / inner
    args = command(inp, "ratio")
    if args:
        ratio += float(args[0])
    ratio = clamp(ratio, 0.2, 0.8)

    master_w = box["width"] if n == 1 else int(inner * ratio)
    stack_w = inner - master_w
    k = n - 1
    stack_h = (box["height"] - GAP * (k - 1)) // k if k > 0 else 0

    frames = [
        (master, {"x": box["x"], "y": box["y"], "width": master_w, "height": box["height"]})
    ]
    stack = [w for w in windows if w["number"] != master]
    for i, w in enumerate(stack):
        frames.append(
            (
                w["number"],
                {
                    "x": box["x"] + master_w + GAP,
                    "y": box["y"] + i * (stack_h + GAP),
                    "width": stack_w,
                    "height": stack_h,
                },
            )
        )

    state.update(master=master, ratio=ratio)
    write_output(maximised(inp, state, frames, box), state)


if __name__ == "__main__":
    serve(main)
