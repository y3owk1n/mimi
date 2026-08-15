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

	// A command the daemon must reject once it has decoded it, and nothing had
	// checked before that: an action name that is not one, and — since the
	// socket is now a trust boundary rather than a parser's output — a payload
	// no constructor would have built. Each is checked against what the direct
	// path says about the same command, because the two paths reporting the
	// same thing in the same words is the point of carrying it typed.
	rejected := []action.Command{
		{Name: "unknown_action"},
		{
			Name:        action.NameFocusWindow,
			FocusWindow: action.FocusWindowArgs{Backward: true, Direction: "up"},
		},
		{Name: action.NameSpace},
	}

	for _, cmd := range rejected {
		err := ipc.TryExecute(socketPath, cmd)
		if err == nil {
			t.Fatalf("TryExecute(%+v) error = nil, want an error", cmd)
		}

		if !derrors.IsCode(err, derrors.CodeInvalidInput) {
			t.Fatalf("TryExecute(%+v) got %v, want CodeInvalidInput", cmd, err)
		}

		wantErr := action.ExecuteCommand(cmd)
		if wantErr == nil || err.Error() != wantErr.Error() {
			t.Errorf(
				"the daemon path and the direct path disagree about %+v:\n daemon: %v\n direct: %v",
				cmd,
				err,
				wantErr,
			)
		}
	}

	cancel()
	<-errCh
	server.Shutdown()
}
