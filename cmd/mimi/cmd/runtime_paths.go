package cmd

import (
	"github.com/y3owk1n/mimi/internal/config"
)

func resolveRuntimePaths() (string, string) {
	resolvedConfig := config.ResolvePath(configPath)

	cfg, err := config.Load(resolvedConfig)
	if err == nil {
		return cfg.Settings.PIDFile, cfg.Settings.SocketFile
	}

	return config.DefaultPIDPath, config.DefaultSocketPath
}
