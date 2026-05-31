package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running mimi daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID(defaultPIDPath())
		if err != nil {
			fmt.Println("mimi: not running (no PID file)")

			return nil
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("process %d not found: %w", pid, err)
		}

		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("signaling process %d: %w", pid, err)
		}

		fmt.Printf("Sent SIGTERM to mimi (pid %d)\n", pid)

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
