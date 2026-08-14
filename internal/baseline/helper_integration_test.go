//go:build integration

package baseline_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The recorder never touches a window it did not open, so it opens its own: a
// throwaway TextEdit instance holding one document per window it needs. This
// file owns that process — starting it, identifying it, and stopping it — and
// nothing about the windows themselves.
const (
	// helperApp is opened with -n, so it is always a fresh instance and never
	// the copy the person at the keyboard may already be using.
	helperApp = "TextEdit"
	// windowCount is one document per cell of the focus grid.
	windowCount = gridSide * gridSide

	launchTimeout = 45 * time.Second
	quitTimeout   = 10 * time.Second
)

// launchHelper starts a fresh helper instance holding windowCount documents,
// registers a cleanup that terminates exactly the processes it started, and
// returns their process IDs.
//
// It has to run before the first window enumeration. NSWorkspace only refreshes
// its running-application list from the run loop, which a test binary never
// spins, so an application launched after that list is first read stays
// invisible to the action layer for the rest of the process.
func launchHelper(t *testing.T) map[int]bool {
	t.Helper()

	dir := t.TempDir()

	const openFlags = 3

	args := make([]string, 0, openFlags+windowCount)
	args = append(args, "-n", "-a", helperApp)

	for index := range windowCount {
		path := filepath.Join(dir, fmt.Sprintf("mimi-baseline-%d.txt", index))

		err := os.WriteFile(path, []byte("mimi window baseline\n"), 0o600)
		if err != nil {
			t.Skipf("cannot write the helper document: %v", err)
		}

		args = append(args, path)
	}

	before := helperPIDs(t)

	out, err := exec.CommandContext(t.Context(), "open", args...).CombinedOutput()
	if err != nil {
		t.Skipf("cannot launch %s: %v: %s", helperApp, err, strings.TrimSpace(string(out)))
	}

	// The cleanup closes over started so that it stops exactly the processes
	// this launch produced, and never a helper somebody else opened meanwhile.
	started := map[int]bool{}
	t.Cleanup(func() { terminateHelper(t, started) })

	waitForHelperProcess(t, before, started)

	return started
}

// waitForHelperProcess waits for the helper instance to exist as a process,
// which it can do without reading the window list, and records what it saw into
// started.
//
// It polls from the moment the launch returns, so the only processes it can
// attribute to this launch are ones that appeared within a few milliseconds of
// it. Anything that shows up later is left alone — leaking a helper window is a
// nuisance, signaling somebody else's editor is not.
func waitForHelperProcess(t *testing.T, before, started map[int]bool) {
	t.Helper()

	deadline := time.Now().Add(launchTimeout)

	for {
		maps.Copy(started, helperPIDsSince(t, before))

		if len(started) > 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Skipf("%s did not start within %s", helperApp, launchTimeout)
		}

		time.Sleep(settleInterval)
	}
}

// terminateHelper stops the helper processes the launch confirmed, and only
// those. It runs as a cleanup, after t.Context is already canceled, so it uses
// its own context.
func terminateHelper(t *testing.T, started map[int]bool) {
	t.Helper()

	targets := slices.Collect(maps.Keys(started))

	signalHelpers(t, targets, syscall.SIGTERM)

	deadline := time.Now().Add(quitTimeout)
	for time.Now().Before(deadline) {
		if !anyRunning(t, targets) {
			return
		}

		time.Sleep(settleInterval)
	}

	signalHelpers(t, targets, syscall.SIGKILL)
}

// signalHelpers sends sig to each of the given helper processes.
func signalHelpers(t *testing.T, pids []int, sig syscall.Signal) {
	t.Helper()

	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			continue
		}

		err = process.Signal(sig)
		if err != nil {
			t.Logf("could not stop helper pid %d: %v", pid, err)
		}
	}
}

// anyRunning reports whether any of the given helper processes is still alive.
func anyRunning(t *testing.T, pids []int) bool {
	t.Helper()

	running := helperPIDs(t)
	for _, pid := range pids {
		if running[pid] {
			return true
		}
	}

	return false
}

// helperPIDsSince returns the running helper processes that were not in before.
func helperPIDsSince(t *testing.T, before map[int]bool) map[int]bool {
	t.Helper()

	since := map[int]bool{}

	for pid := range helperPIDs(t) {
		if !before[pid] {
			since[pid] = true
		}
	}

	return since
}

// helperPIDs returns the process IDs of every running helper instance. It uses
// its own context because it is also called from cleanup.
func helperPIDs(t *testing.T) map[int]bool {
	t.Helper()

	pids := map[int]bool{}

	out, err := exec.CommandContext(context.Background(), "pgrep", "-x", helperApp).Output()
	if err != nil {
		return pids
	}

	for field := range strings.FieldsSeq(string(out)) {
		pid, convErr := strconv.Atoi(field)
		if convErr == nil {
			pids[pid] = true
		}
	}

	return pids
}
