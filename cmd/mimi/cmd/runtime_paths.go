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

// logFilePath returns the configured settings.log_file, already expanded by
// the loader. It returns "" both when the setting is unset — it is optional —
// and when the config cannot be read, which callers must treat the same way:
// there is no log file to place anything beside.
func (s *cliState) logFilePath() string {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return ""
	}

	return cfg.Settings.LogFile
}
