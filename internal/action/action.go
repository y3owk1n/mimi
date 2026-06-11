package action

import (
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/space"
)

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

type parsedFocusWindowArgs struct {
	useBackward bool
	direction   string // "up", "down", "left", "right", or ""
}

func parseFocusWindowArgs(rawArgs []string) (parsedFocusWindowArgs, error) {
	var parsed parsedFocusWindowArgs

	for _, arg := range rawArgs {
		switch arg {
		case "--backward":
			parsed.useBackward = true
		case "--up", "--down", "--left", "--right":
			if parsed.direction != "" {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"only one direction flag allowed (--up, --down, --left, --right)",
				)
			}

			parsed.direction = arg[2:]
		default:
			if strings.HasPrefix(arg, "--") {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"invalid or missing flag value",
				)
			}
		}
	}

	if parsed.direction != "" && parsed.useBackward {
		return parsed, derrors.New(
			derrors.CodeInvalidInput,
			"--backward cannot be combined with a direction flag",
		)
	}

	return parsed, nil
}

type spaceArg struct {
	index     int
	direction int // +1 for next, -1 for prev; 0 means absolute index
}

func parseSpaceArg(args []string) (spaceArg, error) {
	if len(args) != 1 {
		return spaceArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"space requires exactly one argument: a 1-based number, \"next\", or \"prev\"",
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return spaceArg{}, derrors.New(derrors.CodeInvalidInput, "space argument cannot be empty")
	}

	switch raw {
	case "next":
		return spaceArg{direction: 1}, nil
	case "prev", "previous":
		return spaceArg{direction: -1}, nil
	}

	index, parseErr := strconv.Atoi(raw)
	if parseErr != nil || index < 1 {
		return spaceArg{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"space must be a positive integer, \"next\", or \"prev\", got %q",
			raw,
		)
	}

	return spaceArg{index: index}, nil
}

func (s spaceArg) resolve() (int, error) {
	if s.direction != 0 {
		current, err := space.ActiveIndex()
		if err != nil {
			return 0, err
		}

		count := space.Count()
		if count == 0 {
			return 0, derrors.New(derrors.CodeActionFailed, "no Mission Control spaces found")
		}

		return ((current - 1 + s.direction + count) % count) + 1, nil
	}

	return s.index, nil
}

// parsedResizeWindowArgs holds parsed flags for the resize_window action.
type parsedResizeWindowArgs struct {
	preset    string
	width     int
	height    int
	widthPct  float64
	heightPct float64
	x         int
	y         int
	anchor    string
	hasX      bool
	hasY      bool
	useMargin *bool // nil = system default
}

// validAnchors for window positioning.
var validAnchors = map[string]bool{
	"tl": true, "tc": true, "tr": true,
	"cl": true, "cc": true, "cr": true,
	"bl": true, "bc": true, "br": true,
}

// resizePresets maps preset names to dimension percentages and anchor.
var resizePresets = map[string]struct {
	widthPct  float64
	heightPct float64
	anchor    string
}{
	"left-half":    {50, 100, "tl"},
	"right-half":   {50, 100, "tr"},
	"top-half":     {100, 50, "tl"},
	"bottom-half":  {100, 50, "bl"},
	"top-left":     {50, 50, "tl"},
	"top-right":    {50, 50, "tr"},
	"bottom-left":  {50, 50, "bl"},
	"bottom-right": {50, 50, "br"},
	"center":       {60, 80, "cc"},
	"fill":         {100, 100, "tl"},
}

// IsResizePreset reports whether s is a known resize window preset.
func IsResizePreset(s string) bool {
	_, ok := resizePresets[s]

	return ok
}

