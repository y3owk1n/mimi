#!/bin/sh
# Master and stack: the focused window fills the left share of the display,
# every other window stacks top to bottom on the right.
#
# Copy, edit, own. Needs jq. Usage: master-stack.sh [ratio] [gap]
set -eu

ratio="${1:-0.6}"
gap="${2:-8}"

windows="$(mimi query windows)"
displays="$(mimi query displays)"

here="$(cd "$(dirname "$0")" && pwd)"
windows="$(printf '%s' "$windows" | jq -f "$here/float-rules.jq")"

count="$(printf '%s' "$windows" | jq '.windows | length')"
[ "$count" -gt 0 ] || exit 0

frames="$(jq -n \
	--argjson w "$windows" \
	--argjson d "$displays" \
	--argjson ratio "$ratio" \
	--argjson gap "$gap" '
	def center: {x: (.x + .width / 2), y: (.y + .height / 2)};
	def contains($p): ($p.x >= .x and $p.x < .x + .width and $p.y >= .y and $p.y < .y + .height);

	($w.windows) as $wins
	| (if $w.focused >= 0 then $w.focused else 0 end) as $m
	| ($wins[$m].frame | center) as $c
	| ([$d[] | select(.frame | contains($c))][0] // $d[0]) as $disp
	| $disp.visible as $v
	| ($wins | length) as $n
	| (if $n == 1 then $v.width - 2 * $gap else (($v.width - 3 * $gap) * $ratio | floor) end) as $mw
	| ($v.width - $mw - 3 * $gap) as $sw
	| ($n - 1) as $k
	| (if $k > 0 then (($v.height - $gap * ($k + 1)) / $k | floor) else 0 end) as $sh
	| [ { number: $wins[$m].number,
	      frame: { x: ($v.x + $gap), y: ($v.y + $gap), width: $mw, height: ($v.height - 2 * $gap) } } ]
	  + [ [ $wins | to_entries[] | select(.key != $m) ] | to_entries[]
	      | { number: .value.value.number,
	          frame: { x: ($v.x + $mw + 2 * $gap),
	                   y: ($v.y + $gap + .key * ($sh + $gap)),
	                   width: $sw,
	                   height: $sh } } ]
')"

printf '%s' "$frames" | mimi action apply_frames
