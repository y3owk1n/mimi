package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// numbered names window 4242 of desktopWithListedWindows, which is not the
// frontmost.
var numbered = action.WindowArgs{Number: 4242}

func TestExecuteCommand_ResizeWindow_ResizesTheWindowNumbered(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.screen = geometry.Screen{Visible: leftDisplay.Visible, PrimaryHeight: primaryHeight}
	frontmost := desktop.windows[1].frame

	cmd := resizeCommandFor(t, action.ResizeWindowArgs{Width: 400, WidthSet: true})
	cmd.Window = numbered

	err := action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("resize_window --number error = %v, want nil", err)
	}

	if got := desktop.windows[0].frame.W; got != 400 {
		t.Fatalf("window 4242 width = %v, want 400", got)
	}

	if got := desktop.windows[1].frame; got != frontmost {
		t.Fatalf("frontmost window frame = %+v, want %+v unchanged", got, frontmost)
	}
}

func TestExecuteCommand_MoveWindowToSpace_MovesTheWindowNumbered(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.spaceCount = spaceCount
	desktop.activeSpace = 1

	cmd, err := action.NewMoveWindowToSpaceCommand([]string{"3"}, false)
	if err != nil {
		t.Fatalf("NewMoveWindowToSpaceCommand(3) error = %v, want nil", err)
	}

	cmd.Window = numbered

	err = action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("move_window_to_space 3 --number error = %v, want nil", err)
	}

	if len(desktop.movedByNumber) != 1 || desktop.movedByNumber[0] != 4242 ||
		desktop.windowSpace != 3 {
		t.Fatalf(
			"moved %v to space %d, want window 4242 to space 3",
			desktop.movedByNumber,
			desktop.windowSpace,
		)
	}
}

func TestExecuteCommand_MoveWindowToDisplay_MovesTheWindowNumbered(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.windows[0].frame = leftHalfOfLeft
	desktop.screen = geometry.Screen{Visible: leftDisplay.Visible, PrimaryHeight: primaryHeight}
	desktop.displays = []action.Display{rightDisplay, leftDisplay}
	frontmost := desktop.windows[1].frame

	cmd := displayCommandFor(t, "2")
	cmd.Window = numbered

	err := action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("move_window_to_display 2 --number error = %v, want nil", err)
	}

	if got := desktop.windows[0].frame; got != leftHalfOfRight {
		t.Fatalf("window 4242 frame = %+v, want %+v", got, leftHalfOfRight)
	}

	if got := desktop.windows[1].frame; got != frontmost {
		t.Fatalf("frontmost window frame = %+v, want %+v unchanged", got, frontmost)
	}
}

// TestExecuteCommand_WindowNumberNotOnTheActiveSpaceFails pins that a number
// naming no window on the active space moves nothing rather than falling
// back to the frontmost.
func TestExecuteCommand_WindowNumberNotOnTheActiveSpaceFails(t *testing.T) {
	t.Parallel()

	missing := action.WindowArgs{Number: 9999}

	cases := map[string]func(t *testing.T) action.Command{
		"resize_window": func(t *testing.T) action.Command {
			t.Helper()

			return resizeCommandFor(t, action.ResizeWindowArgs{Width: 400, WidthSet: true})
		},
		"move_window_to_space": func(t *testing.T) action.Command {
			t.Helper()

			cmd, err := action.NewMoveWindowToSpaceCommand([]string{"3"}, false)
			if err != nil {
				t.Fatalf("building move_window_to_space: %v", err)
			}

			return cmd
		},
		"move_window_to_display": func(t *testing.T) action.Command {
			t.Helper()

			return displayCommandFor(t, "2")
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithListedWindows()
			desktop.spaceCount = spaceCount
			desktop.screen = geometry.Screen{
				Visible:       leftDisplay.Visible,
				PrimaryHeight: primaryHeight,
			}
			desktop.displays = []action.Display{rightDisplay, leftDisplay}
			before := []geometry.Rect{desktop.windows[0].frame, desktop.windows[1].frame}

			cmd := build(t)
			cmd.Window = missing

			err := action.NewExecutor(desktop).ExecuteCommand(cmd)
			if !derrors.IsCode(err, derrors.CodeActionFailed) {
				t.Fatalf("error = %v, want CodeActionFailed", err)
			}

			if len(desktop.movedByNumber) != 0 ||
				desktop.windows[0].frame != before[0] || desktop.windows[1].frame != before[1] {
				t.Fatal("a window moved although the number names none on the active space")
			}
		})
	}
}
