#!/bin/sh
# Master and stack: one window fills the left share of the display, every
# other window stacks top to bottom on the right.
#
# The master and the ratio live in the state, so focusing another window does
# not reshuffle the layout, and the layout answers two commands of its own:
#
#   mimi tiling cmd swap           make the focused window the master
#   mimi tiling cmd ratio +0.05    widen the master (or -0.05 to narrow it)
#
# mimi gives those names no meaning; this file does. Add your own. With
# tiling.relayout_on_resize set, dragging the edge between the master and
# the stack sets the ratio too, from whichever side was dragged. Dragging a
# stack window taller has nowhere to go in this layout and snaps back; see
# bsp.py for a layout where every edge is a split.
#
# A layout program: reads the tiling input on stdin, prints the output on
# stdout. Copy, edit, own. Needs jq.
#
# Usage: master-stack.sh [ratio] [gap]
set -eu

ratio="${1:-0.6}"
gap="${2:-8}"
here="$(cd "$(dirname "$0")" && pwd)"

jq -c -L "$here" --argjson ratio "$ratio" --argjson gap "$gap" '
	include "float-rules";
	tileable
	| (display_for.visible) as $v
	| .windows as $wins
	| ($wins | map(.number)) as $numbers
	| ($wins | length) as $n
	| (if .focused >= 0 then $wins[.focused].number else null end) as $focused
	| if $n == 0 then {frames: [], state: null} else
	    # The master: "swap" makes the focused window the master; otherwise
	    # the remembered one while it is still here, else the focused, else
	    # the first.
	    (.state.master as $m
	     | if .event.kind == "command" and .event.name == "swap" and $focused != null then $focused
	       elif $m != null and ($numbers | index($m)) != null then $m
	       elif $focused != null then $focused
	       else $numbers[0] end) as $master
	    # The ratio: remembered, nudged by "ratio +0.05", read off a
	    # dragged edge (the master width, or the width of a dragged stack
	    # window taken from the other side), clamped.
	    | (.event.windows // []) as $dragged
	    | ([$wins[] | select(.number == $master)][0].frame.width) as $mwNow
	    | ([$wins[] | select(.number != $master and (.number | IN($dragged[])))][0].frame.width) as $swNow
	    | ((if .event.kind == "window_resize" and $n > 1 and $swNow != null
	        then 1 - $swNow / ($v.width - 3 * $gap)
	        elif .event.kind == "window_resize" and $n > 1 and $mwNow != null and ($master | IN($dragged[]))
	        then $mwNow / ($v.width - 3 * $gap)
	        else (.state.ratio // $ratio) end)
	       + (if .event.kind == "command" and .event.name == "ratio"
	          then (.event.args[0] // "0" | tonumber) else 0 end)
	       | [[., 0.2] | max, 0.8] | min) as $r
	    | (if $n == 1 then $v.width - 2 * $gap else (($v.width - 3 * $gap) * $r | floor) end) as $mw
	    | ($v.width - $mw - 3 * $gap) as $sw
	    | ($n - 1) as $k
	    | (if $k > 0 then (($v.height - $gap * ($k + 1)) / $k | floor) else 0 end) as $sh
	    | { frames: (
	          [ { number: $master,
	              frame: { x: ($v.x + $gap), y: ($v.y + $gap), width: $mw, height: ($v.height - 2 * $gap) } } ]
	          + [ [ $wins[] | select(.number != $master) ] | to_entries[]
	              | { number: .value.number,
	                  frame: { x: ($v.x + $mw + 2 * $gap),
	                           y: ($v.y + $gap + .key * ($sh + $gap)),
	                           width: $sw,
	                           height: $sh } } ] ),
	        state: { master: $master, ratio: $r } }
	  end
'