func parseResizeWindowArgs(rawArgs []string) (parsedResizeWindowArgs, error) {
	var parsed parsedResizeWindowArgs

	parsed.anchor = "cc"

	args := rawArgs

	// Check for preset as first positional arg
	if len(args) > 0 && IsResizePreset(args[0]) {
		parsed.preset = args[0]
		args = args[1:]
	}

	// Parse --flags
	for argIndex := 0; argIndex < len(args); argIndex++ {
		arg := args[argIndex]

		switch arg {
		case "--width", "-w":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(derrors.CodeInvalidInput, "--width requires a value")
			}

			width, err := strconv.Atoi(args[argIndex+1])
			if err != nil || width < 0 {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid width: %q",
					args[argIndex+1],
				)
			}

			parsed.width = width
			argIndex++
		case "--height", "-h":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(derrors.CodeInvalidInput, "--height requires a value")
			}

			height, err := strconv.Atoi(args[argIndex+1])
			if err != nil || height < 0 {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid height: %q",
					args[argIndex+1],
				)
			}

			parsed.height = height
			argIndex++
		case "--width-percent":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"--width-percent requires a value",
				)
			}

			widthPercent, err := strconv.ParseFloat(args[argIndex+1], 64)
			if err != nil || widthPercent < 0 || widthPercent > 100 {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid width-percent: %q (0-100)",
					args[argIndex+1],
				)
			}

			parsed.widthPct = widthPercent
			argIndex++
		case "--height-percent":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"--height-percent requires a value",
				)
			}

			heightPercent, err := strconv.ParseFloat(args[argIndex+1], 64)
			if err != nil || heightPercent < 0 || heightPercent > 100 {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid height-percent: %q (0-100)",
					args[argIndex+1],
				)
			}

			parsed.heightPct = heightPercent
			argIndex++
		case "--x":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(derrors.CodeInvalidInput, "--x requires a value")
			}

			xCoord, err := strconv.Atoi(args[argIndex+1])
			if err != nil {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid x: %q",
					args[argIndex+1],
				)
			}

			parsed.x = xCoord
			parsed.hasX = true
			argIndex++
		case "--y":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(derrors.CodeInvalidInput, "--y requires a value")
			}

			yCoord, err := strconv.Atoi(args[argIndex+1])
			if err != nil {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid y: %q",
					args[argIndex+1],
				)
			}

			parsed.y = yCoord
			parsed.hasY = true
			argIndex++
		case "--anchor", "-a":
			if argIndex+1 >= len(args) {
				return parsed, derrors.New(derrors.CodeInvalidInput, "--anchor requires a value")
			}

			anchorVal := args[argIndex+1]
			if !validAnchors[anchorVal] {
				return parsed, derrors.Newf(
					derrors.CodeInvalidInput,
					"invalid anchor: %q (use tl, tc, tr, cl, cc, cr, bl, bc, br)",
					anchorVal,
				)
			}

			parsed.anchor = anchorVal
			argIndex++
		case "--margin":
			val := true
			parsed.useMargin = &val
		case "--no-margin":
			val := false
			parsed.useMargin = &val
		default:
			if strings.HasPrefix(arg, "--") {
				return parsed, derrors.Newf(derrors.CodeInvalidInput, "unknown flag: %s", arg)
			}

			return parsed, derrors.Newf(derrors.CodeInvalidInput, "unexpected argument: %s", arg)
		}
	}

	// If preset given, apply its values as defaults (can be overridden by explicit flags)
	if parsed.preset != "" {
		preset, ok := resizePresets[parsed.preset]
		if ok {
			parsed.anchor = preset.anchor
			if parsed.widthPct == 0 && parsed.width == 0 {
				parsed.widthPct = preset.widthPct
			}

			if parsed.heightPct == 0 && parsed.height == 0 {
				parsed.heightPct = preset.heightPct
			}
		}
	}

	return parsed, nil
}

// Execute runs a named action with the given arguments.
func Execute(name string, args []string) error {
	switch Name(name) {
	case NameFocusWindow:
		parsed, err := parseFocusWindowArgs(args)
		if err != nil {
			return err
		}

		return FocusWindow(parsed.useBackward, parsed.direction)
	case NameSpace:
		parsed, err := parseSpaceArg(args)
		if err != nil {
			return err
		}

		index, err := parsed.resolve()
		if err != nil {
			return err
		}

		return FocusSpace(index)
	case NameMoveWindowToSpace:
		parsed, err := parseSpaceArg(args)
		if err != nil {
			return err
		}

		index, err := parsed.resolve()
		if err != nil {
			return err
		}

		return MoveWindowToSpace(index)
	case NameResizeWindow:
		parsed, err := parseResizeWindowArgs(args)
		if err != nil {
			return err
		}

		return ResizeWindow(parsed)
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown action %q (supported: focus_window, space, move_window_to_space, resize_window)",
			name,
		)
	}
}
