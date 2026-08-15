//nolint:testpackage // execLauncher is unexported; the seam under test is intentionally internal.
package service

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// stubPerm is the mode of a stand-in launchctl these tests put on PATH:
// executable, because the point of one is being run.
const stubPerm = 0o755

// writeStubLaunchctl puts a launchctl of the test's own in dir and makes dir
// the whole of PATH, so that what [execLauncher] shells out to is a script the
// test wrote rather than the machine's real launchd. body is a shell script
// without its shebang.
//
// dir is the caller's rather than this function's because a stub that reports
// anything back does it through a file beside itself, and the body naming that
// file has to be written before there is a stub to write.
func writeStubLaunchctl(t *testing.T, dir, body string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(dir, "launchctl"), []byte("#!/bin/sh\n"+body), stubPerm)
	if err != nil {
		t.Fatalf("writing the stub launchctl: %v", err)
	}

	t.Setenv("PATH", dir)
}

// TestExecLauncher_List_ReportsALaunchctlThatCouldNotRun covers half of the
// distinction the rest of this package is built on, in the one place it is
// actually drawn: everywhere else a fake launcher supplies the three states
// directly, and here a failed `launchctl list` has to be read as two different
// things. The other half — launchctl running and reporting no such job — needs
// a real launchctl, and lives in the integration tier.
func TestExecLauncher_List_ReportsALaunchctlThatCouldNotRun(t *testing.T) {
	// No PATH, so there is no launchctl to run and nothing ever asks launchd
	// anything. Answering "not loaded" here is the bug this whole change is
	// about.
	t.Setenv("PATH", "")

	loaded, err := execLauncher{}.list(context.Background(), Label)
	if err == nil {
		t.Fatal("list() = nil, want the failure to run launchctl at all")
	}

	if loaded {
		t.Error("list() reported a job loaded on a launchctl that never ran")
	}
}

// TestExecLauncher_Kickstart_RestartsRatherThanStarts pins the one argument
// that separates the two. `launchctl kickstart` on a job that is already
// running does nothing at all; only -k kills it first, and a restart that
// silently left the old process up is a restart that never picked up the
// config it was run for. The fake launcher elsewhere is handed a target and
// cannot see the flag, so this is the only place it is checked.
func TestExecLauncher_Kickstart_RestartsRatherThanStarts(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")

	// A launchctl that records what it was asked to do and succeeds, so what
	// this asserts is the command line rather than anything launchd did.
	writeStubLaunchctl(t, dir, "printf '%s\\n' \"$@\" > "+argsFile+"\n")

	target := "gui/501/" + Label

	err := execLauncher{}.kickstart(context.Background(), target)
	if err != nil {
		t.Fatalf("kickstart() = %v, want nil", err)
	}

	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading what the stub launchctl was asked: %v", err)
	}

	want := "kickstart\n-k\n" + target + "\n"
	if string(recorded) != want {
		t.Errorf("kickstart() ran launchctl with %q, want %q", string(recorded), want)
	}
}

// TestExecLauncher_List_DoesNotReadACanceledCallAsAnAbsentJob covers the one
// way a launchctl that ran can still be no answer at all.
//
// CommandContext kills the process when the context is canceled, and a killed
// process is an exited process: it comes back as the *exec.ExitError that
// [execLauncher.list] otherwise reads as launchd saying no such job. Nothing
// about the error itself separates the two, so a canceled install would
// otherwise conclude the service is gone and bootstrap over a daemon that is
// still up — the reading #164 removed, walked back in through the context this
// package now threads.
func TestExecLauncher_List_DoesNotReadACanceledCallAsAnAbsentJob(t *testing.T) {
	dir := t.TempDir()

	// A launchctl that never finishes on its own, so the only thing that can
	// end this call is the cancellation under test. It reports that it is
	// running through a fifo, which is what lets the cancel land after the
	// process exists rather than before it: the two are different errors, and
	// only the later one is the one this is about.
	started := filepath.Join(dir, "started")

	err := syscall.Mkfifo(started, filePerm)
	if err != nil {
		t.Fatalf("creating the fifo the stub launchctl reports through: %v", err)
	}

	writeStubLaunchctl(t, dir, "echo started > "+started+"\nexec /bin/sleep 30\n")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	type answer struct {
		loaded bool
		err    error
	}

	answers := make(chan answer, 1)

	go func() {
		loaded, err := execLauncher{}.list(ctx, Label)
		answers <- answer{loaded: loaded, err: err}
	}()

	running := make(chan struct{})

	go func() {
		// Blocks until the stub opens its end, which it does once it is running.
		_, _ = os.ReadFile(started)

		close(running)
	}()

	var got answer

	select {
	case <-running:
		cancel()

		got = <-answers
	case got = <-answers:
		// Nothing ever started, so nothing was killed, and whatever this
		// answered is about a launchctl that could not run rather than one
		// that was canceled. Failing here says so, rather than leaving the
		// read above waiting for a stub that will never open its end.
		t.Fatalf("list() = (%v, %v) before the stub launchctl was running", got.loaded, got.err)
	}

	if got.err == nil {
		t.Fatal("list() = nil, want the cancellation reported rather than an answer about the job")
	}

	if got.loaded {
		t.Error("list() reported a job loaded on a call that was canceled")
	}
}
