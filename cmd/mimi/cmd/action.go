package cmd

import (
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// newActionCmd builds the command tree that performs immediate window and
// space utility actions.
func newActionCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "action",
		Short: "Perform window and space utility actions",
		Long: `Perform immediate window and space utility actions.

Available subcommands:
  Window control:   focus_window, resize_window
  Space control:    space, move_window_to_space

Examples:
  mimi action focus_window
  mimi action focus_window --backward
  mimi action space 1
  mimi action space next
  mimi action space prev
  mimi action move_window_to_space 2
  mimi action move_window_to_space next
  mimi action move_window_to_space prev
  mimi action resize_window left-half
  mimi action resize_window --width 800 --height 600 --anchor cc
  mimi action resize_window --width-percent 50 --height-percent 100 --anchor tl`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return derrors.New(
				derrors.CodeInvalidInput,
				"action subcommand required (e.g., mimi action focus_window, mimi action space 1)",
			)
		},
	}

	cmd.AddCommand(buildFocusWindowCommand(state))
	cmd.AddCommand(buildSpaceCommand(state))
	cmd.AddCommand(buildMoveWindowToSpaceCommand(state))
	cmd.AddCommand(buildResizeWindowCommand(state))

	return cmd
}

func buildFocusWindowCommand(state *cliState) *cobra.Command {
	var (
		backward   bool
		focusUp    bool
		focusDown  bool
		focusLeft  bool
		focusRight bool
	)

	cmd := &cobra.Command{
		Use:   "focus_window",
		Short: "Cycle or navigate focus through windows on the active space",
		Long: `Cycle keyboard focus through all focusable windows on the current space,
or move focus spatially with direction flags.

Cycles forward (or backward with --backward), wrapping at the end. Use
--up, --down, --left, or --right to move focus to the nearest window
in that direction based on screen position.

Only windows that are focusable (not minimized, not hidden) and on the
current space are included.`,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			focusCmd, err := action.NewFocusWindowCommand(
				backward,
				focusUp,
				focusDown,
				focusLeft,
				focusRight,
			)
			if err != nil {
				return err
			}

			return state.runAction(cobraCmd, focusCmd)
		},
	}

	cmd.Flags().
		BoolVar(&backward, "backward", false, "Cycle to the previous window instead of the next one")
	cmd.Flags().
		BoolVar(&focusUp, "up", false, "Move focus to the nearest window above")
	cmd.Flags().
		BoolVar(&focusDown, "down", false, "Move focus to the nearest window below")
	cmd.Flags().
		BoolVar(&focusLeft, "left", false, "Move focus to the nearest window on the left")
	cmd.Flags().
		BoolVar(&focusRight, "right", false, "Move focus to the nearest window on the right")

	return cmd
}

func buildSpaceCommand(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "space <number|next|prev>",
		Short: "Focus a Mission Control space by index or cycle next/prev",
		Long: `Focus a Mission Control space by its 1-based index, or cycle to the next or
previous space.

Spaces are enumerated in Mission Control ordering across all connected
displays. Index 1 is the first space (typically the leftmost on the
primary display), index 2 the second, and so on.

The "next" and "prev" keywords cycle through spaces with wrapping — "next"
on the last space wraps to space 1, and "prev" on space 1 wraps to the
last space.

macOS does not provide a public API to activate a space, so mimi
synthesizes a high-velocity horizontal dock swipe gesture to fast-forward
to the destination space without the standard swipe animation. When the
destination sits on a different display, the cursor is warped to its
center first so the gesture is attributed to the correct screen.

Examples:
  mimi action space 1        Focus the first Mission Control space
  mimi action space next     Cycle to the next space (with wrap)
  mimi action space prev     Cycle to the previous space (with wrap)`,
		Args: validateSpaceArg(action.NameSpace),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			spaceCmd, err := action.NewSpaceCommand(args)
			if err != nil {
				return err
			}

			return state.runAction(cobraCmd, spaceCmd)
		},
	}
}

