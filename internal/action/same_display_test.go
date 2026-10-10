package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// desktopWithSpacesOnTwoDisplays has spaces 1 to 3 on display 1 and space 4
// alone on display 2, with active in front.
func desktopWithSpacesOnTwoDisplays(active int) *fakeDesktop {
	desktop := desktopWithSpaces(active)
	desktop.spaceCount = 4
	desktop.spaces = []action.Space{
		{ID: 101, DisplayID: 1},
		{ID: 102, DisplayID: 1},
		{ID: 103, DisplayID: 1},
		{ID: 201, DisplayID: 2},
	}

	return desktop
}

func TestExecuteCommand_Space_SameDisplayCyclesWithinTheDisplay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		active int
		arg    string
		want   int
	}{
		{
			name:   "next from the last on the display wraps to its first",
			active: 3,
			arg:    nextKeyword,
			want:   1,
		},
		{
			name:   "prev from the first on the display wraps to its last",
			active: 1,
			arg:    prevKeyword,
			want:   3,
		},
		{name: "next in the middle steps forward", active: 2, arg: nextKeyword, want: 3},
		{name: "a display with one space stays put", active: 4, arg: nextKeyword, want: 4},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithSpacesOnTwoDisplays(testCase.active)

			cmd, err := action.NewSpaceCommand([]string{testCase.arg})
			if err != nil {
				t.Fatalf("NewSpaceCommand(%s) error = %v", testCase.arg, err)
			}

			cmd.Space, err = cmd.Space.SameDisplayOnly(action.NameSpace)
			if err != nil {
				t.Fatalf("SameDisplayOnly() error = %v", err)
			}

			err = action.NewExecutor(desktop).ExecuteCommand(cmd)
			if err != nil {
				t.Fatalf("space %s --same-display error = %v", testCase.arg, err)
			}

			if desktop.activeSpace != testCase.want {
				t.Fatalf("active space = %d, want %d", desktop.activeSpace, testCase.want)
			}
		})
	}
}

// TestSpaceArg_SameDisplayTakesNoNumber pins that --same-display with a space
// number is invalid input, whether the CLI built the command or the daemon
// decoded it off the socket.
func TestSpaceArg_SameDisplayTakesNoNumber(t *testing.T) {
	t.Parallel()

	_, err := action.SpaceArg{Index: 2}.SameDisplayOnly(action.NameSpace)
	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("SameDisplayOnly() on space 2 error = %v, want CodeInvalidInput", err)
	}

	desktop := desktopWithSpacesOnTwoDisplays(1)

	err = action.NewExecutor(desktop).ExecuteCommand(action.Command{
		Name:  action.NameSpace,
		Space: action.SpaceArg{Index: 2, SameDisplay: true},
	})
	if !derrors.IsCode(err, derrors.CodeInvalidInput) || desktop.activeSpace != 1 {
		t.Fatalf(
			"decoded space 2 --same-display error = %v, active %d, want CodeInvalidInput and no switch",
			err,
			desktop.activeSpace,
		)
	}
}
