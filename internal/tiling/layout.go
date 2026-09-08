package tiling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// maxLayoutOutputBytes caps what is kept of a layout's stderr for the error
// it is reported input.
const maxLayoutOutputBytes = 4096

// Layout turns an Input into an Output. The engine holds one and asks it on
// every pass; a Layout keeps no state of its own, the state travels input the
// Input and Output.
type Layout interface {
	Reduce(ctx context.Context, input Input) (Output, error)
}

// Program is the Layout the config names: a command line run through a
// shell, given the Input on stdin and read for the Output on stdout.
type Program struct {
	// Shell is the shell the command line runs under, as settings.hook_shell.
	Shell string
	// Command is the command line, as tiling.layout.
	Command string
	// Timeout bounds one run. The program is killed past it.
	Timeout time.Duration
}

// Reduce runs the program once. It fails when the program cannot be run,
// exits non-zero, times out, or prints something that is not an Output; an
// empty stdout is an empty Output.
func (p Program) Reduce(ctx context.Context, input Input) (Output, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return Output{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"encoding layout input",
		)
	}

	if p.Timeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, p.Shell, "-c", p.Command)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &limitedBuffer{buf: &stderr, limit: maxLayoutOutputBytes}

	err = cmd.Run()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Output{}, derrors.Newf(
				derrors.CodeActionFailed,
				"layout timed out after %s",
				p.Timeout,
			)
		}

		return Output{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"layout failed%s",
			stderrSuffix(stderr.String()),
		)
	}

	return decodeOutput(stdout.Bytes())
}

// decodeOutput reads one Output, treating a blank stdout as an empty one.
func decodeOutput(data []byte) (Output, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Output{}, nil
	}

	var out Output

	err := json.Unmarshal(data, &out)
	if err != nil {
		return Output{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"decoding layout output: expected {\"frames\": [...], \"state\": ...}",
		)
	}

	return out, nil
}

// stderrSuffix is what a failed layout wrote, trimmed and on one line, for
// its error; "" when it wrote nothing.
func stderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}

	return ": " + strings.ReplaceAll(stderr, "\n", " | ")
}

// limitedBuffer keeps the first limit bytes written and drops the rest, so a
// chatty program cannot grow the daemon's memory.
type limitedBuffer struct {
	buf   *bytes.Buffer
	limit int
}

func (l *limitedBuffer) Write(data []byte) (int, error) {
	room := l.limit - l.buf.Len()
	if room > 0 {
		if len(data) > room {
			data = data[:room]
		}

		l.buf.Write(data)
	}

	return len(data), nil
}
