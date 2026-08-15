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

// plistSettings returns the config values `mimi services install` bakes into
// the launchd plist: settings.log_file, already expanded by the loader, and
// settings.service_path. Both are read from one load, so an install cannot
// take two of them from two different reads of the file.
//
// Either is "" when the setting is unset — both are optional — and both are
// when the config cannot be read, which callers must treat the same way: there
// is no log file to place anything beside, and no configured PATH, so the
// plist's own defaults stand.
func (s *cliState) plistSettings() (string, string) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return "", ""
	}

	return cfg.Settings.LogFile, cfg.Settings.ServicePath
}
