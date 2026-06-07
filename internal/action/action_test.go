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

	err := action.Execute("space", []string{"0"})
	if err == nil {
		t.Fatal("Execute(space 0) expected error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
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
