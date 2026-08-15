package ipc //nolint:testpackage // pins the unexported request encoding

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
)

// payloadCase is one command the wire has to carry unchanged, under the name
// its failure should be reported by.
type payloadCase struct {
	name string
	cmd  action.Command
}

// everyPayloadSet is one command per action carrying a non-zero value in
// every field of that action's payload, next to the same action carrying
// nothing but its name.
//
// The pairs need not be commands the CLI would build: --backward alongside a
// direction, or a space index alongside next/prev, are exactly the payloads a
// malformed sender can put on the socket. Carrying them faithfully is what
// this file is about; rejecting them is action.ExecuteCommand's job, and
// TestExecuteCommand_RejectsAPayloadNoConstructorWouldBuild is where that
// lives.
func everyPayloadSet() []payloadCase {
	return []payloadCase{
		{
			name: "focus_window with nothing set",
			cmd:  action.Command{Name: action.NameFocusWindow},
		},
		{
			name: "focus_window with every field set",
			cmd: action.Command{
				Name:        action.NameFocusWindow,
				FocusWindow: action.FocusWindowArgs{Backward: true, Direction: "right"},
			},
		},
		{
			name: "space with nothing set",
			cmd:  action.Command{Name: action.NameSpace},
		},
		{
			name: "space with every field set",
			cmd: action.Command{
				Name:  action.NameSpace,
				Space: action.SpaceArg{Index: 4, Direction: -1},
			},
		},
		{
			name: "move_window_to_space with nothing set",
			cmd:  action.Command{Name: action.NameMoveWindowToSpace},
		},
		{
			name: "move_window_to_space with every field set",
			cmd: action.Command{
				Name:              action.NameMoveWindowToSpace,
				MoveWindowToSpace: action.SpaceArg{Index: 7, Direction: 1},
			},
		},
		{
			name: "resize_window with nothing set",
			cmd:  action.Command{Name: action.NameResizeWindow},
		},
		{
			name: "resize_window with every field set",
			cmd: action.Command{
				Name: action.NameResizeWindow,
				ResizeWindow: action.ResizeWindowArgs{
					Preset:           "left-half",
					Width:            800,
					WidthSet:         true,
					Height:           600,
					HeightSet:        true,
					WidthPercent:     45,
					WidthPercentSet:  true,
					HeightPercent:    55,
					HeightPercentSet: true,
					X:                100,
					XSet:             true,
					Y:                230,
					YSet:             true,
					Anchor:           "cc",
					AnchorSet:        true,
					UseMargin:        true,
					NoMargin:         true,
				},
			},
		},
	}
}

// TestRequest_RoundTripsEveryAction is the first of the two checks that pin
// the wire: an encoding that cannot decode its own output is caught here.
//
// It covers every action twice — once carrying nothing but the action's name,
// once carrying a non-zero value in every field of that action's payload — so
// a field that encodes but does not decode fails rather than passing silently
// at its zero value.
func TestRequest_RoundTripsEveryAction(t *testing.T) {
	t.Parallel()

	for _, testCase := range everyPayloadSet() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			req := Request{Version: ProtocolVersion, Command: testCase.cmd}

			err := writeRequest(&buf, req)
			if err != nil {
				t.Fatalf("writeRequest(%+v) error = %v", req, err)
			}

			got, err := readRequest(bufio.NewReader(&buf))
			if err != nil {
				t.Fatalf("readRequest() error = %v", err)
			}

			if !reflect.DeepEqual(got, req) {
				t.Errorf("round trip = %+v, want %+v", got, req)
			}
		})
	}
}

