package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func TestExecutor_QuerySpace_ReportsTheActiveSpaceAndTheCount(t *testing.T) {
	t.Parallel()

	desktop := desktopWithSpaces(2)

	got, err := action.NewExecutor(desktop).QuerySpace()
	if err != nil {
		t.Fatalf("QuerySpace() error = %v, want nil", err)
	}

	want := action.SpaceInfo{Index: 2, Count: spaceCount}
	if got != want {
		t.Fatalf("QuerySpace() = %+v, want %+v", got, want)
	}
}

// TestExecutor_QuerySpace_NeedsNoAccessibility pins the one way a query
// differs from an action: the workspace hooks report the space without the
// permission, so the query that answers the same question does too.
func TestExecutor_QuerySpace_NeedsNoAccessibility(t *testing.T) {
	t.Parallel()

	desktop := desktopWithSpaces(1)
	desktop.accessibilityErr = derrors.New(derrors.CodeAccessibilityDenied, "denied")

	_, err := action.NewExecutor(desktop).QuerySpace()
	if err != nil {
		t.Fatalf("QuerySpace() without Accessibility error = %v, want nil", err)
	}
}

func TestExecutor_QuerySpace_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		desktop func() *fakeDesktop
	}{
		{
			name: "no spaces enumerated",
			desktop: func() *fakeDesktop {
				desktop := desktopWithSpaces(1)
				desktop.spaceCount = 0

				return desktop
			},
		},
		{
			name: "active space unresolved",
			desktop: func() *fakeDesktop {
				desktop := desktopWithSpaces(1)
				desktop.activeSpaceErr = derrors.New(derrors.CodeActionFailed, "no active space")

				return desktop
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := action.NewExecutor(testCase.desktop()).QuerySpace()
			if err == nil {
				t.Fatal("QuerySpace() error = nil, want an error")
			}

			if !derrors.IsCode(err, derrors.CodeActionFailed) {
				t.Fatalf(
					"QuerySpace() error code = %q, want %q",
					derrors.GetCode(err),
					derrors.CodeActionFailed,
				)
			}
		})
	}
}

func TestExecutor_QueryWindow_ReportsTheFrontmostWindowAndItsFrame(t *testing.T) {
	t.Parallel()

	desktop := desktopWithOneWindow()

	got, err := action.NewExecutor(desktop).QueryWindow()
	if err != nil {
		t.Fatalf("QueryWindow() error = %v, want nil", err)
	}

	want := action.WindowInfo{
		PID:   100,
		Frame: action.Frame{X: 10, Y: 10, Width: 200, Height: 200},
	}
	if got != want {
		t.Fatalf("QueryWindow() = %+v, want %+v", got, want)
	}
}

func TestExecutor_QueryWindow_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		desktop  func() *fakeDesktop
		wantCode derrors.Code
	}{
		{
			name: "accessibility denied",
			desktop: func() *fakeDesktop {
				desktop := desktopWithOneWindow()
				desktop.accessibilityErr = derrors.New(derrors.CodeAccessibilityDenied, "denied")

				return desktop
			},
			wantCode: derrors.CodeAccessibilityDenied,
		},
		{
			name: "no frontmost window",
			desktop: func() *fakeDesktop {
				desktop := desktopWithOneWindow()
				desktop.frontmostErr = derrors.New(
					derrors.CodeActionFailed,
					"no active window found",
				)

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
		{
			name: "frame unreadable",
			desktop: func() *fakeDesktop {
				desktop := desktopWithOneWindow()
				desktop.windows[0].frameErr = derrors.New(
					derrors.CodeAccessibilityFailed,
					"no frame",
				)

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := action.NewExecutor(testCase.desktop()).QueryWindow()
			if err == nil {
				t.Fatal("QueryWindow() error = nil, want an error")
			}

			if !derrors.IsCode(err, testCase.wantCode) {
				t.Fatalf(
					"QueryWindow() error code = %q, want %q",
					derrors.GetCode(err),
					testCase.wantCode,
				)
			}
		})
	}
}

// TestExecutor_QueryWindow_ReadsButNeverWrites is what makes a query a query:
// the window it reports is left exactly where it was.
func TestExecutor_QueryWindow_ReadsButNeverWrites(t *testing.T) {
	t.Parallel()

	desktop := desktopWithOneWindow()
	before := desktop.windows[0].frame

	_, err := action.NewExecutor(desktop).QueryWindow()
	if err != nil {
		t.Fatalf("QueryWindow() error = %v, want nil", err)
	}

	if got := desktop.windows[0].frame; got != before {
		t.Fatalf("window frame after query = %+v, want %+v unchanged", got, before)
	}

	wantRefreshCalls(t, desktop, 0)
	wantFocused(t, desktop, desktop.windows[0].id)
}
