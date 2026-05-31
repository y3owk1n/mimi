package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default config file",
	Long: `Writes the default config to the config path (default: ~/.config/mimi/config.toml).
Safe to re-run — it will overwrite any existing config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := config.WriteDefault(configPath)
		if err != nil {
			return fmt.Errorf("writing default config: %w", err)
		}

		fmt.Printf("Default config written to %s\n", configPath)
		fmt.Println("Edit it to customize hooks, then run 'mimi start'.")

		return nil
	},
}
