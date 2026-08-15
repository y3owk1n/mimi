package action_test

import (
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

const (
	nextKeyword     = "next"
	prevKeyword     = "prev"
	previousKeyword = "previous"

	presetLeftHalf    = "left-half"
	presetRightHalf   = "right-half"
	presetTopHalf     = "top-half"
	presetBottomHalf  = "bottom-half"
	presetTopLeft     = "top-left"
	presetTopRight    = "top-right"
	presetBottomLeft  = "bottom-left"
	presetBottomRight = "bottom-right"
	presetCenter      = "center"
	presetFill        = "fill"

	// unknownPreset is a name that is not one of the ten, and never becomes
	// one: the input every rejection case below is written against.
	unknownPreset = "left-third"

	flagWidth         = "--width"
	flagHeight        = "--height"
	flagWidthPercent  = "--width-percent"
	flagHeightPercent = "--height-percent"
	flagAnchor        = "--anchor"
	flagUp            = "--up"
	flagDown          = "--down"
	flagLeft          = "--left"
	flagRight         = "--right"
)

func TestIsKnownName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "focus_window", input: "focus_window", want: true},
		{name: "space", input: "space", want: true},
		{name: "move_window_to_space", input: "move_window_to_space", want: true},
		{name: "resize_window", input: "resize_window", want: true},
		{name: "unknown", input: "left_click", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := action.IsKnownName(tc.input); got != tc.want {
				t.Fatalf("IsKnownName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestExecute_InvalidAction(t *testing.T) {
	t.Parallel()

	err := action.Execute("left_click", nil)
	if err == nil {
		t.Fatal("Execute(left_click) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

// TestParseSpaceArg_AcceptedForms covers the accepted half of the one space
// argument rule: a 1-based number, "next", "prev", or "previous", surrounding
// whitespace included.
func TestParseSpaceArg_AcceptedForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arg  string
		want action.SpaceArg
	}{
		{name: "first space", arg: "1", want: action.SpaceArg{Index: 1}},
		{name: "padded number", arg: " 2 ", want: action.SpaceArg{Index: 2}},
		{name: nextKeyword, arg: nextKeyword, want: action.SpaceArg{Direction: 1}},
		{name: prevKeyword, arg: prevKeyword, want: action.SpaceArg{Direction: -1}},
		{name: previousKeyword, arg: previousKeyword, want: action.SpaceArg{Direction: -1}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, name := range []action.Name{action.NameSpace, action.NameMoveWindowToSpace} {
				got, err := action.ParseSpaceArg(name, []string{testCase.arg})
				if err != nil {
					t.Fatalf("ParseSpaceArg(%s, %q) error = %v, want nil", name, testCase.arg, err)
				}

				if got != testCase.want {
					t.Errorf(
						"ParseSpaceArg(%s, %q) = %+v, want %+v",
						name,
						testCase.arg,
						got,
						testCase.want,
					)
				}
			}
		})
	}
}

// TestParseSpaceArg_MalformedNamesTheAction checks a rejected argument is
// reported as invalid input and names the action it was given to — the only
// part of the wording that differs between the two space actions.
func TestParseSpaceArg_MalformedNamesTheAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{""}},
		{name: "whitespace only", args: []string{"   "}},
		{name: "non-numeric", args: []string{"foo"}},
		{name: "zero", args: []string{"0"}},
		{name: "negative", args: []string{"-1"}},
		{name: "no argument", args: nil},
		{name: "two arguments", args: []string{"1", "2"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, name := range []action.Name{action.NameSpace, action.NameMoveWindowToSpace} {
				_, err := action.ParseSpaceArg(name, testCase.args)
				if err == nil {
					t.Fatalf(
						"ParseSpaceArg(%s, %v) error = nil, want an error",
						name,
						testCase.args,
					)
				}

				if !derrors.IsCode(err, derrors.CodeInvalidInput) {
					t.Errorf(
						"ParseSpaceArg(%s, %v) got %v, want CodeInvalidInput",
						name,
						testCase.args,
						err,
					)
				}

				if !strings.Contains(err.Error(), string(name)) {
					t.Errorf(
						"ParseSpaceArg(%s, %v) did not name the action: %v",
						name,
						testCase.args,
						err,
					)
				}
			}
		})
	}
}

