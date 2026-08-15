package ipc_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
)

func TestTryExecuteDaemonUnavailable(t *testing.T) {
	t.Parallel()

	cmd := action.Command{Name: action.NameSpace, Space: action.SpaceArg{Index: 1}}

	err := ipc.TryExecute(filepath.Join(t.TempDir(), "missing.sock"), cmd)
	if err == nil {
		t.Fatal("expected error for missing socket")
	}

	if !derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		t.Fatalf("expected CodeDaemonUnavailable, got %v", err)
	}
}

func TestServerClientRoundTrip(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "mimi.sock")
	server := ipc.NewServer(socketPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dialer := net.Dialer{}

		_, dialErr := dialer.DialContext(ctx, "unix", socketPath)
		if dialErr == nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	err := ipc.TryExecute(socketPath, action.Command{Name: "unknown_action"})
	if err == nil {
		t.Fatal("expected error for unknown action")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}

	cancel()
	<-errCh
	server.Shutdown()
}
