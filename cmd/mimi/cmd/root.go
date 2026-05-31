package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/y3owk1n/mimi/internal/config"
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

var RootCmd = &cobra.Command{
	Use:   "mimi",
	Short: "macOS event daemon — run hooks on system events",
	Long: `mimi listens to macOS system events (app focus, sleep/wake, volume mount, etc.)
and executes shell commands you define in ~/.config/mimi/config.toml.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		configPath = config.ResolvePath(configPath)
	},
}

func Execute() error {
	return RootCmd.Execute()
}

func init() {
	// Set version info right before execution, not at init time
	RootCmd.Version = Version
	RootCmd.SetVersionTemplate(
		fmt.Sprintf(
			"Mimi version %s\nGit commit: %s\nBuild date: %s\n",
			Version,
			GitCommit,
			BuildDate,
		),
	)

	RootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "",
		"path to config file")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"verbose output")

	RootCmd.AddCommand(startCmd)
	RootCmd.AddCommand(stopCmd)
	RootCmd.AddCommand(statusCmd)
	RootCmd.AddCommand(installCmd)
	RootCmd.AddCommand(uninstallCmd)
	RootCmd.AddCommand(eventsCmd)
	RootCmd.AddCommand(testCmd)
	RootCmd.AddCommand(initCmd)
	RootCmd.AddCommand(configCmd)
}
