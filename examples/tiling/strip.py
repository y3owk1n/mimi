#!/usr/bin/env python3
"""Scrollable strip, the way niri tiles.

Windows sit in columns on a strip that is wider than the display. The
display is a viewport onto it: focusing a window scrolls the strip until its
column is fully in view. A column that overlaps the viewport is shown at its
place on the strip, cut at the edge if it does not fit, so a neighbour of a
column wider than half stays partly in sight. The ones wholly beyond the
edges are parked there with a sliver peeking in, enough to reach with the
focus command. Nothing is ever squeezed to fit; a new column keeps its width
and the strip gets longer.

Commands the layout answers (mimi gives them no meaning; this file does):

  mimi tiling cmd focus <left|right>    focus the next column that way,
                                        scrolling to it; up/down within one
  mimi tiling cmd move <left|right>     move the focused column along the strip;
                                        up/down swaps the window within one
  mimi tiling cmd consume                pull the focused window into the column
                                         on its left, as another row
  mimi tiling cmd expel                  push the focused window out into a
                                         column of its own, to the right
  mimi tiling cmd width [fraction|prev|+d|-d]
                                        cycle the focused column through a
                                        third, a half, two thirds (prev goes
                                        back); set a fraction; or nudge it by
                                        d, as niri's +10%
  mimi tiling cmd center                 scroll the focused column to the middle
  mimi tiling cmd togglefloat            take the focused window off the strip
                                        and leave it where it is, or put it
                                        back in a column of its own
  mimi tiling cmd scroll <left|right> [fraction]
                                        scroll the strip a step that way, a
                                        quarter of the display unless given
  mimi tiling cmd togglemax              fill the display with the focused
                                         window, for now
  mimi tiling cmd togglestack            make the focused column hold its
                                         windows in one place rather than as
                                         rows, so only the focused one is
                                         seen, and back again

PRIORITY, at the top of the file, lists bundle identifiers whose windows
open at a fixed place on the strip, each in a column of its own beside the
app's others. Anything else opens right of the focused column.

A column holds its windows as rows by default, each with a share of the
height. togglestack makes it hold them in one place instead, the way niri
tabs a column: every window fills the column and only the focused one is
seen. With [tiling.stackbar] enabled mimi marks such a column, one segment
per window, which is the only thing that says how many are in there. focus
up and down still move between them, and move up and down still reorder
them.

With tiling.relayout_on_drag set, dragging a column's edge sets its width,
and dropping a window on another column moves it into that column. A window
sharing a column, dropped on empty strip or in the outer quarter of its own
column, gets a column of its own on that side. Dropped higher or lower in a
column of rows, it takes that row. A tabbed column has no rows, so it stays
where it is. Use the
focus command rather than mimi action focus_window --left/--right: the
parked columns all sit at the edge, so spatial focus cannot tell them apart.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.
"""

from rules import area, clamp, command, gap, maximised, serve, unmanaged_of, write_output

PRESETS = [1 / 3, 1 / 2, 2 / 3]
DEFAULT = 1 / 2
SCROLL_STEP = 1 / 4
MIN_WIDTH, MAX_WIDTH = 0.2, 1.0
# How much of a column wholly off the strip's visible part stays at the
# display's edge, in points. macOS refuses to put a window entirely off
# screen but allows this little, so a parked column is as hidden as a window
# on the space can be. It is reached with the focus command, not by sight.
# (paneru, the other sliding tiler for macOS, parks at 5 for the same reason.)
PEEK = 4
# Bundle identifiers whose windows open at a fixed place on the strip,
# leftmost first, each in a column of its own beside the app's others.
# Everything else opens right of the focused column. Only
# a new window is placed this way: move, consume, expel, and dragging still
# rearrange what is there, and that order stays. On startup and reload every
# window is new, so the strip comes up in this order.
PRIORITY = []  # e.g. ["com.apple.Terminal", "com.apple.Safari"]


# --- the strip ----------------------------------------------------------------
# state = {"columns": [{"windows": [numbers], "width": fraction}], "offset": points}


def column_of(columns, number):
    for index, column in enumerate(columns):
        if number in column["windows"]:
            return index
    return None


def rank(number, by_number):
    """Where number's app sits in PRIORITY, or None when it is not listed."""
    window = by_number.get(number)
    if window is None or window.get("bundleId") not in PRIORITY:
        return None
    return PRIORITY.index(window["bundleId"])


