"""Shared pieces the layout programs import: which windows to leave alone,
and which display to fill. Copy, edit, own.

Every layout here reads a JSON document per line on stdin and prints one
per line on stdout, once or for as long as stdin stays open. See README.md
for the shapes. mimi runs a layout once per display,
with that display's windows and a state of that display's own, so a layout
only ever thinks about one display. [[tiling.rules]] in config.toml says
which windows never tile. A rule there keeps a window out of every layout,
so this file has no float list.
"""

import json
import os
import subprocess
import sys

# Points to keep clear at each edge, inside the gap, for a status bar or
# desktop widgets. A number pads every display. A dict from display index
# to a number pads those displays only: "right": {2: 320}.
PADDING = {"top": 0, "bottom": 0, "left": 0, "right": 0}

def serve(layout):
    """Run `layout(inp)` for every input the daemon sends, in either mode
    mimi runs a layout: once, for one document on stdin (`layout_mode =
    "oneshot"`, the default), or once per line for as long as stdin stays
    open (`layout_mode = "resident"`), which skips the interpreter's startup
    on every pass after the first.

    Run with nothing on stdin, from a terminal or a hotkey, a layout drives
    itself instead: it builds the inputs the daemon would, runs itself once
    per display, applies the frames, and exits. That is how a layout is
    used as a one-shot command with tiling off and no daemon running."""
    if sys.stdin.isatty():
        run_once()
        return

    for line in sys.stdin:
        if line.strip():
            layout(json.loads(line))
            sys.stdout.flush()


def gap(inp):
    """The gap between windows and at the display's edges, as mimi resolved
    it: tiling.gap from the config when set, else the macOS tiled-window
    margin, the same setting `mimi action resize_window` honours, or 0 when
    that is off. A gap of one margin between two windows is what
    resize_window leaves too: half a margin on each side of the split."""
    return float(inp.get("gap", 0))


def padding(inp, side, state=None):
    """PADDING at one edge for the display this input is for, or what a
    `padding` command put in `state` for this display and space instead."""
    override = (state or {}).get("padding", {})
    if override.get("off"):
        return 0.0
    p = override.get(side, PADDING[side])
    if isinstance(p, dict):
        p = p.get(inp["display"]["index"], 0)
    return float(p)


def padding_command(inp, state):
    """Apply a `padding` command to `state`, when the event is one:

      mimi tiling cmd padding off            # pad nothing, keep the values
      mimi tiling cmd padding on
      mimi tiling cmd padding right 320      # one side, every display
      mimi tiling cmd padding reset          # back to PADDING in this file

    State is per display and space, so the change is too."""
    args = command(inp, "padding")
    if not args:
        return
    override = state.setdefault("padding", {})
    if args[0] == "reset":
        override.clear()
    elif args[0] in ("on", "off"):
        override["off"] = args[0] == "off"
    elif args[0] in PADDING and len(args) == 2:
        override[args[0]] = float(args[1])


