package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// Two displays side by side: the primary on the left with a 25-point menu
// bar, and a larger one to its right whose bottom edge lines up with the
// primary's. The window fills the left half of the primary's visible frame.
const (
	primaryHeight  = 1080
	leftDisplayID  = 11
	rightDisplayID = 22
)

var (
	leftDisplay = action.Display{
		ID:      leftDisplayID,
		Frame:   geometry.Rect{X: 0, Y: 0, W: 1920, H: 1080},
		Visible: geometry.Rect{X: 0, Y: 0, W: 1920, H: 1055},
	}
	rightDisplay = action.Display{
		ID:      rightDisplayID,
		Frame:   geometry.Rect{X: 1920, Y: 0, W: 2560, H: 1440},
		Visible: geometry.Rect{X: 1920, Y: 0, W: 2560, H: 1415},
	}
	leftHalfOfLeft = geometry.Rect{X: 0, Y: 25, W: 960, H: 1055}
	// leftHalfOfRight is where leftHalfOfLeft lands on the right display:
	// the same share of a taller, wider visible frame, whose top in window
	// coordinates sits above the primary's.
	leftHalfOfRight = geometry.Rect{X: 1920, Y: -335, W: 1280, H: 1415}
)

// desktopWithDisplays builds a desktop of the two displays above, listed
// right first so the ordering the action imposes is what a test sees, with
// one window on the left display.
func desktopWithDisplays() *fakeDesktop {
	desktop := desktopWithWindows(1, 0)
	desktop.windows[0].frame = leftHalfOfLeft
	desktop.screen = geometry.Screen{Visible: leftDisplay.Visible, PrimaryHeight: primaryHeight}
	desktop.displays = []action.Display{rightDisplay, leftDisplay}

	return desktop
}

func displayCommandFor(t *testing.T, arg string) action.Command {
	t.Helper()

	cmd, err := action.NewMoveWindowToDisplayCommand([]string{arg})
	if err != nil {
		t.Fatalf("building move_window_to_display %q: %v", arg, err)
	}

	return cmd
}

func TestExecutor_MoveWindowToDisplay_KeepsTheWindowsShareOfTheDisplay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arg  string
	}{
		{name: "next", arg: nextKeyword},
		{name: "prev wraps", arg: prevKeyword},
		{name: "by number", arg: "2"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithDisplays()

			err := action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, testCase.arg))
			if err != nil {
				t.Fatalf(
					"ExecuteCommand(move_window_to_display %s) error = %v, want nil",
					testCase.arg,
					err,
				)
			}

			if got := desktop.windows[0].frame; got != leftHalfOfRight {
				t.Fatalf("window frame = %+v, want %+v", got, leftHalfOfRight)
			}

			if desktop.activatedDisplay != rightDisplayID {
				t.Fatalf(
					"activated display = %d, want %d",
					desktop.activatedDisplay,
					rightDisplayID,
				)
			}

			wantRefreshCalls(t, desktop, 1)
		})
	}
}

// TestExecutor_MoveWindowToDisplay_WritesAgainWhenTheFrameLandsShort pins the
// correction for a frame written across displays: the application clamps the
// first size to the display the window is leaving, so the frame is read back
// and written once more, and only when it landed somewhere else.
func TestExecutor_MoveWindowToDisplay_WritesAgainWhenTheFrameLandsShort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		clamps     bool
		wantWrites int
	}{
		{name: "lands short and is written again", clamps: true, wantWrites: 2},
		{name: "lands as asked and is left alone", clamps: false, wantWrites: 1},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithDisplays()
			desktop.windows[0].clampsFirstWrite = testCase.clamps

			err := action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, nextKeyword))
			if err != nil {
				t.Fatalf("ExecuteCommand(move_window_to_display next) error = %v, want nil", err)
			}

			if got := desktop.windows[0].frame; got != leftHalfOfRight {
				t.Fatalf("window frame = %+v, want %+v", got, leftHalfOfRight)
			}

			if got := desktop.windows[0].setFrameWrites; got != testCase.wantWrites {
				t.Fatalf("frame writes = %d, want %d", got, testCase.wantWrites)
			}
		})
	}
}

// TestExecutor_MoveWindowToDisplay_StaysPutOnTheDestination covers a window
// already where it was asked to go: by number, and by "next" on a machine
// with one display. Nothing is written and nothing is activated.
func TestExecutor_MoveWindowToDisplay_StaysPutOnTheDestination(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arg     string
		desktop func() *fakeDesktop
	}{
		{name: "by number", arg: "1", desktop: desktopWithDisplays},
		{
			name: "next with one display",
			arg:  nextKeyword,
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.displays = []action.Display{leftDisplay}

				return desktop
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := testCase.desktop()
			desktop.windows[0].setFrameErr = derrors.New(
				derrors.CodeActionFailed,
				"nothing should be written",
			)

			err := action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, testCase.arg))
			if err != nil {
				t.Fatalf(
					"ExecuteCommand(move_window_to_display %s) error = %v, want nil",
					testCase.arg,
					err,
				)
			}

			if desktop.activatedDisplay != 0 {
				t.Fatalf("activated display = %d, want none", desktop.activatedDisplay)
			}

			wantRefreshCalls(t, desktop, 0)
		})
	}
}

