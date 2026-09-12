#!/usr/bin/env python3
"""Dwindle BSP, the way Hyprland tiles by default.

Every window is a leaf of a binary tree. A new window splits the focused
window's area in two, side by side when that area is wider than tall and one
above the other otherwise. Closing a window hands its area back to its
sibling. The tree lives in the state mimi keeps for the space, so nothing
here touches a file.

A leaf can hold more than one window, as yabai stacks. They share the whole
area, so only the focused one is seen, and with [tiling.stackbar] enabled mimi
draws the rest as cards behind it. stack moves a window into its
neighbour that way, unstack gives it an area of its own again, and next and
prev move round the windows sharing one. A swap or a drag moves a whole leaf,
so a stack travels together.

Commands the layout answers (mimi gives them no meaning; this file does):

  mimi tiling cmd swap <left|right|up|down>   swap with the neighbour that way
  mimi tiling cmd togglesplit                 flip the focused window's split
  mimi tiling cmd ratio <delta>               grow (+) or shrink (-) the focused
                                              window's share of its split
  mimi tiling cmd togglefloat                 take the focused window out of the
                                              tree, or put it back
  mimi tiling cmd togglemax                   fill the area with the focused
                                              window, for now
  mimi tiling cmd stack <left|right|up|down>  move the focused window into the
                                              neighbour that way, sharing its
                                              area rather than splitting it
  mimi tiling cmd unstack                     give the focused window an area
                                              of its own again
  mimi tiling cmd next / prev                 move round the windows sharing
                                              one area
  mimi tiling cmd focus <left|right|up|down>  focus the neighbour that way,
                                              landing on the window it is
                                              showing when it holds several

With tiling.relayout_on_drag set, dragging any edge of any window resizes the
split that edge belongs to, and the rest of the tree follows; dragging a
window and dropping it on another swaps the two, as Hyprland does.

A layout program: reads the tiling input on stdin, prints the output on
stdout. Copy, edit, own. Standard library only.

Usage: bsp.py     (the gap is tiling.gap, else the macOS tiled-window margin)
"""

import sys

from rules import clamp as clamp_to, shown, unmanaged_of
from rules import area, command, gap, maximised, serve, write_output

# The gap, set from the input once it is read. The tree functions below read
# it as a global.
GAP = 0.0
MIN_RATIO, MAX_RATIO = 0.1, 0.9


# --- the tree -------------------------------------------------------------
# A leaf is {"win": number}, or {"win": number, "order": [numbers]} when more
# than one window shares its area. "order" is every window in the leaf, in the
# order they were stacked, and "win" is whichever of them is seen. Every window
# gets the leaf's whole rect, so the rest are behind it, and focus is what
# brings one to the front. A split is {"dir": "h"|"v", "ratio": r, "a": node,
# "b": node}: "h" puts a left of b, "v" puts a above b.


def members(leaf):
    """Every window in a leaf, in the order they were stacked.

    The order does not change when another of them is brought to the front:
    which one is seen is "win", and where it sits in this list is where the
    user is in the stack. Moving the seen window to the head instead would
    make "next" swap the same two windows forever, and would leave the mark
    mimi draws unable to say which one of them is being looked at."""
    return leaf.get("order") or [leaf["win"]]


def leaf_holding(tree, number):
    """The leaf number is in, whether it is the one seen or behind it."""
    for leaf in leaves(tree):
        if number in members(leaf):
            return leaf
    return None


def set_members(leaf, numbers):
    """Put exactly these windows in a leaf, keeping the one seen when it is
    still among them."""
    seen = leaf.get("win")
    leaf["win"] = seen if seen in numbers else numbers[0]
    if len(numbers) > 1:
        leaf["order"] = list(numbers)
    else:
        leaf.pop("order", None)


def surface(leaf, number):
    """Make number the window seen in its leaf, leaving the order alone."""
    if number in members(leaf):
        leaf["win"] = number


def leaves(node):
    if node is None:
        return []
    if "win" in node:
        return [node]
    return leaves(node["a"]) + leaves(node["b"])


