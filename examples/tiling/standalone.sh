#!/bin/sh
# Run a layout program once without the daemon, the way the daemon would:
# once per display that has a window, with that display's windows, then
# apply everything it printed in one go. State is always null here; the
# daemon is what remembers state between runs, and the gap is the macOS
# tiled-window margin, since no config is read here.
#
# Usage: standalone.sh <layout> [args...]
#   standalone.sh ./columns.py 12
set -eu

[ $# -ge 1 ] || { echo "usage: standalone.sh <layout> [args...]" >&2; exit 2; }

windows="$(mimi query windows)"
displays="$(mimi query displays)"
space="$(mimi query space | jq '.index')"
margins="$(mimi query margins)"

# One input per display: the windows whose centres are on it, with
# "focused" re-pointed, or -1 when the focused window is elsewhere.
inputs="$(jq -c -n \
	--argjson w "$windows" \
	--argjson d "$displays" \
	--argjson s "$space" \
	--argjson m "$margins" '
	def on($disp): (.frame.x + .frame.width / 2) as $cx | (.frame.y + .frame.height / 2) as $cy
	  | $disp.frame | ($cx >= .x and $cx < .x + .width and $cy >= .y and $cy < .y + .height);
	(if $w.focused >= 0 then $w.windows[$w.focused].number else null end) as $focused
	| $d[] as $disp
	| [$w.windows[] | select(on($disp))] as $mine
	| select(($mine | length) > 0)
	| {version: 1, event: {kind: "relayout"}, display: $disp, space: $s,
	   gap: (if $m.enabled then $m.size else 0 end),
	   displays: $d, windows: $mine,
	   focused: (($mine | map(.number) | index($focused)) // -1), state: null}
')"

frames="$(printf '%s\n' "$inputs" | while IFS= read -r input; do
	printf '%s' "$input" | "$@" | jq -c '.frames // []'
done | jq -c -s 'add // []')"

[ "$(printf '%s' "$frames" | jq 'length')" -gt 0 ] || exit 0

printf '%s' "$frames" | mimi action apply_frames
