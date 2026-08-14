package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// rowOfWindows lays count windows out left to right, 100 points wide and 100
// apart, so that "the window to the right" is unambiguous.
func rowOfWindows(count, focused int) *fakeDesktop {
	desktop := desktopWithWindows(count, focused)
	for index := range desktop.windows {
		desktop.windows[index].frame = geometry.Rect{
			X: float64(index * 100), Y: 0, W: 100, H: 100,
		}
	}

	return desktop
}

func TestExecutor_FocusWindow_ForwardCyclingWrapsToFirst(t *testing.T) {
	t.Parallel()

	desktop := desktopWithWindows(3, 2)

	err := action.NewExecutor(desktop).FocusWindow(false, "")
	if err != nil {
		t.Fatalf("FocusWindow() error = %v, want nil", err)
	}

	wantFocused(t, desktop, 1)
}

func TestExecutor_FocusWindow_BackwardCyclingWrapsToLast(t *testing.T) {
	t.Parallel()

	desktop := desktopWithWindows(3, 0)

	err := action.NewExecutor(desktop).FocusWindow(true, "")
	if err != nil {
		t.Fatalf("FocusWindow() error = %v, want nil", err)
	}

	wantFocused(t, desktop, 3)
}

func TestExecutor_FocusWindow_UnfocusedDesktopFallsBackToFirstWindow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		backward bool
	}{
		{name: "forward", backward: false},
		{name: "backward", backward: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithWindows(3, -1)

			err := action.NewExecutor(desktop).FocusWindow(testCase.backward, "")
			if err != nil {
				t.Fatalf("FocusWindow() error = %v, want nil", err)
			}

			wantFocused(t, desktop, 1)
		})
	}
}

func TestExecutor_FocusWindow_DirectionalSkipsAWindowWhoseFrameCannotBeRead(t *testing.T) {
	t.Parallel()

	desktop := rowOfWindows(3, 0)
	desktop.windows[1].frameErr = derrors.New(
		derrors.CodeAccessibilityFailed,
		"failed to get window frame",
	)

	err := action.NewExecutor(desktop).FocusWindow(false, "right")
	if err != nil {
		t.Fatalf("FocusWindow() error = %v, want nil", err)
	}

	wantFocused(t, desktop, 3)
}

func TestExecutor_FocusWindow_DirectionalErrorsWhenNothingLiesThatWay(t *testing.T) {
	t.Parallel()

	desktop := rowOfWindows(3, 0)

	err := action.NewExecutor(desktop).FocusWindow(false, "left")
	if err == nil {
		t.Fatal("FocusWindow() error = nil, want an error")
	}

	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Fatalf("FocusWindow() error = %v, want an action failure", err)
	}

	wantFocused(t, desktop, 1)
}

func TestExecutor_FocusWindow_ErrorsWhenTheActiveSpaceHasNoFocusableWindows(t *testing.T) {
	t.Parallel()

	desktop := desktopWithWindows(0, -1)

	err := action.NewExecutor(desktop).FocusWindow(false, "")
	if err == nil {
		t.Fatal("FocusWindow() error = nil, want an error")
	}

	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Fatalf("FocusWindow() error = %v, want an action failure", err)
	}
}
