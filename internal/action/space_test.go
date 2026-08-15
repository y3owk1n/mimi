package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// spaceCount is how many Mission Control spaces the space tests run against;
// three is the smallest number with a middle to step through.
const spaceCount = 3

// desktopWithSpaces builds a desktop of three spaces sitting on the active one,
// with a single window on space 1 for move_window_to_space to carry.
func desktopWithSpaces(active int) *fakeDesktop {
	desktop := desktopWithWindows(1, 0)
	desktop.spaceCount = spaceCount
	desktop.activeSpace = active
	desktop.windowSpace = 1

	return desktop
}

func TestExecutor_Space_NextAndPrevWrapAround(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		active int
		arg    string
		want   int
	}{
		{name: "next from the last space wraps to the first", active: 3, arg: nextKeyword, want: 1},
		{name: "prev from the first space wraps to the last", active: 1, arg: prevKeyword, want: 3},
		{name: "next in the middle steps forward", active: 1, arg: nextKeyword, want: 2},
		{name: "prev in the middle steps back", active: 3, arg: prevKeyword, want: 2},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithSpaces(testCase.active)

			err := action.NewExecutor(desktop).ExecuteCommand(
				spaceCommandFor(t, action.NameSpace, testCase.arg),
			)
			if err != nil {
				t.Fatalf("ExecuteCommand(space %q) error = %v, want nil", testCase.arg, err)
			}

			if desktop.activeSpace != testCase.want {
				t.Fatalf("active space = %d, want %d", desktop.activeSpace, testCase.want)
			}
		})
	}
}

func TestExecutor_Space_OutOfRangeNumberIsInvalidInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		action action.Name
	}{
		{name: string(action.NameSpace), action: action.NameSpace},
		{name: string(action.NameMoveWindowToSpace), action: action.NameMoveWindowToSpace},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithSpaces(2)

			err := action.NewExecutor(desktop).ExecuteCommand(
				spaceCommandFor(t, testCase.action, "9"),
			)
			if err == nil {
				t.Fatal("ExecuteCommand() error = nil, want an error")
			}

			if !derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf("ExecuteCommand() error = %v, want invalid input", err)
			}

			if desktop.activeSpace != 2 {
				t.Fatalf("active space = %d, want it left on 2", desktop.activeSpace)
			}

			if desktop.windowSpace != 1 {
				t.Fatalf("window space = %d, want it left on 1", desktop.windowSpace)
			}
		})
	}
}

func TestExecutor_Space_MissionControlRefusesBothActions(t *testing.T) {
	t.Parallel()

	t.Run("space", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithSpaces(2)
		desktop.missionControlActive = true

		err := action.NewExecutor(desktop).FocusSpace(3)
		if err == nil {
			t.Fatal("FocusSpace() error = nil, want an error")
		}

		if !derrors.IsCode(err, derrors.CodeActionFailed) {
			t.Fatalf("FocusSpace() error = %v, want an action failure", err)
		}

		if desktop.activeSpace != 2 {
			t.Fatalf("active space = %d, want it left on 2", desktop.activeSpace)
		}
	})

	t.Run("move_window_to_space", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithSpaces(2)
		desktop.missionControlActive = true

		err := action.NewExecutor(desktop).MoveWindowToSpace(3)
		if err == nil {
			t.Fatal("MoveWindowToSpace() error = nil, want an error")
		}

		if !derrors.IsCode(err, derrors.CodeActionFailed) {
			t.Fatalf("MoveWindowToSpace() error = %v, want an action failure", err)
		}

		if desktop.windowSpace != 1 {
			t.Fatalf("window space = %d, want it left on 1", desktop.windowSpace)
		}
	})
}

func TestExecutor_MoveWindowToSpace_MovesTheWindow(t *testing.T) {
	t.Parallel()

	desktop := desktopWithSpaces(1)

	err := action.NewExecutor(desktop).MoveWindowToSpace(3)
	if err != nil {
		t.Fatalf("MoveWindowToSpace() error = %v, want nil", err)
	}

	if desktop.windowSpace != 3 {
		t.Fatalf("window space = %d, want 3", desktop.windowSpace)
	}
}

// TestExecutor_Space_RefreshesWorkspaceTitleOnSuccess pins the fix for
// mimi#98: a successful space switch or window move refreshes the systray's
// workspace title through the same Desktop seam regardless of which path
// (daemon or direct execution) drove the desktop.
func TestExecutor_Space_RefreshesWorkspaceTitleOnSuccess(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		action action.Name
	}{
		{name: string(action.NameSpace), action: action.NameSpace},
		{name: string(action.NameMoveWindowToSpace), action: action.NameMoveWindowToSpace},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithSpaces(1)

			err := action.NewExecutor(desktop).ExecuteCommand(
				spaceCommandFor(t, testCase.action, "2"),
			)
			if err != nil {
				t.Fatalf("ExecuteCommand(%s) error = %v, want nil", testCase.name, err)
			}

			wantRefreshCalls(t, desktop, 1)
		})
	}
}

// TestExecutor_Space_DoesNotRefreshWorkspaceTitleOnFailure checks the refresh
// is tied to a change actually landing: neither an out-of-range space number
// nor Mission Control refusing the action should touch the systray title.
func TestExecutor_Space_DoesNotRefreshWorkspaceTitleOnFailure(t *testing.T) {
	t.Parallel()

	t.Run("out of range", func(t *testing.T) {
		t.Parallel()

		for _, name := range []action.Name{action.NameSpace, action.NameMoveWindowToSpace} {
			desktop := desktopWithSpaces(2)

			err := action.NewExecutor(desktop).ExecuteCommand(spaceCommandFor(t, name, "9"))
			if err == nil {
				t.Fatalf("ExecuteCommand(%s) error = nil, want an error", name)
			}

			wantRefreshCalls(t, desktop, 0)
		}
	})

	t.Run("mission control active", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithSpaces(2)
		desktop.missionControlActive = true

		err := action.NewExecutor(desktop).FocusSpace(3)
		if err == nil {
			t.Fatal("FocusSpace() error = nil, want an error")
		}

		err = action.NewExecutor(desktop).MoveWindowToSpace(3)
		if err == nil {
			t.Fatal("MoveWindowToSpace() error = nil, want an error")
		}

		wantRefreshCalls(t, desktop, 0)
	})

	t.Run("underlying desktop error", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithSpaces(2)
		desktop.focusSpaceErr = derrors.New(derrors.CodeActionFailed, "boom")
		desktop.moveErr = derrors.New(derrors.CodeActionFailed, "boom")

		err := action.NewExecutor(desktop).FocusSpace(3)
		if err == nil {
			t.Fatal("FocusSpace() error = nil, want an error")
		}

		err = action.NewExecutor(desktop).MoveWindowToSpace(3)
		if err == nil {
			t.Fatal("MoveWindowToSpace() error = nil, want an error")
		}

		wantRefreshCalls(t, desktop, 0)
	})
}
