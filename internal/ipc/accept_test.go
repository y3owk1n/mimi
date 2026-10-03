package ipc //nolint:testpackage // drives the unexported accept loop with a fake listener

import (
	"context"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// failingListener fails every Accept with err, counting the calls.
type failingListener struct {
	err   error
	calls atomic.Int64
}

func (l *failingListener) Accept() (net.Conn, error) {
	l.calls.Add(1)

	return nil, l.err
}

func (l *failingListener) Close() error   { return nil }
func (l *failingListener) Addr() net.Addr { return &net.UnixAddr{Name: "test", Net: "unix"} }

// TestServer_Serve_BacksOffWhenAcceptKeepsFailing pins that a failure that
// persists, here running out of file descriptors, is retried a handful of
// times a second rather than in a loop that keeps a CPU core busy.
func TestServer_Serve_BacksOffWhenAcceptKeepsFailing(t *testing.T) {
	t.Parallel()

	listener := &failingListener{err: syscall.EMFILE}
	server := NewServer(t.TempDir() + "/mimi.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- server.serve(ctx, listener) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v, want nil once the context ends", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after the context ended")
	}

	// 5, 10, 20, 40, 80 and 160 ms of waiting fit in 300 ms, so about seven
	// tries. A loop with no wait makes millions.
	if calls := listener.calls.Load(); calls < 2 || calls > 12 {
		t.Errorf("accept was tried %d times in 300 ms, want a handful", calls)
	}
}

func TestServer_Serve_ReturnsWhenTheListenerCloses(t *testing.T) {
	t.Parallel()

	listener := &failingListener{err: net.ErrClosed}
	server := NewServer(t.TempDir() + "/mimi.sock")

	err := server.serve(context.Background(), listener)
	if err != nil {
		t.Errorf("serve returned %v, want nil for a closed listener", err)
	}

	if calls := listener.calls.Load(); calls != 1 {
		t.Errorf("accept was tried %d times, want 1", calls)
	}
}
