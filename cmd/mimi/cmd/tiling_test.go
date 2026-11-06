//nolint:testpackage
package cmd

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
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
	server.HandleDirect(action.NameTiling, func(cmd action.Command) (json.RawMessage, error) {
		received <- cmd

		return nil, nil
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

// oneWindowDesktop is a desktop of one display with one window on it, so a
// tiling engine in the CLI has something to lay out without the real desktop.
// It accepts every write and drops it.
type oneWindowDesktop struct{}

func (oneWindowDesktop) Windows() (action.WindowsInfo, error) {
	return action.WindowsInfo{Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: action.Frame{Width: 500, Height: 500}},
	}}, nil
}

func (oneWindowDesktop) Displays() ([]action.DisplayEntry, error) {
	return []action.DisplayEntry{
		{Index: 1, ID: 7, Visible: action.Frame{Width: 1000, Height: 1000}},
	}, nil
}

func (oneWindowDesktop) ActiveSpaces() (map[uint32]int, error) {
	return map[uint32]int{7: 1}, nil
}

func (oneWindowDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	return map[uint32]uint64{7: 1001}, nil
}

func (oneWindowDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return map[uint32]bool{}, nil
}

func (oneWindowDesktop) Margins() (action.MarginsInfo, error) { return action.MarginsInfo{}, nil }

func (oneWindowDesktop) Apply([]action.WindowFrame, *action.Animation) error { return nil }

func (oneWindowDesktop) Focus(uint32) error { return nil }

// TestTilingCommands_RunALocalEngineWithoutADaemon: no daemon, so the layout
// runs here. The layout fails on purpose, which is how the test knows it ran,
// and its stderr is what the user sees.
func TestTilingCommands_RunALocalEngineWithoutADaemon(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(shortSocketDir(t), "none.sock")
	configPath := tilingConfigWith(t, socketPath, `echo ran-locally >&2; exit 3`)

	state := &cliState{tilingDesktop: oneWindowDesktop{}}

	_, err := runCommandWith(t, state, "--config", configPath, "tiling", "relayout")
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
	server.HandleDirect(action.NameTiling, func(cmd action.Command) (json.RawMessage, error) {
		received <- cmd

		return nil, nil
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

// TestInputOnlyLayout_RunsOnceWhateverTheConfigSays pins the fix for a preview
// that would not answer: --input swaps the layout for one that prints nothing,
// which reads as an empty output from a process that exits and never answers
// at all from a resident one. The substitute is run as a one-shot whatever
// mode the user's own layout is named with.
func TestInputOnlyLayout_RunsOnceWhateverTheConfigSays(t *testing.T) {
	t.Parallel()

	tests := []string{config.LayoutModeOneshot, config.LayoutModeResident, ""}

	for _, mode := range tests {
		t.Run("named as "+mode, func(t *testing.T) {
			t.Parallel()

			got := inputOnlyLayout(config.TilingConfig{
				Enabled:    true,
				Layout:     "my-layout",
				LayoutMode: mode,
			})

			if got.LayoutMode != config.LayoutModeOneshot {
				t.Fatalf("layout_mode = %q, want %q", got.LayoutMode, config.LayoutModeOneshot)
			}

			if got.Layout == "my-layout" {
				t.Fatal("the user's layout would be run, and --input runs none")
			}
		})
	}
}
