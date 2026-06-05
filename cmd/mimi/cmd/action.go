package cmd

import (
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/actions"
)

var actionFocusWindowBackward bool

// actionCmd is the parent command for direct (non-event-driven)
// mimi actions. Subcommands invoke individual handlers
// implemented in the internal/actions package.
var actionCmd = &cobra.Command{
	Use:   "action",
	Short: "Perform an immediate mimi action",
	Long:  "Perform an immediate mimi action without going through a running daemon.\n\nEach subcommand runs the requested action synchronously and exits.\nFrom a hook shell command, invoke these as ordinary mimi\nsubcommands, e.g.:\n\n  on_app_activate = ['mimi action focus_window']\n  on_app_activate = ['mimi action focus_window --backward']\n  on_workspace_changed = ['mimi action space 1']\n  on_workspace_changed = ['mimi action move_window_to_space 2']\n\nAvailable subcommands:\n  focus_window               Cycle focus through windows on the active space\n  <n>                  Focus Mission Control space at index N (1-based)\n  move_window_to_space <n>   Move the frontmost window to Mission Control space at index N",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var actionFocusWindowCmd = &cobra.Command{
	Use:   actions.NameFocusWindow,
	Short: "Cycle focus through windows on the active space",
	Long: `Cycle keyboard focus through all focusable windows on the current space.

Cycles forward by default; pass --backward to cycle in the opposite
direction. Both directions wrap at the end of the list. Only windows
that are focusable (not minimized, not hidden) and on the current
space are included.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		ensureCocoaInitialized()

		err := actions.FocusWindow(actionFocusWindowBackward)
		if err != nil {
			return err
		}

		RootCmd.Printf("%s performed\n", actions.NameFocusWindow)

		return nil
	},
}

var actionSpaceCmd = &cobra.Command{
	Use:   actions.NameSpace + " <number>",
	Short: "Focus a Mission Control space by 1-based index",
	Long: `Focus a Mission Control space by its 1-based index.

Spaces are enumerated in Mission Control ordering across all
connected displays. Index 1 is the first space (typically the
leftmost on the primary display), index 2 the second, and so on.

macOS does not expose a public API to activate a space, so the
command synthesizes a high-velocity horizontal dock swipe
gesture to fast-forward to the destination space without the
standard swipe animation. When the destination sits on a
different display, the cursor is warped to its center first so
the gesture is attributed to the correct screen.

Examples:
  mimi action space 1
  mimi action space 3`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ensureCocoaInitialized()

		index, err := actions.ParseIndexArg(args, actions.NameSpace)
		if err != nil {
			return err
		}

		err = actions.Space(index)
		if err != nil {
			return err
		}

		RootCmd.Printf("%s performed (index %d)\n", actions.NameSpace, index)

		return nil
	},
}

var actionMoveWindowToSpaceCmd = &cobra.Command{
	Use:   actions.NameMoveWindowToSpace + " <number>",
	Short: "Move the frontmost window to a Mission Control space by 1-based index",
	Long: `Move the currently focused window to a Mission Control space by its
1-based index.

Spaces are enumerated in Mission Control ordering across all
connected displays. Index 1 is the first space, index 2 the
second, and so on. The move uses private SkyLight APIs and
completes asynchronously; the command returns once the move has
been dispatched.

Examples:
  mimi action move_window_to_space 2
  mimi action move_window_to_space 4`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ensureCocoaInitialized()

		index, err := actions.ParseIndexArg(args, actions.NameMoveWindowToSpace)
		if err != nil {
			return err
		}

		err = actions.MoveWindowToSpace(index)
		if err != nil {
			return err
		}

		RootCmd.Printf("%s performed (index %d)\n", actions.NameMoveWindowToSpace, index)

		return nil
	},
}

func init() {
	actionFocusWindowCmd.Flags().
		BoolVar(&actionFocusWindowBackward, "backward", false,
			"cycle to the previous window instead of the next one")

	actionCmd.AddCommand(actionFocusWindowCmd)
	actionCmd.AddCommand(actionSpaceCmd)
	actionCmd.AddCommand(actionMoveWindowToSpaceCmd)

	RootCmd.AddCommand(actionCmd)
}
