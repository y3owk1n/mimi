package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func newConfigCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Mimi configuration",
		Long: `Commands for managing the Mimi configuration file and runtime settings.

Subcommands:
  dump       Print the resolved configuration as JSON
  reload     Reload configuration from disk without restarting
  init       Create a default configuration file to get started
  validate   Check a configuration file for errors

See 'mimi config <subcommand> --help' for details on each.`,
	}

	cmd.AddCommand(newConfigDumpCmd(state))
	cmd.AddCommand(newConfigReloadCmd(state))
	cmd.AddCommand(newConfigInitCmd(state))
	cmd.AddCommand(newConfigValidateCmd(state))

	return cmd
}

func newConfigDumpCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "Print the resolved configuration as JSON",
		Long:  "Print the currently resolved Mimi configuration as pretty-printed JSON. Useful for verifying that your config file is being parsed correctly.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeSerializationFailed, "marshaling config")
			}

			cmd.Println(string(data))

			return nil
		},
	}
}

func newConfigReloadCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload configuration from disk",
		Long: `Reload the Mimi configuration file from disk without restarting the running
daemon, then report how it went. A config the daemon could not apply is an
error, and the daemon keeps the config it had. The command lists the settings
a reload cannot apply with what applies them: a restart of the daemon, or for
settings.service_path, mimi services install.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			pid, running := daemon.RunningPID(cfg.Settings.PIDFile)
			if !running {
				return derrors.New(derrors.CodeDaemonUnavailable, notRunning(pid))
			}

			// The reload the daemon reports next is the one this signal
			// asks for. A daemon the CLI cannot ask still gets the signal,
			// and its log says how the reload went.
			before, probeErr := probeDaemon(cfg.Settings.SocketFile)
			answers := probeErr == nil

			proc, err := os.FindProcess(pid)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "process %d not found", pid)
			}

			err = proc.Signal(syscall.SIGHUP)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "signaling process %d", pid)
			}

			if !answers {
				cmd.Println("Configuration reload requested")

				return nil
			}

			outcome, err := awaitReload(cfg.Settings.SocketFile, before.LastReload)
			if err != nil {
				return err
			}

			return reportReload(cmd, outcome)
		},
	}
}

// reloadWait is how long mimi config reload waits for the daemon to report
// the reload it asked for, and reloadPoll how often it asks.
const (
	reloadWait = 5 * time.Second
	reloadPoll = 50 * time.Millisecond
)

// awaitReload asks the daemon at socketPath how its last reload went until
// it reports one later than before, which may be nil.
func awaitReload(socketPath string, before *daemon.Reload) (daemon.Reload, error) {
	deadline := time.Now().Add(reloadWait)

	for time.Now().Before(deadline) {
		status, err := probeDaemon(socketPath)
		if err == nil && status.LastReload != nil &&
			(before == nil || status.LastReload.At.After(before.At)) {
			return *status.LastReload, nil
		}

		time.Sleep(reloadPoll)
	}

	return daemon.Reload{}, derrors.Newf(
		derrors.CodeDaemonUnavailable,
		"reload requested, but the daemon reported none within %s. Its log says how it went",
		reloadWait,
	)
}

// reportReload prints a reload that applied, with the settings it could not,
// and returns a reload that did not apply as the error.
func reportReload(cmd *cobra.Command, outcome daemon.Reload) error {
	if !outcome.OK {
		return derrors.New(
			derrors.CodeInvalidConfig,
			"config not reloaded, the daemon keeps the one it had: "+outcome.Error,
		)
	}

	cmd.Println("Configuration reloaded")

	if len(outcome.RestartOnly) > 0 {
		cmd.Printf(
			"Restart the daemon to apply: %s\n",
			strings.Join(outcome.RestartOnly, ", "),
		)
	}

	if len(outcome.ReinstallOnly) > 0 {
		cmd.Printf(
			"Run mimi services install to apply: %s\n",
			strings.Join(outcome.ReinstallOnly, ", "),
		)
	}

	return nil
}

func newConfigInitCmd(state *cliState) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default configuration file",
		Long: `Writes the default config to the config path (default: ~/.config/mimi/config.toml).
It refuses when a config is already there, unless --force replaces it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !force && config.Exists(state.configPath) {
				return derrors.Newf(
					derrors.CodeInvalidInput,
					"a config already exists at %s, pass --force to replace it with the default",
					state.configPath,
				)
			}

			err := config.WriteDefault(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing default config")
			}

			cmd.Printf("Default config written to %s\n", state.configPath)
			cmd.Println("Edit it to customize hooks, then run 'mimi start'.")

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing config with the default")

	return cmd
}

func newConfigValidateCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Parse and validate the config file, reporting any errors",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)

			problems := configProblems(cfg, err)
			if problems != "" {
				fmt.Fprint(os.Stderr, problems)
				os.Exit(1)
			}

			hookCount := cfg.Hooks.Count()
			cmd.Printf("Config valid (%d hook(s) defined)\n", hookCount)

			return nil
		},
	}
}

// configProblems renders everything wrong with a loaded config as the text
// `mimi config validate` prints, or "" when there is nothing wrong.
//
// It takes both results of config.Load because the two kinds of problem arrive
// separately: a failed check comes back as the error, while a hook key mimi
// does not recognize is recorded on the config, which Load still returns when
// validation fails. Reporting them together is the point -- a user fixing a
// typo should not have to fix an unrelated error first to discover it.
func configProblems(cfg *config.Config, loadErr error) string {
	var unknown, unknownKeys []string
	if cfg != nil {
		unknown = cfg.UnknownHookKeys
		unknownKeys = cfg.UnknownKeys
	}

	if loadErr == nil && len(unknown) == 0 && len(unknownKeys) == 0 {
		return ""
	}

	var report strings.Builder

	report.WriteString("Config invalid:\n")

	if loadErr != nil {
		fmt.Fprintf(&report, "  %s\n", loadErr)
	}

	// A key mimi does not know sets nothing, so a misspelled one leaves its
	// setting at the default. The daemon carries on, and validate says so.
	for _, key := range unknownKeys {
		fmt.Fprintf(&report, "  %s: not a recognized setting\n", key)
	}

	// A hook kind mimi does not know is a hook that will never fire. The
	// daemon carries on without it; validate exists to say so.
	for _, key := range unknown {
		fmt.Fprintf(&report, "  hooks.%s: not a recognized hook kind\n", key)
	}

	if len(unknown) > 0 {
		fmt.Fprintf(
			&report,
			"\nRecognized hook kinds:\n  %s\n",
			strings.Join(config.HookKindNames(), "\n  "),
		)
	}

	return report.String()
}
