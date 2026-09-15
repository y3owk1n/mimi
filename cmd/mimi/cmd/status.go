package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
	"github.com/y3owk1n/mimi/internal/permissions"
)

func newStatusCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon and permission status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidPath, socketPath := state.runtimePaths()

			pid, err := readPID(pidPath)
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

			_, statErr := os.Stat(paths.ExpandHome(socketPath))
			if statErr == nil {
				cmd.Printf("ipc: socket available at %s\n", paths.ExpandHome(socketPath))
			} else {
				cmd.Println("ipc: socket not available (actions run directly until daemon starts)")
			}

			printProbe(cmd, socketPath)

			return nil
		},
	}
}

// printProbe adds what the daemon says about itself, when one answers: its
// build against this one, its uptime, its config, and what that config
// turns on. A daemon of another build gets one line saying so, since the
// mismatch is the answer.
func printProbe(cmd *cobra.Command, socketPath string) {
	status, err := probeDaemon(socketPath)
	if derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		return
	}

	if derrors.IsCode(err, derrors.CodeProtocolMismatch) ||
		derrors.IsCode(err, derrors.CodeInvalidInput) {
		cmd.Printf("daemon: another build than this CLI (%s), restart it\n", Version)

		return
	}

	if err != nil {
		cmd.Printf("daemon: could not be asked (%s)\n", derrors.Message(err))

		return
	}

	build := status.Version
	if status.Version != Version {
		build += fmt.Sprintf(" (this CLI is %s, restart the daemon)", Version)
	}

	cmd.Printf(
		"daemon: %s, up %s, config %s\n",
		build,
		time.Duration(status.UptimeSecs)*time.Second,
		status.ConfigPath,
	)
	cmd.Printf(
		"features: %d hook(s), tiling %s, borders %s, systray %s\n",
		status.Features.Hooks,
		onOff(status.Features.Tiling),
		onOff(status.Features.Borders),
		onOff(status.Features.Systray),
	)
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}

	return "off"
}
