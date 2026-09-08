"""Shared pieces the layout programs import: which windows to leave alone,
and which display to fill. Copy, edit, own.

Every layout here reads one JSON document on stdin and prints one on
stdout; see README.md for the shapes. mimi runs a layout once per display,
with that display's windows and a state of that display's own, so a layout
only ever thinks about one display. This file is the one place to add a
bundle identifier or a title pattern that should never be tiled.
"""

import json
import re
import sys

FLOATING_BUNDLES = {
    "com.apple.systempreferences",
    "com.apple.finder",
    "com.apple.ActivityMonitor",
    "com.1password.1password",
}

FLOATING_TITLES = re.compile(r"^(Preferences|Settings)$")


def floating(win):
    """True for a window a layout should leave where it is."""
    return (
        win["bundleId"] in FLOATING_BUNDLES
        or FLOATING_TITLES.match(win["title"]) is not None
        or (win["frame"]["width"] < 400 and win["frame"]["height"] < 300)
    )


def read_input():
    """The layout input from stdin, with `windows` narrowed to the tileable
    ones and `focused` re-pointed at the same window, or -1 if it went."""
    inp = json.load(sys.stdin)
    focused_number = (
        inp["windows"][inp["focused"]]["number"] if inp["focused"] >= 0 else None
    )
    inp["windows"] = [w for w in inp["windows"] if not floating(w)]
    numbers = [w["number"] for w in inp["windows"]]
    inp["focused"] = numbers.index(focused_number) if focused_number in numbers else -1
    return inp


def area(inp, gap):
    """The visible frame of the display this input is for, inset by gap on
    every side. mimi runs a layout once per display, so this is the one area
    a run ever fills."""
    v = inp["display"]["visible"]
    return {
        "x": v["x"] + gap,
        "y": v["y"] + gap,
        "width": v["width"] - 2 * gap,
        "height": v["height"] - 2 * gap,
    }


def write_output(frames, state):
    """Print the layout output: frames in whole points, and the state to
    get back next time."""
    frames = [
        {"number": number, "frame": {k: int(round(v)) for k, v in frame.items()}}
        for number, frame in frames
    ]
    json.dump({"frames": frames, "state": state}, sys.stdout)


def maximised(inp, state, frames, area):
    """A temporary maximise, as Hyprland's fullscreen toggle: the focused
    window fills the whole area over the layout, whose own frames and state
    are left exactly as they were underneath.

      mimi tiling cmd togglemax

    It ends when the command runs again, when the window goes away, or when
    focus moves to another tiled window, so the layout comes back the moment
    you leave. Call it last, on the frames the layout computed; it returns
    the frames to print and keeps its one fact in state["maximised"]."""
    numbers = [number for number, _ in frames]
    focused = (
        inp["windows"][inp["focused"]]["number"] if inp["focused"] >= 0 else None
    )
    current = state.get("maximised")

    if command(inp, "togglemax") is not None and focused in numbers:
        current = None if current == focused else focused
    elif current not in numbers:
        current = None
    elif inp["event"]["kind"] == "window_focus" and focused not in (None, current):
        current = None

    state["maximised"] = current
    if current is None:
        return frames
    return [(n, area if n == current else f) for n, f in frames]


def command(inp, name):
    """The command's arguments when the event is that command, else None."""
    event = inp["event"]
    if event["kind"] == "command" and event.get("name") == name:
        return event.get("args", [])
    return None


def clamp(value, low, high):
    return max(low, min(high, value))
