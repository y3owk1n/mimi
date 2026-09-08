#!/usr/bin/env python3
"""Scrollable strip, the way niri tiles.

Windows sit in columns on a strip that is wider than the display. The
display is a viewport onto it: focusing a window scrolls the strip until its
column is fully in view. A column is shown whole or not at all: the ones
beyond the edges, and one that would be cut across, are parked at the edge
with a sliver peeking in, enough to reach with focus_window --left/--right.
Nothing is ever squeezed to fit; a new column keeps its width and the strip
gets longer.

Commands the layout answers (mimi gives them no meaning; this file does):

  mimi tiling cmd focus <left|right>    focus the next column that way,
                                        scrolling to it; up/down within one
  mimi tiling cmd move <left|right>     move the focused column along the strip
  mimi tiling cmd consume                pull the focused window into the column
                                         on its left, stacked below
  mimi tiling cmd expel                  push the focused window out into a
                                         column of its own, to the right
  mimi tiling cmd width [fraction]       cycle the focused column through a
                                         third, a half, two thirds; or set one
  mimi tiling cmd center                 scroll the focused column to the middle
  mimi tiling cmd scroll <left|right>    scroll the strip by one column
  mimi tiling cmd togglemax              fill the display with the focused
                                         window, for now

With tiling.relayout_on_drag set, dragging a column's edge sets its width,
and dropping a window on another column moves it into that column. Use the
focus command rather than mimi action focus_window --left/--right: the
parked columns all sit at the edge, so spatial focus cannot tell them apart.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.
"""

from rules import area, clamp, command, gap, maximised, read_input, write_output

PRESETS = [1 / 3, 1 / 2, 2 / 3]
DEFAULT = 1 / 2
MIN_WIDTH, MAX_WIDTH = 0.2, 1.0
# How much of a column that does not fit whole stays visible at the edge, in
# points. macOS refuses to put a window entirely off screen: asked for this,
# it leaves about 16 points of a full-height window showing, its floor, so a
# parked column is as hidden as a window on the space can be. It is reached
# with the focus command, not by sight. A column is shown whole or as this
# sliver, never cut somewhere across.
PEEK = 8


# --- the strip ----------------------------------------------------------------
# state = {"columns": [{"windows": [numbers], "width": fraction}], "offset": points}


def column_of(columns, number):
    for index, column in enumerate(columns):
        if number in column["windows"]:
            return index
    return None


def sync(columns, present, focused):
    """Drop what closed, add what opened as a column right of the focused one."""
    for column in columns:
        column["windows"] = [n for n in column["windows"] if n in present]
    columns[:] = [c for c in columns if c["windows"]]

    known = {n for c in columns for n in c["windows"]}
    at = column_of(columns, focused)
    for number in present:
        if number in known:
            continue
        new = {"windows": [number], "width": DEFAULT}
        at = len(columns) if at is None else at + 1
        columns.insert(at, new)
        known.add(number)


def col_width(column, box, gap):
    """A column's width in points: its fraction of the area counted with the
    gaps, so two halves and the gap between them fill the area exactly."""
    return column["width"] * (box["width"] + gap) - gap


def starts(columns, box, gap):
    """The strip x of every column and the strip's total length."""
    xs, x = [], 0
    for column in columns:
        xs.append(x)
        x += col_width(column, box, gap) + gap
    return xs, max(0, x - gap)


def scroll_into_view(columns, index, box, gap, offset):
    """The offset that shows column index whole, moving as little as needed,
    and snapped to a column boundary when one lies in the range that keeps
    it whole, so a neighbour is either fully in view or off the edge rather
    than cut somewhere across."""
    xs, total = starts(columns, box, gap)
    if index is None:
        return clamp(offset, 0, max(0, total - box["width"]))
    left, width = xs[index], col_width(columns[index], box, gap)
    # The strip may scroll past its end to land on a boundary: empty space
    # after the last column beats a neighbour parked over it.
    lo, hi = max(0, left + width - box["width"]), max(0, left)
    boundaries = [x for x in xs if lo - 0.5 <= x <= hi + 0.5]
    if boundaries:
        return min(boundaries, key=lambda x: abs(x - offset))
    return clamp(offset, lo, hi)


def frames_for(columns, box, gap, offset):
    """Frames for every column: the ones that fit whole at their place on
    the strip, the ones that do not parked at the edge they are past, as a
    sliver."""
    xs, _ = starts(columns, box, gap)
    frames = []
    for column, left in zip(columns, xs):
        width = col_width(column, box, gap)
        x = box["x"] + left - offset
        if left < offset - 0.5:
            x = box["x"] - width + PEEK
        elif left + width > offset + box["width"] + 0.5:
            x = box["x"] + box["width"] - PEEK
        n = len(column["windows"])
        height = (box["height"] - gap * (n - 1)) / n
        for row, number in enumerate(column["windows"]):
            frames.append(
                (
                    number,
                    {
                        "x": x,
                        "y": box["y"] + row * (height + gap),
                        "width": width,
                        "height": height,
                    },
                )
            )
    return frames


