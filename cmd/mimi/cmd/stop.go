package cmd

import (
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
)

func newStopCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running mimi daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidPath, _ := state.runtimePaths()

			pid, err := readPID(pidPath)
			if err != nil {
				cmd.Println("mimi: not running (no PID file)")

				//nolint:nilerr // no PID file means the daemon is not running, which is not a failure.
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

func readPID(path string) (int, error) {
	path = paths.ExpandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.TrimSpace(string(data)))
}
