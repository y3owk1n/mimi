package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
	"github.com/y3owk1n/mimi/internal/permissions"
)

func newStatusCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon and permission status",
		Long: `Show whether the daemon is running, whether this CLI holds Accessibility,
and whether the IPC socket is there. When a daemon answers, the command also
shows which build it is, what its config turns on, whether it holds
Accessibility, and how its last reload went. --json prints the same as one
line of JSON.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidPath, socketPath := state.runtimePaths()

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return printStatusJSON(cmd, pidPath, socketPath)
			}

			pid, running := daemon.RunningPID(pidPath)
			if running {
				cmd.Printf("mimi: running (pid %d)\n", pid)
			} else {
				cmd.Println(notRunning(pid))
			}

			perm := permissions.Check()
			if perm.Accessibility {
				cmd.Println("accessibility (this CLI): granted")
			} else {
				cmd.Println(
					"accessibility (this CLI): not granted (required for actions run without the daemon)",
				)
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

	cmd.Flags().Bool("json", false, "Print the status as JSON")

	return cmd
}

// statusReport is mimi status as JSON.
type statusReport struct {
	Running bool `json:"running"`
	// PID is the pid the PID file names, running or not, 0 without one.
	PID             int    `json:"pid,omitempty"`
	Accessibility   bool   `json:"accessibility"`
	Socket          string `json:"socket"`
	SocketAvailable bool   `json:"socketAvailable"`
	// Daemon is what the daemon says about itself, when one answered.
	Daemon *daemon.Status `json:"daemon,omitempty"`
	// DaemonError is why a daemon on the socket could not be asked.
	DaemonError string `json:"daemonError,omitempty"`
}

// printStatusJSON prints what mimi status reports as one line of JSON.
func printStatusJSON(cmd *cobra.Command, pidPath, socketPath string) error {
	report := statusReport{
		Accessibility: permissions.Check().Accessibility,
		Socket:        paths.ExpandHome(socketPath),
	}

	report.PID, report.Running = daemon.RunningPID(pidPath)

	_, statErr := os.Stat(report.Socket)
	report.SocketAvailable = statErr == nil

	status, err := probeDaemon(socketPath)

	switch {
	case err == nil:
		report.Daemon = &status
	case derrors.IsCode(err, derrors.CodeDaemonUnavailable):
	case derrors.IsCode(err, derrors.CodeProtocolMismatch),
		derrors.IsCode(err, derrors.CodeInvalidInput):
		report.DaemonError = "another build than this CLI (" + Version + "), restart it"
	default:
		report.DaemonError = derrors.Message(err)
	}

	err = json.NewEncoder(cmd.OutOrStdout()).Encode(report)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeSerializationFailed, "encoding status")
	}

	return nil
}

// printProbe adds what the daemon says about itself, when one answers: its
// build against this one, its uptime, its config, and what that config
// turns on. A daemon of another build gets one line saying so and nothing
// else.
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

	if status.Accessibility != nil {
		if *status.Accessibility {
			cmd.Println("accessibility (daemon): granted")
		} else {
			cmd.Println(
				"accessibility (daemon): not granted (required for hooks, tiling and borders)",
			)
		}
	}

	printLastReload(cmd, status.LastReload)
}

// printLastReload prints how the daemon's last reload went, when it has run
// one.
func printLastReload(cmd *cobra.Command, last *daemon.Reload) {
	if last == nil {
		return
	}

	ago := time.Since(last.At).Round(time.Second)

	if !last.OK {
		cmd.Printf("last reload: failed %s ago (%s): %s\n", ago, last.Trigger, last.Error)

		return
	}

	cmd.Printf("last reload: applied %s ago (%s)\n", ago, last.Trigger)

	if len(last.RestartOnly) > 0 {
		cmd.Printf("  restart to apply: %s\n", strings.Join(last.RestartOnly, ", "))
	}

	if len(last.ReinstallOnly) > 0 {
		cmd.Printf(
			"  mimi services install to apply: %s\n",
			strings.Join(last.ReinstallOnly, ", "),
		)
	}
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}

	return "off"
}
