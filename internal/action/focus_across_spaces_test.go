package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// desktopWithAWindowOnSpaceTwo is space 1 in front with window 1 focused,
// and window 5555 of pid 321 on space 2.
func desktopWithAWindowOnSpaceTwo() *fakeDesktop {
	desktop := desktopWithSpaces(1)
	desktop.spaceCount = 2
	desktop.spaces = []action.Space{{ID: 100}, {ID: 200}}
	desktop.windows = append(desktop.windows, fakeWindow{id: 2, pid: 321, number: 5555})
	desktop.spaceOf = map[uint32]int{5555: 2}
	desktop.windowSpaceIDs = map[uint32]uint64{5555: 200}
	desktop.windowOwners = map[uint32]int{5555: 321}
	desktop.appWindows = map[int][]action.AppWindow{321: {{Number: 5555, SpaceIndex: 2}}}

	return desktop
}

func focusNumberCommand(t *testing.T, number uint32) action.Command {
	t.Helper()

	cmd, err := action.NewFocusWindowCommand(false, false, false, false, false, false, number)
	if err != nil {
		t.Fatalf("NewFocusWindowCommand(--number %d) error = %v", number, err)
	}

	return cmd
}

func TestExecuteCommand_FocusWindowNumber_SwitchesToTheWindowsSpace(t *testing.T) {
	t.Parallel()

	desktop := desktopWithAWindowOnSpaceTwo()

	err := action.NewExecutor(desktop).ExecuteCommand(focusNumberCommand(t, 5555))
	if err != nil {
		t.Fatalf("focus_window --number 5555 error = %v", err)
	}

	if desktop.activeSpace != 2 {
		t.Fatalf("active space = %d, want 2, the window's", desktop.activeSpace)
	}

	wantFocused(t, desktop, 2)
	wantRefreshCalls(t, desktop, 1)
}

func TestExecuteCommand_FocusWindowNumber_NoWindowWithTheNumber(t *testing.T) {
	t.Parallel()

	desktop := desktopWithAWindowOnSpaceTwo()

	err := action.NewExecutor(desktop).ExecuteCommand(focusNumberCommand(t, 9999))
	if !derrors.IsCode(err, derrors.CodeActionFailed) || desktop.activeSpace != 1 {
		t.Fatalf("error = %v, active space %d, want CodeActionFailed and no switch",
			err, desktop.activeSpace)
	}
}

// TestExecutor_FocusWindowNumber_KeepsToTheActiveSpace pins that the call the
// tiling engine and focus follows mouse make never switches spaces.
func TestExecutor_FocusWindowNumber_KeepsToTheActiveSpace(t *testing.T) {
	t.Parallel()

	desktop := desktopWithAWindowOnSpaceTwo()

	err := action.NewExecutor(desktop).FocusWindowNumber(5555)
	if !derrors.IsCode(err, derrors.CodeActionFailed) || desktop.activeSpace != 1 {
		t.Fatalf("error = %v, active space %d, want CodeActionFailed and no switch",
			err, desktop.activeSpace)
	}
}
