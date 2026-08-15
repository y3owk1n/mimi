package cmd

import (
	"fmt"
	"os"
	"os/signal"

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
		PersistentPreRun: func(cobraCmd *cobra.Command, _ []string) {
			state.resolveConfigPath()

			// Silencing usage here rather than on the root command is what
			// keeps usage for the errors it answers. Cobra parses flags and
			// validates arguments before it reaches any persistent pre-run, so
			// a bad flag or a rejected argument still prints usage; by the time
			// this runs the command line was understood, and anything that
			// fails from here on is a runtime failure whose message should not
			// be buried under a list of flags.
			cobraCmd.SilenceUsage = true
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

// Execute runs the root command under a context the user's interrupt cancels,
// and returns any error.
//
// Watching SIGINT here is what takes Go's default response to it away —
// terminating the process — from every command at once, which is why the
// second interrupt ends the process instead. What each command does with the
// first one is recorded, command by command, in the audit in
// interrupt_audit_test.go.
//
// Only SIGINT is watched. SIGTERM keeps Go's default, which is what it has
// always had here: nothing in the CLI is written to be shut down politely by a
// supervisor, and taking a killing signal away from one would be the change
// this makes to Ctrl-C without the second stage that pays for it.
func Execute() error {
	signals := make(chan os.Signal, interruptBufferSize)

	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	return runUnderInterrupts(RootCmd, signals, exitProcess)
}
