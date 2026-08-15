package ipc //nolint:testpackage // drives the unexported request encoding directly

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// TestServer_RejectsARequestOnAnotherProtocolVersion pins the daemon half of
// mimi#128: a request whose envelope this build does not speak is refused
// before its command reaches the action runner, and refused under a code of
// its own so the CLI can tell it apart from an absent daemon.
//
// The raw request lines below are what a CLI built either side of the typed
// wire puts on the socket, which is why they are written as bytes rather than
// through writeRequest — writeRequest can only ever produce this build's
// version. The absent-version case is the dangerous one the change exists for:
// it decodes to a zero version and an empty command, which the action runner
// would otherwise happily act on.
func TestServer_RejectsARequestOnAnotherProtocolVersion(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantCode derrors.Code
		wantRun  bool
	}{
		{
			name: "a version this build does not speak",
			line: fmt.Sprintf(
				`{"version":%d,"command":{"name":"space","space":{"index":2,"direction":0}}}`,
				ProtocolVersion+1,
			),
			wantCode: derrors.CodeProtocolMismatch,
		},
		{
			name:     "no version at all, as a CLI built before the typed wire sends",
			line:     `{"action":"space","args":["2"]}`,
			wantCode: derrors.CodeProtocolMismatch,
		},
		{
			name: "this build's own version",
			line: fmt.Sprintf(
				`{"version":%d,"command":{"name":"space","space":{"index":2,"direction":0}}}`,
				ProtocolVersion,
			),
			wantRun: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ran := make(chan struct{}, 1)

			socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
			server := NewServer(socketPath)
			server.execute = func(_ action.Command) error {
				ran <- struct{}{}

				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			runDone := make(chan error, 1)
			go func() { runDone <- server.Run(ctx) }()

			waitForSocket(t, socketPath)

			resp := sendRawRequest(t, socketPath, testCase.line)

			cancel()
			<-runDone
			server.Shutdown()

			if testCase.wantRun {
				if !resp.OK {
					t.Fatalf("response = %+v, want ok for a matching protocol version", resp)
				}
			} else {
				if resp.OK {
					t.Fatalf("response = %+v, want a rejection", resp)
				}

				if resp.Code != string(testCase.wantCode) {
					t.Errorf("response code = %q, want %q", resp.Code, testCase.wantCode)
				}
			}

			select {
			case <-ran:
				if !testCase.wantRun {
					t.Error("the command reached the action runner despite the version mismatch")
				}
			default:
				if testCase.wantRun {
					t.Error("the command never reached the action runner")
				}
			}
		})
	}
}

// sendRawRequest writes one request line to the daemon's socket verbatim and
// returns the response it answers with.
func sendRawRequest(t *testing.T, socketPath, line string) Response {
	t.Helper()

	dialer := net.Dialer{Timeout: 2 * time.Second}

	conn, err := dialer.DialContext(t.Context(), "unix", socketPath)
	if err != nil {
		t.Fatalf("dialing the daemon socket: %v", err)
	}

	defer func() { _ = conn.Close() }()

	_, err = conn.Write([]byte(line + "\n"))
	if err != nil {
		t.Fatalf("writing the request line: %v", err)
	}

	resp, err := readResponse(bufio.NewReader(conn))
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}

	return resp
}
