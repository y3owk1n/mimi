//nolint:testpackage
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// shortSocketDir returns a short-lived directory for a Unix socket file.
// t.TempDir() embeds the full (sub)test name in the path, which for this
// test's long, descriptive name blows past sockaddr_un's ~104-byte limit and
// makes the real bind(2) below fail with EINVAL — os.MkdirTemp keeps the
// prefix short instead.
//
//nolint:usetesting // t.TempDir() is too long for a Unix socket path here; see comment above
func shortSocketDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "mimi-ipc")
	if err != nil {
		t.Fatalf("creating short socket dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// configWithSocket writes a minimal config setting socket_file to socketPath
// and returns the path it wrote it to — how a test points a command tree at a
// socket of its own rather than the default one.
func configWithSocket(t *testing.T, socketPath string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, configPath, fmt.Sprintf("[settings]\nsocket_file = %q\n", socketPath))

	return configPath
}

// stateWithSocketConfig returns a cliState pointed at a config whose
// socket_file is socketPath — the setup both routing tests below share.
func stateWithSocketConfig(t *testing.T, socketPath string) *cliState {
	t.Helper()

	return &cliState{configPath: configWithSocket(t, socketPath)}
}

// TestRunAction_RoutesToDaemonWhenListening pins the routing mimi#90 asks to
// be documented and tested: since PR #88, runAction resolves socket_file
// from the (now always-resolved) config path and finds a daemon listening on
// a custom socket where before #88 it always looked on the default one
// instead. A bare Unix listener stands in for the daemon here — it answers
// every request with ok:true, which an empty-name action.Command can never
// produce on its own (ExecuteCommand's default case always errors), so a nil
// result proves runAction reached the fake daemon rather than falling back.
func TestRunAction_RoutesToDaemonWhenListening(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	state := stateWithSocketConfig(t, socketPath)

	lc := net.ListenConfig{}

	listener, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		t.Fatalf("listening on fake daemon socket: %v", err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		// Drain the request line, then answer ok — the fake daemon never
		// needs to inspect what was asked of it for this test.
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		_, _ = conn.Write([]byte("{\"ok\":true}\n"))
	}()

	err = state.runAction(action.Command{})
	if err != nil {
		t.Fatalf(
			"runAction with a daemon listening on the configured socket = %v, want nil (it should have routed there instead of falling back to direct execution, which always errors on an empty action name)",
			err,
		)
	}
}

// TestRunAction_FallsBackToDirectWhenNoDaemonListening is
// TestRunAction_RoutesToDaemonWhenListening's mirror: the configured socket
// exists nowhere on disk, so runAction must fall back to direct execution —
// observable here because action.ExecuteCommand rejects the empty action
// name with CodeInvalidInput, the same error direct execution has always
// returned for an unrecognized action.
func TestRunAction_FallsBackToDirectWhenNoDaemonListening(t *testing.T) {
	t.Parallel()

	// never created — nothing listens here
	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	state := stateWithSocketConfig(t, socketPath)

	err := state.runAction(action.Command{})
	if err == nil {
		t.Fatal(
			"expected runAction to fall back to direct execution and fail on the empty action name",
		)
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput from direct execution's fallback, got %v", err)
	}
}
