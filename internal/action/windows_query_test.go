package action_test

import (
	"reflect"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// Names the windows query reports for its two applications, and the case name
// the error-path tables share.
const (
	safariName     = "Safari"
	safariBundleID = "com.apple.Safari"
	deniedCase     = "accessibility denied"
)

// desktopWithListedWindows builds a desktop holding two windows of two
// applications, the second focused, with everything the windows query reads
// filled in.
func desktopWithListedWindows() *fakeDesktop {
	desktop := desktopWithWindows(2, 1)
	desktop.windows[0].number = 4242
	desktop.windows[0].title = "Start Page"
	desktop.windows[0].frame = geometry.Rect{X: 0, Y: 25, W: 960, H: 1055}
	desktop.windows[1].number = 4243
	desktop.windows[1].title = "notes.md"
	desktop.windows[1].frame = geometry.Rect{X: 960, Y: 25, W: 960, H: 1055}
	desktop.appInfo = map[int]action.AppInfo{
		100: {Name: safariName, BundleID: safariBundleID},
		101: {Name: "TextEdit", BundleID: "com.apple.TextEdit"},
	}

	return desktop
}

func TestExecutor_QueryWindows_ListsEveryWindowWithItsApplicationAndFrame(t *testing.T) {
	t.Parallel()

	got, err := action.NewExecutor(desktopWithListedWindows()).QueryWindows()
	if err != nil {
		t.Fatalf("QueryWindows() error = %v, want nil", err)
	}

	want := action.WindowsInfo{
		Focused: 1,
		Windows: []action.WindowEntry{
			{
				Number: 4242, PID: 100, App: safariName, BundleID: safariBundleID,
				Title: "Start Page",
				Frame: action.Frame{X: 0, Y: 25, Width: 960, Height: 1055},
			},
			{
				Number: 4243, PID: 101, App: "TextEdit", BundleID: "com.apple.TextEdit",
				Title: "notes.md",
				Frame: action.Frame{X: 960, Y: 25, Width: 960, Height: 1055},
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryWindows() = %+v, want %+v", got, want)
	}
}

// TestExecutor_QueryWindows_LeavesOutAWindowWithoutAFrame pins the one
// omission the listing makes, and that the focused index follows the window
// it names rather than the index it had before the omission.
func TestExecutor_QueryWindows_LeavesOutAWindowWithoutAFrame(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.windows[0].frameErr = derrors.New(derrors.CodeAccessibilityFailed, "unreadable")

	got, err := action.NewExecutor(desktop).QueryWindows()
	if err != nil {
		t.Fatalf("QueryWindows() error = %v, want nil", err)
	}

	if len(got.Windows) != 1 || got.Windows[0].Number != 4243 {
		t.Fatalf("QueryWindows().Windows = %+v, want only window 4243", got.Windows)
	}

	if got.Focused != 0 {
		t.Fatalf("QueryWindows().Focused = %d, want 0", got.Focused)
	}
}

// TestExecutor_QueryWindows_AsksAgainForAWindowWithoutAFrame pins that one
// unreadable frame costs a second enumeration, which lists the window when
// its application has handed out a new element under the same number.
func TestExecutor_QueryWindows_AsksAgainForAWindowWithoutAFrame(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.windows[0].frameErr = derrors.New(derrors.CodeAccessibilityFailed, "unreadable")
	desktop.windows[0].frameErrOnce = true

	got, err := action.NewExecutor(desktop).QueryWindows()
	if err != nil {
		t.Fatalf("QueryWindows() error = %v, want nil", err)
	}

	if len(got.Windows) != 2 || desktop.enumerations != 2 {
		t.Fatalf(
			"QueryWindows().Windows = %+v after %d enumerations, want both windows after 2",
			got.Windows,
			desktop.enumerations,
		)
	}
}

// TestExecutor_QueryWindows_KeepsAWindowWhoseApplicationIsUnknown: the frame
// is what a layout needs; a missing name is reported as "" and nothing else.
func TestExecutor_QueryWindows_KeepsAWindowWhoseApplicationIsUnknown(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	delete(desktop.appInfo, 100)

	got, err := action.NewExecutor(desktop).QueryWindows()
	if err != nil {
		t.Fatalf("QueryWindows() error = %v, want nil", err)
	}

	if len(got.Windows) != 2 || got.Windows[0].App != "" || got.Windows[0].BundleID != "" {
		t.Fatalf(
			"QueryWindows().Windows = %+v, want the first with an empty application",
			got.Windows,
		)
	}
}

func TestExecutor_QueryWindows_ReportsNoFocusAsMinusOne(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.focused = -1

	got, err := action.NewExecutor(desktop).QueryWindows()
	if err != nil {
		t.Fatalf("QueryWindows() error = %v, want nil", err)
	}

	if got.Focused != -1 {
		t.Fatalf("QueryWindows().Focused = %d, want -1", got.Focused)
	}
}

func TestExecutor_QueryWindows_ErrorPaths(t *testing.T) {
	t.Parallel()

	failure := derrors.New(derrors.CodeAccessibilityDenied, "denied")

	cases := []struct {
		name    string
		breakIt func(*fakeDesktop)
		code    derrors.Code
	}{
		{
			name:    deniedCase,
			breakIt: func(d *fakeDesktop) { d.accessibilityErr = failure },
			code:    derrors.CodeAccessibilityDenied,
		},
		{
			name:    "windows cannot be enumerated",
			breakIt: func(d *fakeDesktop) { d.enumerateErr = failure },
			code:    derrors.CodeActionFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithListedWindows()
			testCase.breakIt(desktop)

			_, err := action.NewExecutor(desktop).QueryWindows()
			if !derrors.IsCode(err, testCase.code) {
				t.Fatalf("QueryWindows() error = %v, want code %s", err, testCase.code)
			}
		})
	}
}

// TestExecutor_QueryDisplays_OrdersAndConvertsTheDisplays pins the two things
// a layout relies on: the numbering matches move_window_to_display, and the
// frames are in window coordinates, so the right display, whose visible
// frame is taller than the primary, has a negative top.
func TestExecutor_QueryDisplays_OrdersAndConvertsTheDisplays(t *testing.T) {
	t.Parallel()

	got, err := action.NewExecutor(desktopWithDisplays()).QueryDisplays()
	if err != nil {
		t.Fatalf("QueryDisplays() error = %v, want nil", err)
	}

	want := []action.DisplayEntry{
		{
			Index:   1,
			ID:      leftDisplayID,
			Frame:   action.Frame{X: 0, Y: 0, Width: 1920, Height: 1080},
			Visible: action.Frame{X: 0, Y: 25, Width: 1920, Height: 1055},
		},
		{
			Index:   2,
			ID:      rightDisplayID,
			Frame:   action.Frame{X: 1920, Y: -360, Width: 2560, Height: 1440},
			Visible: action.Frame{X: 1920, Y: -335, Width: 2560, Height: 1415},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryDisplays() = %+v, want %+v", got, want)
	}
}

// TestExecutor_QueryDisplays_NeedsNoAccessibility: it reads screens, not
// windows, so the permission the window queries check is not asked for.
func TestExecutor_QueryDisplays_NeedsNoAccessibility(t *testing.T) {
	t.Parallel()

	desktop := desktopWithDisplays()
	desktop.accessibilityErr = derrors.New(derrors.CodeAccessibilityDenied, "denied")

	_, err := action.NewExecutor(desktop).QueryDisplays()
	if err != nil {
		t.Fatalf("QueryDisplays() without Accessibility error = %v, want nil", err)
	}
}

func TestExecutor_QueryDisplays_ErrorPaths(t *testing.T) {
	t.Parallel()

	failure := derrors.New(derrors.CodeActionFailed, "broken")

	cases := []struct {
		name    string
		breakIt func(*fakeDesktop)
	}{
		{
			name:    "displays cannot be enumerated",
			breakIt: func(d *fakeDesktop) { d.displaysErr = failure },
		},
		{name: "no displays", breakIt: func(d *fakeDesktop) { d.displays = nil }},
		{
			name:    "primary screen unreadable",
			breakIt: func(d *fakeDesktop) { d.screenErr = failure },
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithDisplays()
			testCase.breakIt(desktop)

			_, err := action.NewExecutor(desktop).QueryDisplays()
			if !derrors.IsCode(err, derrors.CodeActionFailed) {
				t.Fatalf("QueryDisplays() error = %v, want CodeActionFailed", err)
			}
		})
	}
}
