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

// IsKnownName reports whether name is a supported action.
func IsKnownName(name string) bool {
	switch Name(name) {
	case NameFocusWindow, NameSpace, NameMoveWindowToSpace, NameResizeWindow:
		return true
	default:
		return false
	}
}

func parseFocusWindowArgs(rawArgs []string) (FocusWindowArgs, error) {
	var parsed FocusWindowArgs

	for _, arg := range rawArgs {
		switch arg {
		case "--backward":
			parsed.Backward = true
		case "--up", "--down", "--left", "--right":
			if parsed.Direction != "" {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"only one direction flag allowed (--up, --down, --left, --right)",
				)
			}

			parsed.Direction = arg[2:]
		default:
			if strings.HasPrefix(arg, "--") {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"invalid or missing flag value",
				)
			}
		}
	}

	err := validateFocusWindowCombo(parsed.Direction, parsed.Backward)
	if err != nil {
		return parsed, err
	}

	return parsed, nil
}

// SpaceArg is the parsed form of the one argument space and
// move_window_to_space take: either an absolute 1-based Index, or a relative
// Direction (+1 for next, -1 for prev), never both.
type SpaceArg struct {
	Index     int
	Direction int // +1 for next, -1 for prev; 0 means absolute index
}

// ParseSpaceArg parses the one argument the space actions take. It is the
// only place the rule lives — a space argument is a 1-based number, "next",
// "prev", or "previous" — so every path that takes one, direct or daemon,
// rejects a malformed argument in the same words.
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

// resolveSpaceArg parses the one argument the space actions take and turns it
// into the 1-based space number it names.
func (e *Executor) resolveSpaceArg(name Name, args []string) (int, error) {
	parsed, err := ParseSpaceArg(name, args)
	if err != nil {
		return 0, err
	}

	return e.resolveSpaceArgTyped(parsed)
}

