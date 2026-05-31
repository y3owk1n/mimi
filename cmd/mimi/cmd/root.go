package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	configPath string
	verbose    bool
	// Version is set via ldflags at build time.
	Version = "dev"
	// GitCommit is set via ldflags at build time.
	GitCommit = "unknown"
	// BuildDate is set via ldflags at build time.
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "mimi",
	Short: "macOS event daemon — run hooks on system events",
	Long: `mimi listens to macOS system events (app focus, sleep/wake, volume mount, etc.)
and executes shell commands you define in ~/.config/mimi/config.toml.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Set version info right before execution, not at init time
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate(
		fmt.Sprintf(
			"Mimi version %s\nGit commit: %s\nBuild date: %s\n",
			Version,
			GitCommit,
			BuildDate,
		),
	)

	rootCmd.PersistentFlags().StringVar(&configPath, "config", "~/.config/mimi/config.toml",
		"path to config file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"verbose output")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(eventsCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(configCmd)
}