func TestExecutor_MoveWindowToDisplay_OutOfRangeNumberIsInvalidInput(t *testing.T) {
	t.Parallel()

	desktop := desktopWithDisplays()

	err := action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, "3"))
	if err == nil {
		t.Fatal("ExecuteCommand(move_window_to_display 3) error = nil, want an error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("error = %v, want invalid input", err)
	}

	if got := desktop.windows[0].frame; got != leftHalfOfLeft {
		t.Fatalf("window frame = %+v, want it left at %+v", got, leftHalfOfLeft)
	}
}

func TestExecutor_MoveWindowToDisplay_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		desktop  func() *fakeDesktop
		wantCode derrors.Code
	}{
		{
			name: "displays cannot be enumerated",
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.displaysErr = derrors.New(derrors.CodeAccessibilityFailed, "no screens")

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
		{
			name: "no displays at all",
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.displays = nil

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
		{
			name: "window on no display",
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.windows[0].frame = geometry.Rect{X: -5000, Y: -5000, W: 100, H: 100}

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
		{
			name: "frame cannot be written",
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.windows[0].setFrameErr = derrors.New(
					derrors.CodeAccessibilityFailed,
					"refused",
				)

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := testCase.desktop()

			err := action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, nextKeyword))
			if err == nil {
				t.Fatal("ExecuteCommand(move_window_to_display next) error = nil, want an error")
			}

			if !derrors.IsCode(err, testCase.wantCode) {
				t.Fatalf("error code = %q, want %q", derrors.GetCode(err), testCase.wantCode)
			}

			if desktop.activatedDisplay != 0 {
				t.Fatalf(
					"activated display = %d, want none after a failure",
					desktop.activatedDisplay,
				)
			}

			wantRefreshCalls(t, desktop, 0)
		})
	}
}

// TestExecutor_MoveWindowToDisplay_CountsDisplaysLeftToRight pins the order
// the display argument counts in, whatever order the desktop lists them:
// leftmost first, and of two starting at the same x, the higher one first.
func TestExecutor_MoveWindowToDisplay_CountsDisplaysLeftToRight(t *testing.T) {
	t.Parallel()

	above := action.Display{
		ID:      33,
		Frame:   geometry.Rect{X: 0, Y: 1080, W: 1920, H: 1080},
		Visible: geometry.Rect{X: 0, Y: 1080, W: 1920, H: 1055},
	}

	desktop := desktopWithDisplays()
	desktop.displays = []action.Display{rightDisplay, leftDisplay, above}

	// The window is on the left display, which counts second: the one above
	// it starts at the same x and sits higher. "3" is therefore the right one.
	err := action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, "3"))
	if err != nil {
		t.Fatalf("ExecuteCommand(move_window_to_display 3) error = %v, want nil", err)
	}

	if desktop.activatedDisplay != rightDisplayID {
		t.Fatalf("activated display = %d, want %d", desktop.activatedDisplay, rightDisplayID)
	}

	desktop = desktopWithDisplays()
	desktop.displays = []action.Display{rightDisplay, leftDisplay, above}

	err = action.NewExecutor(desktop).ExecuteCommand(displayCommandFor(t, prevKeyword))
	if err != nil {
		t.Fatalf("ExecuteCommand(move_window_to_display prev) error = %v, want nil", err)
	}

	if desktop.activatedDisplay != above.ID {
		t.Fatalf(
			"activated display = %d, want %d (the one above)",
			desktop.activatedDisplay,
			above.ID,
		)
	}
}

func TestParseDisplayArg_ReportsTheDisplayNoun(t *testing.T) {
	t.Parallel()

	_, err := action.ParseDisplayArg([]string{"0"})
	if err == nil {
		t.Fatal("ParseDisplayArg(0) error = nil, want an error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("error = %v, want invalid input", err)
	}

	_, err = action.ParseDisplayArg(nil)
	if err == nil || !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("ParseDisplayArg(nil) error = %v, want invalid input naming a display number", err)
	}

	got, err := action.ParseDisplayArg([]string{" next "})
	if err != nil {
		t.Fatalf("ParseDisplayArg(next) error = %v, want nil", err)
	}

	if got != (action.DisplayArg{Direction: 1}) {
		t.Fatalf("ParseDisplayArg(next) = %+v, want a step forward", got)
	}
}
