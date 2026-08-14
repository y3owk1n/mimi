package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version is set via ldflags at build time.
	Version = "dev"
	// GitCommit is set via ldflags at build time.
	GitCommit = "unknown"
	// BuildDate is set via ldflags at build time.
	BuildDate = "unknown"
)

// RootCmd is the root cobra command for the mimi CLI.
var RootCmd = newRootCmd()

// newRootCmd builds a complete mimi command tree.
//
// Every tree owns its flag state, so two trees never share a config path and a
// test can drive one without leaking into the next.
func newRootCmd() *cobra.Command {
	state := &cliState{}

	root := &cobra.Command{
		Use:   "mimi",
		Short: "macOS window and space utility",
		Long: `mimi provides macOS-native window and space management without disabling SIP.

Use "mimi action" for immediate commands (focus window, switch space, move window).
Use "mimi start" to run the background daemon and react to window/space events via hooks.`,
		Version: Version,
		// Cobra runs the nearest persistent pre-run it finds walking up from the
		// command that executes, so resolving here covers every command in the
		// tree — including leaves under a parent that has no RunE of its own.
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			state.resolveConfigPath()
		},
	}

	root.SetVersionTemplate(
		fmt.Sprintf(
			"Mimi version %s\nGit commit: %s\nBuild date: %s\n",
			Version,
			GitCommit,
			BuildDate,
		),
	)

	root.PersistentFlags().StringVarP(&state.configPath, "config", "c", "",
		"path to config file")
	root.PersistentFlags().BoolP("verbose", "v", false,
		"verbose output")

	root.AddCommand(newStartCmd(state))
	root.AddCommand(newStopCmd(state))
	root.AddCommand(newStatusCmd(state))
	root.AddCommand(newConfigCmd(state))
	root.AddCommand(newServicesCmd(state))
	root.AddCommand(newActionCmd(state))

	return root
}

// Execute runs the root command and returns any error.
func Execute() error {
	return RootCmd.Execute()
}
