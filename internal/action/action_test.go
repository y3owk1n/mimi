package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
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
		{name: "next", arg: "next"},
		{name: "prev", arg: "prev"},
		{name: "previous", arg: "previous"},
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
		{name: "next", arg: "next"},
		{name: "prev", arg: "prev"},
		{name: "previous", arg: "previous"},
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