def remove(node, number):
    """The tree without number. A leaf holding other windows keeps its place
    and one of them is seen instead. The last window out takes the leaf with
    it, and its sibling takes the parent's place."""
    if node is None or "win" in node:
        if node is None or number not in members(node):
            return node
        rest = [n for n in members(node) if n != number]
        if not rest:
            return None
        set_members(node, rest)
        return node
    a, b = remove(node["a"], number), remove(node["b"], number)
    if a is None:
        return b
    if b is None:
        return a
    node["a"], node["b"] = a, b
    return node


def insert(node, target, number, rects):
    """Split target's leaf to make room for number beside it."""
    if node is None:
        return {"win": number}
    if "win" in node:
        if node["win"] != target:
            return node
        rect = rects.get(target, {"width": 1, "height": 0})
        direction = "h" if rect["width"] >= rect["height"] else "v"
        return {"dir": direction, "ratio": 0.5, "a": node, "b": {"win": number}}
    node["a"] = insert(node["a"], target, number, rects)
    node["b"] = insert(node["b"], target, number, rects)
    return node


def path_to(node, number, path=()):
    """The (node, side) pairs from the root down to number's leaf."""
    if node is None:
        return None
    if "win" in node:
        return list(path) if node["win"] == number else None
    for side in ("a", "b"):
        found = path_to(node[side], number, path + ((node, side),))
        if found is not None:
            return found
    return None


# --- geometry -------------------------------------------------------------


def layout(node, rect, rects):
    """Fill rects with the rect of every leaf under node, gaps included."""
    if node is None:
        return
    if "win" in node:
        rects[node["win"]] = rect
        return
    x, y, w, h = rect["x"], rect["y"], rect["width"], rect["height"]
    r = node["ratio"]
    if node["dir"] == "h":
        aw = round((w - GAP) * r)
        a = {"x": x, "y": y, "width": aw, "height": h}
        b = {"x": x + aw + GAP, "y": y, "width": w - aw - GAP, "height": h}
    else:
        ah = round((h - GAP) * r)
        a = {"x": x, "y": y, "width": w, "height": ah}
        b = {"x": x, "y": y + ah + GAP, "width": w, "height": h - ah - GAP}
    layout(node["a"], a, rects)
    layout(node["b"], b, rects)


def split_rects(node, rect, out):
    """The rect of every split node, for turning a dragged edge into a ratio."""
    if node is None or "win" in node:
        return
    out.append((node, rect))
    x, y, w, h = rect["x"], rect["y"], rect["width"], rect["height"]
    r = node["ratio"]
    if node["dir"] == "h":
        aw = round((w - GAP) * r)
        split_rects(node["a"], {"x": x, "y": y, "width": aw, "height": h}, out)
        split_rects(node["b"], {"x": x + aw + GAP, "y": y, "width": w - aw - GAP, "height": h}, out)
    else:
        ah = round((h - GAP) * r)
        split_rects(node["a"], {"x": x, "y": y, "width": w, "height": ah}, out)
        split_rects(node["b"], {"x": x, "y": y + ah + GAP, "width": w, "height": h - ah - GAP}, out)


def clamp(r):
    return clamp_to(r, MIN_RATIO, MAX_RATIO)


def apply_drag(tree, number, placed, now, area):
    """The user moved an edge of number: give that edge's split the new ratio.

    Each edge of a leaf is a boundary of exactly one ancestor split: the
    right edge of a leaf on the "a" side of an "h" split is that split's
    boundary, and so on up the tree. The nearest such ancestor is the one
    whose ratio the drag changes."""
    path = path_to(tree, number)
    if not path:
        return
    rects = []
    split_rects(tree, area, rects)
    rect_of = {id(node): rect for node, rect in rects}

    moved = []
    if abs(now["x"] - placed["x"]) > 1:
        moved.append(("h", "b", now["x"]))  # left edge: a split where we are on the right
    if abs((now["x"] + now["width"]) - (placed["x"] + placed["width"])) > 1:
        moved.append(("h", "a", now["x"] + now["width"]))  # right edge
    if abs(now["y"] - placed["y"]) > 1:
        moved.append(("v", "b", now["y"]))  # top edge
    if abs((now["y"] + now["height"]) - (placed["y"] + placed["height"])) > 1:
        moved.append(("v", "a", now["y"] + now["height"]))  # bottom edge

    for direction, side, edge in moved:
        for node, which in reversed(path):
            if node["dir"] != direction or which != side:
                continue
            rect = rect_of[id(node)]
            if direction == "h":
                inner = rect["width"] - GAP
                a_size = (edge - rect["x"]) if side == "a" else (edge - GAP - rect["x"])
            else:
                inner = rect["height"] - GAP
                a_size = (edge - rect["y"]) if side == "a" else (edge - GAP - rect["y"])
            if inner > 0:
                node["ratio"] = clamp(a_size / inner)
            break


