package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// desktopWithOneWindow builds a desktop holding a single 200x200 window,
// frontmost, on a 1000x1000 screen.
func desktopWithOneWindow() *fakeDesktop {
	desktop := desktopWithWindows(1, 0)
	desktop.windows[0].frame = geometry.Rect{X: 10, Y: 10, W: 200, H: 200}
	desktop.screen = geometry.Screen{
		Visible:       geometry.Rect{X: 0, Y: 0, W: 1000, H: 1000},
		PrimaryHeight: 1000,
	}

	return desktop
}

// TestExecutor_ResizeWindow_WritesTheResizedFrame checks that the read, the
// geometry and the write are joined up — that a resize reaches the window at
// all. What the geometry computes is internal/geometry's own business, and is
// covered there.
func TestExecutor_ResizeWindow_WritesTheResizedFrame(t *testing.T) {
	t.Parallel()

	desktop := desktopWithOneWindow()

	err := action.NewExecutor(desktop).Execute(
		string(action.NameResizeWindow),
		[]string{"--width", "400"},
	)
	if err != nil {
		t.Fatalf("Execute(resize_window) error = %v, want nil", err)
	}

	if got := desktop.windows[0].frame.W; got != 400 {
		t.Fatalf("window width = %v, want 400", got)
	}

	// Resizing never moves a window between spaces, so it must not touch the
	// systray's workspace title the way space and move_window_to_space do.
	wantRefreshCalls(t, desktop, 0)
}

func TestExecutor_ResizeWindow_ErrorPaths(t *testing.T) {
	t.Parallel()

	failure := derrors.New(derrors.CodeAccessibilityFailed, "macOS said no")

	cases := []struct {
		name     string
		breakIt  func(*fakeDesktop)
		wantCode derrors.Code
	}{
		{
			name:     "the window frame cannot be read",
			breakIt:  func(d *fakeDesktop) { d.windows[0].frameErr = failure },
			wantCode: derrors.CodeActionFailed,
		},
		{
			// The desktop is the one that knows which of the screen reads
			// behind ScreenAt failed, so its error reaches the user as it is.
			name:     "the screen cannot be read",
			breakIt:  func(d *fakeDesktop) { d.screenErr = failure },
			wantCode: derrors.CodeAccessibilityFailed,
		},
		{
			// A refused write is the desktop's own failure too, and likewise
			// reaches the user with the code the desktop gave it.
			name:     "the frame cannot be written",
			breakIt:  func(d *fakeDesktop) { d.windows[0].setFrameErr = failure },
			wantCode: derrors.CodeAccessibilityFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithOneWindow()
			before := desktop.windows[0].frame

			testCase.breakIt(desktop)

			err := action.NewExecutor(desktop).
				ResizeWindow(geometry.Request{Preset: presetFor(t, presetFill)})
			if err == nil {
				t.Fatal("ResizeWindow() error = nil, want an error")
			}

			if !derrors.IsCode(err, testCase.wantCode) {
				t.Fatalf("ResizeWindow() error = %v, want code %s", err, testCase.wantCode)
			}

			if desktop.windows[0].frame != before {
				t.Fatalf(
					"window frame = %+v, want it left at %+v",
					desktop.windows[0].frame,
					before,
				)
			}
		})
	}
}
