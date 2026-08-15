package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/service"
)

// unknownStateLine is what a status prints when launchctl could not be run at
// all, and so could not be asked whether the service is loaded. It names
// launchctl because that, and not the service, is what has to be fixed before
// any of these commands can say anything.
const unknownStateLine = "Service state unknown: launchctl could not be run"

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
		Long:  "Install the Mimi launchd service so it starts automatically on login. Creates the plist file and loads it with launchctl.\n\nThe plist is a snapshot of the config taken at install time, so run install again after changing a setting it bakes in — it replaces the plist and reloads the service, and does nothing at all when the plist already matches.\n\nReloading waits for the running service to unload before loading the new plist, for up to five seconds; a service still loaded after that fails the install with its old plist untouched. A load that fails for any other reason leaves the new plist in place, so running install again retries just the load.\n\nThe daemon's stdout and stderr are captured beside settings.log_file, as <name>.out.log and <name>.err.log, and that directory is created if missing. When log_file is unset — or is not an absolute path, which launchd cannot open — they fall back to /tmp/mimi.log and /tmp/mimi.err.log. The plist also names both paths in the service's environment, which is what tells the daemon to empty them once at each start; nothing rotates them otherwise, and a service installed before that existed picks it up here.\n\nThe PATH the installed service runs its hooks with comes from settings.service_path; unset, it is /opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin. Nothing else reads that setting, so this command is what makes a change to it take effect.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logFile, servicePath := state.plistSettings()

			outcome, err := defaultService.Install(
				cmd.Context(),
				state.configPath,
				logFile,
				servicePath,
			)
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
		Long:  "Unload the Mimi launchd service and remove its plist file. Mimi will no longer start automatically on login.\n\nA service that is not loaded is uninstalled without complaint. When a loaded service cannot be unloaded, the plist is left in place and this fails, so the uninstall can be retried once whatever blocked the unload is fixed.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := defaultService.Uninstall(cmd.Context())
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
			err := defaultService.Start(cmd.Context())
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
			err := defaultService.Stop(cmd.Context())
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
			err := defaultService.Restart(cmd.Context())
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
		Long:  "Check whether the Mimi launchd service is currently loaded, and whether it is actually running.\n\nThe two are not the same: the installed plist sets KeepAlive, so a daemon that crashes at startup is relaunched every ten seconds and stays loaded the whole time. A running service reports the pid it runs under; a loaded one that is not running reports the status it last exited with — a repeated non-zero status there is a daemon in a crash loop, and the captured stderr beside settings.log_file says why.\n\nUnder that line come the captured stdout and stderr the installed plist names, with how large each has grown. A daemon launchd started empties both at startup, so each size is one run's console output.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(formatStatus(defaultService.Status(cmd.Context())))

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

// formatStatus renders a service.Status as the lines `mimi services status`
// prints.
//
// The first of them is the service itself. Loaded and running are not the same
// thing, and that is the point of it: the installed plist sets KeepAlive, so a
// daemon that crashes at startup is relaunched every ten seconds forever while
// staying exactly as loaded as a healthy one. The pid says it is up; the exit
// status it left behind says it is not. When launchd's description carries
// neither — it is undocumented text and may change shape — this falls back to
// the two words the command printed before it ever asked.
//
// Under that line come the captured console streams, one per line, each with
// how large it has grown. They are the only place a crash before the logger
// exists surfaces, and their size is the fact the line adds: the daemon empties
// them once per start, so a size is one run's console output and a big one
// under a service that keeps exiting is the crash loop this command is run to
// find. A stream the installed plist did not name — because nothing is
// installed, or because mimi did not write that plist — gets no line at all,
// rather than a guess at where its output would have gone.
func formatStatus(status service.Status) string {
	lines := []string{loadedLine(status)}

	captured := []struct {
		stream string
		log    service.CapturedLog
	}{
		{stream: "stdout", log: status.CapturedStdout},
		{stream: "stderr", log: status.CapturedStderr},
	}
	for _, stream := range captured {
		if stream.log.Path == "" {
			continue
		}

		lines = append(lines, fmt.Sprintf(
			"Captured %s: %s (%s)",
			stream.stream,
			stream.log.Path,
			formatCapturedSize(stream.log),
		))
	}

	return strings.Join(lines, "\n")
}

// loadedLine is the first line of a status: whether the service is loaded, and
// the one number that says whether it is up.
//
// Whether it is loaded has a third answer, and it gets a line of its own rather
// than being folded into either of the other two. "Service not loaded" over a
// service nobody could ask about is the reading this command used to print, and
// it is the one that sends a user off to reinstall something that may be
// running perfectly well.
func loadedLine(status service.Status) string {
	switch status.State {
	case service.LoadStateLoaded:
		return runningLine(status)
	case service.LoadStateNotLoaded:
		return "Service not loaded"
	case service.LoadStateUnknown:
		return unknownStateLine
	default:
		// A state added later with no line of its own. Saying nothing is known
		// is true of every state this cannot name, and it is the only one of
		// these lines that is safe over a service whose state was never read.
		return unknownStateLine
	}
}

// runningLine is the loaded half of that first line: launchd holds the job, and
// the one number available says whether it is actually up.
func runningLine(status service.Status) string {
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

// formatCapturedSize renders how large a captured stream has grown, in the
// units a person reads. A file that is not there is said in words rather than
// as a zero: launchd creates both when it first spawns the daemon, so an absent
// one means the service has never run, which a "0 B" would hide.
func formatCapturedSize(log service.CapturedLog) string {
	if !log.Present {
		return "not created yet"
	}

	const unit = 1024

	if log.Size < unit {
		return fmt.Sprintf("%d B", log.Size)
	}

	div, exp := int64(unit), 0
	for size := log.Size / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(log.Size)/float64(div), "KMGTPE"[exp])
}