def leaf_at(rects, number, point):
    """The leaf other than number whose rect holds point, or None."""
    px, py = point
    for other, rect in rects.items():
        if other == number:
            continue
        if rect["x"] <= px < rect["x"] + rect["width"] and rect["y"] <= py < rect["y"] + rect["height"]:
            return other
    return None


def swap_leaves(tree, first, second):
    """Exchange the two leaves' windows. A leaf is a place, so a leaf holding
    several windows travels whole: swapping one out of a stack and leaving the
    rest behind would stack them with a window nobody put there."""
    mine = leaves(tree)
    la = next(l for l in mine if l["win"] == first)
    lb = next(l for l in mine if l["win"] == second)
    held_a, held_b = members(la), members(lb)
    set_members(la, held_b)
    set_members(lb, held_a)


def neighbour(rects, number, direction):
    """The leaf whose rect lies that way from number's, nearest by centre."""
    me = rects.get(number)
    if me is None:
        return None
    mcx, mcy = me["x"] + me["width"] / 2, me["y"] + me["height"] / 2
    best, best_d = None, None
    for other, rect in rects.items():
        if other == number:
            continue
        cx, cy = rect["x"] + rect["width"] / 2, rect["y"] + rect["height"] / 2
        dx, dy = cx - mcx, cy - mcy
        ok = {
            "left": dx < 0 and abs(dy) <= abs(dx),
            "right": dx > 0 and abs(dy) <= abs(dx),
            "up": dy < 0 and abs(dx) <= abs(dy),
            "down": dy > 0 and abs(dx) <= abs(dy),
        }.get(direction, False)
        if not ok:
            continue
        d = dx * dx + dy * dy
        if best_d is None or d < best_d:
            best, best_d = other, d
    return best


# --- one pass -------------------------------------------------------------


