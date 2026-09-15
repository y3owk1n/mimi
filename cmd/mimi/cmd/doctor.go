package cmd

import (
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/doctor"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/paths"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/service"
	"github.com/y3owk1n/mimi/internal/tiling"
)

func newDoctorCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the install for what stops mimi working",
		Long: `Run the checks docs/TROUBLESHOOTING.md walks through, one line each:
the config parses, Accessibility is granted, the daemon is running and its
socket is where the CLI looks, the daemon is the same build as this CLI,
the launchd service is up, every hook command
is on the PATH the service runs with, mimi can write the log file, every
layout the config names answers a sample input with frames, and which dock
swipe encoding a space switch is sent with on this macOS.

The layout check runs each layout once on a made-up desktop of two windows
on one display, so it needs no Accessibility and touches no window. It
catches a layout that cannot be run, exits with an error, times out, prints
something that is not the output contract, or prints nothing.

A failed check prints what to do about it. The command exits 1 when any
check fails, so a script can gate on it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := doctor.Assess(gatherFacts(cmd, state))
			cmd.Print(doctor.Format(checks))

			if doctor.Failed(checks) {
				return derrors.New(derrors.CodeActionFailed, "some checks failed")
			}

			return nil
		},
	}
}

// gatherFacts reads everything doctor.Assess judges.
func gatherFacts(cmd *cobra.Command, state *cliState) doctor.Facts {
	facts := doctor.Facts{ConfigPath: state.configPath}
	facts.Config, facts.ConfigErr = config.Load(state.configPath)

	pidPath, socketPath := state.runtimePaths()
	facts.PIDPath, facts.SocketPath = paths.ExpandHome(pidPath), paths.ExpandHome(socketPath)

	pid, err := readPID(pidPath)
	if err == nil {
		facts.PID, facts.PIDFound = pid, true

		proc, findErr := os.FindProcess(pid)
		facts.Alive = findErr == nil && proc.Signal(syscall.Signal(0)) == nil
	}

	_, err = os.Stat(facts.SocketPath)
	facts.SocketPresent = err == nil

	facts.CLIVersion = Version

	if facts.Alive && facts.SocketPresent {
		status, err := probeDaemon(socketPath)
		facts.DaemonVersion, facts.ProbeErr = status.Version, err
	}

	facts.Accessibility = permissions.Check().Accessibility
	facts.Service = defaultService.Status(cmd.Context())

	if facts.Alive && facts.Service.State == service.LoadStateNotLoaded {
		facts.ForeignAgent = service.AgentForPID(cmd.Context(), facts.PID)
	}

	if facts.Config != nil {
		servicePath := service.EffectivePath(facts.Config.Settings.ServicePath)
		facts.MissingCommands = doctor.MissingHookCommands(facts.Config, servicePath)
		facts.LogFile = facts.Config.Settings.LogFile
		facts.LogDirWritable = dirWritable(filepath.Dir(facts.LogFile))

		timeout := time.Duration(facts.Config.Tiling.TimeoutSecs) * time.Second
		for _, command := range tiling.LayoutCommands(facts.Config.Tiling) {
			run := doctor.RunLayout(
				cmd.Context(),
				facts.Config.Settings.HookShell,
				command,
				timeout,
			)
			facts.Layouts = append(facts.Layouts, run)
		}
	}

	facts.SwipeAugmented = native.DockSwipeAugmented()
	facts.SwipeOverride = os.Getenv("MIMI_FORCE_DOCK_SWIPE_AUGMENTATION")

	return facts
}

// dirWritable reports whether a file can be created in dir.
func dirWritable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".mimi-doctor-*")
	if err != nil {
		return false
	}

	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)

	return true
}
