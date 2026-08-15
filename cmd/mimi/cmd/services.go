package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/service"
)

// defaultService is what the services commands run on: there is one launchd
// service mimi manages on this machine.
//
//nolint:gochecknoglobals // there is one machine, and one service on it
var defaultService = service.New()

// newServicesCmd builds the command tree that manages the system service used
// for automatic startup.
//
// macOS: backed by launchd, via internal/service.
func newServicesCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Manage the Mimi system service (macOS launchd)",
		Long: `Manage the Mimi system service for automatic startup on login.

On macOS, this manages a launchd plist so Mimi starts automatically
when you log in. Available on macOS only.

Subcommands:
  install     Install and load the system service
  uninstall   Unload and remove the system service
  start       Start the system service
  stop        Stop the system service
  restart     Restart the system service
  status      Check whether the service is loaded, and whether it is running`,
	}

	cmd.AddCommand(newServicesInstallCmd(state))
	cmd.AddCommand(newServicesUninstallCmd())
	cmd.AddCommand(newServicesStartCmd())
	cmd.AddCommand(newServicesStopCmd())
	cmd.AddCommand(newServicesRestartCmd())
	cmd.AddCommand(newServicesStatusCmd())

	return cmd
}

func newServicesInstallCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install and load the system service",
		Long:  "Install the Mimi launchd service so it starts automatically on login. Creates the plist file and loads it with launchctl.\n\nThe plist is a snapshot of the config taken at install time, so run install again after changing a setting it bakes in — it replaces the plist and reloads the service, and does nothing at all when the plist already matches.\n\nThe daemon's stdout and stderr are captured beside settings.log_file, as <name>.out.log and <name>.err.log, and that directory is created if missing. When log_file is unset — or is not an absolute path, which launchd cannot open — they fall back to /tmp/mimi.log and /tmp/mimi.err.log.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			outcome, err := defaultService.Install(state.configPath, state.logFilePath())
			if err != nil {
				return err
			}

			cmd.Println(formatInstallOutcome(outcome))

			return nil
		},
	}
}

func newServicesUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Unload and remove the system service",
		Long:  "Unload the Mimi launchd service and remove its plist file. Mimi will no longer start automatically on login.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := defaultService.Uninstall()
			if err != nil {
				return err
			}

			cmd.Println("Service uninstalled successfully")

			return nil
		},
	}
}

func newServicesStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the system service",
		Long:  "Start the Mimi launchd service. The daemon will begin running in the background.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := defaultService.Start()
			if err != nil {
				return err
			}

			cmd.Println("Service started")

			return nil
		},
	}
}

func newServicesStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the system service",
		Long:  "Stop the Mimi launchd service. The daemon process will be terminated.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := defaultService.Stop()
			if err != nil {
				return err
			}

			cmd.Println("Service stopped")

			return nil
		},
	}
}

func newServicesRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the system service",
		Long:  "Stop then immediately start the Mimi launchd service. Useful after configuration changes or to recover from an unresponsive state.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := defaultService.Restart()
			if err != nil {
				return err
			}

			cmd.Println("Service restarted")

			return nil
		},
	}
}

func newServicesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check the status of the system service",
		Long:  "Check whether the Mimi launchd service is currently loaded, and whether it is actually running.\n\nThe two are not the same: the installed plist sets KeepAlive, so a daemon that crashes at startup is relaunched every ten seconds and stays loaded the whole time. A running service reports the pid it runs under; a loaded one that is not running reports the status it last exited with — a repeated non-zero status there is a daemon in a crash loop, and the captured stderr beside settings.log_file says why.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(formatStatus(defaultService.Status()))

			return nil
		},
	}
}

// formatInstallOutcome renders what `mimi services install` did. Install is
// idempotent, so the line has to say which of the three things happened —
// "installed successfully" printed over an untouched service is how a stale
// plist stays believed-current.
func formatInstallOutcome(outcome service.InstallOutcome) string {
	switch outcome {
	case service.InstallOutcomeInstalled:
		return "Service installed and loaded successfully"
	case service.InstallOutcomeReplaced:
		return "Service plist updated and service reloaded"
	case service.InstallOutcomeUnchanged:
		return "Service already up to date"
	default:
		// An outcome added without a line of its own. The install returned
		// no error, so say only that much.
		return "Service install completed"
	}
}

// formatStatus renders a service.Status as the one line `mimi services status`
// prints.
//
// Loaded and running are not the same thing, and that is the point of the
// line: the installed plist sets KeepAlive, so a daemon that crashes at
// startup is relaunched every ten seconds forever while staying exactly as
// loaded as a healthy one. The pid says it is up; the exit status it left
// behind says it is not. When launchd's description carries neither — it is
// undocumented text and may change shape — this falls back to the two words
// the command printed before it ever asked.
func formatStatus(status service.Status) string {
	if !status.Loaded {
		return "Service not loaded"
	}

	switch {
	case status.PID.Known:
		return fmt.Sprintf("Service loaded and running (pid %d)", status.PID.Value)
	case status.LastExitStatus.Known:
		return fmt.Sprintf(
			"Service loaded but not running (last exit status %d)",
			status.LastExitStatus.Value,
		)
	default:
		return "Service loaded"
	}
}
