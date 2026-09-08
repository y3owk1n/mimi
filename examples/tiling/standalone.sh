#!/bin/sh
# Run a layout program once without the daemon: build its input from the
# queries, run it, apply what it prints. State is always null here; the
# daemon is what remembers state between passes.
#
# Usage: standalone.sh <layout> [args...]
#   standalone.sh ./columns.sh 12
set -eu

[ $# -ge 1 ] || { echo "usage: standalone.sh <layout> [args...]" >&2; exit 2; }

input="$(jq -c -n \
	--argjson w "$(mimi query windows)" \
	--argjson d "$(mimi query displays)" \
	--argjson s "$(mimi query space)" \
	'{version: 1, event: {kind: "relayout"}, space: $s.index,
	  displays: $d, focused: $w.focused, windows: $w.windows, state: null}')"

frames="$(printf '%s' "$input" | "$@" | jq -c '.frames // []')"

[ "$(printf '%s' "$frames" | jq 'length')" -gt 0 ] || exit 0

printf '%s' "$frames" | mimi action apply_frames
