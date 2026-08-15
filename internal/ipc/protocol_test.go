package ipc //nolint:testpackage // tests unexported protocol functions

import (
	"bufio"
	"bytes"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func TestResponseFromError(t *testing.T) {
	t.Parallel()

	resp := responseFromError(nil)
	if !resp.OK {
		t.Fatal("expected ok response for nil error")
	}

	resp = responseFromError(derrors.New(derrors.CodeActionFailed, "boom"))
	if resp.OK || resp.Code != string(derrors.CodeActionFailed) {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

// TestErrorSurvivesTheResponseRoundTripUnchanged pins that an error the daemon
// answers with reads, back at the CLI, exactly as it read where it was raised.
// The code travels in its own field, so leaving it in the message too is what
// used to print it twice on the daemon path and once on the direct one.
func TestErrorSurvivesTheResponseRoundTripUnchanged(t *testing.T) {
	t.Parallel()

	tests := []error{
		derrors.New(
			derrors.CodeInvalidInput,
			"space number 9 is out of range; valid range is 1..3",
		),
		derrors.Wrapf(
			derrors.New(derrors.CodeAccessibilityFailed, "macOS said no"),
			derrors.CodeActionFailed,
			"failed to activate window",
		),
	}

	for _, want := range tests {
		t.Run(want.Error(), func(t *testing.T) {
			t.Parallel()

			got := errorFromResponse(responseFromError(want))
			if got == nil {
				t.Fatal("errorFromResponse() = nil, want an error")
			}

			if got.Error() != want.Error() {
				t.Errorf("round trip = %q, want %q", got.Error(), want.Error())
			}
		})
	}
}

func TestWriteReadResponseRoundTrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	resp := Response{OK: false, Code: "ACTION_FAILED", Message: "nope"}

	err := writeResponse(&buf, resp)
	if err != nil {
		t.Fatal(err)
	}

	got, err := readResponse(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}

	if got.OK || got.Code != resp.Code || got.Message != resp.Message {
		t.Fatalf("unexpected response: %+v", got)
	}
}
