#!/usr/bin/env python3
"""Columns that hold more than one window, as yabai stacks and niri tabs.

Windows are laid out in equal columns left to right. A column holds one
window or several, and windows in the same column share one frame, so only
the one with keyboard focus is seen. mimi draws the others as cards behind
it, so a column of four does not look like a column of one.

  mimi tiling cmd stack          put the focused window into the column to
                                 its left, or the one to its right when it
                                 is already leftmost
  mimi tiling cmd unstack        give the focused window a column of its own
  mimi tiling cmd next           focus the next window in this column
  mimi tiling cmd prev           focus the previous one
  mimi tiling cmd focus <left|right>
                                 focus the column that way, landing on the
                                 window it was last on
  mimi tiling cmd togglemax      the temporary maximise every layout answers

There is no z-order here on purpose. macOS gives no way to raise one
application's window above another's without also focusing it, so the window
seen in a column is the one with focus, and `next` moves focus rather than
raising anything. `mimi query windows` reports each window's `order`, which
is how this layout knows which member is really on top.
"""

from rules import area, command, gap, maximised, serve, shown, unmanaged_of, write_output


def columns_of(state, numbers):
    """The remembered columns, with windows that went dropped and windows
    that appeared added as columns of their own, in the order they arrive."""
    columns = [[n for n in column if n in numbers] for column in state.get("columns", [])]
    columns = [column for column in columns if column]

    known = {n for column in columns for n in column}
    for number in numbers:
        if number not in known:
            columns.append([number])

    return columns


def seen_of(column, remembered):
    """The window of a column that was last looked at, which is the one shown
    when the column has no focus and the one focus comes back to. A column
    never looked at shows its first window."""
    for number in column:
        if number in remembered:
            return number
    return column[0]


def remember(remembered, column, number):
    """Record that this window is the one its column was last on, forgetting
    whichever of its column-mates held that before."""
    kept = [n for n in remembered if n not in column]
    kept.append(number)
    return kept


def find(columns, number):
    """Where a window sits, as (column index, row index), or (None, None)."""
    for index, column in enumerate(columns):
        if number in column:
            return index, column.index(number)
    return None, None


def main(inp):
    GAP = gap(inp)
    state = inp.get("state") or {}
    box = area(inp, GAP)
    numbers = [w["number"] for w in inp["windows"]]
    focused = inp["windows"][inp["focused"]]["number"] if inp["focused"] >= 0 else None

    columns = columns_of(state, numbers)
    if not columns:
        write_output([], {"columns": []}, unmanaged=unmanaged_of(inp, state))
        return

    at, _ = find(columns, focused)
    remembered = [n for n in state.get("seen", []) if n in numbers]
    if at is not None:
        remembered = remember(remembered, columns[at], focused)

    focus = None

    if command(inp, "stack") is not None and at is not None and len(columns) > 1:
        into = at - 1 if at > 0 else 1
        columns[into].append(focused)
        columns[at].remove(focused)
        columns = [column for column in columns if column]
    elif command(inp, "unstack") is not None and at is not None and len(columns[at]) > 1:
        columns[at].remove(focused)
        columns.insert(at + 1, [focused])
    else:
        # `command` answers with an empty list for a command that takes no
        # arguments, so each one is asked about on its own: `a or b` would
        # read that empty list as "no".
        step = 0
        if command(inp, "next") is not None:
            step = 1
        elif command(inp, "prev") is not None:
            step = -1

        if step and at is not None and len(columns[at]) > 1:
            column = columns[at]
            focus = column[(column.index(focused) + step) % len(column)]

        # Between columns, landing on the window that one was last on rather
        # than on its first. Leaving a stack and coming back should not
        # change which of its windows is shown.
        side = command(inp, "focus")
        if side and at is not None and len(columns) > 1:
            to = at + (1 if side[0] == "right" else -1)
            to = max(0, min(to, len(columns) - 1))
            focus = seen_of(columns[to], remembered)

    # Every window in a column gets the column's frame. The one with focus
    # is the one seen; the rest are behind it, which is what the stack key
    # tells mimi to mark.
    width = (box["width"] - GAP * (len(columns) - 1)) / len(columns)
    frames = []
    stacks = []

    for index, column in enumerate(columns):
        frame = {
            "x": box["x"] + index * (width + GAP),
            "y": box["y"],
            "width": width,
            "height": box["height"],
        }
        for number in column:
            frames.append((number, dict(frame)))

        if len(column) > 1:
            stacks.append({"windows": list(column), "active": shown(inp, column, focus)})

    landed = focus or focused
    where, _ = find(columns, landed)
    if where is not None:
        remembered = remember(remembered, columns[where], landed)

    state["columns"] = columns
    state["seen"] = remembered
    out = maximised(inp, state, frames, box)

    write_output(out, state, focus, unmanaged=unmanaged_of(inp, state), stacks=stacks)


if __name__ == "__main__":
    serve(main)
