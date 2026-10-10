// Package shellexec runs a command line the user wrote through their shell,
// so that a timeout ends everything the line started.
package shellexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// waitDelay is how long Run waits, once the shell has exited or been killed,
// for whatever the line left running to close the shell's output.
const waitDelay = 500 * time.Millisecond

// Command is line run through shell -c in a process group of its own. When ctx
// ends, the command kills the whole group, not only the shell, so a line such
// as `sleep 5; true` or `uv run layout.py` cannot outlive its timeout through
// a child holding the output open. Nothing kills a child that the line leaves
// running in the background once the shell has exited.
func Command(ctx context.Context, shell, line string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, shell, "-c", line)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		return err
	}
	cmd.WaitDelay = waitDelay

	return cmd
}

// Run runs cmd and waits for it. If the shell exits cleanly while a child it
// left in the background still holds its output, Run counts that as success
// and returns without waiting for the child.
func Run(cmd *exec.Cmd) error {
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}

	return err
}
