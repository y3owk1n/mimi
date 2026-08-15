//nolint:testpackage // execLauncher is unexported; the seam under test is intentionally internal.
package service

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// stubPerm is the mode of the stand-in launchctl a cancellation test puts on
// PATH: executable, because the point of it is being run.
const stubPerm = 0o755

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

	stub := "#!/bin/sh\necho started > " + started + "\nexec /bin/sleep 30\n"

	err = os.WriteFile(filepath.Join(dir, "launchctl"), []byte(stub), stubPerm)
	if err != nil {
		t.Fatalf("writing the stub launchctl: %v", err)
	}

	t.Setenv("PATH", dir)

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
