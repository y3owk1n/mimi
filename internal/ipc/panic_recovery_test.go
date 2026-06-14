package ipc //nolint:testpackage // tests unexported startActionWorker recovery path

import (
	"strings"
	"testing"
	"time"
)

// TestActionWorkerRecoversFromPanic verifies that a panic inside the
// per-job action runner is recovered, the panic value is sent to
// job.done, and the worker keeps consuming further jobs (i.e. the
// panic is contained and doesn't kill the worker goroutine).
func TestActionWorkerRecoversFromPanic(t *testing.T) {
	t.Parallel()

	server := NewServer(t.TempDir() + "/unused.sock")
	server.execute = func(string, []string) error {
		panic("simulated action panic")
	}

	// startActionWorker spawns a goroutine that ranges over actionCh.
	server.startActionWorker()
	t.Cleanup(server.Shutdown)

	// 1) First job: expect the panic value wrapped into a non-nil error
	//    delivered to job.done.
	first := newJob()
	server.actionCh <- first.job

	select {
	case err := <-first.done:
		if err == nil {
			t.Fatal("expected error from panicked job, got nil")
		}

		if !strings.Contains(err.Error(), "action worker panic") {
			t.Fatalf("expected wrapped panic, got: %v", err)
		}

		if !strings.Contains(err.Error(), "simulated action panic") {
			t.Fatalf("expected panic value in error, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first job did not produce a result within 1s")
	}

	// 2) Replace the runner with a successful one, then send a second
	//    job to prove the worker is still alive and consuming.
	server.execute = func(string, []string) error { return nil }

	second := newJob()
	server.actionCh <- second.job

	select {
	case err := <-second.done:
		if err != nil {
			t.Fatalf("second job error after panic recovery: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second job did not produce a result within 1s — worker likely dead")
	}
}

// jobFixture is a tiny holder for an actionJob plus a pre-built done
// channel of sufficient buffering to receive a single error.
type jobFixture struct {
	job  actionJob
	done chan error
}

func newJob() jobFixture {
	done := make(chan error, 1)

	return jobFixture{
		job:  actionJob{name: "anything", args: nil, done: done},
		done: done,
	}
}