def column_at(columns, box, gap, offset, x):
    """The column under strip-relative screen x, or None."""
    xs, _ = starts(columns, box, gap)
    for index, (column, left) in enumerate(zip(columns, xs)):
        screen_left = box["x"] + left - offset
        if screen_left <= x < screen_left + col_width(column, box, gap):
            return index
    return None


# --- one run ------------------------------------------------------------------


def main():
    inp = read_input()
    state = inp.get("state") or {}
    columns = state.get("columns") or []
    offset = float(state.get("offset") or 0)
    GAP = gap(inp)
    box = area(inp, GAP)
    event = inp["event"]
    windows = inp["windows"]
    by_number = {w["number"]: w for w in windows}
    focused = windows[inp["focused"]]["number"] if inp["focused"] >= 0 else None

    sync(columns, [w["number"] for w in windows], focused)
    if not columns:
        write_output([], {"columns": [], "offset": 0})
        return

    at = column_of(columns, focused)
    focus = None

    if event["kind"] == "command" and at is not None:
        name, args = event.get("name"), event.get("args", [])
        column = columns[at]
        if name == "focus" and args:
            if args[0] in ("left", "right"):
                to = clamp(at + (1 if args[0] == "right" else -1), 0, len(columns) - 1)
                focus = columns[to]["windows"][0]
                at = to
            else:
                rows = column["windows"]
                row = rows.index(focused)
                focus = rows[clamp(row + (1 if args[0] == "down" else -1), 0, len(rows) - 1)]
        elif name == "move" and args:
            to = clamp(at + (1 if args[0] == "right" else -1), 0, len(columns) - 1)
            columns.insert(to, columns.pop(at))
            at = to
        elif name == "consume" and at > 0:
            column["windows"].remove(focused)
            columns[at - 1]["windows"].append(focused)
            if not column["windows"]:
                columns.pop(at)
            at -= 1
        elif name == "expel" and len(column["windows"]) > 1:
            column["windows"].remove(focused)
            columns.insert(at + 1, {"windows": [focused], "width": column["width"]})
            at += 1
        elif name == "width":
            if args:
                column["width"] = clamp(float(args[0]), MIN_WIDTH, MAX_WIDTH)
            else:
                later = [p for p in PRESETS if p > column["width"] + 0.01]
                column["width"] = later[0] if later else PRESETS[0]
        elif name == "center":
            xs, total = starts(columns, box, GAP)
            width = col_width(column, box, GAP)
            offset = max(0, xs[at] + width / 2 - box["width"] / 2)
            state.update(columns=columns, offset=offset)
            write_output(maximised(inp, state, frames_for(columns, box, GAP, offset), box), state)
            return
        elif name == "scroll" and args:
            # To the next column boundary past the viewport's left edge, in
            # the direction asked, so every scroll moves a whole column.
            xs, total = starts(columns, box, GAP)
            if args[0] == "right":
                ahead = [x for x in xs if x > offset + 0.5]
                offset = ahead[0] if ahead else offset
            else:
                behind = [x for x in xs if x < offset - 0.5]
                offset = behind[-1] if behind else 0
            offset = max(0, offset)
            state.update(columns=columns, offset=offset)
            write_output(maximised(inp, state, frames_for(columns, box, GAP, offset), box), state)
            return

    elif event["kind"] == "window_resize":
        placed = state.get("placed", {})
        for number in event.get("windows", []):
            index = column_of(columns, number)
            if index is None or number not in by_number:
                continue
            now = by_number[number]["frame"]["width"]
            was = placed.get(str(number), {}).get("width", now)
            if abs(now - was) > 1:
                columns[index]["width"] = clamp((now + GAP) / (box["width"] + GAP), MIN_WIDTH, MAX_WIDTH)

    elif event["kind"] == "window_move":
        for number in event.get("windows", []):
            index = column_of(columns, number)
            if index is None or number not in by_number:
                continue
            f = by_number[number]["frame"]
            target = column_at(columns, box, GAP, offset, f["x"] + f["width"] / 2)
            if target is not None and target != index:
                columns[index]["windows"].remove(number)
                columns[target]["windows"].append(number)
                if not columns[index]["windows"]:
                    columns.pop(index)
                at = column_of(columns, focused)

    # Whatever happened, the focused column ends up in view.
    offset = scroll_into_view(columns, at, box, GAP, offset)
    frames = maximised(inp, state, frames_for(columns, box, GAP, offset), box)
    state.update(columns=columns, offset=offset)
    state["placed"] = {str(n): {k: int(round(v)) for k, v in f.items()} for n, f in frames}
    write_output(frames, state, focus)


if __name__ == "__main__":
    main()
