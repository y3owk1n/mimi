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

func TestWriteReadRequestRoundTrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	req := Request{Action: "space", Args: []string{"2"}}

	err := writeRequest(&buf, req)
	if err != nil {
		t.Fatal(err)
	}

	got, err := readRequest(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}

	if got.Action != req.Action || len(got.Args) != 1 || got.Args[0] != "2" {
		t.Fatalf("unexpected request: %+v", got)
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