// resolveSpaceArgTyped turns an already-parsed SpaceArg into the 1-based
// space number it names.
//
// Only "next" and "prev" need the desktop: they are relative to whichever space
// is in front, and they wrap around at both ends.
func (e *Executor) resolveSpaceArgTyped(parsed SpaceArg) (int, error) {
	if parsed.Direction == 0 {
		return parsed.Index, nil
	}

	// Reading where the desktop is before acting on it is still driving it, so
	// the permission comes first here too — the action this feeds checks it
	// again, and a revoked permission has to be the error either way round.
	err := e.desktop.EnsureAccessible()
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
// rejected. Every path that takes a preset — the CLI's own argument check, the
// conversion from a command's arguments, and the string parser the daemon
// still decodes with — goes through it, so an unknown name is rejected in the
// same words wherever it arrives from.
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

// ParseResizeRequest turns resize_window's command line into the geometry
// request it describes.
//
// The optional fields are pointers because their absence is a real input to
// the geometry — an anchor nobody gave is what lets a preset supply one, and a
// margin preference nobody expressed is what defers to the system setting.
//
// A zero --width or --width-percent keeps the window's current size rather
// than collapsing it, which is the convention the CLI has always followed.
func ParseResizeRequest(rawArgs []string) (geometry.Request, error) {
	var (
		req                         geometry.Request
		width, height               float64
		widthPercent, heightPercent float64
		posX, posY                  float64
		hasPosX, hasPosY            bool
	)

	args := rawArgs

	// The preset is the one positional argument resize_window takes, so
	// anything leading that is not a flag is meant as one. A name that is not
	// a preset is rejected here, on the rule the CLI rejects it with, rather
	// than falling through to the flag loop to be reported as a stray
	// argument.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		preset, err := ParseResizePreset(args[0])
		if err != nil {
			return req, err
		}

		req.Preset = preset
		args = args[1:]
	}

	// Parse --flags
	for argIndex := 0; argIndex < len(args); argIndex++ {
		arg := args[argIndex]

		switch arg {
		case "--width", "-w":
			if argIndex+1 >= len(args) {
				return req, derrors.New(derrors.CodeInvalidInput, "--width requires a value")
			}

			points, err := strconv.Atoi(args[argIndex+1])
			if err != nil || points < 0 {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid width: %q",
					args[argIndex+1],
				)
			}

			width = float64(points)
			argIndex++
		case "--height", "-h":
			if argIndex+1 >= len(args) {
				return req, derrors.New(derrors.CodeInvalidInput, "--height requires a value")
			}

			points, err := strconv.Atoi(args[argIndex+1])
			if err != nil || points < 0 {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid height: %q",
					args[argIndex+1],
				)
			}

			height = float64(points)
			argIndex++
		case "--width-percent":
			if argIndex+1 >= len(args) {
				return req, derrors.New(
					derrors.CodeInvalidInput,
					"--width-percent requires a value",
				)
			}

			share, err := strconv.ParseFloat(args[argIndex+1], 64)
			if err != nil || share < 0 || share > percentageWhole {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid width-percent: %q (0-100)",
					args[argIndex+1],
				)
			}

			widthPercent = share
			argIndex++
		case "--height-percent":
			if argIndex+1 >= len(args) {
				return req, derrors.New(
					derrors.CodeInvalidInput,
					"--height-percent requires a value",
				)
			}

			share, err := strconv.ParseFloat(args[argIndex+1], 64)
			if err != nil || share < 0 || share > percentageWhole {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid height-percent: %q (0-100)",
					args[argIndex+1],
				)
			}

			heightPercent = share
			argIndex++
		case "--x":
			if argIndex+1 >= len(args) {
				return req, derrors.New(derrors.CodeInvalidInput, "--x requires a value")
			}

			coord, err := strconv.Atoi(args[argIndex+1])
			if err != nil {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid x: %q",
					args[argIndex+1],
				)
			}

			posX, hasPosX = float64(coord), true
			argIndex++
		case "--y":
			if argIndex+1 >= len(args) {
				return req, derrors.New(derrors.CodeInvalidInput, "--y requires a value")
			}

			coord, err := strconv.Atoi(args[argIndex+1])
			if err != nil {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid y: %q",
					args[argIndex+1],
				)
			}

			posY, hasPosY = float64(coord), true
			argIndex++
		case "--anchor", "-a":
			if argIndex+1 >= len(args) {
				return req, derrors.New(derrors.CodeInvalidInput, "--anchor requires a value")
			}

			anchor, ok := geometry.ParseAnchor(args[argIndex+1])
			if !ok {
				return req, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid anchor: %q (use tl, tc, tr, cl, cc, cr, bl, bc, br)",
					args[argIndex+1],
				)
			}

			req.Anchor = &anchor
			argIndex++
		case "--margin":
			useMargins := true
			req.UseMargins = &useMargins
		case "--no-margin":
			useMargins := false
			req.UseMargins = &useMargins
		default:
			if strings.HasPrefix(arg, "--") {
				return req, derrors.Newf(derrors.CodeInvalidInput, "unknown flag: %s", arg)
			}

			return req, derrors.Newf(derrors.CodeInvalidInput, "unexpected argument: %s", arg)
		}
	}

	req.Width = dimensionOf(width, widthPercent)
	req.Height = dimensionOf(height, heightPercent)

	if hasPosX {
		req.X = &posX
	}

	if hasPosY {
		req.Y = &posY
	}

	return req, nil
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

// Execute runs a named action with the given arguments against the desktop
// mimi is running on.
func Execute(name string, args []string) error {
	return defaultExecutor.Execute(name, args)
}

// Execute runs a named action with the given arguments.
func (e *Executor) Execute(name string, args []string) error {
	switch Name(name) {
	case NameFocusWindow:
		parsed, err := parseFocusWindowArgs(args)
		if err != nil {
			return err
		}

		return e.FocusWindow(parsed.Backward, parsed.Direction)
	case NameSpace:
		index, err := e.resolveSpaceArg(NameSpace, args)
		if err != nil {
			return err
		}

		return e.FocusSpace(index)
	case NameMoveWindowToSpace:
		index, err := e.resolveSpaceArg(NameMoveWindowToSpace, args)
		if err != nil {
			return err
		}

		return e.MoveWindowToSpace(index)
	case NameResizeWindow:
		req, err := ParseResizeRequest(args)
		if err != nil {
			return err
		}

		return e.ResizeWindow(req)
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown action %q (supported: focus_window, space, move_window_to_space, resize_window)",
			name,
		)
	}
}
