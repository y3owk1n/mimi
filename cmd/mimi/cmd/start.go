package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/daemon"
	"github.com/y3owk1n/mimi/internal/logging"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the mimi daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !config.Exists(configPath) {
			fmt.Printf("No config found at %s — creating with defaults.\n", configPath)
			fmt.Println("Edit it to customize hooks, then run 'mimi start' again.")
			return config.WriteDefault(configPath)
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		logger := logging.New(cfg)
		logger.Info("mimi starting", "version", version, "config", configPath)
		return daemon.Run(cfg, logger, configPath)
	},
}
