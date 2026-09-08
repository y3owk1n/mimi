#!/bin/sh
# Master and stack: one window fills the left share of the display, every
# other window stacks top to bottom on the right.
#
# The master is remembered in the state, so focusing another window does not
# reshuffle the layout: the master changes only when it goes away, and then
# the focused window takes over. That is what the state is for.
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
	| if $n == 0 then {frames: [], state: null} else
	    # Keep the remembered master while it is still here; otherwise the
	    # focused window, or the first.
	    (.state.master as $m
	     | if $m != null and ($numbers | index($m)) != null then $m
	       elif .focused >= 0 then $wins[.focused].number
	       else $numbers[0] end) as $master
	    | (if $n == 1 then $v.width - 2 * $gap else (($v.width - 3 * $gap) * $ratio | floor) end) as $mw
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
	        state: { master: $master } }
	  end
'
