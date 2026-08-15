package cmd

import (
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
  status      Check whether the service is loaded and running`,
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
		Long:  "Install the Mimi launchd service so it starts automatically on login. Creates the plist file and loads it with launchctl.\n\nThe daemon's stdout and stderr are captured beside settings.log_file, as <name>.out.log and <name>.err.log, and that directory is created if missing. When log_file is unset — or is not an absolute path, which launchd cannot open — they fall back to /tmp/mimi.log and /tmp/mimi.err.log.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := defaultService.Install(state.configPath, state.logFilePath())
			if err != nil {
				return err
			}

			cmd.Println("Service installed and loaded successfully")

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
		Long:  "Check whether the Mimi launchd service is currently loaded and running. Displays whether the service is active.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(formatStatus(defaultService.Status()))

			return nil
		},
	}
}

// formatStatus renders a service.Status the way the CLI has always printed
// it, so nothing that scripts against the old string output sees a
// difference now that Status is a typed result.
func formatStatus(status service.Status) string {
	if status.Loaded {
		return "Service loaded"
	}

	return "Service not loaded"
}
