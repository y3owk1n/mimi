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
)

// IsKnownName reports whether name is a supported action.
func IsKnownName(name string) bool {
	switch Name(name) {
	case NameFocusWindow, NameSpace, NameMoveWindowToSpace:
		return true
	default:
		return false
	}
}

type parsedFocusWindowArgs struct {
	useBackward bool
}

func parseFocusWindowArgs(rawArgs []string) (parsedFocusWindowArgs, error) {
	var parsed parsedFocusWindowArgs

	for _, arg := range rawArgs {
		switch arg {
		case "--backward":
			parsed.useBackward = true
		default:
			if strings.HasPrefix(arg, "--") {
				return parsed, derrors.New(
					derrors.CodeInvalidInput,
					"invalid or missing flag value",
				)
			}
		}
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

// Execute runs a named action with the given arguments.
func Execute(name string, args []string) error {
	switch Name(name) {
	case NameFocusWindow:
		parsed, err := parseFocusWindowArgs(args)
		if err != nil {
			return err
		}

		return FocusWindow(parsed.useBackward)
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
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown action %q (supported: focus_window, space, move_window_to_space)",
			name,
		)
	}
}
