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

var (
	actionFocusWindowCmd       = buildFocusWindowCommand()
	actionSpaceCmd             = buildSpaceCommand()
	actionMoveWindowToSpaceCmd = buildMoveWindowToSpaceCommand()
	actionResizeWindowCmd      = buildResizeWindowCommand()
)

func buildFocusWindowCommand() *cobra.Command {
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
		RunE: func(_ *cobra.Command, _ []string) error {
			args := []string{}
			if backward {
				args = append(args, "--backward")
			}

			if focusUp {
				args = append(args, "--up")
			}

			if focusDown {
				args = append(args, "--down")
			}

			if focusLeft {
				args = append(args, "--left")
			}

			if focusRight {
				args = append(args, "--right")
			}

			return runAction(string(action.NameFocusWindow), args)
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

func buildSpaceCommand() *cobra.Command {
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
		Args: validateActionSpaceArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return runAction(string(action.NameSpace), []string{strings.TrimSpace(args[0])})
		},
	}
}

func buildMoveWindowToSpaceCommand() *cobra.Command {
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
		Args: validateActionMoveWindowToSpaceArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return runAction(
				string(action.NameMoveWindowToSpace),
				[]string{strings.TrimSpace(args[0])},
			)
		},
	}
}

func buildResizeWindowCommand() *cobra.Command {
	var (
		width     int
		height    int
		widthPct  float64
		heightPct float64
		xCoord    int
		yCoord    int
		anchor    string
		useMargin bool
		noMargin  bool
	)

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
  single gap. Use --margin or --no-margin to override.

Examples:
  mimi action resize_window left-half
  mimi action resize_window --width 800 --height 600 --anchor cc
  mimi action resize_window --width-percent 50 --height-percent 100 --anchor tl
  mimi action resize_window --width 1024 --height 768 --x 0 --y 0 --anchor tl
  mimi action resize_window fill --no-margin
  mimi action resize_window center --width-percent 80 --height-percent 90`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			cmdArgs := []string{}

			if len(args) > 0 {
				preset := strings.TrimSpace(args[0])
				if action.IsResizePreset(preset) {
					cmdArgs = append(cmdArgs, preset)
				} else {
					return derrors.Newf(
						derrors.CodeInvalidInput,
						"unknown preset %q (valid: left-half, right-half, top-half, bottom-half, top-left, top-right, bottom-left, bottom-right, center, fill)",
						preset,
					)
				}
			}

			if width > 0 {
				cmdArgs = append(cmdArgs, "--width", strconv.Itoa(width))
			}

			if height > 0 {
				cmdArgs = append(cmdArgs, "--height", strconv.Itoa(height))
			}

			if widthPct > 0 {
				cmdArgs = append(
					cmdArgs,
					"--width-percent",
					strconv.FormatFloat(widthPct, 'f', -1, 64),
				)
			}

			if heightPct > 0 {
				cmdArgs = append(
					cmdArgs,
					"--height-percent",
					strconv.FormatFloat(heightPct, 'f', -1, 64),
				)
			}

			if cobraCmd.Flags().Changed("x") {
				cmdArgs = append(cmdArgs, "--x", strconv.Itoa(xCoord))
			}

			if cobraCmd.Flags().Changed("y") {
				cmdArgs = append(cmdArgs, "--y", strconv.Itoa(yCoord))
			}

			if cobraCmd.Flags().Changed("anchor") {
				cmdArgs = append(cmdArgs, "--anchor", anchor)
			}

			if useMargin {
				cmdArgs = append(cmdArgs, "--margin")
			}

			if noMargin {
				cmdArgs = append(cmdArgs, "--no-margin")
			}

			return runAction(string(action.NameResizeWindow), cmdArgs)
		},
	}

	cmd.Flags().IntVarP(&width, "width", "w", 0, "Absolute window width in points")
	cmd.Flags().IntVar(&height, "height", 0, "Absolute window height in points")
	cmd.Flags().Float64Var(&widthPct, "width-percent", 0, "Width as percentage of screen (0-100)")
	cmd.Flags().
		Float64Var(&heightPct, "height-percent", 0, "Height as percentage of screen (0-100)")
	cmd.Flags().IntVar(&xCoord, "x", 0, "Absolute x position in screen coordinates")
	cmd.Flags().IntVar(&yCoord, "y", 0, "Absolute y position in screen coordinates")
	cmd.Flags().
		StringVarP(&anchor, "anchor", "a", "", "Anchor point for positioning (tl, tc, tr, cl, cc, cr, bl, bc, br)")
	cmd.Flags().
		BoolVar(&useMargin, "margin", false, "Enable tiled window margins (overrides system setting)")
	cmd.Flags().
		BoolVar(&noMargin, "no-margin", false, "Disable tiled window margins (overrides system setting)")

	return cmd
}

func validateActionSpaceArgs(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space requires exactly one positional argument: a 1-based number, \"next\", or \"prev\" (e.g., mimi action space 1, mimi action space next)",
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return derrors.New(derrors.CodeInvalidInput, "space argument cannot be empty")
	}

	keywords := map[string]bool{"next": true, "prev": true, "previous": true}
	if keywords[raw] {
		return nil
	}

	return validateActionIndexArgs(args, "space")
}

func validateActionMoveWindowToSpaceArgs(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"move_window_to_space requires exactly one positional argument: a 1-based number, \"next\", or \"prev\" (e.g., mimi action move_window_to_space 1, mimi action move_window_to_space next)",
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return derrors.New(derrors.CodeInvalidInput, "space argument cannot be empty")
	}

	keywords := map[string]bool{"next": true, "prev": true, "previous": true}
	if keywords[raw] {
		return nil
	}

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
	actionCmd.AddCommand(actionResizeWindowCmd)
}
