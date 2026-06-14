package ipc //nolint:testpackage // tests unexported Server.handleConn / enqueueTimeout

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// TestServerHandleConnEnqueueTimeout verifies that handleConn returns
// an IPCF_FAILED "timed out enqueueing action" error when the action
// worker isn't consuming from actionCh, instead of blocking forever.
func TestServerHandleConnEnqueueTimeout(t *testing.T) {
	t.Parallel()

	server := NewServer(t.TempDir() + "/unused.sock")
	// Shrink the timeout for a fast test; the production default is 5s.
	server.enqueueTimeout = 50 * time.Millisecond

	// No worker goroutine is started, so actionCh has no consumer and
	// the unbuffered send in handleConn must time out.
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})

	handleDone := make(chan struct{})
	go func() {
		server.handleConn(serverConn)
		close(handleDone)
	}()

	// Write a request from the client side of the pipe.
	writeErr := writeRequest(
		clientConn,
		Request{Action: "anything", Args: nil},
	)
	if writeErr != nil {
		t.Fatalf("writeRequest: %v", writeErr)
	}

	// Read the response — handleConn should write a timeout error
	// after enqueueTimeout elapses.
	type readResult struct {
		resp Response
		err  error
	}

	readDone := make(chan readResult, 1)
	go func() {
		resp, err := readResponse(bufio.NewReader(clientConn))
		readDone <- readResult{resp: resp, err: err}
	}()

	select {
	case res := <-readDone:
		if res.err != nil {
			t.Fatalf("readResponse: %v", res.err)
		}

		if res.resp.OK {
			t.Fatalf("expected timeout error, got OK response: %+v", res.resp)
		}

		if res.resp.Code != string(derrors.CodeIPCFailed) {
			t.Fatalf("expected code %q, got %q", derrors.CodeIPCFailed, res.resp.Code)
		}

		if !strings.Contains(res.resp.Message, "timed out") {
			t.Fatalf("expected message to contain \"timed out\", got %q", res.resp.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not produce a response within 2s")
	}

	// handleConn must return after the timeout, leaving no goroutine
	// leak.
	select {
	case <-handleDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not return after writing the timeout response")
	}
}
