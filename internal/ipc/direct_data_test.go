package ipc //nolint:testpackage // shares the socket helpers in wire_test.go

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
)

// TestTryExecuteData_ReturnsWhatADirectHandlerAnswered is the status
// request's path: a handler registered by name answers with data, and the
// client gets that data back untouched.
func TestTryExecuteData_ReturnsWhatADirectHandlerAnswered(t *testing.T) {
	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	server := NewServer(socketPath)
	server.HandleDirect(action.NameStatus, func(action.Command) (json.RawMessage, error) {
		return json.RawMessage(`{"version":"v1"}`), nil
	})

	ctx := t.Context()

	go func() { _ = server.Run(ctx) }()

	waitForSocket(t, socketPath)

	data, err := TryExecuteData(socketPath, action.NewStatusCommand())
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != `{"version":"v1"}` {
		t.Fatalf("got %s", data)
	}
}
