package action

import (
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// percentageWhole is the largest percentage a size flag accepts.
const percentageWhole = 100.0

// Name identifies a supported action subcommand.
type Name string

// Supported CLI action names.
const (
	NameFocusWindow       Name = "focus_window"
	NameSpace             Name = "space"
	NameMoveWindowToSpace Name = "move_window_to_space"
	NameResizeWindow      Name = "resize_window"
)

// SpaceArg is the parsed form of the one argument space and
// move_window_to_space take: either an absolute 1-based Index, or a relative
// Direction (+1 for next, -1 for prev), never both.
type SpaceArg struct {
	Index     int `json:"index"`
	Direction int `json:"direction"` // +1 for next, -1 for prev; 0 means absolute index
}

// ParseSpaceArg is the only place the written form of a space argument is
// read — a 1-based number, "next", "prev", or "previous" — so every path that
// takes one from a user rejects a malformed argument in the same words.
//
// It answers "what does this string mean"; validateSpaceArg answers "does this
// value name a space", which is the question left once the string is gone and
// a SpaceArg arrives off the socket instead. This runs that check on what it
// parsed rather than restating it, so the two cannot come to disagree about
// which values are well-formed.
//
// name is the action the argument was given to, and appears in those words;
// it is the only part of them that differs between the two actions.
func ParseSpaceArg(name Name, args []string) (SpaceArg, error) {
	if len(args) != 1 {
		return SpaceArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s requires exactly one argument: a 1-based space number, \"next\", or \"prev\"",
			name,
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return SpaceArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s argument cannot be empty: give a 1-based space number, \"next\", or \"prev\"",
			name,
		)
	}

	arg, err := spaceArgOf(name, raw)
	if err != nil {
		return SpaceArg{}, err
	}

	// The value the spelling denotes still has to name a space. It always
	// does today, so this never fires — which is the point: it is what stops
	// a new spelling from being added here that the daemon would then reject
	// on a command the CLI happily built.
	err = validateSpaceArg(name, arg)
	if err != nil {
		return SpaceArg{}, err
	}

	return arg, nil
}

// spaceArgOf is the spelling half of ParseSpaceArg: which SpaceArg a written
// space argument denotes, with raw already trimmed and known non-empty.
func spaceArgOf(name Name, raw string) (SpaceArg, error) {
	switch raw {
	case "next":
		return SpaceArg{Direction: 1}, nil
	case "prev", "previous":
		return SpaceArg{Direction: -1}, nil
	}

	index, parseErr := strconv.Atoi(raw)
	if parseErr != nil || index < 1 {
		return SpaceArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s argument must be a positive integer, \"next\", or \"prev\", got %q",
			name,
			raw,
		)
	}

	return SpaceArg{Index: index}, nil
}

// validateSpaceArg rejects a SpaceArg that names no space.
//
// It is the typed half of the one space argument rule: ParseSpaceArg decides
// what a string means, and this decides whether the value is one the actions
// can act on. Both the parser and every space action run it, so a SpaceArg the
// CLI built and one the daemon decoded are held to the same rule.
//
// name is the action the argument was given to, and appears in the rejection.
func validateSpaceArg(name Name, arg SpaceArg) error {
	absolute := arg.Direction == 0 && arg.Index >= 1
	relative := (arg.Direction == 1 || arg.Direction == -1) && arg.Index == 0

	if absolute || relative {
		return nil
	}

	return derrors.Newf(
		derrors.CodeInvalidInput,
		"%s must name exactly one space: a 1-based space number, \"next\", or \"prev\"",
		name,
	)
}

// resolveSpaceArg turns a SpaceArg into the 1-based space number it names,
// rejecting one that names no space first.
//
// Only "next" and "prev" need the desktop: they are relative to whichever space
// is in front, and they wrap around at both ends.
func (e *Executor) resolveSpaceArg(name Name, parsed SpaceArg) (int, error) {
	err := validateSpaceArg(name, parsed)
	if err != nil {
		return 0, err
	}

	if parsed.Direction == 0 {
		return parsed.Index, nil
	}

	// Reading where the desktop is before acting on it is still driving it, so
	// the permission comes first here too — the action this feeds checks it
	// again, and a revoked permission has to be the error either way round.
	err = e.desktop.EnsureAccessible()
	if err != nil {
		return 0, err
	}

	current, err := e.desktop.ActiveSpaceIndex()
	if err != nil {
		return 0, err
	}

	count := e.desktop.SpaceCount()
	if count == 0 {
		return 0, derrors.New(derrors.CodeActionFailed, "no Mission Control spaces found")
	}

	return ((current - 1 + parsed.Direction + count) % count) + 1, nil
}

// ParseResizePreset maps the name resize_window's positional argument carries onto
// the preset it names, and is the one place a name that is not one of them is
// rejected. Every path that takes a preset goes through it — the CLI's own
// argument check and the conversion from a command's arguments, which the
// daemon runs again on a command it decoded — so an unknown name is rejected
// in the same words wherever it arrives from.
//
// The rejection lists the ten valid names, read from the geometry's own table
// rather than restated here, since mistyping one is the likely way to get
// here. The empty name is not a preset either: a command that names no preset
// never asks for one.
func ParseResizePreset(name string) (geometry.Preset, error) {
	preset, ok := geometry.ParsePreset(name)
	if !ok {
		return geometry.Preset{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown preset %q (valid: %s)",
			name,
			strings.Join(geometry.PresetNames(), ", "),
		)
	}

	return preset, nil
}

// dimensionOf folds one axis's two size flags into the dimension they
// describe. An absolute size wins over a percentage, and a zero of either
// means the flag was never given, so the window keeps the size it has.
func dimensionOf(points, percent float64) geometry.Dimension {
	switch {
	case points > 0:
		return geometry.Absolute(points)
	case percent > 0:
		return geometry.Percent(percent)
	default:
		return geometry.Keep()
	}
}
