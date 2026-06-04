package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

var configCmd = &cobra.Command{
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

var configDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Print the resolved configuration as JSON",
	Long:  "Print the currently resolved Mimi configuration as pretty-printed JSON. Useful for verifying that your config file is being parsed correctly.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
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

var configReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload configuration from disk",
	Long:  "Reload the Mimi configuration file from disk without restarting the running daemon. Changes to hooks and settings take effect immediately.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
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

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default configuration file",
	Long: `Writes the default config to the config path (default: ~/.config/mimi/config.toml).
Safe to re-run — it will overwrite any existing config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := config.WriteDefault(configPath)
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing default config")
		}

		cmd.Printf("Default config written to %s\n", configPath)
		cmd.Println("Edit it to customize hooks, then run 'mimi start'.")

		return nil
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Parse and validate the config file, reporting any errors",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Config invalid:\n  %s\n", err)
			os.Exit(1)
		}

		hookCount := countHooks(cfg)
		cmd.Printf("Config valid (%d hook(s) defined)\n", hookCount)

		return nil
	},
}

func init() {
	configCmd.AddCommand(configDumpCmd)
	configCmd.AddCommand(configReloadCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configValidateCmd)
}

func countHooks(cfg *config.Config) int {
	count := 0
	for _, entries := range [][]config.HookEntry{
		cfg.Hooks.AppActivate, cfg.Hooks.AppDeactivate,
		cfg.Hooks.AppLaunch, cfg.Hooks.AppQuit,
		cfg.Hooks.AppHide, cfg.Hooks.AppUnhide,
		cfg.Hooks.WindowFocus, cfg.Hooks.WindowTitleChange,
		cfg.Hooks.WindowCreated, cfg.Hooks.WindowClosed,
		cfg.Hooks.WindowResize,
		cfg.Hooks.SystemSleep, cfg.Hooks.SystemWake,
		cfg.Hooks.ScreenLock, cfg.Hooks.ScreenUnlock,
		cfg.Hooks.SystemShutdown, cfg.Hooks.UserSessionEnd,
		cfg.Hooks.VolumeMount, cfg.Hooks.VolumeUnmount,
		cfg.Hooks.ExternalDisplayConnected, cfg.Hooks.ExternalDisplayDisconnected,
		cfg.Hooks.AppearanceChanged,
		cfg.Hooks.PowerAdapterConnected, cfg.Hooks.PowerAdapterDisconnected,
		cfg.Hooks.BatteryLow, cfg.Hooks.BatteryCritical,
		cfg.Hooks.AudioDeviceChanged,
		cfg.Hooks.WorkspaceChanged,
		cfg.Hooks.USBDeviceConnected, cfg.Hooks.USBDeviceDisconnected,
		cfg.Hooks.NetworkUp, cfg.Hooks.NetworkDown,
		cfg.Hooks.ClipboardChanged,
	} {
		count += len(entries)
	}

	return count
}
