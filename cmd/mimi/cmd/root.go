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

// RootCmd is the root cobra command for the mimi CLI.
var RootCmd = &cobra.Command{
	Use:   "mimi",
	Short: "macOS window and space utility",
	Long: `mimi provides macOS-native window and space management without disabling SIP.

Use "mimi action" for immediate commands (focus window, switch space, move window).
Use "mimi start" to run the background daemon and react to window/space events via hooks.`,
}

// Execute runs the root command and returns any error.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
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
	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(servicesCmd)
	RootCmd.AddCommand(actionCmd)
}
