package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Config file utilities",
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
