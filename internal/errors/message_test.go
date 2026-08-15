package errors_test

import (
	stderrors "errors"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// errPlain stands in for an error raised outside this package: it carries no
// code, so Message has nothing to take off it.
//
//nolint:gochecknoglobals // a static error is what err113 asks for here
var errPlain = stderrors.New("macOS said no")

// TestMessage_LeavesTheCodeToTheCallerThatCarriesIt covers what Message
// exists for: a caller holding the code in a field of its own — the daemon's
// IPC response does — needs the text without it, so rebuilding the error from
// the pair does not show the code twice.
func TestMessage_LeavesTheCodeToTheCallerThatCarriesIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "no error", err: nil, want: ""},
		{
			name: "a coded error keeps only its message",
			err:  derrors.New(derrors.CodeInvalidInput, "space number 9 is out of range"),
			want: "space number 9 is out of range",
		},
		{
			name: "a wrapped error keeps the cause it names",
			err: derrors.Wrapf(
				errPlain,
				derrors.CodeActionFailed,
				"failed to activate window",
			),
			want: "failed to activate window: macOS said no",
		},
		{
			name: "an error from outside this package is left alone",
			err:  errPlain,
			want: "macOS said no",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := derrors.Message(testCase.err); got != testCase.want {
				t.Errorf("Message() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestMessage_RoundTripsThroughACodeAndMessagePair is the property the IPC
// response relies on: split a coded error into its code and its message, put
// it back together, and it reads exactly as it did.
func TestMessage_RoundTripsThroughACodeAndMessagePair(t *testing.T) {
	t.Parallel()

	want := derrors.New(
		derrors.CodeInvalidInput,
		"--backward cannot be combined with a direction flag",
	)

	got := derrors.New(derrors.GetCode(want), derrors.Message(want))
	if got.Error() != want.Error() {
		t.Errorf("round trip = %q, want %q", got.Error(), want.Error())
	}
}
