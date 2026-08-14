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

			return daemon.Run(cfg, logger, state.configPath, Version)
		},
	}
}