def main(inp):
    global GAP

    GAP = gap(inp)
    state = inp.get("state") or {}
    box = area(inp, GAP)
    tree = state.get("tree")
    event = inp["event"]
    focused_win = inp["windows"][inp["focused"]] if inp["focused"] >= 0 else None
    focused = focused_win["number"] if focused_win else None

    # togglefloat first: it changes which windows belong in the tree. The
    # shared rules never see this list, so it lives in the state.
    if command(inp, "togglefloat") is not None and focused:
        floats = set(state.get("floating", []))
        floats ^= {focused}
        state["floating"] = sorted(floats)

    tiled = [w for w in inp["windows"] if w["number"] not in state.get("floating", [])]
    by_number = {w["number"]: w for w in tiled}
    present = set(by_number)

    # Sync the tree with what is on the space: drop what closed, add what
    # opened beside the focused window (or the last leaf).
    for leaf in leaves(tree):
        for number in members(leaf):
            if number not in present:
                tree = remove(tree, number)
    for number in [w["number"] for w in tiled]:
        if any(number in members(leaf) for leaf in leaves(tree)):
            continue
        rects = {}
        layout(tree, box, rects)
        known = [leaf["win"] for leaf in leaves(tree)]
        target = focused if focused in known else (known[-1] if known else None)
        tree = insert(tree, target, number, rects) if target else {"win": number}

    # The focused window is the one seen in its leaf. Everything below reads
    # the tree through rects, which hold only the window seen, so this is what
    # lets a window behind another be swapped, resized and dropped on.
    held = leaf_holding(tree, focused) if focused else None
    if held is not None:
        surface(held, focused)

    focus = None

    # Then the event. mimi has already told a move from a resize, from where
    # the window ended up. A move dropped on another window swaps with it,
    # dropped on nothing it snaps back. A resize moved an edge, and resizes
    # that edge's split.
    if event["kind"] == "window_move":
        rects = {}
        layout(tree, box, rects)
        for number in event.get("windows", []):
            if number not in by_number:
                continue
            now = by_number[number]["frame"]
            centre = (now["x"] + now["width"] / 2, now["y"] + now["height"] / 2)
            other = leaf_at(rects, number, centre)
            if other is not None:
                swap_leaves(tree, number, other)
    elif event["kind"] == "window_resize":
        placed = state.get("placed", {})
        for number in event.get("windows", []):
            key = str(number)
            if key in placed and number in by_number:
                apply_drag(tree, number, placed[key], by_number[number]["frame"], box)
    elif event["kind"] == "command" and focused:
        name, args = event.get("name"), event.get("args", [])
        path = path_to(tree, focused)
        parent = path[-1] if path else None
        if name == "togglesplit" and parent:
            node, _ = parent
            node["dir"] = "v" if node["dir"] == "h" else "h"
        elif name == "ratio" and parent and args:
            node, side = parent
            delta = float(args[0]) * (1 if side == "a" else -1)
            node["ratio"] = clamp(node["ratio"] + delta)
        elif name == "focus" and args:
            # To the leaf that way, landing on the window it is showing
            # rather than on whichever of its windows the window server lists
            # first. Every window in a stacked leaf has the same frame, so
            # spatial focus cannot tell them apart.
            rects = {}
            layout(tree, box, rects)
            other = neighbour(rects, focused, args[0])
            if other:
                into = leaf_holding(tree, other)
                focus = into["win"] if into else other
        elif name == "swap" and args:
            rects = {}
            layout(tree, box, rects)
            other = neighbour(rects, focused, args[0])
            if other:
                swap_leaves(tree, focused, other)
        elif name == "stack" and args and focused:
            # Into the leaf that way, which loses its own area and keeps the
            # neighbour's. The window that moved keeps focus, so it is the one
            # left seen.
            rects = {}
            layout(tree, box, rects)
            other = neighbour(rects, focused, args[0])
            if other:
                tree = remove(tree, focused)
                into = leaf_holding(tree, other)
                if into is not None:
                    set_members(into, [*members(into), focused])
                    surface(into, focused)
        elif name == "unstack" and focused:
            # Out of its leaf and into a split beside it, which is where a new
            # window would have gone.
            held = leaf_holding(tree, focused)
            if held is not None and len(members(held)) > 1:
                tree = remove(tree, focused)
                rects = {}
                layout(tree, box, rects)
                beside = leaf_holding(tree, held["win"])
                tree = insert(tree, beside["win"], focused, rects)
                focus = focused
        elif name in ("next", "prev") and focused:
            # Round the windows in this leaf. Focus is the whole move: the
            # one with focus is the one seen.
            held = leaf_holding(tree, focused)
            if held is not None and len(members(held)) > 1:
                order = members(held)
                step = 1 if name == "next" else -1
                focus = order[(order.index(focused) + step) % len(order)]
                surface(held, focus)


    rects = {}
    layout(tree, box, rects)

    # Every window in a leaf gets the leaf's rect, so only the one seen shows.
    frames = []
    stacks = []
    for leaf in leaves(tree):
        rect = rects.get(leaf["win"])
        if rect is None:
            continue
        held = members(leaf)
        frames.extend((number, rect) for number in held)
        if len(held) > 1:
            stacks.append({"windows": held, "active": shown(inp, held, focus)})

    state["tree"] = tree
    frames = maximised(inp, state, frames, box)
    state["placed"] = {
        str(number): {k: int(round(v)) for k, v in rect.items()} for number, rect in frames
    }
    write_output(frames, state, focus, unmanaged=unmanaged_of(inp, state), stacks=stacks)


if __name__ == "__main__":
    serve(main)