def area(inp, gap, state=None):
    """The visible frame of the display this input is for, inset by gap on
    every side and by PADDING at each edge. mimi runs a layout once per
    display, so this is the one area a run ever fills. Given `state`, a
    `padding` command is applied to it first."""
    if state is not None:
        padding_command(inp, state)
    v = inp["display"]["visible"]
    top, bottom = padding(inp, "top", state), padding(inp, "bottom", state)
    left, right = padding(inp, "left", state), padding(inp, "right", state)
    return {
        "x": v["x"] + gap + left,
        "y": v["y"] + gap + top,
        "width": v["width"] - 2 * gap - left - right,
        "height": v["height"] - 2 * gap - top - bottom,
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


def shown(inp, windows, focus=None):
    """Which window of a stack is the one being seen.

    The one this pass is about to focus, if it named one. Otherwise the one
    with focus now. Otherwise whichever of them is in front on screen, which
    mimi reports as each window's `order`.

    That last part is what keeps a stack marked correctly while focus is
    somewhere else entirely. Falling back to the first window in the list
    instead makes an unfocused stack claim to be showing a window it is not,
    and makes it jump when focus comes back."""
    if focus in windows:
        return focus

    at = inp.get("focused", -1)
    focused = inp["windows"][at]["number"] if at >= 0 else None
    if focused in windows:
        return focused

    order = {w["number"]: w.get("order", 0) for w in inp["windows"]}
    return min(windows, key=lambda number: order.get(number, 1 << 30))


def modifiers(inp):
    """The modifier keys held during a window_move or window_resize, as a
    set of "shift", "control", "option" and "command". Empty for any other
    event, and for a drag with none held. The shipped layouts float the
    dragged window on an option-drag and stack it onto the target on a
    shift-drag."""
    return set(inp.get("event", {}).get("modifiers", []))


def unmanaged_of(inp, state=None):
    """The windows mimi should leave alone: the ones it handed back as
    `unmanaged`, plus any the layout floated itself and keeps in
    `state["floating"]`. Hand it to `write_output` so mimi's drag reading and
    its drop zone agree with the layout about which windows are the layout's.

    Without it mimi keeps watching a window the layout has stopped placing,
    because not placing a window is also what a temporary maximise does to
    the windows under it, and those it should keep watching."""
    numbers = set(inp.get("unmanaged", []))
    if state:
        numbers |= set(state.get("floating", []))
    return sorted(numbers)


def write_output(frames, state, focus=None, unmanaged=None, stacks=None, target=None):
    """Print the layout output: frames in whole points, the state to get
    back next time, the window to focus once the frames are applied, when
    the layout moved focus along its own structure, and the windows this
    layout is not managing.

    `unmanaged` is what `unmanaged_of()` returns, so mimi leaves those
    windows alone too, and dragging one raises no pass and shows no drop
    zone. Leaving a window out of `frames` says only that it does not move
    this time, which is what a temporary maximise does to the windows under
    it.

    `stacks` names the sets of windows this layout put in one place, as
    [{"windows": [...], "active": n}], so mimi marks each with a bar saying
    how many windows are there. Every member needs a frame of its own in
    `frames`, and giving them the same frame is what makes a stack. See
    stacked.py.

    `target`, on a window_move or window_resize, is `(number, action)`: the
    window the drop acts on other than the dragged one, and the layout's
    word for what happens to it, "swap" or "insert" say. The drop zone
    marks that window while the button is down. A pass ignores it."""
    frames = [
        {"number": number, "frame": {k: int(round(v)) for k, v in frame.items()}}
        for number, frame in frames
    ]
    out = {"frames": frames, "state": state}
    if focus is not None:
        out["focus"] = focus
    if unmanaged:
        out["unmanaged"] = list(unmanaged)
    if stacks:
        out["stacks"] = [
            {"windows": list(s["windows"]), "active": s["active"]} for s in stacks
        ]
    if target is not None:
        out["target"] = {"window": target[0], "action": target[1]}
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


def min_sizes(inp):
    """{number: (width, height)} for every window mimi has seen refuse a
    smaller size: the width and height it kept instead, 0 on an axis it
    took as asked. mimi learns these by writing and reading back, since
    macOS gives no way to ask, and hands them back as each window's
    `minSize`. Give such a window that much and share the rest out, or it
    lands over its neighbour."""
    sizes = {}
    for w in inp["windows"]:
        m = w.get("minSize")
        if m:
            sizes[w["number"]] = (float(m.get("width", 0)), float(m.get("height", 0)))
    return sizes


def fit(sizes, mins):
    """sizes with each raised to its minimum, the extra taken from the rest
    in proportion to what they have to spare. When they have too little to
    spare the overflow stays, and something overlaps, which is what
    happens anyway when the minimums do not fit."""
    sizes = list(sizes)
    short = [max(0.0, m - s) for s, m in zip(sizes, mins)]
    need = sum(short)
    if need <= 0:
        return sizes
    spare = [0.0 if sh > 0 else max(0.0, s - m) for s, m, sh in zip(sizes, mins, short)]
    total = sum(spare)
    take = min(need, total)
    return [
        s + sh - (take * sp / total if total > 0 else 0)
        for s, sh, sp in zip(sizes, short, spare)
    ]
