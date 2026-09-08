//nolint:testpackage
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const framesJSON = `[{"number":4242,"frame":{"x":0,"y":25,"width":960,"height":1055}}]`

// framesCommandWith builds an apply_frames command whose stdin is the given
// text, with a state that reaches no daemon.
func framesCommandWith(t *testing.T, stdin string, argv ...string) (*cobra.Command, error) {
	t.Helper()

	cmd := buildApplyFramesCommand(&cliState{configPath: filepath.Join(t.TempDir(), "none.toml")})
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(argv)

	return cmd, cmd.ParseFlags(argv)
}

func TestApplyFramesCommand_ReadsTheFramesFromStdin(t *testing.T) {
	t.Parallel()

	cmd, err := framesCommandWith(t, framesJSON)
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	frames, err := readFramesPayload(cmd)
	if err != nil {
		t.Fatalf("readFramesPayload() error = %v, want nil", err)
	}

	if len(frames) != 1 || frames[0].Number != 4242 || frames[0].Frame.Width != 960 {
		t.Fatalf("readFramesPayload() = %+v, want window 4242 at width 960", frames)
	}
}

func TestApplyFramesCommand_ReadsTheFramesFromAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "frames.json")

	err := os.WriteFile(path, []byte(framesJSON), 0o600)
	if err != nil {
		t.Fatalf("writing frames: %v", err)
	}

	cmd, err := framesCommandWith(t, "not read", "--file", path)
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	frames, err := readFramesPayload(cmd)
	if err != nil {
		t.Fatalf("readFramesPayload() error = %v, want nil", err)
	}

	if len(frames) != 1 || frames[0].Number != 4242 {
		t.Fatalf("readFramesPayload() = %+v, want window 4242", frames)
	}
}

func TestApplyFramesCommand_RejectsAMalformedPayloadAsInvalidInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		stdin string
	}{
		{name: "not json", stdin: "left-half"},
		{name: "an object instead of an array", stdin: `{"number":1}`},
		{name: "two documents", stdin: framesJSON + framesJSON},
		{name: "empty", stdin: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd, err := framesCommandWith(t, testCase.stdin)
			if err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}

			_, err = readFramesPayload(cmd)
			if !derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf("readFramesPayload() error = %v, want CodeInvalidInput", err)
			}
		})
	}
}
