#!/bin/sh
# Equal-width columns across the display holding the focused window.
#
# Copy, edit, own. Needs jq. Usage: columns.sh [gap]
set -eu

gap="${1:-8}"

windows="$(mimi query windows)"
displays="$(mimi query displays)"

# Windows to tile: everything on the active space that the float rules keep.
# Delete the float-rules line to tile them all.
here="$(cd "$(dirname "$0")" && pwd)"
windows="$(printf '%s' "$windows" | jq -f "$here/float-rules.jq")"

count="$(printf '%s' "$windows" | jq '.windows | length')"
[ "$count" -gt 0 ] || exit 0

# The display to fill is the one under the focused window, or the first.
frames="$(jq -n \
	--argjson w "$windows" \
	--argjson d "$displays" \
	--argjson gap "$gap" '
	def center: {x: (.x + .width / 2), y: (.y + .height / 2)};
	def contains($p): ($p.x >= .x and $p.x < .x + .width and $p.y >= .y and $p.y < .y + .height);

	($w.windows) as $wins
	| (if $w.focused >= 0 then $wins[$w.focused].frame | center else null end) as $c
	| ([$d[] | select($c != null and (.frame | contains($c)))][0] // $d[0]) as $disp
	| $disp.visible as $v
	| ($wins | length) as $n
	| (($v.width - $gap * ($n + 1)) / $n | floor) as $col
	| [ $wins | to_entries[]
	    | { number: .value.number,
	        frame: { x: ($v.x + $gap + .key * ($col + $gap)),
	                 y: ($v.y + $gap),
	                 width: $col,
	                 height: ($v.height - 2 * $gap) } } ]
')"

printf '%s' "$frames" | mimi action apply_frames
