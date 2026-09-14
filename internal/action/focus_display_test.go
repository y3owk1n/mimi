package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// desktopWithWindowsOnBothDisplays is desktopWithDisplays with a second and
// third window on the right display, the third in front of the second.
func desktopWithWindowsOnBothDisplays() *fakeDesktop {
	desktop := desktopWithDisplays()
	desktop.windows[0].number = 1
	desktop.windows = append(desktop.windows,
		fakeWindow{id: 2, pid: 101, number: 2, order: 2, frame: leftHalfOfRight},
		fakeWindow{id: 3, pid: 102, number: 3, order: 1, frame: leftHalfOfRight},
	)

	return desktop
}

func TestExecutor_FocusDisplay_FocusesTheWindowInFrontOnThatDisplay(t *testing.T) {
	t.Parallel()

	desktop := desktopWithWindowsOnBothDisplays()

	cmd, err := action.NewFocusDisplayCommand([]string{"2"})
	if err != nil {
		t.Fatalf("building focus_display: %v", err)
	}

	err = action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("focus_display error = %v, want nil", err)
	}

	if desktop.activatedDisplay != rightDisplayID {
		t.Fatalf("activated display = %d, want %d", desktop.activatedDisplay, rightDisplayID)
	}

	wantFocused(t, desktop, 3)
}

func TestExecutor_FocusDisplay_NextStepsFromTheFrontmostWindowsDisplay(t *testing.T) {
	t.Parallel()

	desktop := desktopWithWindowsOnBothDisplays()

	cmd, err := action.NewFocusDisplayCommand([]string{nextKeyword})
	if err != nil {
		t.Fatalf("building focus_display: %v", err)
	}

	err = action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("focus_display next error = %v, want nil", err)
	}

	wantFocused(t, desktop, 3)

	// From the right display, next wraps back to the left.
	err = action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("focus_display next again error = %v, want nil", err)
	}

	wantFocused(t, desktop, 1)
}

func TestExecutor_FocusDisplay_ActivatesAnEmptyDisplayWithoutFocusing(t *testing.T) {
	t.Parallel()

	desktop := desktopWithDisplays()
	desktop.windows[0].number = 1

	cmd, err := action.NewFocusDisplayCommand([]string{"2"})
	if err != nil {
		t.Fatalf("building focus_display: %v", err)
	}

	err = action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("focus_display error = %v, want nil", err)
	}

	if desktop.activatedDisplay != rightDisplayID {
		t.Fatalf("activated display = %d, want %d", desktop.activatedDisplay, rightDisplayID)
	}

	wantFocused(t, desktop, 1)
}

func TestExecutor_FocusDisplay_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arg     string
		desktop func() *fakeDesktop
		code    derrors.Code
	}{
		{
			name: deniedCase,
			arg:  "2",
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.accessibilityErr = derrors.New(derrors.CodeAccessibilityDenied, "denied")

				return desktop
			},
			code: derrors.CodeAccessibilityDenied,
		},
		{
			name: "next with no frontmost window",
			arg:  nextKeyword,
			desktop: func() *fakeDesktop {
				desktop := desktopWithDisplays()
				desktop.frontmostErr = derrors.New(derrors.CodeActionFailed, "none")

				return desktop
			},
			code: derrors.CodeActionFailed,
		},
		{
			name:    "index past the last display",
			arg:     "3",
			desktop: desktopWithDisplays,
			code:    derrors.CodeInvalidInput,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd, err := action.NewFocusDisplayCommand([]string{testCase.arg})
			if err != nil {
				t.Fatalf("building focus_display %q: %v", testCase.arg, err)
			}

			err = action.NewExecutor(testCase.desktop()).ExecuteCommand(cmd)
			if !derrors.IsCode(err, testCase.code) {
				t.Fatalf("focus_display %q error = %v, want %s", testCase.arg, err, testCase.code)
			}
		})
	}
}
