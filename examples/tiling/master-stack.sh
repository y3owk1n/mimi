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
# mimi gives those names no meaning; this file does. Add your own.
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
	    # The ratio: remembered, nudged by "ratio +0.05", clamped.
	    ((.state.ratio // $ratio)
	     + (if .event.kind == "command" and .event.name == "ratio"
	        then (.event.args[0] // "0" | tonumber) else 0 end)
	     | [[., 0.2] | max, 0.8] | min) as $r
	    # The master: "swap" makes the focused window the master; otherwise
	    # the remembered one while it is still here, else the focused, else
	    # the first.
	    | (.state.master as $m
	       | if .event.kind == "command" and .event.name == "swap" and $focused != null then $focused
	         elif $m != null and ($numbers | index($m)) != null then $m
	         elif $focused != null then $focused
	         else $numbers[0] end) as $master
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
