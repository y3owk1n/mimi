package cmd

import (
	"os"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func newStopCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running mimi daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidPath, _ := state.runtimePaths()

			pid, running := daemon.RunningPID(pidPath)
			if !running {
				cmd.Println(notRunning(pid))

				return nil
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "process %d not found", pid)
			}

			err = proc.Signal(syscall.SIGTERM)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "signaling process %d", pid)
			}

			cmd.Printf("Sent SIGTERM to mimi (pid %d)\n", pid)

			return nil
		},
	}
}

// notRunning says why no daemon is running, from the pid daemon.RunningPID
// read. A pid of 0 means there was no PID file, and any other pid is stale.
func notRunning(pid int) string {
	if pid == 0 {
		return "mimi: not running (no PID file)"
	}

	return "mimi: not running (stale PID file)"
}
