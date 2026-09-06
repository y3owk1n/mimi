package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Two applications interleaved in a row of five windows: app A owns windows
// 1, 3 and 5, app B owns 2 and 4. Focus starts on window 3.
const (
	appA = 100
	appB = 200
)

func twoAppsInterleaved(focused int) *fakeDesktop {
	desktop := rowOfWindows(5, focused)
	for index := range desktop.windows {
		if index%2 == 0 {
			desktop.windows[index].pid = appA
		} else {
			desktop.windows[index].pid = appB
		}
	}

	return desktop
}

func sameApp(args action.FocusWindowArgs) action.FocusWindowArgs {
	args.SameApp = true

	return args
}

func TestExecutor_FocusWindow_SameAppSkipsOtherApplicationsWindows(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args action.FocusWindowArgs
		want action.WindowID
	}{
		{name: "forward", args: action.FocusWindowArgs{}, want: 5},
		{name: "backward", args: action.FocusWindowArgs{Backward: true}, want: 1},
		{name: directionRight, args: action.FocusWindowArgs{Direction: directionRight}, want: 5},
		{name: directionLeft, args: action.FocusWindowArgs{Direction: directionLeft}, want: 1},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := twoAppsInterleaved(2)

			err := action.NewExecutor(desktop).FocusWindow(sameApp(testCase.args))
			if err != nil {
				t.Fatalf("FocusWindow(same-app %s) error = %v, want nil", testCase.name, err)
			}

			wantFocused(t, desktop, testCase.want)
		})
	}
}

func TestExecutor_FocusWindow_SameAppWrapsWithinTheApplication(t *testing.T) {
	t.Parallel()

	desktop := twoAppsInterleaved(4)

	err := action.NewExecutor(desktop).FocusWindow(sameApp(action.FocusWindowArgs{}))
	if err != nil {
		t.Fatalf("FocusWindow(same-app) error = %v, want nil", err)
	}

	wantFocused(t, desktop, 1)
}

// TestExecutor_FocusWindow_SameAppWithOneWindowStaysPut: an application with
// a single window has nowhere to cycle to, and that is not an error.
func TestExecutor_FocusWindow_SameAppWithOneWindowStaysPut(t *testing.T) {
	t.Parallel()

	desktop := twoAppsInterleaved(1)
	desktop.windows[3].pid = appA

	err := action.NewExecutor(desktop).FocusWindow(sameApp(action.FocusWindowArgs{}))
	if err != nil {
		t.Fatalf("FocusWindow(same-app) error = %v, want nil", err)
	}

	wantFocused(t, desktop, 2)
}

func TestExecutor_FocusWindow_SameAppNeedsAFocusedWindow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		desktop func() *fakeDesktop
	}{
		{name: "nothing focused", desktop: func() *fakeDesktop { return twoAppsInterleaved(-1) }},
		{
			name: "owner unreadable",
			desktop: func() *fakeDesktop {
				desktop := twoAppsInterleaved(2)
				desktop.windows[2].pid = 0

				return desktop
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := testCase.desktop()
			before := desktop.focusedID()

			err := action.NewExecutor(desktop).FocusWindow(sameApp(action.FocusWindowArgs{}))
			if err == nil {
				t.Fatal("FocusWindow(same-app) error = nil, want an error")
			}

			if !derrors.IsCode(err, derrors.CodeActionFailed) {
				t.Fatalf("error = %v, want an action failure", err)
			}

			wantFocused(t, desktop, before)
		})
	}
}

// TestNewFocusWindowCommand_CarriesSameApp pins the flag onto the payload the
// socket carries.
func TestNewFocusWindowCommand_CarriesSameApp(t *testing.T) {
	t.Parallel()

	cmd, err := action.NewFocusWindowCommand(true, false, false, false, false, true)
	if err != nil {
		t.Fatalf("NewFocusWindowCommand(backward, same-app) error = %v, want nil", err)
	}

	want := action.FocusWindowArgs{Backward: true, SameApp: true}
	if cmd.FocusWindow != want {
		t.Fatalf("FocusWindow = %+v, want %+v", cmd.FocusWindow, want)
	}
}