def sync(columns, windows, focused):
    """Drop what closed, add what opened: a listed app at its place in
    PRIORITY, anything else as a column right of the focused one."""
    by_number = {w["number"]: w for w in windows}
    for column in columns:
        column["windows"] = [n for n in column["windows"] if n in by_number]
    columns[:] = [c for c in columns if c["windows"]]

    known = {n for c in columns for n in c["windows"]}
    at = column_of(columns, focused)
    for number in by_number:
        if number in known:
            continue
        known.add(number)
        mine = rank(number, by_number)
        if mine is None:
            at = len(columns) if at is None else at + 1
            columns.insert(at, {"windows": [number], "width": DEFAULT})
            continue
        # A column of its own, after the last column ranked at or above
        # this one, so an app's windows sit side by side; unranked
        # columns the user placed among them stay where they are.
        to = 0
        for index, column in enumerate(columns):
            ranks = [r for r in (rank(n, by_number) for n in column["windows"]) if r is not None]
            if ranks and min(ranks) <= mine:
                to = index + 1
        columns.insert(to, {"windows": [number], "width": DEFAULT})
        if at is not None and to <= at:
            at += 1


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
    """The offset that shows column index whole, moving as little as
    needed, so a position the user scrolled to stays until the focused
    column would leave the view."""
    xs, total = starts(columns, box, gap)
    if index is None:
        return clamp(offset, 0, max(0, total - box["width"]))
    left, width = xs[index], col_width(columns[index], box, gap)
    return clamp(offset, max(0, left + width - box["width"]), max(0, left))


def frames_for(columns, box, edge, gap, offset):
    """Frames for every column: the ones that overlap the viewport at their
    place on the strip, cut at the edge where they do not fit; the ones
    wholly outside parked past the display's edge (not the gap-inset area's,
    or the gap would show too), as a sliver."""
    xs, _ = starts(columns, box, gap)
    frames = []
    for column, left in zip(columns, xs):
        width = col_width(column, box, gap)
        x = box["x"] + left - offset
        if left + width <= offset + 0.5:
            x = edge["x"] - width + PEEK
        elif left >= offset + box["width"] - 0.5:
            x = edge["x"] + edge["width"] - PEEK
        # A tabbed column puts every window in the whole of it, so only the
        # focused one is seen and mimi marks how many are there. Otherwise
        # the windows are rows sharing the height between them.
        tabbed = bool(column.get("tabbed"))
        n = 1 if tabbed else len(column["windows"])
        height = (box["height"] - gap * (n - 1)) / n
        for row, number in enumerate(column["windows"]):
            frames.append(
                (
                    number,
                    {
                        "x": x,
                        "y": box["y"] + (0 if tabbed else row * (height + gap)),
                        "width": width,
                        "height": height,
                    },
                )
            )
    return frames


def stacks_of(columns, focused):
    """The tabbed columns holding more than one window, in the shape
    mimi's stacks key takes, each marking the focused window when it is one of
    them."""
    stacks = []
    for column in columns:
        if not column.get("tabbed") or len(column["windows"]) < 2:
            continue
        windows = column["windows"]
        stacks.append(
            {
                "windows": list(windows),
                "active": focused if focused in windows else windows[0],
            }
        )
    return stacks


def any_in_view(columns, box, gap, offset):
    """Whether at least one column is shown whole at this offset."""
    xs, _ = starts(columns, box, gap)
    for column, left in zip(columns, xs):
        if left >= offset - 0.5 and left + col_width(column, box, gap) <= offset + box["width"] + 0.5:
            return True
    return False


def column_at(columns, box, gap, offset, x):
    """The column under strip-relative screen x, or None."""
    xs, _ = starts(columns, box, gap)
    for index, (column, left) in enumerate(zip(columns, xs)):
        screen_left = box["x"] + left - offset
        if screen_left <= x < screen_left + col_width(column, box, gap):
            return index
    return None


# --- one run ------------------------------------------------------------------


