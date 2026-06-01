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
	for _, entries := range []struct {
		name    string
		entries []config.HookEntry
	}{
		{"on_app_activate", cfg.Hooks.AppActivate},
		{"on_app_deactivate", cfg.Hooks.AppDeactivate},
		{"on_app_launch", cfg.Hooks.AppLaunch},
		{"on_app_quit", cfg.Hooks.AppQuit},
		{"on_app_hide", cfg.Hooks.AppHide},
		{"on_app_unhide", cfg.Hooks.AppUnhide},
		{"on_window_focus", cfg.Hooks.WindowFocus},
		{"on_window_title_change", cfg.Hooks.WindowTitleChange},
		{"on_window_created", cfg.Hooks.WindowCreated},
		{"on_window_closed", cfg.Hooks.WindowClosed},
		{"on_system_sleep", cfg.Hooks.SystemSleep},
		{"on_system_wake", cfg.Hooks.SystemWake},
		{"on_screen_lock", cfg.Hooks.ScreenLock},
		{"on_screen_unlock", cfg.Hooks.ScreenUnlock},
		{"on_system_shutdown", cfg.Hooks.SystemShutdown},
		{"on_volume_mount", cfg.Hooks.VolumeMount},
		{"on_volume_unmount", cfg.Hooks.VolumeUnmount},
	} {
		_ = entries.name
		count += len(entries.entries)
	}

	return count
}
