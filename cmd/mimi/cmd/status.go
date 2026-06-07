package cmd

import (
	"os"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/permissions"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon and permission status",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID(defaultPIDPath())
		if err != nil {
			cmd.Println("mimi: not running")
		} else {
			proc, findErr := os.FindProcess(pid)

			running := findErr == nil && proc.Signal(syscall.Signal(0)) == nil
			if running {
				cmd.Printf("mimi: running (pid %d)\n", pid)
			} else {
				cmd.Println("mimi: not running (stale PID file)")
			}
		}

		perm := permissions.Check()
		if perm.Accessibility {
			cmd.Println("accessibility: granted")
		} else {
			cmd.Println("accessibility: not granted (required for window hooks and actions)")
		}

		return nil
	},
}
