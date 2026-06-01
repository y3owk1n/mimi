package cmd

import (
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default config file",
	Long: `Writes the default config to the config path (default: ~/.config/mimi/config.toml).
Safe to re-run — it will overwrite any existing config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := config.WriteDefault(configPath)
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing default config")
		}

		cmd.Printf("Default config written to %s\n", configPath)
		cmd.Println("Edit it to customize hooks, then run 'mimi start'.")

		return nil
	},
}
