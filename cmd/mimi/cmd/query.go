package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// newQueryCmd builds the command tree that reports desktop state.
//
// Queries take no cliState: they never route through the daemon, so the
// socket path is nothing they need. A running daemon serializes the actions
// it performs; a read has nothing to serialize with, and routing it over the
// socket would buy a wire change and nothing else.
func newQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Report desktop state as JSON",
		Long: `Report what the desktop looks like right now, as one line of JSON on stdout.

Queries only read. They never move focus, a window, or a space, and they run
in this process whether or not the daemon is running.

Available subcommands:
  space     the active Mission Control space and how many there are
  window    the frontmost window's owner and frame

Examples:
  mimi query space
  mimi query window
  mimi query space | jq .index`,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			// As for "mimi action": nothing ran, the user is missing a
			// subcommand, and usage is the only place the subcommands are
			// named.
			cobraCmd.SilenceUsage = false

			return derrors.New(
				derrors.CodeInvalidInput,
				"query subcommand required (e.g., mimi query space, mimi query window)",
			)
		},
	}

	cmd.AddCommand(buildQuerySpaceCommand())
	cmd.AddCommand(buildQueryWindowCommand())

	return cmd
}

func buildQuerySpaceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "space",
		Short: "Report the active Mission Control space and the space count",
		Long: `Report the active Mission Control space as JSON:

  {"index":2,"count":5}

"index" is the 1-based index of the space in front, in the same ordering
"mimi action space" takes, and "count" is how many spaces there are across
every connected display. Accessibility permission is not needed.`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			return answerQuery(cobraCmd, action.QuerySpace)
		},
	}
}

func buildQueryWindowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "window",
		Short: "Report the frontmost window's owner and frame",
		Long: `Report the frontmost window as JSON:

  {"pid":4242,"frame":{"x":100,"y":50,"width":1024,"height":768}}

"pid" is the process ID of the application that owns the window. The frame
is in window coordinates: the origin is the top-left corner of the primary
display and y grows downward, which is what "mimi action resize_window"
takes for --x and --y. Accessibility permission is required.`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			return answerQuery(cobraCmd, action.QueryWindow)
		},
	}
}

// answerQuery runs one query and prints its answer as a single line of JSON
// on the command's stdout. A query that fails prints nothing there: the error
// goes out the way every other command's does, so a script reading stdout
// never has to tell an answer from a complaint.
func answerQuery[T any](cobraCmd *cobra.Command, query func() (T, error)) error {
	answer, err := query()
	if err != nil {
		return err
	}

	err = json.NewEncoder(cobraCmd.OutOrStdout()).Encode(answer)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeSerializationFailed, "encoding query result")
	}

	return nil
}
