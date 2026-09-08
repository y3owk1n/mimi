package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The application the focus_app tests reach for: three windows across the
// spaces, listed front to back as macOS lists them, most recently used first.
// Window 3 (space 1) is the one used last, window 2 (space 2) before it, and
// window 4 (space 3) least recently. One window of another application, id 1,
// sits on space 1 and holds focus at the start.
const (
	appQuery = "Safari"
	appPID   = 500
	otherPID = 600

	otherWindow          action.WindowID = 1
	appWindowOnSpaceTwo  action.WindowID = 2
	appWindowOnSpaceOne  action.WindowID = 3
	appWindowOnSpaceThre action.WindowID = 4
)

// desktopWithApp builds that desktop, focused on the window at the given
// 0-based index (window id index+1), sitting on space 1 of three.
func desktopWithApp(focused int) *fakeDesktop {
	desktop := desktopWithWindows(4, focused)
	desktop.spaceCount = 3
	desktop.activeSpace = 1

	for index := range desktop.windows {
		desktop.windows[index].number = uint32(desktop.windows[index].id) * 10
		desktop.windows[index].pid = appPID
	}

	desktop.windows[0].pid = otherPID

	desktop.apps = map[string]int{appQuery: appPID}
	desktop.appWindows = map[int][]action.AppWindow{
		appPID: {
			appWindowFor(desktop, appWindowOnSpaceOne, 1),
			appWindowFor(desktop, appWindowOnSpaceTwo, 2),
			appWindowFor(desktop, appWindowOnSpaceThre, 3),
		},
	}

	return desktop
}

func appWindowFor(desktop *fakeDesktop, id action.WindowID, space int) action.AppWindow {
	index, _ := desktop.indexOf(id)

	return action.AppWindow{Number: desktop.windows[index].number, SpaceIndex: space}
}

func focusApp(t *testing.T, desktop *fakeDesktop) {
	t.Helper()

	err := action.NewExecutor(desktop).FocusApp(appQuery)
	if err != nil {
		t.Fatalf("FocusApp(%q) error = %v, want nil", appQuery, err)
	}
}

// TestExecutor_FocusApp_NotInFrontGoesToTheMostRecentWindow: the first press
// from another application lands on the window the user last used, on the
// space it is on. Here that window is on the current space, so no switch.
func TestExecutor_FocusApp_NotInFrontGoesToTheMostRecentWindow(t *testing.T) {
	t.Parallel()

	desktop := desktopWithApp(0)

	focusApp(t, desktop)

	wantFocused(t, desktop, appWindowOnSpaceOne)

	if desktop.activeSpace != 1 {
		t.Fatalf("active space = %d, want it left on 1", desktop.activeSpace)
	}

	wantRefreshCalls(t, desktop, 0)
}

// TestExecutor_FocusApp_SwitchesToTheWindowsSpaceFirst: a window on another
// space is reached by switching there, then raising it, and the systray learns
// of the switch.
func TestExecutor_FocusApp_SwitchesToTheWindowsSpaceFirst(t *testing.T) {
	t.Parallel()

	desktop := desktopWithApp(0)
	// The most recently used window is now the one on space 2.
	desktop.appWindows[appPID] = []action.AppWindow{
		appWindowFor(desktop, appWindowOnSpaceTwo, 2),
		appWindowFor(desktop, appWindowOnSpaceOne, 1),
	}

	focusApp(t, desktop)

	wantFocused(t, desktop, appWindowOnSpaceTwo)

	if desktop.activeSpace != 2 {
		t.Fatalf("active space = %d, want 2", desktop.activeSpace)
	}

	wantRefreshCalls(t, desktop, 1)
}

// TestExecutor_FocusApp_InFrontCyclesInDesktopOrder pins the cycle: with the
// application already in front, each press moves to the next window by space
// and then by age, whatever was used last, and wraps.
func TestExecutor_FocusApp_InFrontCyclesInDesktopOrder(t *testing.T) {
	t.Parallel()

	// Focus starts on the app's window on space 1.
	desktop := desktopWithApp(2)

	steps := []struct {
		want  action.WindowID
		space int
	}{
		{want: appWindowOnSpaceTwo, space: 2},
		{want: appWindowOnSpaceThre, space: 3},
		{want: appWindowOnSpaceOne, space: 1},
		{want: appWindowOnSpaceTwo, space: 2},
	}

	for press, step := range steps {
		focusApp(t, desktop)

		if got := desktop.focusedID(); got != step.want {
			t.Fatalf("press %d: focused = %d, want %d", press+1, got, step.want)
		}

		if desktop.activeSpace != step.space {
			t.Fatalf(
				"press %d: active space = %d, want %d",
				press+1,
				desktop.activeSpace,
				step.space,
			)
		}
	}
}

