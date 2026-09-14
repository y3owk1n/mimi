package action_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func TestExecutor_CloseWindow_ClosesTheFrontmostByDefault(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()

	err := action.NewExecutor(desktop).
		ExecuteCommand(action.NewWindowCommand(action.NameCloseWindow, 0))
	if err != nil {
		t.Fatalf("close_window error = %v, want nil", err)
	}

	if want := []action.WindowID{desktop.frontmost}; !slices.Equal(desktop.closed, want) {
		t.Fatalf("closed = %v, want %v", desktop.closed, want)
	}
}

func TestExecutor_MinimizeWindow_TakesAWindowByNumber(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()

	err := action.NewExecutor(desktop).
		ExecuteCommand(action.NewWindowCommand(action.NameMinimizeWindow, 4242))
	if err != nil {
		t.Fatalf("minimize_window error = %v, want nil", err)
	}

	if !desktop.minimized[desktop.windows[0].id] || desktop.minimized[desktop.windows[1].id] {
		t.Fatalf("minimized = %v, want only window 4242", desktop.minimized)
	}
}

func TestExecutor_FullscreenWindow_TogglesBothWays(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	executor := action.NewExecutor(desktop)
	toggle := action.NewWindowCommand(action.NameFullscreenWindow, 0)

	for _, want := range []bool{true, false} {
		err := executor.ExecuteCommand(toggle)
		if err != nil {
			t.Fatalf("fullscreen_window error = %v, want nil", err)
		}

		if got := desktop.fullScreen[desktop.frontmost]; got != want {
			t.Fatalf("full screen = %v, want %v", got, want)
		}
	}
}

func TestExecutor_WindowActions_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		desktop func() *fakeDesktop
		number  uint32
		code    derrors.Code
	}{
		{
			name: deniedCase,
			desktop: func() *fakeDesktop {
				desktop := desktopWithListedWindows()
				desktop.accessibilityErr = derrors.New(derrors.CodeAccessibilityDenied, "denied")

				return desktop
			},
			code: derrors.CodeAccessibilityDenied,
		},
		{
			name:    "number not on the active space",
			desktop: desktopWithListedWindows,
			number:  9,
			code:    derrors.CodeActionFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := action.NewExecutor(testCase.desktop()).
				ExecuteCommand(action.NewWindowCommand(action.NameCloseWindow, testCase.number))
			if !derrors.IsCode(err, testCase.code) {
				t.Fatalf("close_window error = %v, want %s", err, testCase.code)
			}
		})
	}
}

func TestExecutor_QueryMinimized_ListsEveryWindowInTheDock(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.minimizedWindows = []action.MinimizedWindow{{PID: 100, Number: 7, Title: "Downloads"}}

	got, err := action.NewExecutor(desktop).QueryMinimized()
	if err != nil {
		t.Fatalf("QueryMinimized() error = %v, want nil", err)
	}

	want := action.MinimizedInfo{Windows: []action.MinimizedEntry{
		{Number: 7, PID: 100, App: safariName, BundleID: safariBundleID, Title: "Downloads"},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryMinimized() = %+v, want %+v", got, want)
	}
}

func TestExecutor_UnminimizeWindow_RestoresAWindowInTheDock(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.minimizedWindows = []action.MinimizedWindow{{PID: 100, Number: 7}}

	err := action.NewExecutor(desktop).
		ExecuteCommand(action.NewWindowCommand(action.NameUnminimizeWindow, 7))
	if err != nil {
		t.Fatalf("unminimize_window error = %v, want nil", err)
	}

	if !slices.Equal(desktop.unminimized, []uint32{7}) {
		t.Fatalf("unminimized = %v, want [7]", desktop.unminimized)
	}
}

func TestExecutor_UnminimizeWindow_ErrorPaths(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		number uint32
		code   derrors.Code
	}{
		"no number":     {0, derrors.CodeInvalidInput},
		"not minimized": {4242, derrors.CodeActionFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := action.NewExecutor(desktopWithListedWindows()).
				ExecuteCommand(action.NewWindowCommand(action.NameUnminimizeWindow, testCase.number))
			if !derrors.IsCode(err, testCase.code) {
				t.Fatalf("unminimize_window error = %v, want %s", err, testCase.code)
			}
		})
	}
}
