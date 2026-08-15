package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The three helpers below build a command through the constructor the CLI
// builds it with, so a test that drives the executor drives it with the very
// command a `mimi action …` invocation produces rather than a hand-assembled
// one. A constructor that rejects the arguments fails the test on the spot:
// these are for the valid cases, and the rejections have tests of their own.

func focusCommandFor(t *testing.T, backward, up, down, left, right bool) action.Command {
	t.Helper()

	cmd, err := action.NewFocusWindowCommand(backward, up, down, left, right)
	if err != nil {
		t.Fatalf("building %s: %v", action.NameFocusWindow, err)
	}

	return cmd
}

func spaceCommandFor(t *testing.T, name action.Name, arg string) action.Command {
	t.Helper()

	var (
		cmd action.Command
		err error
	)

	switch name {
	case action.NameSpace:
		cmd, err = action.NewSpaceCommand([]string{arg})
	case action.NameMoveWindowToSpace:
		cmd, err = action.NewMoveWindowToSpaceCommand([]string{arg})
	case action.NameFocusWindow, action.NameResizeWindow:
		t.Fatalf("%s takes no space argument", name)
	default:
		t.Fatalf("unknown action %q", name)
	}

	if err != nil {
		t.Fatalf("building %s %q: %v", name, arg, err)
	}

	return cmd
}

func resizeCommandFor(t *testing.T, args action.ResizeWindowArgs) action.Command {
	t.Helper()

	cmd, err := action.NewResizeWindowCommand(args)
	if err != nil {
		t.Fatalf("building %s: %v", action.NameResizeWindow, err)
	}

	return cmd
}

// TestNewFocusWindowCommand_BuildsTheTypedPayload pins the mapping from the
// CLI's already-typed bools onto focus_window's command, without a string
// round trip.
func TestNewFocusWindowCommand_BuildsTheTypedPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		backward              bool
		up, down, left, right bool
		want                  action.FocusWindowArgs
	}{
		{name: "nothing set", want: action.FocusWindowArgs{}},
		{name: "backward only", backward: true, want: action.FocusWindowArgs{Backward: true}},
		{name: "up", up: true, want: action.FocusWindowArgs{Direction: "up"}},
		{name: "down", down: true, want: action.FocusWindowArgs{Direction: "down"}},
		{name: "left", left: true, want: action.FocusWindowArgs{Direction: "left"}},
		{name: "right", right: true, want: action.FocusWindowArgs{Direction: "right"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := action.NewFocusWindowCommand(
				testCase.backward,
				testCase.up,
				testCase.down,
				testCase.left,
				testCase.right,
			)
			if err != nil {
				t.Fatalf("NewFocusWindowCommand() error = %v, want nil", err)
			}

			if got.Name != action.NameFocusWindow {
				t.Fatalf("Name = %q, want %q", got.Name, action.NameFocusWindow)
			}

			if got.FocusWindow != testCase.want {
				t.Fatalf("FocusWindow = %+v, want %+v", got.FocusWindow, testCase.want)
			}
		})
	}
}

