//nolint:testpackage
package cmd

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// TestStop_LeavesAloneAProcessThatTookAStalePID pins that mimi stop signals
// only a running mimi. After a crash the PID file names whatever process took
// the pid since, and SIGTERM would end it.
func TestStop_LeavesAloneAProcessThatTookAStalePID(t *testing.T) {
	xdg := isolateConfigHome(t)
	pidPath := filepath.Join(t.TempDir(), "mimi.pid")
	writeConfigFile(
		t,
		filepath.Join(xdg, "mimi", "config.toml"),
		"[settings]\npid_file = \""+pidPath+"\"\n",
	)

	other := exec.CommandContext(t.Context(), "sleep", "60")

	err := other.Start()
	if err != nil {
		t.Fatalf("starting sleep: %v", err)
	}

	t.Cleanup(func() {
		_ = other.Process.Kill()
		_ = other.Wait()
	})

	writeConfigFile(t, pidPath, strconv.Itoa(other.Process.Pid))

	out, err := runCommand(t, "stop")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	if !strings.Contains(out, "stale PID file") {
		t.Errorf("stop output = %q, want it to call the PID file stale", out)
	}

	signalErr := other.Process.Signal(syscall.Signal(0))
	if signalErr != nil {
		t.Fatalf("stop ended the process that took the stale pid: %v", signalErr)
	}
}
