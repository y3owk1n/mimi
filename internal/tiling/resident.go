package tiling

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// The backoff after a resident layout fails: doubled per failure in a row,
// from the first up to the last.
const (
	residentBackoffFirst = 100 * time.Millisecond
	residentBackoffLast  = 5 * time.Second
	// maxLayoutLineBytes bounds one output line, so a layout that prints
	// without end cannot grow the daemon's memory.
	maxLayoutLineBytes = 8 << 20
)

// Resident is the Layout the config names with layout_mode = "resident": the
// same command line as Program, started once and kept running. Each pass
// writes the Input as one line on its stdin and reads the Output as one line
// from its stdout, so a layout skips its own startup on every pass after
// the first. A layout that exits, prints something that is not an Output,
// or takes longer than Timeout is stopped. The next pass starts it again,
// after a backoff that grows while it keeps failing.
type Resident struct {
	// Shell, Command and Timeout are as Program's. Timeout bounds one
	// pass, not the process.
	Shell   string
	Command string
	Timeout time.Duration

	logger *zap.SugaredLogger

	mu       sync.Mutex
	proc     *residentProcess
	failures int
	failedAt time.Time
}

// NewResident returns a Resident that starts its program on the first
// Reduce. A nil logger logs nothing.
func NewResident(
	shell, command string,
	timeout time.Duration,
	logger *zap.SugaredLogger,
) *Resident {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Resident{Shell: shell, Command: command, Timeout: timeout, logger: logger}
}

// residentProcess is one run of the program: its pipes, and the lines it
// prints, read by a goroutine of its own so a read can be given up on.
type residentProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *limitedBuffer
	lines  chan residentLine
	served int
}

type residentLine struct {
	data []byte
	err  error
}

// Reduce hands the input to the running program, starting it if it is not,
// and reads its answer. A program that exited between passes is started
// again and given the input once more, so that costs nothing. A failure
// while answering fails the pass.
func (r *Resident) Reduce(ctx context.Context, input Input) (Output, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return Output{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"encoding layout input",
		)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for attempt := range 2 {
		proc, startErr := r.ensureLocked()
		if startErr != nil {
			return Output{}, startErr
		}

		served := proc.served

		out, reduceErr := r.exchangeLocked(ctx, proc, payload)
		if reduceErr == nil {
			r.failures = 0

			return out, nil
		}

		stderr := r.stopLocked()

		// A program that had answered before and now gave end of file
		// exited between passes. It is started once more before the pass
		// is given up on.
		if attempt == 0 && served > 0 && errors.Is(reduceErr, errResidentGone) {
			r.logger.Debugw("resident layout exited; restarting")

			continue
		}

		r.failures++
		r.failedAt = time.Now()

		// What it wrote to stderr is read once it has exited, and is what
		// a program that answered nothing has to say for itself.
		if errors.Is(reduceErr, errNoAnswer) {
			return Output{}, derrors.Newf(
				derrors.CodeActionFailed,
				"layout exited without answering%s",
				stderrSuffix(stderr),
			)
		}

		return Output{}, reduceErr
	}

	return Output{}, derrors.New(derrors.CodeActionFailed, "layout failed")
}

// Stop ends the program, if it runs. The next Reduce starts it again.
func (r *Resident) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopLocked()
}

// errResidentGone is a program that gave end of file where an answer was
// due: it exited, or was killed. errLineTooLong is an output line past
// maxLayoutLineBytes.
var (
	errResidentGone = errors.New("layout exited")
	errNoAnswer     = errors.New("layout exited without answering")
	errLineTooLong  = errors.New("layout output line too long")
)

