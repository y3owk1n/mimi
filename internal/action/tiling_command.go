package action

import (
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The kinds of tiling command a user sends: a plain relayout, a named command
// the layout program gives meaning to, a read of the state the engine holds,
// and a clearing of it.
const (
	TilingRelayout = "relayout"
	TilingCommand  = "command"
	TilingState    = "state"
	TilingReset    = "reset"
)

// TilingArgs is the tiling action's typed payload: what kind of command it
// is, and for a named one, its name and arguments. The action itself lives
// in the daemon, which holds the layout's state; internal/action only
// carries it over the wire and checks it.
type TilingArgs struct {
	Kind string   `json:"kind"`
	Name string   `json:"name"`
	Args []string `json:"args"`
	// All widens a reset from the space in front on each display to every
	// space the engine remembers. It means nothing to the other kinds.
	All bool `json:"all"`
}

// NewTilingCommand builds the tiling action's command, rejecting a payload
// the daemon would refuse: a kind that is neither relayout nor command, or a
// command with no name.
func NewTilingCommand(kind, name string, args []string) (Command, error) {
	return newTilingCommand(TilingArgs{Kind: kind, Name: strings.TrimSpace(name), Args: args})
}

// NewTilingResetCommand builds the tiling action's command for a reset, which
// is the one kind with a flag of its own.
func NewTilingResetCommand(all bool) (Command, error) {
	return newTilingCommand(TilingArgs{Kind: TilingReset, All: all})
}

func newTilingCommand(payload TilingArgs) (Command, error) {
	err := validateTilingArgs(payload)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameTiling, Tiling: payload}, nil
}

// validateTilingArgs is the one rule a tiling payload is held to, on both
// paths.
func validateTilingArgs(args TilingArgs) error {
	switch args.Kind {
	case TilingRelayout, TilingState, TilingReset:
		if args.Name != "" || len(args.Args) > 0 {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"a %s takes no command name or arguments",
				args.Kind,
			)
		}

		if args.All && args.Kind != TilingReset {
			return derrors.Newf(derrors.CodeInvalidInput, "a %s is never for all spaces", args.Kind)
		}
	case TilingCommand:
		if args.Name == "" {
			return derrors.New(derrors.CodeInvalidInput, "tiling command name is required")
		}

		if args.All {
			return derrors.New(derrors.CodeInvalidInput, "a command is never for all spaces")
		}
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown tiling command kind %q (supported: relayout, command, state, reset)",
			args.Kind,
		)
	}

	return nil
}
