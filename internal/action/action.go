package action

import (
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
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

func parseIndexArg(args []string, actionName string) (int, error) {
	if len(args) != 1 {
		return 0, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s requires exactly one positional argument: the 1-based space number",
			actionName,
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return 0, derrors.New(derrors.CodeInvalidInput, "space number cannot be empty")
	}

	index, parseErr := strconv.Atoi(raw)
	if parseErr != nil || index < 1 {
		return 0, derrors.Newf(
			derrors.CodeInvalidInput,
			"space number must be a positive integer, got %s",
			raw,
		)
	}

	return index, nil
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
		index, err := parseIndexArg(args, string(NameSpace))
		if err != nil {
			return err
		}

		return FocusSpace(index)
	case NameMoveWindowToSpace:
		index, err := parseIndexArg(args, string(NameMoveWindowToSpace))
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
