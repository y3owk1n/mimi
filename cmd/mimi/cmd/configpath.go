package cmd

import (
	"github.com/y3owk1n/mimi/internal/config"
)

// cliState holds the state one command tree shares between its commands — the
// config path the persistent --config flag writes into, resolved to a real
// path before any command runs.
type cliState struct {
	configPath string
}

// resolveConfigPath replaces an empty --config with the default config path.
func (s *cliState) resolveConfigPath() {
	s.configPath = config.ResolvePath(s.configPath)
}