func TestExecute_SpaceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  string
		code derrors.Code
	}{
		{name: "zero", arg: "0", code: derrors.CodeInvalidInput},
		{name: "negative", arg: "-1", code: derrors.CodeInvalidInput},
		{name: "non-numeric", arg: "foo", code: derrors.CodeInvalidInput},
		{name: "empty", arg: "", code: derrors.CodeInvalidInput},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := action.Execute("space", []string{testCase.arg})
			if err == nil {
				t.Fatalf("Execute(space %q) expected error", testCase.arg)
			}

			if !derrors.IsCode(err, testCase.code) {
				t.Fatalf("Execute(space %q) got code %v, want %v", testCase.arg, err, testCase.code)
			}
		})
	}
}

func TestExecute_SpaceNextPrev(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arg  string
	}{
		{name: nextKeyword, arg: nextKeyword},
		{name: prevKeyword, arg: prevKeyword},
		{name: previousKeyword, arg: previousKeyword},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := action.Execute("space", []string{testCase.arg})

			if derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf(
					"Execute(space %q) got unexpected CodeInvalidInput; keyword should be recognized: %v",
					testCase.arg,
					err,
				)
			}
		})
	}
}

func TestExecute_MoveWindowToSpaceNextPrev(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arg  string
	}{
		{name: nextKeyword, arg: nextKeyword},
		{name: prevKeyword, arg: prevKeyword},
		{name: previousKeyword, arg: previousKeyword},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := action.Execute("move_window_to_space", []string{testCase.arg})

			if derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf(
					"Execute(move_window_to_space %q) got unexpected CodeInvalidInput; keyword should be recognized: %v",
					testCase.arg,
					err,
				)
			}
		})
	}
}

// everyPreset lists the ten preset names resize_window takes. The cases below
// are written against this list rather than against the geometry's own, so a
// preset silently leaving the table is a failure rather than a case that stops
// running.
func everyPreset() []string {
	return []string{
		presetLeftHalf, presetRightHalf, presetTopHalf, presetBottomHalf,
		presetTopLeft, presetTopRight, presetBottomLeft, presetBottomRight,
		presetCenter, presetFill,
	}
}

// presetFor is the preset a name names, for the cases that expect one to reach
// the geometry.
func presetFor(t *testing.T, name string) geometry.Preset {
	t.Helper()

	preset, ok := geometry.ParsePreset(name)
	if !ok {
		t.Fatalf("ParsePreset(%q) is not a preset", name)
	}

	return preset
}

// assertListsEveryPreset checks a rejection names all ten presets, which is
// what makes it useful to whoever mistyped one.
func assertListsEveryPreset(t *testing.T, err error) {
	t.Helper()

	for _, name := range everyPreset() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("rejection does not list %q: %v", name, err)
		}
	}
}

// TestResizePreset covers mimi#125: one rule decides whether a name is a
// preset, and it hands back the preset itself rather than a boolean beside it,
// so a caller cannot carry an unrecognized name past the check.
func TestResizePreset(t *testing.T) {
	t.Parallel()

	for _, name := range everyPreset() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ParseResizePreset(name)
			if err != nil {
				t.Fatalf("ParseResizePreset(%q) error = %v", name, err)
			}

			if got != presetFor(t, name) {
				t.Fatalf("ParseResizePreset(%q) = %q, want %q", name, got, name)
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()

		_, err := action.ParseResizePreset(unknownPreset)
		if err == nil {
			t.Fatalf("ParseResizePreset(%q) expected error", unknownPreset)
		}

		if !derrors.IsCode(err, derrors.CodeInvalidInput) {
			t.Fatalf("expected CodeInvalidInput, got %v", err)
		}

		assertListsEveryPreset(t, err)
	})
}