// TestRequest_EncodesTheGoldenBytes is the second check, and the one the round
// trip cannot make: a renamed field round-trips perfectly against itself and
// still breaks every daemon that has not restarted. The bytes below are what a
// running daemon of this protocol version reads, so changing them is a
// protocol change — bump ProtocolVersion with it, and a daemon still running
// the old build will reject this build's requests rather than misread them.
//
// One command per action, each built the way the CLI builds it. Within a
// payload that is encoded at all, every field is, whether or not it carries a
// value — so one command per action pins every field name the wire has. A
// payload still at its zero value is left out entirely, which is a wire shape
// of its own and gets the case below it: `mimi action focus_window` with no
// flags, the commonest invocation there is.
func TestRequest_EncodesTheGoldenBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() (action.Command, error)
		want  string
	}{
		{
			name: "mimi action focus_window --backward",
			build: func() (action.Command, error) {
				return action.NewFocusWindowCommand(true, false, false, false, false)
			},
			want: `{"version":1,"command":{"name":"focus_window",` +
				`"focusWindow":{"backward":true,"direction":""}}}`,
		},
		{
			name: "mimi action focus_window",
			build: func() (action.Command, error) {
				return action.NewFocusWindowCommand(false, false, false, false, false)
			},
			want: `{"version":1,"command":{"name":"focus_window"}}`,
		},
		{
			name: "mimi action space 3",
			build: func() (action.Command, error) {
				return action.NewSpaceCommand([]string{"3"})
			},
			want: `{"version":1,"command":{"name":"space",` +
				`"space":{"index":3,"direction":0}}}`,
		},
		{
			name: "mimi action move_window_to_space next",
			build: func() (action.Command, error) {
				return action.NewMoveWindowToSpaceCommand([]string{"next"})
			},
			want: `{"version":1,"command":{"name":"move_window_to_space",` +
				`"moveWindowToSpace":{"index":0,"direction":1}}}`,
		},
		{
			name: "mimi action resize_window left-half --width 800 --anchor cc --no-margin",
			build: func() (action.Command, error) {
				return action.NewResizeWindowCommand(action.ResizeWindowArgs{
					Preset:    "left-half",
					Width:     800,
					WidthSet:  true,
					Anchor:    "cc",
					AnchorSet: true,
					NoMargin:  true,
				})
			},
			want: `{"version":1,"command":{"name":"resize_window",` +
				`"resizeWindow":{"preset":"left-half",` +
				`"width":800,"widthSet":true,` +
				`"height":0,"heightSet":false,` +
				`"widthPercent":0,"widthPercentSet":false,` +
				`"heightPercent":0,"heightPercentSet":false,` +
				`"x":0,"xSet":false,` +
				`"y":0,"ySet":false,` +
				`"anchor":"cc","anchorSet":true,` +
				`"useMargin":false,"noMargin":true}}}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd, err := testCase.build()
			if err != nil {
				t.Fatalf("building %s: %v", testCase.name, err)
			}

			var buf bytes.Buffer

			err = writeRequest(&buf, Request{Version: ProtocolVersion, Command: cmd})
			if err != nil {
				t.Fatalf("writeRequest() error = %v", err)
			}

			got := strings.TrimSuffix(buf.String(), "\n")
			if got != testCase.want {
				t.Errorf("wire bytes changed — this is a protocol change:\n got: %s\nwant: %s",
					got, testCase.want)
			}
		})
	}
}

// TestServer_RunsTheCommandTheClientBuiltUnchanged is the success-path half of
// the two paths behaving identically: whatever the CLI hands TryExecute is
// what the daemon's action runner receives, field for field, for every action.
// The rejection half — the same command failing in the same words on both
// paths — is TestServerClientRoundTrip.
//
// The runner is injected rather than left at action.ExecuteCommand, because a
// command that got this far on a machine with Accessibility granted would move
// a real window.
func TestServer_RunsTheCommandTheClientBuiltUnchanged(t *testing.T) {
	socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
	server := NewServer(socketPath)

	received := make(chan action.Command, 1)
	server.execute = func(cmd action.Command) error {
		received <- cmd

		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- server.Run(ctx) }()

	waitForSocket(t, socketPath)

	for _, testCase := range everyPayloadSet() {
		err := TryExecute(socketPath, testCase.cmd)
		if err != nil {
			t.Fatalf("TryExecute(%s) error = %v, want nil", testCase.name, err)
		}

		select {
		case got := <-received:
			if got != testCase.cmd {
				t.Errorf(
					"the daemon ran a different command than the client sent:\n got: %+v\nsent: %+v",
					got,
					testCase.cmd,
				)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s never reached the action runner", testCase.name)
		}
	}

	cancel()
	<-runDone
	server.Shutdown()
}

// shortSocketDir returns a short-lived directory for a Unix socket file.
// t.TempDir() embeds the full test name in the path, which for the long,
// descriptive name above blows past sockaddr_un's ~104-byte limit and makes
// bind(2) fail with EINVAL — os.MkdirTemp keeps the prefix short instead.
//
//nolint:usetesting // t.TempDir() is too long for a Unix socket path here; see comment above
func shortSocketDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "mimi-ipc")
	if err != nil {
		t.Fatalf("creating short socket dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// waitForSocket blocks until the server is accepting connections on path, or
// the test's patience runs out.
func waitForSocket(t *testing.T, path string) {
	t.Helper()

	dialer := net.Dialer{}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(t.Context(), "unix", path)
		if err == nil {
			_ = conn.Close()

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("no daemon accepted a connection on %s within 2s", path)
}
