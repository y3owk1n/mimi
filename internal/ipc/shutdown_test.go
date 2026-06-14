package ipc //nolint:testpackage // tests unexported Server.actionCh

import (
	"testing"
)

func TestServerShutdownIdempotent(t *testing.T) {
	t.Parallel()

	server := NewServer(t.TempDir() + "/unused.sock")

	// Multiple Shutdown calls must not panic — sync.Once guards the close.
	server.Shutdown()
	server.Shutdown()
	server.Shutdown()
}

func TestServerShutdownClosesActionCh(t *testing.T) {
	t.Parallel()

	server := NewServer(t.TempDir() + "/unused.sock")

	// Drain the channel in the background so close() doesn't block on a
	// full unbuffered channel. Receiving from a closed channel returns
	// the zero value with ok=false, so the goroutine exits cleanly.
	done := make(chan struct{})
	go func() {
		for range server.actionCh {
		}

		close(done)
	}()

	server.Shutdown()

	// After Shutdown, the worker should see the channel closed and exit.
	<-done

	// Sending on a closed channel must panic — this is what sync.Once
	// is preventing when Shutdown is called more than once.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when sending on closed actionCh")
		}
	}()

	server.actionCh <- actionJob{}
}