// ensureLocked is the running program, started if need be, or the reason
// it cannot be. The caller holds the lock.
func (r *Resident) ensureLocked() (*residentProcess, error) {
	if r.proc != nil {
		return r.proc, nil
	}

	if r.failures > 0 {
		backoff := min(residentBackoffFirst<<(r.failures-1), residentBackoffLast)
		if wait := time.Until(r.failedAt.Add(backoff)); wait > 0 {
			return nil, derrors.Newf(
				derrors.CodeActionFailed,
				"layout failed %d times in a row; restarting in %s",
				r.failures,
				wait.Round(time.Millisecond),
			)
		}
	}

	// The process outlives any one pass, so no pass's context bounds it.
	// stopLocked ends it.
	cmd := exec.CommandContext(context.Background(), r.Shell, "-c", r.Command)
	stderr := &limitedBuffer{buf: &bytes.Buffer{}, limit: maxLayoutOutputBytes}
	cmd.Stderr = stderr
	// Its own process group, so that killing it takes what it started
	// with it, which is what holds the pipes open otherwise.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeActionFailed, "starting layout")
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeActionFailed, "starting layout")
	}

	err = cmd.Start()
	if err != nil {
		r.failures++
		r.failedAt = time.Now()

		return nil, derrors.Wrapf(err, derrors.CodeActionFailed, "starting layout")
	}

	proc := &residentProcess{cmd: cmd, stdin: stdin, stderr: stderr, lines: make(chan residentLine)}
	go readLines(stdout, proc.lines)

	r.proc = proc
	r.logger.Debugw("resident layout started", "pid", cmd.Process.Pid)

	return proc, nil
}

// readLines hands each line stdout prints to lines, then the error that
// ended it, and closes lines.
func readLines(stdout io.Reader, lines chan<- residentLine) {
	defer close(lines)

	reader := bufio.NewReaderSize(stdout, 64<<10) //nolint:mnd

	for {
		data, err := readLine(reader)
		if len(data) > 0 {
			lines <- residentLine{data: data}
		}

		if err != nil {
			lines <- residentLine{err: err}

			return
		}
	}
}

// readLine reads one line without its newline, failing past
// maxLayoutLineBytes.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte

	for {
		chunk, err := reader.ReadSlice('\n')
		line = append(line, chunk...)

		if err == nil {
			return bytes.TrimRight(line, "\r\n"), nil
		}

		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}

		if len(line) > maxLayoutLineBytes {
			return nil, errLineTooLong
		}
	}
}

// exchangeLocked writes one input line and reads one output line, within
// Timeout. The caller holds the lock.
func (r *Resident) exchangeLocked(
	ctx context.Context,
	proc *residentProcess,
	payload []byte,
) (Output, error) {
	_, err := proc.stdin.Write(append(payload, '\n'))
	if err != nil {
		return Output{}, errResidentGone
	}

	var timeout <-chan time.Time
	if r.Timeout > 0 {
		timer := time.NewTimer(r.Timeout)
		defer timer.Stop()

		timeout = timer.C
	}

	select {
	case line, open := <-proc.lines:
		if open && line.err != nil && !errors.Is(line.err, io.EOF) {
			return Output{}, derrors.Wrapf(
				line.err,
				derrors.CodeActionFailed,
				"reading layout output",
			)
		}

		if !open || line.err != nil {
			if proc.served > 0 {
				return Output{}, errResidentGone
			}

			return Output{}, errNoAnswer
		}

		proc.served++

		out, decodeErr := decodeOutput(line.data)
		if decodeErr != nil {
			return Output{}, decodeErr
		}

		return out, nil
	case <-timeout:
		return Output{}, derrors.Newf(
			derrors.CodeActionFailed,
			"layout timed out after %s",
			r.Timeout,
		)
	case <-ctx.Done():
		return Output{}, derrors.Wrapf(ctx.Err(), derrors.CodeActionFailed, "layout pass canceled")
	}
}

// stopLocked ends the program and forgets it, returning what it wrote to
// stderr. The caller holds the lock.
func (r *Resident) stopLocked() string {
	proc := r.proc
	if proc == nil {
		return ""
	}

	r.proc = nil

	_ = proc.stdin.Close()

	// Closing stdin ends a well-behaved program at once. One that is stuck
	// is killed after a grace period, with everything it started.
	done := make(chan struct{})
	go func() {
		_ = proc.cmd.Wait()

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(residentBackoffFirst):
		_ = syscall.Kill(-proc.cmd.Process.Pid, syscall.SIGKILL)

		<-done
	}

	// Drain the reader so it can end.
	for range proc.lines {
	}

	return proc.stderr.buf.String()
}
