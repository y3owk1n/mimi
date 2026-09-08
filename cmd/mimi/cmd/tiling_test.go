//nolint:testpackage
package cmd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// tilingConfigWith writes a config naming layout, with the socket at
// socketPath, and returns its path.
func tilingConfigWith(t *testing.T, socketPath, layout string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[settings]\nsocket_file = \"" + socketPath + "\"\n" +
		"[tiling]\nlayout = '" + layout + "'\n"

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("writing config: %v", err)
	}

	return path
}

// TestTilingCommands_ReachTheDaemonAsATilingAction pins the route a hotkey
// takes: the typed command over the socket, handled by whatever the daemon
// registered for it, and nothing run in the CLI.
func TestTilingCommands_ReachTheDaemonAsATilingAction(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	server := ipc.NewServer(socketPath)

	received := make(chan action.Command, 1)
	server.HandleDirect(action.NameTiling, func(cmd action.Command) error {
		received <- cmd

		return nil
	})

	ctx := t.Context()

	defer server.Shutdown()

	go func() { _ = server.Run(ctx) }()

	waitForSocket(t, socketPath)

	configPath := tilingConfigWith(t, socketPath, "exit 7")

	_, err := runCommand(t, "--config", configPath, "tiling", "cmd", "swap", "left")
	if err != nil {
		t.Fatalf("tiling cmd swap left: %v", err)
	}

	got := <-received
	want := action.TilingArgs{Kind: action.TilingCommand, Name: "swap", Args: []string{"left"}}

	if got.Name != action.NameTiling || got.Tiling.Kind != want.Kind ||
		got.Tiling.Name != want.Name || strings.Join(got.Tiling.Args, " ") != "left" {
		t.Fatalf("daemon received %+v, want tiling %+v", got, want)
	}
}

// TestTilingCommands_RunALocalEngineWithoutADaemon: no daemon, so the layout
// runs here. The layout below fails on purpose, which is how the test knows
// it ran, and its stderr is what the user sees.
func TestTilingCommands_RunALocalEngineWithoutADaemon(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(shortSocketDir(t), "none.sock")
	configPath := tilingConfigWith(t, socketPath, "echo ran-locally >&2; exit 3")

	_, err := runCommand(t, "--config", configPath, "tiling", "relayout")
	if err == nil || !strings.Contains(err.Error(), "ran-locally") {
		t.Fatalf("tiling relayout without a daemon = %v, want the local layout's failure", err)
	}
}

// TestTilingCmd_PassesADashArgumentToTheLayout pins that "ratio -0.05"
// reaches the layout as written: after the name, nothing is a flag.
func TestTilingCmd_PassesADashArgumentToTheLayout(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	server := ipc.NewServer(socketPath)

	received := make(chan action.Command, 1)
	server.HandleDirect(action.NameTiling, func(cmd action.Command) error {
		received <- cmd

		return nil
	})

	ctx := t.Context()

	defer server.Shutdown()

	go func() { _ = server.Run(ctx) }()

	waitForSocket(t, socketPath)

	configPath := tilingConfigWith(t, socketPath, "true")

	_, err := runCommand(t, "--config", configPath, "tiling", "cmd", "ratio", "-0.05")
	if err != nil {
		t.Fatalf("tiling cmd ratio -0.05: %v", err)
	}

	got := <-received
	if strings.Join(got.Tiling.Args, " ") != "-0.05" {
		t.Fatalf("layout would receive args %v, want [-0.05]", got.Tiling.Args)
	}
}

func TestTilingCommands_RejectAMalformedCommandBeforeAnyPath(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(shortSocketDir(t), "none.sock")
	configPath := tilingConfigWith(t, socketPath, "true")

	_, err := runCommand(t, "--config", configPath, "tiling", "cmd", "   ")
	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("tiling cmd with a blank name = %v, want CodeInvalidInput", err)
	}
}

// waitForSocket blocks until socketPath accepts a connection.
func waitForSocket(t *testing.T, socketPath string) {
	t.Helper()

	for range 200 {
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", socketPath)
		if err == nil {
			_ = conn.Close()

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("socket %s never came up", socketPath)
}
