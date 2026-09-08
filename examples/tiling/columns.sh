#!/bin/sh
# Equal-width columns across the display holding the focused window.
#
# A layout program: reads the tiling input on stdin, prints the output on
# stdout. Run by the daemon (tiling.layout), by "mimi tiling preview", or by
# standalone.sh. Copy, edit, own. Needs jq.
#
# Usage: columns.sh [gap]
set -eu

gap="${1:-8}"
here="$(cd "$(dirname "$0")" && pwd)"

jq -c -L "$here" --argjson gap "$gap" '
	include "float-rules";
	tileable
	| (display_for.visible) as $v
	| .windows as $wins
	| ($wins | length) as $n
	| if $n == 0 then {frames: [], state: null} else
	    (($v.width - $gap * ($n + 1)) / $n | floor) as $col
	    | { frames: [ $wins | to_entries[]
	                  | { number: .value.number,
	                      frame: { x: ($v.x + $gap + .key * ($col + $gap)),
	                               y: ($v.y + $gap),
	                               width: $col,
	                               height: ($v.height - 2 * $gap) } } ],
	        state: null }
	  end
'
