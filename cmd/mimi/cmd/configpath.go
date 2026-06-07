package cmd

import (
	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/config"
)

func resolveConfigPath() {
	configPath = config.ResolvePath(configPath)
}

func addConfigPreRun(cmd *cobra.Command) {
	existing := cmd.PreRun
	cmd.PreRun = func(c *cobra.Command, args []string) {
		resolveConfigPath()

		if existing != nil {
			existing(c, args)
		}
	}
}
