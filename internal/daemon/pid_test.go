package daemon_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/daemon"
)

// sleeperEnv makes the copy of this test binary that startSleeper starts
// sleep instead of running tests. The copy has the same process name as this
// one, the way a second mimi has the same name as the first.
const sleeperEnv = "MIMI_TEST_SLEEPER"

func TestHelperSleeper(t *testing.T) {
	if os.Getenv(sleeperEnv) != "1" {
		t.Skip("only runs as the sleeper startSleeper starts")
	}

	time.Sleep(time.Minute)
}

// startSleeper starts name, or a sleeping copy of this test binary when name
// is empty, and returns its pid. Cleanup kills the process when the test ends.
func startSleeper(t *testing.T, name string) int {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), name, "60")
	if name == "" {
		cmd = exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHelperSleeper$")

		cmd.Env = append(os.Environ(), sleeperEnv+"=1")
	}

	err := cmd.Start()
	if err != nil {
		t.Fatalf("starting %q: %v", name, err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	return cmd.Process.Pid
}

// otherMimiPID is the pid of a second process of this program.
func otherMimiPID(t *testing.T) int {
	t.Helper()

	return startSleeper(t, "")
}

// otherProgramPID is the pid of a live process of another program.
func otherProgramPID(t *testing.T) int {
	t.Helper()

	return startSleeper(t, "sleep")
}

// exitedPID is the pid of a process that has already exited.
func exitedPID(t *testing.T) int {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "true")

	err := cmd.Run()
	if err != nil {
		t.Fatalf("running true: %v", err)
	}

	return cmd.Process.Pid
}

func writePIDFile(t *testing.T, pid int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mimi.pid")

	err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
	if err != nil {
		t.Fatalf("writing pid file: %v", err)
	}

	return path
}

// TestRunningPID pins what counts as a running daemon, which is a live process
// with mimi's own name. A PID file that outlived its daemon names a process
// that has exited or one that took the pid since, and mimi must signal
// neither.
func TestRunningPID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		pid         func(t *testing.T) int
		wantRunning bool
	}{
		{name: "this process", pid: func(*testing.T) int { return os.Getpid() }, wantRunning: true},
		{name: "another process of this program", pid: otherMimiPID, wantRunning: true},
		{name: "another program", pid: otherProgramPID},
		{name: "a process that exited", pid: exitedPID},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			want := testCase.pid(t)

			pid, running := daemon.RunningPID(writePIDFile(t, want))
			if pid != want || running != testCase.wantRunning {
				t.Errorf(
					"RunningPID = %d, %v; want %d, %v",
					pid,
					running,
					want,
					testCase.wantRunning,
				)
			}
		})
	}
}

func TestRunningPID_WithoutAFileIsNotRunning(t *testing.T) {
	t.Parallel()

	pid, running := daemon.RunningPID(filepath.Join(t.TempDir(), "mimi.pid"))
	if pid != 0 || running {
		t.Errorf("RunningPID = %d, %v; want 0, false", pid, running)
	}
}

// TestRun_RefusesWhileAnotherMimiRuns pins that a second daemon stops before
// it starts anything, since it would take over the first one's socket.
func TestRun_RefusesWhileAnotherMimiRuns(t *testing.T) {
	t.Parallel()

	other := startSleeper(t, "")
	cfg := &config.Config{}
	cfg.Settings.PIDFile = writePIDFile(t, other)

	done := make(chan error, 1)

	go func() { done <- daemon.Run(cfg, zap.NewNop().Sugar(), "", "test") }()

	select {
	case err := <-done:
		if err == nil ||
			!strings.Contains(err.Error(), "already running (pid "+strconv.Itoa(other)+")") {
			t.Fatalf("Run() = %v, want it to refuse with the other daemon's pid", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() started a second daemon instead of refusing")
	}
}