func TestExecute_ResizeWindowPresets(t *testing.T) {
	t.Parallel()

	for _, preset := range everyPreset() {
		t.Run(preset, func(t *testing.T) {
			t.Parallel()

			err := action.Execute("resize_window", []string{preset})

			if derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf(
					"Execute(resize_window %q) got unexpected CodeInvalidInput: %v",
					preset,
					err,
				)
			}
		})
	}
}

func TestExecute_ResizeWindowInvalidAnchor(t *testing.T) {
	t.Parallel()

	err := action.Execute("resize_window", []string{flagAnchor, "xx"})
	if err == nil {
		t.Fatal("Execute(resize_window --anchor xx) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestExecute_ResizeWindowInvalidWidth(t *testing.T) {
	t.Parallel()

	err := action.Execute("resize_window", []string{flagWidth, "-100"})
	if err == nil {
		t.Fatal("Execute(resize_window --width -100) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestExecute_ResizeWindowInvalidWidthPercent(t *testing.T) {
	t.Parallel()

	err := action.Execute("resize_window", []string{flagWidthPercent, "150"})
	if err == nil {
		t.Fatal("Execute(resize_window --width-percent 150) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestExecute_ResizeWindowWithFlags(t *testing.T) {
	t.Parallel()

	// This should parse correctly (no CodeInvalidInput) even though execution
	// will fail because there's no window open in the test environment.
	err := action.Execute("resize_window", []string{
		flagWidth, "800",
		flagHeight, "600",
		flagAnchor, "cc",
	})

	if derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("Execute with valid flags got unexpected CodeInvalidInput: %v", err)
	}
}

func TestExecute_ResizeWindowPresetWithOverride(t *testing.T) {
	t.Parallel()

	err := action.Execute("resize_window", []string{
		presetLeftHalf,
		flagWidth, "500",
	})

	if derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("Execute with preset and override got unexpected CodeInvalidInput: %v", err)
	}
}

func TestExecute_ResizeWindowMarginFlags(t *testing.T) {
	t.Parallel()

	err := action.Execute("resize_window", []string{presetLeftHalf, "--margin"})
	if derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("Execute with --margin got unexpected CodeInvalidInput: %v", err)
	}

	err = action.Execute("resize_window", []string{presetLeftHalf, "--no-margin"})
	if derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("Execute with --no-margin got unexpected CodeInvalidInput: %v", err)
	}
}

func TestExecute_FocusWindowInvalidFlag(t *testing.T) {
	t.Parallel()

	err := action.Execute("focus_window", []string{"--x=1"})
	if err == nil {
		t.Fatal("Execute(focus_window --x=1) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

func TestExecute_FocusWindowDirectionFlags(t *testing.T) {
	t.Parallel()

	dirs := []string{flagUp, flagDown, flagLeft, flagRight}

	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			err := action.Execute("focus_window", []string{dir})

			if derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf(
					"Execute(focus_window %s) got unexpected CodeInvalidInput: %v",
					dir,
					err,
				)
			}
		})
	}
}

func TestExecute_FocusWindowBackwardAndDirectionMutuallyExclusive(t *testing.T) {
	t.Parallel()

	dirs := []string{flagUp, flagDown, flagLeft, flagRight}

	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			err := action.Execute("focus_window", []string{"--backward", dir})
			if err == nil {
				t.Fatalf(
					"Execute(focus_window --backward %s) expected error for mutually exclusive flags",
					dir,
				)
			}

			if !derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf(
					"Execute(focus_window --backward %s) expected CodeInvalidInput, got %v",
					dir,
					err,
				)
			}
		})
	}
}

func TestExecute_FocusWindowOnlyOneDirectionAllowed(t *testing.T) {
	t.Parallel()

	err := action.Execute("focus_window", []string{"--up", "--down"})
	if err == nil {
		t.Fatal("Execute(focus_window --up --down) expected error for multiple direction flags")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}