func TestNewFocusWindowCommand_RejectsMoreThanOneDirection(t *testing.T) {
	t.Parallel()

	_, err := action.NewFocusWindowCommand(false, true, true, false, false)
	if err == nil {
		t.Fatal("NewFocusWindowCommand(up, down) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestNewFocusWindowCommand_RejectsBackwardWithDirection(t *testing.T) {
	t.Parallel()

	_, err := action.NewFocusWindowCommand(true, true, false, false, false)
	if err == nil {
		t.Fatal("NewFocusWindowCommand(backward, up) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestResizeRequestFromArgs_InvalidWidth(t *testing.T) {
	t.Parallel()

	_, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs{Width: -100, WidthSet: true})
	if err == nil {
		t.Fatal("ResizeRequestFromArgs(width -100) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestResizeRequestFromArgs_InvalidWidthPercent(t *testing.T) {
	t.Parallel()

	_, err := action.ResizeRequestFromArgs(
		action.ResizeWindowArgs{WidthPercent: 150, WidthPercentSet: true},
	)
	if err == nil {
		t.Fatal("ResizeRequestFromArgs(width-percent 150) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

// TestResizeRequestFromArgs_RejectsAnUnknownPreset covers mimi#125: the
// conversion from a command's raw arguments to a geometry request is where a
// preset name becomes a preset, so a name that is not one is rejected there —
// on every path a command can be built on, the daemon's included, rather than
// only on the one the CLI's own argument check stands in front of.
func TestResizeRequestFromArgs_RejectsAnUnknownPreset(t *testing.T) {
	t.Parallel()

	_, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs{Preset: unknownPreset})
	if err == nil {
		t.Fatalf("ResizeRequestFromArgs(preset %s) expected error", unknownPreset)
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}

	assertListsEveryPreset(t, err)
}

// TestResizeRequestFromArgs_AcceptsEveryPreset is the other half of the rule:
// each of the ten names still converts, and reaches the geometry as the preset
// it names.
func TestResizeRequestFromArgs_AcceptsEveryPreset(t *testing.T) {
	t.Parallel()

	for _, name := range everyPreset() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs{Preset: name})
			if err != nil {
				t.Fatalf("ResizeRequestFromArgs(preset %s) error = %v", name, err)
			}

			if got.Preset != presetFor(t, name) {
				t.Fatalf("Preset = %q, want %q", got.Preset, name)
			}
		})
	}
}

func TestResizeRequestFromArgs_InvalidAnchor(t *testing.T) {
	t.Parallel()

	_, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs{Anchor: "xx", AnchorSet: true})
	if err == nil {
		t.Fatal("ResizeRequestFromArgs(anchor xx) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

// TestNewSpaceCommands_ParseTheArgumentIntoTheirOwnField checks each space
// constructor puts the parsed argument on the field its action reads, so the
// name and the payload can no longer be paired up wrongly by hand.
func TestNewSpaceCommands_ParseTheArgumentIntoTheirOwnField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  string
		want action.SpaceArg
	}{
		{name: "absolute index", arg: "2", want: action.SpaceArg{Index: 2}},
		{name: "next", arg: nextKeyword, want: action.SpaceArg{Direction: 1}},
		{name: "prev", arg: prevKeyword, want: action.SpaceArg{Direction: -1}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spaceCmd, err := action.NewSpaceCommand([]string{testCase.arg})
			if err != nil {
				t.Fatalf("NewSpaceCommand(%q) error = %v, want nil", testCase.arg, err)
			}

			if spaceCmd.Name != action.NameSpace || spaceCmd.Space != testCase.want {
				t.Fatalf(
					"NewSpaceCommand(%q) = %+v, want %s carrying %+v",
					testCase.arg,
					spaceCmd,
					action.NameSpace,
					testCase.want,
				)
			}

			moveCmd, err := action.NewMoveWindowToSpaceCommand([]string{testCase.arg})
			if err != nil {
				t.Fatalf("NewMoveWindowToSpaceCommand(%q) error = %v, want nil", testCase.arg, err)
			}

			if moveCmd.Name != action.NameMoveWindowToSpace ||
				moveCmd.MoveWindowToSpace != testCase.want {
				t.Fatalf(
					"NewMoveWindowToSpaceCommand(%q) = %+v, want %s carrying %+v",
					testCase.arg,
					moveCmd,
					action.NameMoveWindowToSpace,
					testCase.want,
				)
			}
		})
	}
}

// TestNewSpaceCommands_RejectAMalformedArgument checks both space constructors
// reject with ParseSpaceArg — the one rule mimi#124 left in place — rather than
// a second copy of it.
func TestNewSpaceCommands_RejectAMalformedArgument(t *testing.T) {
	t.Parallel()

	malformed := [][]string{
		nil,
		{""},
		{"0"},
		{"-1"},
		{nonNumericArg},
		{"1", "2"},
	}

	builders := []struct {
		actionName action.Name
		build      func([]string) (action.Command, error)
	}{
		{actionName: action.NameSpace, build: action.NewSpaceCommand},
		{actionName: action.NameMoveWindowToSpace, build: action.NewMoveWindowToSpaceCommand},
	}

	for _, builder := range builders {
		actionName, build := builder.actionName, builder.build

		t.Run(string(actionName), func(t *testing.T) {
			t.Parallel()

			for _, args := range malformed {
				_, err := build(args)
				if err == nil {
					t.Fatalf("%s %v: expected an error", actionName, args)
				}

				if !derrors.IsCode(err, derrors.CodeInvalidInput) {
					t.Fatalf("%s %v: expected CodeInvalidInput, got %v", actionName, args, err)
				}

				_, wantErr := action.ParseSpaceArg(actionName, args)
				if err.Error() != wantErr.Error() {
					t.Errorf(
						"%s %v rejected with its own wording:\n got: %s\nwant: %s",
						actionName,
						args,
						err,
						wantErr,
					)
				}
			}
		})
	}
}

// TestNewResizeWindowCommand_RejectsWhatTheConversionRejects covers mimi#126:
// resize_window's command is built through a constructor that validates as it
// builds, and it validates by running the conversion — so every argument the
// conversion rejects is rejected at construction, in the conversion's own
// words, before the command exists to be sent anywhere.
func TestNewResizeWindowCommand_RejectsWhatTheConversionRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args action.ResizeWindowArgs
	}{
		{
			name: "negative width",
			args: action.ResizeWindowArgs{Width: -5, WidthSet: true},
		},
		{
			name: "negative height",
			args: action.ResizeWindowArgs{Height: -5, HeightSet: true},
		},
		{
			name: "width-percent above 100",
			args: action.ResizeWindowArgs{WidthPercent: 150, WidthPercentSet: true},
		},
		{
			name: "negative height-percent",
			args: action.ResizeWindowArgs{HeightPercent: -1, HeightPercentSet: true},
		},
		{
			name: "unknown anchor",
			args: action.ResizeWindowArgs{Anchor: "xx", AnchorSet: true},
		},
		{
			name: "unknown preset",
			args: action.ResizeWindowArgs{Preset: unknownPreset},
		},
		{
			// mimi#138: the pair asks for opposite things, so the conversion
			// refuses it and the constructor inherits that.
			name: "both margin flags at once",
			args: action.ResizeWindowArgs{UseMargin: true, NoMargin: true},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := action.NewResizeWindowCommand(testCase.args)
			if err == nil {
				t.Fatalf("NewResizeWindowCommand(%+v) expected an error", testCase.args)
			}

			if !derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf("expected CodeInvalidInput, got %v", err)
			}

			_, wantErr := action.ResizeRequestFromArgs(testCase.args)
			if err.Error() != wantErr.Error() {
				t.Errorf(
					"rejected with its own wording:\n got: %s\nwant: %s",
					err,
					wantErr,
				)
			}
		})
	}
}

