//nolint:testpackage // drives the command tree through runCommand
package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/daemon"
)

// TestStatus_JSONCarriesWhatTheDaemonSays pins status --json against a daemon
// that answers on the socket: the report says the socket is there and holds
// the daemon's own status, its last reload included.
func TestStatus_JSONCarriesWhatTheDaemonSays(t *testing.T) {
	socket := filepath.Join(shortSocketDir(t), "mimi.sock")
	reloadingDaemon(t, socket, daemon.Reload{At: time.Now(), Trigger: "fsnotify", OK: true})

	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, configPath, fmt.Sprintf("[settings]\nsocket_file = %q\n", socket))

	// The fake answers its first request without a reload, so ask once
	// before the command does.
	_, _ = probeDaemon(socket)

	out, err := runCommand(t, "--config", configPath, "status", "--json")
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}

	var report statusReport

	err = json.Unmarshal([]byte(out), &report)
	if err != nil {
		t.Fatalf("status --json printed %q, not one JSON report: %v", out, err)
	}

	if !report.SocketAvailable || report.Socket != socket || report.Daemon == nil ||
		report.Daemon.Version != Version || report.Daemon.LastReload == nil ||
		report.Daemon.LastReload.Trigger != "fsnotify" {
		t.Fatalf("report = %+v, want the socket and the daemon's status with its reload", report)
	}
}
