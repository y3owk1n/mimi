package cmd

import (
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/daemon"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/logging"
	"github.com/y3owk1n/mimi/internal/permissions"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the mimi daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !config.Exists(configPath) {
			choice := permissions.ShowConfigOnboardingAlert(configPath)
			if choice == permissions.ConfigOnboardingQuit {
				return nil
			}

			err := config.WriteDefault(configPath)
			if err != nil {
				return err
			}

			cmd.Printf("Default config written to %s\n", configPath)
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
		}

		logger := logging.New(cfg)
		logger.Infow("mimi starting", "version", Version, "config", configPath)

		return daemon.Run(cfg, logger, configPath)
	},
}