// TestNewResizeWindowCommand_CarriesTheRawArgumentsUnchanged pins the other
// half: a valid command keeps the raw flags it was built from, rather than the
// geometry request the conversion made of them. Those raw flags are what the
// socket carries (see docs/adr/0001-typed-versioned-daemon-wire.md); the
// conversion runs here for its rejections, not for its result.
func TestNewResizeWindowCommand_CarriesTheRawArgumentsUnchanged(t *testing.T) {
	t.Parallel()

	args := action.ResizeWindowArgs{
		Preset: presetLeftHalf,
		Width:  800, WidthSet: true,
		Height: 600, HeightSet: true,
		Anchor: "cc", AnchorSet: true,
		NoMargin: true,
	}

	cmd, err := action.NewResizeWindowCommand(args)
	if err != nil {
		t.Fatalf("NewResizeWindowCommand(%+v) error = %v, want nil", args, err)
	}

	if cmd.Name != action.NameResizeWindow {
		t.Fatalf("Name = %q, want %q", cmd.Name, action.NameResizeWindow)
	}

	if cmd.ResizeWindow != args {
		t.Fatalf("ResizeWindow = %+v, want %+v", cmd.ResizeWindow, args)
	}
}

// TestExecuteCommand_ReachesEveryAction checks each action's command lands on
// the executor method that performs it, against a fake desktop, so the
// dispatch and the payload each branch reads are covered without a Mac under
// them.
func TestExecuteCommand_ReachesEveryAction(t *testing.T) {
	t.Parallel()

	t.Run("focus_window", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithWindows(3, 2)

		err := action.NewExecutor(desktop).ExecuteCommand(action.Command{
			Name:        action.NameFocusWindow,
			FocusWindow: action.FocusWindowArgs{},
		})
		if err != nil {
			t.Fatalf("ExecuteCommand(focus_window) error = %v, want nil", err)
		}

		wantFocused(t, desktop, 1)
	})

	t.Run("space", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithSpaces(1)

		err := action.NewExecutor(desktop).ExecuteCommand(action.Command{
			Name:  action.NameSpace,
			Space: action.SpaceArg{Index: 2},
		})
		if err != nil {
			t.Fatalf("ExecuteCommand(space) error = %v, want nil", err)
		}

		if desktop.activeSpace != 2 {
			t.Fatalf("active space = %d, want 2", desktop.activeSpace)
		}

		wantRefreshCalls(t, desktop, 1)
	})

	t.Run("move_window_to_space", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithSpaces(1)

		err := action.NewExecutor(desktop).ExecuteCommand(action.Command{
			Name:              action.NameMoveWindowToSpace,
			MoveWindowToSpace: action.SpaceArg{Direction: 1},
		})
		if err != nil {
			t.Fatalf("ExecuteCommand(move_window_to_space) error = %v, want nil", err)
		}

		if desktop.windowSpace != 2 {
			t.Fatalf("window space = %d, want 2", desktop.windowSpace)
		}

		wantRefreshCalls(t, desktop, 1)
	})

	t.Run("resize_window", func(t *testing.T) {
		t.Parallel()

		desktop := desktopWithOneWindow()

		err := action.NewExecutor(desktop).ExecuteCommand(action.Command{
			Name: action.NameResizeWindow,
			ResizeWindow: action.ResizeWindowArgs{
				Width: 800, WidthSet: true,
				Height: 600, HeightSet: true,
			},
		})
		if err != nil {
			t.Fatalf("ExecuteCommand(resize_window) error = %v, want nil", err)
		}
	})
}

