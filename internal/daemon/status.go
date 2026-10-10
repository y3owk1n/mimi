package daemon

import (
	"encoding/json"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
)

// Status is what the daemon says about itself when asked over the socket:
// which build it is, how long it has run, which config it runs, and which
// of its features that config turns on. It is what tells a CLI whether the
// daemon it reached is the same build as itself.
type Status struct {
	Version    string   `json:"version"`
	UptimeSecs int      `json:"uptimeSecs"`
	ConfigPath string   `json:"configPath"`
	Features   Features `json:"features"`
	// Accessibility is whether the daemon holds the permission now. It is
	// the daemon's own, not the CLI's. macOS grants it per binary, and a CLI
	// run from a terminal asks on the terminal's behalf. A daemon too old to
	// report it leaves it nil.
	Accessibility *bool `json:"accessibility,omitempty"`
}

// Features is what the running config turns on, as far as the permission
// the daemon started with lets it.
type Features struct {
	Hooks   int  `json:"hooks"`
	Tiling  bool `json:"tiling"`
	Borders bool `json:"borders"`
	Systray bool `json:"systray"`
}

// statusAnswer is the status request's answer, read from the config the
// last reload applied. startedWith is whether the daemon held Accessibility
// when it started, which decided what it turned on, and holdsNow is whether
// it holds it now.
func statusAnswer(
	version, configPath string,
	started time.Time,
	startedWith, holdsNow bool,
	current func() *config.Config,
) (json.RawMessage, error) {
	cfg := current()

	return json.Marshal(Status{
		Version:    version,
		UptimeSecs: int(time.Since(started).Seconds()),
		ConfigPath: configPath,
		Features: Features{
			Hooks:   cfg.Hooks.Count(),
			Tiling:  cfg.Tiling.Enabled && startedWith,
			Borders: cfg.Border.Enabled && startedWith,
			Systray: cfg.Systray.Enabled,
		},
		Accessibility: &holdsNow,
	})
}