// TestExecutor_FocusApp_InFrontWithOneWindowStaysPut: one window has nowhere
// to cycle to, and that is not an error.
func TestExecutor_FocusApp_InFrontWithOneWindowStaysPut(t *testing.T) {
	t.Parallel()

	desktop := desktopWithApp(2)
	desktop.appWindows[appPID] = []action.AppWindow{appWindowFor(desktop, appWindowOnSpaceOne, 1)}

	focusApp(t, desktop)

	wantFocused(t, desktop, appWindowOnSpaceOne)
	wantRefreshCalls(t, desktop, 0)
}

// TestExecutor_FocusApp_AWindowOnEverySpaceNeedsNoSwitch: index 0 names no
// particular space, so the window is raised where the user is.
func TestExecutor_FocusApp_AWindowOnEverySpaceNeedsNoSwitch(t *testing.T) {
	t.Parallel()

	desktop := desktopWithApp(0)
	desktop.appWindows[appPID] = []action.AppWindow{appWindowFor(desktop, appWindowOnSpaceTwo, 0)}

	focusApp(t, desktop)

	wantFocused(t, desktop, appWindowOnSpaceTwo)

	if desktop.activeSpace != 1 {
		t.Fatalf("active space = %d, want it left on 1", desktop.activeSpace)
	}
}

// TestExecutor_FocusApp_NoWindowsReopensTheApplication: an application with
// nothing to raise is reopened instead.
func TestExecutor_FocusApp_NoWindowsReopensTheApplication(t *testing.T) {
	t.Parallel()

	desktop := desktopWithApp(0)
	desktop.appWindows[appPID] = nil

	focusApp(t, desktop)

	if desktop.reopenedApp != appPID {
		t.Fatalf("reopened application = %d, want %d", desktop.reopenedApp, appPID)
	}

	wantFocused(t, desktop, otherWindow)
}

func TestExecutor_FocusApp_ErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		desktop  func() *fakeDesktop
		wantCode derrors.Code
	}{
		{
			name: "application not running",
			desktop: func() *fakeDesktop {
				desktop := desktopWithApp(0)
				desktop.apps = nil

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
		{
			name: "mission control open when a switch is needed",
			desktop: func() *fakeDesktop {
				desktop := desktopWithApp(0)
				desktop.missionControlActive = true
				desktop.appWindows[appPID] = []action.AppWindow{
					appWindowFor(desktop, appWindowOnSpaceTwo, 2),
				}

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
		{
			name: "the switch fails",
			desktop: func() *fakeDesktop {
				desktop := desktopWithApp(0)
				desktop.focusSpaceErr = derrors.New(derrors.CodeActionFailed, "swipe failed")
				desktop.appWindows[appPID] = []action.AppWindow{
					appWindowFor(desktop, appWindowOnSpaceTwo, 2),
				}

				return desktop
			},
			wantCode: derrors.CodeActionFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := testCase.desktop()

			err := action.NewExecutor(desktop).FocusApp(appQuery)
			if err == nil {
				t.Fatal("FocusApp() error = nil, want an error")
			}

			if !derrors.IsCode(err, testCase.wantCode) {
				t.Fatalf("error code = %q, want %q", derrors.GetCode(err), testCase.wantCode)
			}

			wantFocused(t, desktop, otherWindow)
			wantRefreshCalls(t, desktop, 0)
		})
	}
}

func TestParseFocusAppArg(t *testing.T) {
	t.Parallel()

	got, err := action.ParseFocusAppArg([]string{"  " + safariBundleID + " "})
	if err != nil || got != safariBundleID {
		t.Fatalf("ParseFocusAppArg(padded) = %q, %v, want the trimmed name and nil", got, err)
	}

	for _, args := range [][]string{nil, {""}, {"   "}, {appQuery, "Mail"}} {
		_, err := action.ParseFocusAppArg(args)
		if err == nil || !derrors.IsCode(err, derrors.CodeInvalidInput) {
			t.Errorf("ParseFocusAppArg(%q) error = %v, want invalid input", args, err)
		}
	}
}
