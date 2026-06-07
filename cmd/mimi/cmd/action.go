package cmd

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// actionCmd performs immediate window and space utility actions.
var actionCmd = &cobra.Command{
	Use:   "action",
	Short: "Perform window and space utility actions",
	Long: `Perform immediate window and space utility actions.

Available subcommands:
  Window control:   focus_window
  Space control:    space, move_window_to_space

Examples:
  mimi action focus_window
  mimi action focus_window --backward
  mimi action space 1
  mimi action move_window_to_space 2`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return derrors.New(
			derrors.CodeInvalidInput,
			"action subcommand required (e.g., mimi action focus_window, mimi action space 1)",
		)
	},
}

var (
	actionFocusWindowCmd       = buildFocusWindowCommand()
	actionSpaceCmd             = buildSpaceCommand()
	actionMoveWindowToSpaceCmd = buildMoveWindowToSpaceCommand()
)

func buildFocusWindowCommand() *cobra.Command {
	var backward bool

	cmd := &cobra.Command{
		Use:   "focus_window",
		Short: "Cycle focus through windows on the active space",
		Long: `Cycle keyboard focus through all focusable windows on the current space.

Cycles forward through windows (or backward with --backward), wrapping at the
end. Only windows that are focusable (not minimized, not hidden) and on the
current space are included.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			args := []string{}
			if backward {
				args = append(args, "--backward")
			}

			return action.Execute(string(action.NameFocusWindow), args)
		},
	}

	cmd.Flags().
		BoolVar(&backward, "backward", false, "Cycle to the previous window instead of the next one")

	return cmd
}

func buildSpaceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "space <number>",
		Short: "Focus a Mission Control space by 1-based index",
		Long: `Focus a Mission Control space by its 1-based index.

Spaces are enumerated in Mission Control ordering across all connected
displays. Index 1 is the first space (typically the leftmost on the
primary display), index 2 the second, and so on.

macOS does not provide a public API to activate a space, so mimi
synthesizes a high-velocity horizontal dock swipe gesture to fast-forward
to the destination space without the standard swipe animation. When the
destination sits on a different display, the cursor is warped to its
center first so the gesture is attributed to the correct screen.

Examples:
  mimi action space 1     Focus the first Mission Control space
  mimi action space 3     Focus the third`,
		Args: validateActionSpaceArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return action.Execute(string(action.NameSpace), []string{strings.TrimSpace(args[0])})
		},
	}
}

func buildMoveWindowToSpaceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "move_window_to_space <number>",
		Short: "Move current focused window to a Mission Control space by 1-based index",
		Long: `Move the currently focused window to a Mission Control space by its 1-based index.

Spaces are enumerated in Mission Control ordering across all connected
displays. Index 1 is the first space, index 2 the second, and so on.

This command uses private APIs (SkyLight) to move the window instantly
without scripting additions or disabling SIP on macOS.

Examples:
  mimi action move_window_to_space 2     Move current window to space 2
  mimi action move_window_to_space 4     Move current window to space 4`,
		Args: validateActionMoveWindowToSpaceArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return action.Execute(
				string(action.NameMoveWindowToSpace),
				[]string{strings.TrimSpace(args[0])},
			)
		},
	}
}

func validateActionSpaceArgs(_ *cobra.Command, args []string) error {
	return validateActionIndexArgs(args, "space")
}

func validateActionMoveWindowToSpaceArgs(_ *cobra.Command, args []string) error {
	return validateActionIndexArgs(args, "move_window_to_space")
}

func validateActionIndexArgs(args []string, actionName string) error {
	if len(args) != 1 {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"%s requires exactly one positional argument: the 1-based space number (e.g., mimi action %s 1)",
			actionName,
			actionName,
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return derrors.New(derrors.CodeInvalidInput, "space number cannot be empty")
	}

	index, parseErr := strconv.Atoi(raw)
	if parseErr != nil || index < 1 {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number must be a positive integer, got %q",
			args[0],
		)
	}

	return nil
}

func init() {
	actionCmd.AddCommand(actionFocusWindowCmd)
	actionCmd.AddCommand(actionSpaceCmd)
	actionCmd.AddCommand(actionMoveWindowToSpaceCmd)
}
