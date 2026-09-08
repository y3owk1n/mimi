package action

import (
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The two kinds of tiling command a user sends: a plain relayout, and a named
// command the layout program gives meaning to.
const (
	TilingRelayout = "relayout"
	TilingCommand  = "command"
)

// TilingArgs is the tiling action's typed payload: what kind of command it
// is, and for a named one, its name and arguments. The action itself lives
// in the daemon, which holds the layout's state; internal/action only
// carries it over the wire and checks it.
type TilingArgs struct {
	Kind string   `json:"kind"`
	Name string   `json:"name"`
	Args []string `json:"args"`
}

// NewTilingCommand builds the tiling action's command, rejecting a payload
// the daemon would refuse: a kind that is neither relayout nor command, or a
// command with no name.
func NewTilingCommand(kind, name string, args []string) (Command, error) {
	payload := TilingArgs{Kind: kind, Name: strings.TrimSpace(name), Args: args}

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
	case TilingRelayout:
		if args.Name != "" || len(args.Args) > 0 {
			return derrors.New(
				derrors.CodeInvalidInput,
				"a relayout takes no command name or arguments",
			)
		}
	case TilingCommand:
		if args.Name == "" {
			return derrors.New(derrors.CodeInvalidInput, "tiling command name is required")
		}
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown tiling command kind %q (supported: relayout, command)",
			args.Kind,
		)
	}

	return nil
}
