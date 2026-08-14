package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
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
		Long:  "Reload the Mimi configuration file from disk without restarting the running daemon. Changes to hooks and settings take effect immediately.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			pid, err := readPID(cfg.Settings.PIDFile)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "reading pid file")
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "process %d not found", pid)
			}

			err = proc.Signal(syscall.SIGHUP)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInternal, "signaling process %d", pid)
			}

			cmd.Println("Configuration reload requested")

			return nil
		},
	}
}

func newConfigInitCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a default configuration file",
		Long: `Writes the default config to the config path (default: ~/.config/mimi/config.toml).
Safe to re-run — it will overwrite any existing config.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := config.WriteDefault(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing default config")
			}

			cmd.Printf("Default config written to %s\n", state.configPath)
			cmd.Println("Edit it to customize hooks, then run 'mimi start'.")

			return nil
		},
	}
}

func newConfigValidateCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Parse and validate the config file, reporting any errors",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Config invalid:\n  %s\n", err)
				os.Exit(1)
			}

			hookCount := countHooks(cfg)
			cmd.Printf("Config valid (%d hook(s) defined)\n", hookCount)

			return nil
		},
	}
}

// countHooks totals the hooks the config carries, across every kind.
//
// The kinds come from the fields of config.HooksConfig rather than a list kept
// here: the list this replaced was never extended when the app hook kinds
// arrived, so half of a user's hooks went uncounted.
func countHooks(cfg *config.Config) int {
	hooks := reflect.ValueOf(cfg.Hooks)

	count := 0

	for _, value := range hooks.Fields() {
		entries, ok := value.Interface().([]config.HookEntry)
		if !ok {
			continue
		}

		count += len(entries)
	}

	return count
}