def main(inp):
    state = inp.get("state") or {}
    columns = state.get("columns") or []
    offset = float(state.get("offset") or 0)
    GAP = gap(inp)
    box = area(inp, GAP)
    edge = inp["display"]["visible"]
    event = inp["event"]
    focused = inp["windows"][inp["focused"]]["number"] if inp["focused"] >= 0 else None

    # togglefloat first: it changes which windows belong on the strip. The
    # shared rules never see this list, so it lives in the state.
    if command(inp, "togglefloat") is not None and focused is not None:
        floats = set(state.get("floating", []))
        floats ^= {focused}
        state["floating"] = sorted(floats)
    windows = [w for w in inp["windows"] if w["number"] not in state.get("floating", [])]
    by_number = {w["number"]: w for w in windows}
    focus_floating = inp["focusFloating"] or (focused is not None and focused not in by_number)
    if focused not in by_number:
        focused = None

    sync(columns, windows, focused)
    if not columns:
        write_output(
            [],
            {"columns": [], "offset": 0, "floating": state.get("floating", [])},
            unmanaged=unmanaged_of(inp, state),
        )
        return

    at = column_of(columns, focused)
    focus = None

    if event["kind"] == "command":
        name, args = event.get("name"), event.get("args", [])
        if name == "scroll" and args:
            # A step along the strip, in the direction asked, kept within
            # the strip's ends. Focus stays where it is: the view floats
            # until the next event would leave the focused column hidden.
            # This needs no focused column, so it works while a floating
            # window has focus too.
            _, total = starts(columns, box, GAP)
            step = (float(args[1]) if len(args) > 1 else SCROLL_STEP) * box["width"]
            offset += step if args[0] == "right" else -step
            offset = clamp(offset, 0, max(0, total - box["width"]))
            state.update(columns=columns, offset=offset)
            write_output(
                maximised(inp, state, frames_for(columns, box, edge, GAP, offset), box),
                state,
                unmanaged=unmanaged_of(inp, state),
                stacks=stacks_of(columns, focused),
            )
            return

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
            if args[0] in ("left", "right"):
                to = clamp(at + (1 if args[0] == "right" else -1), 0, len(columns) - 1)
                columns.insert(to, columns.pop(at))
                at = to
            else:
                rows = column["windows"]
                row = rows.index(focused)
                to = clamp(row + (1 if args[0] == "down" else -1), 0, len(rows) - 1)
                rows[row], rows[to] = rows[to], rows[row]
        elif name == "consume" and at > 0:
            column["windows"].remove(focused)
            columns[at - 1]["windows"].append(focused)
            if not column["windows"]:
                columns.pop(at)
            at -= 1
        elif name == "togglestack":
            column["tabbed"] = not column.get("tabbed")
        elif name == "expel" and len(column["windows"]) > 1:
            column["windows"].remove(focused)
            columns.insert(at + 1, {"windows": [focused], "width": column["width"]})
            at += 1
        elif name == "width":
            arg = args[0] if args else ""
            if arg.startswith(("+", "-")):
                column["width"] = clamp(column["width"] + float(arg), MIN_WIDTH, MAX_WIDTH)
            elif arg == "prev":
                earlier = [p for p in PRESETS if p < column["width"] - 0.01]
                column["width"] = earlier[-1] if earlier else PRESETS[-1]
            elif arg:
                column["width"] = clamp(float(arg), MIN_WIDTH, MAX_WIDTH)
            else:
                later = [p for p in PRESETS if p > column["width"] + 0.01]
                column["width"] = later[0] if later else PRESETS[0]
        elif name == "center":
            xs, total = starts(columns, box, GAP)
            width = col_width(column, box, GAP)
            offset = max(0, xs[at] + width / 2 - box["width"] / 2)
            state.update(columns=columns, offset=offset)
            write_output(
                maximised(inp, state, frames_for(columns, box, edge, GAP, offset), box),
                state,
                unmanaged=unmanaged_of(inp, state),
                stacks=stacks_of(columns, focused),
            )
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
            centre = f["x"] + f["width"] / 2
            target = column_at(columns, box, GAP, offset, centre)
            if target is not None and target != index:
                columns[index]["windows"].remove(number)
                columns[target]["windows"].append(number)
                if not columns[index]["windows"]:
                    columns.pop(index)
                at = column_of(columns, focused)
            elif len(columns[index]["windows"]) > 1:
                # Empty strip or the outer quarter of its own column expels
                # a stacked window to that side. The middle half moves it to
                # the row its centre landed on.
                xs, _ = starts(columns, box, GAP)
                stack = columns[index]["windows"]
                if target is None:
                    to = sum(1 for left in xs if box["x"] + left - offset < centre)
                else:
                    left = box["x"] + xs[index] - offset
                    width = col_width(columns[index], box, GAP)
                    if centre < left + width / 4:
                        to = index
                    elif centre >= left + width * 3 / 4:
                        to = index + 1
                    elif columns[index].get("tabbed"):
                        # Every window in a tabbed column fills it, so there
                        # is no row the drop landed on and it stays put.
                        continue
                    else:
                        middle = f["y"] + f["height"] / 2 - box["y"]
                        row = int(clamp(middle // ((box["height"] + GAP) / len(stack)), 0, len(stack) - 1))
                        stack.remove(number)
                        stack.insert(row, number)
                        continue
                stack.remove(number)
                columns.insert(to, {"windows": [number], "width": columns[index]["width"]})
                at = column_of(columns, focused)

    # Whatever happened, the focused column ends up in view. And when no
    # column is in view at all, because the one that was closed or focus
    # went somewhere untiled, the nearest column comes in and takes focus:
    # a viewport with everything parked is never what anyone wanted.
    offset = scroll_into_view(columns, at, box, GAP, offset)
    if not any_in_view(columns, box, GAP, offset):
        xs, _ = starts(columns, box, GAP)
        nearest = min(range(len(columns)), key=lambda i: abs(xs[i] - offset))
        offset = scroll_into_view(columns, nearest, box, GAP, offset)

    # A window that went away leaves focus wherever macOS put it, which may
    # be nothing at all. Then the first column in view takes it. Focus on a
    # floating window stays, as when a Quick Look panel closes and Finder
    # takes focus back. So does focus outside the strip on any other event.
    if focus is None and at is None and not focus_floating and event["kind"] in ("window_closed", "app_quit", "app_hide"):
        xs, _ = starts(columns, box, GAP)
        for column, left in zip(columns, xs):
            if left >= offset - 0.5:
                focus = column["windows"][0]
                break
    frames = maximised(inp, state, frames_for(columns, box, edge, GAP, offset), box)
    state.update(columns=columns, offset=offset)
    state["placed"] = {str(n): {k: int(round(v)) for k, v in f.items()} for n, f in frames}
    write_output(
        frames,
        state,
        focus,
        unmanaged=unmanaged_of(inp, state),
        stacks=stacks_of(columns, focus or focused),
    )


if __name__ == "__main__":
    serve(main)
