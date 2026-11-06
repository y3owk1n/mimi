package cmd

import (
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// cliState holds the state one command tree shares between its commands — the
// config path the persistent --config flag writes into, resolved to a real
// path before any command runs.
type cliState struct {
	configPath string
	// verbose is -v: debug logs on stderr, and actions run in this process so
	// their logs are among them.
	verbose bool
	// tilingDesktop is the desktop a tiling engine in this process drives.
	// nil means the live desktop.
	tilingDesktop tiling.Desktop
}

// desktop is the desktop a tiling engine in this process drives.
func (s *cliState) desktop() tiling.Desktop {
	if s.tilingDesktop != nil {
		return s.tilingDesktop
	}

	return tiling.LiveDesktop{}
}

// resolveConfigPath replaces an empty --config with the default config path.
func (s *cliState) resolveConfigPath() {
	s.configPath = config.ResolvePath(s.configPath)
}
