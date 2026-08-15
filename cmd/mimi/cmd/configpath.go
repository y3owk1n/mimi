package cmd

import (
	"io"
	"os"

	"github.com/y3owk1n/mimi/internal/config"
)

// cliState holds the state one command tree shares between its commands — the
// config path the persistent --config flag writes into, resolved to a real
// path before any command runs.
type cliState struct {
	configPath string
	// warnOut is where warnings that are not the command's own output go.
	// Nil means os.Stderr, which is what every real command tree leaves it
	// at; a test sets it to read what a user would have been told.
	warnOut io.Writer
}

// warnWriter returns the stream warnings go to. Warnings never share the
// command's stdout: an action's own output has to stay exactly what it was
// with no daemon in the picture at all.
func (s *cliState) warnWriter() io.Writer {
	if s.warnOut != nil {
		return s.warnOut
	}

	return os.Stderr
}

// resolveConfigPath replaces an empty --config with the default config path.
func (s *cliState) resolveConfigPath() {
	s.configPath = config.ResolvePath(s.configPath)
}
