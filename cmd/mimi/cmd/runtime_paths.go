package cmd

import (
	"github.com/y3owk1n/mimi/internal/config"
)

// runtimePaths returns the PID and socket paths the configured daemon uses,
// falling back to the defaults when the config cannot be read.
func (s *cliState) runtimePaths() (string, string) {
	cfg, err := config.Load(s.configPath)
	if err == nil {
		return cfg.Settings.PIDFile, cfg.Settings.SocketFile
	}

	return config.DefaultPIDPath, config.DefaultSocketPath
}
