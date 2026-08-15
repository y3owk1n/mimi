package cmd

import (
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/logging"
	"github.com/y3owk1n/mimi/internal/permissions"
)

func newStartCmd(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the mimi hook daemon for window and space events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// First, before this process writes a single byte to stdout or
			// stderr. When launchd started the daemon, those two are the
			// captured console logs it appends to forever, and emptying them
			// here is the only rotation they get; anything printed before it
			// would be printed into a file about to be emptied. When anything
			// else started it, there is nothing to empty and nothing happens.
			//
			// The outcome is logged below rather than here: there is no logger
			// yet, and building one would write to the stream first.
			truncated, truncateErr := daemon.TruncateCapturedLogs()

			if !config.Exists(state.configPath) {
				choice := permissions.ShowConfigOnboardingAlert(state.configPath)
				if choice == permissions.ConfigOnboardingQuit {
					return nil
				}

				err := config.WriteDefault(state.configPath)
				if err != nil {
					return err
				}

				cmd.Printf("Default config written to %s\n", state.configPath)
			}

			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			logger := logging.New(cfg)
			logger.Infow("mimi starting", "version", Version, "config", state.configPath)

			if truncateErr != nil {
				logger.Warnw("captured console logs not truncated", "err", truncateErr)
			}

			if truncated > 0 {
				logger.Infow("captured console logs truncated", "count", truncated)
			}

			return daemon.Run(cfg, logger, state.configPath, Version)
		},
	}
}
