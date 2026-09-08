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

	NameMoveWindowToDisplay Name = "move_window_to_display"
	NameFocusApp            Name = "focus_app"
	NameApplyFrames         Name = "apply_frames"
)

// Nouns the index-argument rule reports in: what a space argument and a
// display argument each name.
const (
	spaceNoun   = "space"
	displayNoun = "display"
)

// SpaceArg is the parsed form of the one argument space and
// move_window_to_space take: either an absolute 1-based Index, or a relative
// Direction (+1 for next, -1 for prev), never both.
type SpaceArg struct {
	Index     int `json:"index"`
	Direction int `json:"direction"` // +1 for next, -1 for prev; 0 means absolute index
}

// DisplayArg is the parsed form of the one argument move_window_to_display
// takes: either an absolute 1-based Index, counted left to right, or a
// relative Direction (+1 for next, -1 for prev), never both. It is SpaceArg's
// shape under another noun, and held to the same rule.
type DisplayArg struct {
	Index     int `json:"index"`
	Direction int `json:"direction"` // +1 for next, -1 for prev; 0 means absolute index
}

// indexArg is the shape a space argument and a display argument share, which
// is what lets one rule read and check both.
type indexArg struct {
	index     int
	direction int
}

// ParseSpaceArg is the only place the written form of a space argument is
// read — a 1-based number, "next", "prev", or "previous" — so every path that
// takes one from a user rejects a malformed argument in the same words.
//
// It answers "what does this string mean"; validateSpaceArg answers "does this
// value name a space", which is the question left once the string is gone and
// a SpaceArg arrives off the socket instead. The rule itself is parseIndexArg,
// shared with the display argument, so the two cannot come to disagree about
// which values are well-formed.
//
// name is the action the argument was given to, and appears in those words;
// it is the only part of them that differs between the two space actions.
func ParseSpaceArg(name Name, args []string) (SpaceArg, error) {
	arg, err := parseIndexArg(name, spaceNoun, args)
	if err != nil {
		return SpaceArg{}, err
	}

	return SpaceArg{Index: arg.index, Direction: arg.direction}, nil
}

// ParseDisplayArg is ParseSpaceArg for the display argument: the same
// spellings, reported against move_window_to_display and the word "display".
func ParseDisplayArg(args []string) (DisplayArg, error) {
	arg, err := parseIndexArg(NameMoveWindowToDisplay, displayNoun, args)
	if err != nil {
		return DisplayArg{}, err
	}

	return DisplayArg{Index: arg.index, Direction: arg.direction}, nil
}

// parseIndexArg reads the one written form both index arguments take, and is
// the only place that form is read. noun is what the argument names, "space"
// or "display", and appears in the rejection alongside the action's name.
func parseIndexArg(name Name, noun string, args []string) (indexArg, error) {
	if len(args) != 1 {
		return indexArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s requires exactly one argument: a 1-based %s number, \"next\", or \"prev\"",
			name,
			noun,
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return indexArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s argument cannot be empty: give a 1-based %s number, \"next\", or \"prev\"",
			name,
			noun,
		)
	}

	arg, err := indexArgOf(name, raw)
	if err != nil {
		return indexArg{}, err
	}

	// The value the spelling denotes still has to name something. It always
	// does today, so this never fires — which is the point: it is what stops
	// a new spelling from being added here that the daemon would then reject
	// on a command the CLI happily built.
	err = validateIndexArg(name, noun, arg)
	if err != nil {
		return indexArg{}, err
	}

	return arg, nil
}

// indexArgOf is the spelling half of parseIndexArg: which value a written
// argument denotes, with raw already trimmed and known non-empty.
func indexArgOf(name Name, raw string) (indexArg, error) {
	switch raw {
	case "next":
		return indexArg{direction: 1}, nil
	case "prev", "previous":
		return indexArg{direction: -1}, nil
	}

	index, parseErr := strconv.Atoi(raw)
	if parseErr != nil || index < 1 {
		return indexArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s argument must be a positive integer, \"next\", or \"prev\", got %q",
			name,
			raw,
		)
	}

	return indexArg{index: index}, nil
}

// validateSpaceArg rejects a SpaceArg that names no space.
//
// It is the typed half of the one index argument rule: ParseSpaceArg decides
// what a string means, and this decides whether the value is one the actions
// can act on. Both the parser and every space action run it, so a SpaceArg the
// CLI built and one the daemon decoded are held to the same rule.
//
// name is the action the argument was given to, and appears in the rejection.
func validateSpaceArg(name Name, arg SpaceArg) error {
	return validateIndexArg(name, spaceNoun, indexArg{index: arg.Index, direction: arg.Direction})
}

// validateDisplayArg is validateSpaceArg for the display argument.
func validateDisplayArg(arg DisplayArg) error {
	return validateIndexArg(
		NameMoveWindowToDisplay,
		displayNoun,
		indexArg{index: arg.Index, direction: arg.Direction},
	)
}

// validateIndexArg rejects a value that names nothing: neither an absolute
// index nor a single step in one direction.
func validateIndexArg(name Name, noun string, arg indexArg) error {
	absolute := arg.direction == 0 && arg.index >= 1
	relative := (arg.direction == 1 || arg.direction == -1) && arg.index == 0

	if absolute || relative {
		return nil
	}

	return derrors.Newf(
		derrors.CodeInvalidInput,
		"%s must name exactly one %s: a 1-based %s number, \"next\", or \"prev\"",
		name,
		noun,
		noun,
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
// Surrounding whitespace is not part of the name and is trimmed here, the way
// ParseSpaceArg trims a space argument, so " left-half " names the same preset
// wherever it arrives from — a shell that padded it, a hook that built the
// argument, or a command the daemon decoded off the socket. The CLI used to
// trim instead, which is what made a padded name work on the direct path and
// be rejected on every other one (mimi#132); this is now the only place it
// happens.
//
// The rejection lists the fifteen valid names, read from the geometry's own table
// rather than restated here, since mistyping one is the likely way to get
// here, and quotes the name as it was given rather than as it was trimmed, so
// padding the user did type is visible in it. A name that is empty, or is
// nothing but whitespace, is not a preset either: a command that names no
// preset never asks for one.
func ParseResizePreset(name string) (geometry.Preset, error) {
	preset, ok := geometry.ParsePreset(strings.TrimSpace(name))
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

// ParseResizePresetArg parses resize_window's one positional argument into the
// preset it names, and is the only implementation of that argument's rule.
//
// The empty string is the argument nobody gave — resize_window's positional
// argument is optional, and a command that names no preset asks for none, so
// the zero preset comes back with no error. Anything else is a name,
// whitespace included, and what it names is ParseResizePreset's decision
// alone.
//
// Both layers that reject a bad preset call this rather than restating it: the
// CLI's Args layer, which stops the argument before the command body runs, and
// ResizeRequestFromArgs, which checks it again for a command that arrived over
// the daemon path with nothing having looked at it.
func ParseResizePresetArg(name string) (geometry.Preset, error) {
	if name == "" {
		return geometry.Preset{}, nil
	}

	return ParseResizePreset(name)
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