func buildMoveWindowToSpaceCommand(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "move_window_to_space <number|next|prev>",
		Short: "Move current focused window to a Mission Control space by index or cycle next/prev",
		Long: `Move the currently focused window to a Mission Control space by its 1-based
index, or cycle to the next or previous space.

Spaces are enumerated in Mission Control ordering across all connected
displays. Index 1 is the first space, index 2 the second, and so on.

The "next" and "prev" keywords cycle through spaces with wrapping — "next"
on the last space wraps to space 1, and "prev" on space 1 wraps to the
last space.

This command uses private APIs (SkyLight) to move the window instantly
without scripting additions or disabling SIP on macOS.

Examples:
  mimi action move_window_to_space 2        Move current window to space 2
  mimi action move_window_to_space next     Move window to next space (with wrap)
  mimi action move_window_to_space prev     Move window to previous space (with wrap)`,
		Args: validateSpaceArg(action.NameMoveWindowToSpace),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			moveCmd, err := action.NewMoveWindowToSpaceCommand(args)
			if err != nil {
				return err
			}

			return state.runAction(cobraCmd, moveCmd)
		},
	}
}

func buildResizeWindowCommand(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resize_window [preset]",
		Short: "Resize and reposition the frontmost window",
		Long: `Resize and reposition the frontmost window using presets or custom flags.

Presets provide quick tiling:
  left-half      Fill the left half of the screen
  right-half     Fill the right half of the screen
  top-half       Fill the top half of the screen
  bottom-half    Fill the bottom half of the screen
  top-left       Fill the top-left quadrant
  top-right      Fill the top-right quadrant
  bottom-left    Fill the bottom-left quadrant
  bottom-right   Fill the bottom-right quadrant
  center         Center the window at 60% x 80% of screen
  fill           Fill the entire screen (respecting margins)

Custom flags allow precise control using an anchor system:
  Anchors: tl (top-left), tc (top-center), tr (top-right),
           cl (center-left), cc (center-center), cr (center-right),
           bl (bottom-left), bc (bottom-center), br (bottom-right)

  When --x or --y are specified, the window's anchor point is
  placed at those absolute screen coordinates. When omitted, the
  anchor point defaults to the corresponding screen edge or center.

  The tiled window margins setting (com.apple.WindowManager
  EnableTiledWindowMargins) is respected by default. Margins are
  applied intelligently: full margin on screen-facing edges, half
  margin on internal (split) edges so adjacent windows share a
  single gap. A window too small to give up its margins is sized
  exactly as asked for instead, with no margins on either axis.
  Use --margin or --no-margin to override.

Examples:
  mimi action resize_window left-half
  mimi action resize_window --width 800 --height 600 --anchor cc
  mimi action resize_window --width-percent 50 --height-percent 100 --anchor tl
  mimi action resize_window --width 1024 --height 768 --x 0 --y 0 --anchor tl
  mimi action resize_window fill --no-margin
  mimi action resize_window center --width-percent 80 --height-percent 90`,
		Args: validateResizePresetArg,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// The preset name is forwarded exactly as it was given. This used
			// to trim it, which is why a padded name named a preset here and
			// was rejected on every other path (mimi#132); the trim now lives
			// in action.ParseResizePreset, which every path runs.
			resizeCmd, err := action.NewResizeWindowCommand(
				resizeWindowArgsFromFlags(cobraCmd, presetArg(args)),
			)
			if err != nil {
				return err
			}

			return state.runAction(cobraCmd, resizeCmd)
		},
	}

	cmd.Flags().IntP("width", "w", 0, "Absolute window width in points")
	cmd.Flags().Int("height", 0, "Absolute window height in points")
	cmd.Flags().Float64("width-percent", 0, "Width as percentage of screen (0-100)")
	cmd.Flags().Float64("height-percent", 0, "Height as percentage of screen (0-100)")
	cmd.Flags().Int("x", 0, "Absolute x position in screen coordinates")
	cmd.Flags().Int("y", 0, "Absolute y position in screen coordinates")
	cmd.Flags().
		StringP("anchor", "a", "", "Anchor point for positioning (tl, tc, tr, cl, cc, cr, bl, bc, br)")
	cmd.Flags().Bool("margin", false, "Enable tiled window margins (overrides system setting)")
	cmd.Flags().Bool("no-margin", false, "Disable tiled window margins (overrides system setting)")

	return cmd
}