// TestExecuteCommand_RejectsAPayloadNoConstructorWouldBuild covers the trust
// boundary the daemon became once it decodes commands off a socket instead of
// parsing them: a payload that no constructor would have produced arrives with
// nothing having checked it, so ExecuteCommand checks it before driving the
// desktop.
//
// The desktop underneath refuses every accessibility check, so a case that
// reached it would fail on that instead of on invalid input — which is what
// makes "rejected before anything moved" the thing being asserted.
func TestExecuteCommand_RejectsAPayloadNoConstructorWouldBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  action.Command
	}{
		{
			name: "focus_window with a direction and --backward at once",
			cmd: action.Command{
				Name:        action.NameFocusWindow,
				FocusWindow: action.FocusWindowArgs{Backward: true, Direction: "up"},
			},
		},
		{
			name: "focus_window with a direction that is not one",
			cmd: action.Command{
				Name:        action.NameFocusWindow,
				FocusWindow: action.FocusWindowArgs{Direction: "sideways"},
			},
		},
		{
			name: "space naming no space at all",
			cmd:  action.Command{Name: action.NameSpace},
		},
		{
			name: "space naming an index and a direction at once",
			cmd: action.Command{
				Name:  action.NameSpace,
				Space: action.SpaceArg{Index: 2, Direction: 1},
			},
		},
		{
			name: "space stepping by something other than one",
			cmd: action.Command{
				Name:  action.NameSpace,
				Space: action.SpaceArg{Direction: 5},
			},
		},
		{
			name: "move_window_to_space naming no space at all",
			cmd:  action.Command{Name: action.NameMoveWindowToSpace},
		},
		{
			name: "move_window_to_space naming an index and a direction at once",
			cmd: action.Command{
				Name:              action.NameMoveWindowToSpace,
				MoveWindowToSpace: action.SpaceArg{Index: 2, Direction: -1},
			},
		},
		{
			name: "resize_window naming a preset that is not one",
			cmd: action.Command{
				Name:         action.NameResizeWindow,
				ResizeWindow: action.ResizeWindowArgs{Preset: unknownPreset},
			},
		},
		{
			name: "resize_window with a negative width",
			cmd: action.Command{
				Name:         action.NameResizeWindow,
				ResizeWindow: action.ResizeWindowArgs{Width: -5, WidthSet: true},
			},
		},
		{
			// mimi#138: the conflicting pair reaching the daemon off the
			// socket, with nothing having looked at it — the CLI is not the
			// only place the rule runs.
			name: "resize_window asking for margins and for no margins",
			cmd: action.Command{
				Name:         action.NameResizeWindow,
				ResizeWindow: action.ResizeWindowArgs{UseMargin: true, NoMargin: true},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := &fakeDesktop{
				accessibilityErr: derrors.New(
					derrors.CodeAccessibilityDenied,
					"the desktop must not be reached",
				),
			}

			err := action.NewExecutor(desktop).ExecuteCommand(testCase.cmd)
			if err == nil {
				t.Fatalf("ExecuteCommand(%+v) error = nil, want an error", testCase.cmd)
			}

			if !derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf("ExecuteCommand(%+v) got %v, want CodeInvalidInput", testCase.cmd, err)
			}
		})
	}
}

func TestExecuteCommand_UnknownName(t *testing.T) {
	t.Parallel()

	err := action.NewExecutor(desktopWithWindows(0, -1)).ExecuteCommand(action.Command{
		Name: "left_click",
	})
	if err == nil {
		t.Fatal("ExecuteCommand(left_click) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}
