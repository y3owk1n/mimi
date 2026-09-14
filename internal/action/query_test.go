package action_test

import (
	"reflect"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
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
			name: deniedCase,
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

// desktopWithListedSpaces builds a desktop of two displays and three spaces,
// the second in front on the first display and the third on the second, with
// the cursor on the first display.
func desktopWithListedSpaces() *fakeDesktop {
	desktop := desktopWithSpaces(2)
	desktop.displays = []action.Display{
		{ID: 7, Frame: geometry.Rect{X: 1920, Y: 0, W: 1920, H: 1080}},
		{ID: 3, Frame: geometry.Rect{X: 0, Y: 0, W: 1920, H: 1080}},
	}
	desktop.spaces = []action.Space{
		{ID: 100, DisplayID: 3},
		{ID: 200, DisplayID: 3, Visible: true},
		{ID: 300, DisplayID: 7, Visible: true, FullScreen: true},
	}
	desktop.spaceWindows = map[uint64][]uint32{200: {4243, 4242}, 300: {9}}

	return desktop
}

func TestExecutor_QuerySpaces_ListsEverySpaceWithItsDisplayAndWindows(t *testing.T) {
	t.Parallel()

	got, err := action.NewExecutor(desktopWithListedSpaces()).QuerySpaces()
	if err != nil {
		t.Fatalf("QuerySpaces() error = %v, want nil", err)
	}

	want := action.SpacesInfo{
		Focused: 1,
		Spaces: []action.SpaceEntry{
			{Index: 1, ID: 100, Display: 1, Windows: []uint32{}},
			{Index: 2, ID: 200, Display: 1, Visible: true, Windows: []uint32{4243, 4242}},
			{Index: 3, ID: 300, Display: 2, Visible: true, FullScreen: true, Windows: []uint32{9}},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QuerySpaces() = %+v, want %+v", got, want)
	}
}

func TestExecutor_QuerySpaces_NeedsNoAccessibility(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedSpaces()
	desktop.accessibilityErr = derrors.New(derrors.CodeAccessibilityDenied, "denied")

	_, err := action.NewExecutor(desktop).QuerySpaces()
	if err != nil {
		t.Fatalf("QuerySpaces() without Accessibility error = %v, want nil", err)
	}
}

func TestExecutor_QuerySpaces_FailsWhenSpacesCannotBeEnumerated(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedSpaces()
	desktop.spacesErr = derrors.New(derrors.CodeActionFailed, "no spaces")

	_, err := action.NewExecutor(desktop).QuerySpaces()
	if err == nil {
		t.Fatal("QuerySpaces() error = nil, want an error")
	}
}