// resizeWindowArgsFromFlags reads resize_window's typed payload directly off
// cobraCmd's flags — the one place these values are read, since nothing
// keeps a bound variable of its own. It uses Flags().Changed(...) — not a
// flag's value — to tell whether the caller gave it, the rule --x and --y
// already followed and --width, --height and their -percent counterparts
// now match, so an explicit zero is forwarded rather than silently dropped.
func resizeWindowArgsFromFlags(cobraCmd *cobra.Command, preset string) action.ResizeWindowArgs {
	flags := cobraCmd.Flags()

	width, _ := flags.GetInt("width")
	height, _ := flags.GetInt("height")
	widthPct, _ := flags.GetFloat64("width-percent")
	heightPct, _ := flags.GetFloat64("height-percent")
	xCoord, _ := flags.GetInt("x")
	yCoord, _ := flags.GetInt("y")
	anchor, _ := flags.GetString("anchor")
	useMargin, _ := flags.GetBool("margin")
	noMargin, _ := flags.GetBool("no-margin")

	return action.ResizeWindowArgs{
		Preset:           preset,
		Width:            width,
		WidthSet:         flags.Changed("width"),
		Height:           height,
		HeightSet:        flags.Changed("height"),
		WidthPercent:     widthPct,
		WidthPercentSet:  flags.Changed("width-percent"),
		HeightPercent:    heightPct,
		HeightPercentSet: flags.Changed("height-percent"),
		X:                xCoord,
		XSet:             flags.Changed("x"),
		Y:                yCoord,
		YSet:             flags.Changed("y"),
		Anchor:           anchor,
		AnchorSet:        flags.Changed("anchor"),
		UseMargin:        useMargin,
		NoMargin:         noMargin,
	}
}

// presetArg is resize_window's optional positional argument, or "" for the
// argument nobody gave — the spelling action.ParseResizePresetArg reads it in.
func presetArg(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[0]
}

// validateResizePresetArg is resize_window's Args validator, which puts its
// positional argument in the same layer the space actions validate theirs in
// (mimi#133). Cobra rejects the argument before RunE runs — that is what
// produces the usage output and the exit code — and the rule it rejects with is
// action.ParseResizePresetArg, the one place that rule lives, and the same rule
// ResizeRequestFromArgs calls for a command that reached the daemon path
// without passing here.
//
// Arity stays cobra's, checked before the preset, so an extra positional
// argument still reads as the arity mistake it is rather than as a mistyped
// preset. That is where this differs in shape from validateSpaceArg, whose
// rule counts the arguments itself.
//
// Nothing a user sees moves by validating here: the usage output, the exit
// code and which of several mistakes gets reported are the same as when the
// preset was checked in the command body. Cobra parses flags before it
// validates arguments, so a flag it cannot parse at all was already reported
// ahead of the preset; a flag whose value parses but breaks a rule is still
// reported after it, since the preset is rejected here before RunE reads a
// flag at all, and action.ResizeRequestFromArgs checks the preset first for the
// daemon path. What does move is invisible: the persistent pre-run that
// resolves the config path no longer runs for a bad preset, and it only assigns
// a field.
// TestResizeWindowCommand_ReportsThePositionalArgumentAndTheFlagsInOneOrder
// pins the orderings.
func validateResizePresetArg(cobraCmd *cobra.Command, args []string) error {
	err := cobra.MaximumNArgs(1)(cobraCmd, args)
	if err != nil {
		return err
	}

	_, err = action.ParseResizePresetArg(presetArg(args))

	return err
}

// validateSpaceArg builds the Args validator for an action that takes one
// space argument. Cobra keeps rejecting the argument before RunE runs — that
// is what produces the usage output and the exit code — but the rule it
// rejects with is action.ParseSpaceArg, the one place that rule lives, and the
// same rule the constructor in RunE calls. A malformed argument therefore
// never gets as far as being built into a command, and reads the same either
// way if it ever does.
func validateSpaceArg(name action.Name) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		_, err := action.ParseSpaceArg(name, args)

		return err
	}
}
