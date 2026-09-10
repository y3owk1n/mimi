"""Shared pieces the layout programs import: which windows to leave alone,
and which display to fill. Copy, edit, own.

Every layout here reads a JSON document per line on stdin and prints one
per line on stdout, once or for as long as stdin stays open. See README.md
for the shapes. mimi runs a layout once per display,
with that display's windows and a state of that display's own, so a layout
only ever thinks about one display. This file is the one place to add a
bundle identifier or a title pattern that should never be tiled.
"""

import json
import os
import re
import subprocess
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


def serve(layout):
    """Run `layout(inp)` for every input the daemon sends, in either mode
    mimi runs a layout: once, for one document on stdin (`layout_mode =
    "oneshot"`, the default), or once per line for as long as stdin stays
    open (`layout_mode = "resident"`), which skips the interpreter's startup
    on every pass after the first. Each input's `windows` is narrowed to
    the tileable ones and `focused` re-pointed at the same window, or -1 if
    it went.

    Run with nothing on stdin, from a terminal or a hotkey, a layout drives
    itself instead: it builds the inputs the daemon would, runs itself once
    per display, applies the frames, and exits. That is how a layout is
    used as a one-shot command with tiling off and no daemon running."""
    if sys.stdin.isatty():
        run_once()
        return

    for line in sys.stdin:
        if line.strip():
            layout(narrow(json.loads(line)))
            sys.stdout.flush()


def narrow(inp):
    """The input with `windows` narrowed to the tileable ones and `focused`
    re-pointed at the same window, or -1 if it went. `focusFloating` is
    True when it went because the focused window floats, so a layout can
    tell focus on a floating window from focus on nothing."""
    focused_number = (
        inp["windows"][inp["focused"]]["number"] if inp["focused"] >= 0 else None
    )
    inp["windows"] = [w for w in inp["windows"] if not floating(w)]
    numbers = [w["number"] for w in inp["windows"]]
    inp["focused"] = numbers.index(focused_number) if focused_number in numbers else -1
    inp["focusFloating"] = focused_number is not None and inp["focused"] < 0
    return inp


def gap(inp):
    """The gap between windows and at the display's edges, as mimi resolved
    it: tiling.gap from the config when set, else the macOS tiled-window
    margin, the same setting `mimi action resize_window` honours, or 0 when
    that is off. A gap of one margin between two windows is what
    resize_window leaves too: half a margin on each side of the split."""
    return float(inp.get("gap", 0))


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


def run_once():
    """Lay the desktop out once, the way the daemon would: one run of this
    program per display that has a window, with that display's windows,
    then every frame applied together. State is null, since only the daemon
    remembers state, and the gap is the macOS tiled-window margin, since no
    config is read here."""
    query = lambda what: json.loads(subprocess.check_output(["mimi", "query", what]))
    windows, displays = query("windows"), query("displays")
    space, margins = query("space")["index"], query("margins")
    gap = margins["size"] if margins["enabled"] else 0
    focused = windows["windows"][windows["focused"]]["number"] if windows["focused"] >= 0 else None

    frames = []
    for display in displays:
        mine = [w for w in windows["windows"] if _on(w["frame"], display["frame"])]
        if not mine:
            continue
        numbers = [w["number"] for w in mine]
        inp = {
            "version": 1,
            "event": {"kind": "relayout"},
            "display": display,
            "space": space,
            "gap": gap,
            "displays": displays,
            "focused": numbers.index(focused) if focused in numbers else -1,
            "windows": mine,
            "state": None,
        }
        out = subprocess.run(
            [sys.executable, os.path.abspath(sys.argv[0])] + sys.argv[1:],
            input=json.dumps(inp), capture_output=True, text=True, check=True,
        )
        if out.stdout.strip():
            frames.extend(json.loads(out.stdout).get("frames") or [])

    if frames:
        subprocess.run(["mimi", "action", "apply_frames"], input=json.dumps(frames), text=True, check=True)


def _on(frame, bounds):
    cx, cy = frame["x"] + frame["width"] / 2, frame["y"] + frame["height"] / 2
    return bounds["x"] <= cx < bounds["x"] + bounds["width"] and bounds["y"] <= cy < bounds["y"] + bounds["height"]


def write_output(frames, state, focus=None):
    """Print the layout output: frames in whole points, the state to get
    back next time, and the window to focus once the frames are applied,
    when the layout moved focus along its own structure."""
    frames = [
        {"number": number, "frame": {k: int(round(v)) for k, v in frame.items()}}
        for number, frame in frames
    ]
    out = {"frames": frames, "state": state}
    if focus is not None:
        out["focus"] = focus
    json.dump(out, sys.stdout)
    sys.stdout.write("\n")


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
