package ipc //nolint:testpackage // shares the socket helpers in wire_test.go

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// TestStream_HandsEveryLineToTheClientUntilItHangsUp is the events
// request's path: the handler sends values as lines, the client reads them
// in order, and closing the client's context ends the handler.
func TestStream_HandsEveryLineToTheClientUntilItHangsUp(t *testing.T) {
	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	server := NewServer(socketPath)

	ended := make(chan struct{})
	server.HandleStream(
		action.NameEvents,
		func(ctx context.Context, _ action.Command, send func(any) error) error {
			defer close(ended)

			for _, n := range []int{1, 2, 3} {
				err := send(map[string]int{"n": n})
				if err != nil {
					return err
				}
			}

			<-ctx.Done()

			return nil
		},
	)

	serverCtx, stopServer := context.WithCancel(t.Context())
	defer stopServer()

	go func() { _ = server.Run(serverCtx) }()

	waitForSocket(t, socketPath)

	ctx, cancel := context.WithCancel(t.Context())

	var lines []string

	err := Stream(ctx, socketPath, action.NewEventsCommand(), func(line []byte) error {
		lines = append(lines, string(line))
		if len(lines) == 3 {
			cancel()
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	want := []string{"{\"n\":1}\n", "{\"n\":2}\n", "{\"n\":3}\n"}
	if len(lines) != 3 || lines[0] != want[0] || lines[1] != want[1] || lines[2] != want[2] {
		t.Fatalf("got %q, want %q", lines, want)
	}

	<-ended
}

func TestStream_ReportsADaemonThatIsNotThere(t *testing.T) {
	t.Parallel()

	err := Stream(
		t.Context(),
		filepath.Join(t.TempDir(), "missing.sock"),
		action.NewEventsCommand(),
		nil,
	)
	if !derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		t.Fatalf("got %v, want CodeDaemonUnavailable", err)
	}
}
