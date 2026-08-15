package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// TestNewFocusWindowArgs_BuildsTheTypedPayload pins the mapping from the
// CLI's already-typed bools onto FocusWindowArgs, without a string round
// trip.
func TestNewFocusWindowArgs_BuildsTheTypedPayload(t *testing.T) {
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

			got, err := action.NewFocusWindowArgs(
				testCase.backward,
				testCase.up,
				testCase.down,
				testCase.left,
				testCase.right,
			)
			if err != nil {
				t.Fatalf("NewFocusWindowArgs() error = %v, want nil", err)
			}

			if got != testCase.want {
				t.Fatalf("NewFocusWindowArgs() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestNewFocusWindowArgs_RejectsMoreThanOneDirection(t *testing.T) {
	t.Parallel()

	_, err := action.NewFocusWindowArgs(false, true, true, false, false)
	if err == nil {
		t.Fatal("NewFocusWindowArgs(up, down) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestNewFocusWindowArgs_RejectsBackwardWithDirection(t *testing.T) {
	t.Parallel()

	_, err := action.NewFocusWindowArgs(true, true, false, false, false)
	if err == nil {
		t.Fatal("NewFocusWindowArgs(backward, up) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

// TestResizeRequestFromArgs_MatchesParseResizeRequest pins that the typed
// direct-execution path resolves every ResizeWindowArgs case exactly the way
// ParseResizeRequest resolves the equivalent string flags, so the two paths
// can never quietly diverge.
func TestResizeRequestFromArgs_MatchesParseResizeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args action.ResizeWindowArgs
		want geometry.Request
	}{
		{
			name: "nothing set asks for nothing",
			args: action.ResizeWindowArgs{},
			want: geometry.Request{},
		},
		{
			name: "absolute sizes",
			args: action.ResizeWindowArgs{Width: 800, WidthSet: true, Height: 600, HeightSet: true},
			want: geometry.Request{Width: geometry.Absolute(800), Height: geometry.Absolute(600)},
		},
		{
			name: "percentage sizes",
			args: action.ResizeWindowArgs{
				WidthPercent: 45, WidthPercentSet: true,
				HeightPercent: 55, HeightPercentSet: true,
			},
			want: geometry.Request{Width: geometry.Percent(45), Height: geometry.Percent(55)},
		},
		{
			name: "an absolute size wins over a percentage one",
			args: action.ResizeWindowArgs{
				Width: 800, WidthSet: true,
				WidthPercent: 45, WidthPercentSet: true,
			},
			want: geometry.Request{Width: geometry.Absolute(800)},
		},
		{
			name: "a zero width, explicitly set, still keeps the current width",
			args: action.ResizeWindowArgs{Width: 0, WidthSet: true},
			want: geometry.Request{Width: geometry.Keep()},
		},
		{
			name: "an explicit position",
			args: action.ResizeWindowArgs{X: 100, XSet: true, Y: 230, YSet: true},
			want: geometry.Request{X: new(100.0), Y: new(230.0)},
		},
		{
			name: "an anchor",
			args: action.ResizeWindowArgs{Anchor: "br", AnchorSet: true},
			want: geometry.Request{Anchor: new(geometry.BottomRight)},
		},
		{
			name: "margins forced on",
			args: action.ResizeWindowArgs{UseMargin: true},
			want: geometry.Request{UseMargins: new(true)},
		},
		{
			name: "margins forced off",
			args: action.ResizeWindowArgs{NoMargin: true},
			want: geometry.Request{UseMargins: new(false)},
		},
		{
			name: "a preset carries through",
			args: action.ResizeWindowArgs{Preset: presetLeftHalf},
			want: geometry.Request{Preset: presetFor(t, presetLeftHalf)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ResizeRequestFromArgs(testCase.args)
			if err != nil {
				t.Fatalf("ResizeRequestFromArgs(%+v) error = %v", testCase.args, err)
			}

			assertRequest(t, got, testCase.want)
		})
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

// TestExecuteCommand_MirrorsExecute checks ExecuteCommand reaches the same
// executor methods Execute's string parsing does, for every action, using
// the same fake desktop tests already exercise Execute against.
func TestExecuteCommand_MirrorsExecute(t *testing.T) {
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
