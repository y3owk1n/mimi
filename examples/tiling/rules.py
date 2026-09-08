"""Shared pieces the layout programs import: which windows to leave alone,
and which display to fill. Copy, edit, own.

Every layout here reads one JSON document on stdin and prints one on
stdout; see README.md for the shapes. This file is the one place to add a
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


def visible_area(inp, gap):
    """The visible frame of the display under the focused window (or the
    first display), inset by gap on every side."""
    display = inp["displays"][0]
    if inp["focused"] >= 0:
        f = inp["windows"][inp["focused"]]["frame"]
        cx, cy = f["x"] + f["width"] / 2, f["y"] + f["height"] / 2
        for d in inp["displays"]:
            r = d["frame"]
            if r["x"] <= cx < r["x"] + r["width"] and r["y"] <= cy < r["y"] + r["height"]:
                display = d
                break
    v = display["visible"]
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


def command(inp, name):
    """The command's arguments when the event is that command, else None."""
    event = inp["event"]
    if event["kind"] == "command" and event.get("name") == name:
        return event.get("args", [])
    return None


def clamp(value, low, high):
    return max(low, min(high, value))
