package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running mimi daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID(defaultPIDPath())
		if err != nil {
			cmd.Println("mimi: not running (no PID file)")

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

func readPID(path string) (int, error) {
	path = expandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func defaultPIDPath() string {
	return "~/.local/share/mimi/mimi.pid"
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
}
