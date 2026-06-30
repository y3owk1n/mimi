package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
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

	flagWidth = "--width"
	flagUp    = "--up"
	flagDown  = "--down"
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

func TestIsResizePreset(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: presetLeftHalf, input: presetLeftHalf, want: true},
		{name: presetRightHalf, input: presetRightHalf, want: true},
		{name: presetTopHalf, input: presetTopHalf, want: true},
		{name: presetBottomHalf, input: presetBottomHalf, want: true},
		{name: presetTopLeft, input: presetTopLeft, want: true},
		{name: presetTopRight, input: presetTopRight, want: true},
		{name: presetBottomLeft, input: presetBottomLeft, want: true},
		{name: presetBottomRight, input: presetBottomRight, want: true},
		{name: presetCenter, input: presetCenter, want: true},
		{name: presetFill, input: presetFill, want: true},
		{name: "unknown", input: "left-third", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := action.IsResizePreset(tc.input); got != tc.want {
				t.Fatalf("IsResizePreset(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestExecute_ResizeWindowPresets(t *testing.T) {
	t.Parallel()

	presets := []string{
		presetLeftHalf, presetRightHalf, presetTopHalf, presetBottomHalf,
		presetTopLeft, presetTopRight, presetBottomLeft, presetBottomRight,
		presetCenter, presetFill,
	}

	for _, preset := range presets {
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

	err := action.Execute("resize_window", []string{"--anchor", "xx"})
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

	err := action.Execute("resize_window", []string{"--width-percent", "150"})
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
		"--height", "600",
		"--anchor", "cc",
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

	dirs := []string{flagUp, flagDown, "--left", "--right"}

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

	dirs := []string{flagUp, flagDown, "--left", "--right"}

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
