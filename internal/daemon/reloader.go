package daemon

import (
	"sync"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/hooks"
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/observe"
	"github.com/y3owk1n/mimi/internal/permissions"
)

// reloader is the single path by which a new config is applied to a running
// daemon. fsnotify, SIGHUP, and the systray's reload menu item all end up
// calling Apply, so a failed reload is reported the same way regardless of
// which trigger fired it — previously the fsnotify path returned the error
// from hooks.Registry.Reload while the SIGHUP path discarded it, leaving an
// invalid config's stale hooks in place with no signal to the user.
//
// reloader itself never logs — Apply reports outcomes purely through its
// return values, and the caller (reloadConfig, in daemon.go) is the single
// place that turns those into a log line, so every trigger logs identically.
type reloader struct {
	// mu serializes Apply. Two goroutines can ask for a reload at the same
	// moment — the config file watcher and the signal loop — and holding the
	// lock for the whole of Apply is what stops a simultaneous file save and
	// SIGHUP from interleaving into hooks from one config and executor
	// settings from another. See docs/adr/0002-reload-is-signal-mediated.md.
	mu sync.Mutex

	// running is the config the daemon started with, and is never replaced.
	// Its restart-only settings are the values actually in effect for this
	// process's lifetime, whatever a later config says, so they are what a
	// reload compares against to decide whether to ask for a restart.
	running *config.Config

	reg       *hooks.Registry
	executor  *hooks.Executor
	axTracker *observe.AXTracker
	router    *observe.Router
}

// newReloader bundles the dependencies a reload touches — the config the
// daemon started with, the hook registry, its executor, the AX tracker, and
// the router's debounce window — so callers pass them once at construction
// instead of threading the same pointers by hand through every reload call
// site.
//
// running is the config the daemon is about to run and must not be nil; with
// no config to compare against, Apply has nothing to say about restart-only
// settings and reports none.
func newReloader(
	running *config.Config,
	reg *hooks.Registry,
	executor *hooks.Executor,
	axTracker *observe.AXTracker,
	router *observe.Router,
) *reloader {
	return &reloader{
		running:   running,
		reg:       reg,
		executor:  executor,
		axTracker: axTracker,
		router:    router,
	}
}

// Apply reloads hooks from cfg and, only once that succeeds, updates
// executor settings, native observers, the router's debounce window, and AX
// tracking to match. hooks.Registry.Reload is transactional — it leaves the
// existing hook map untouched on error — so returning immediately on
// failure means a bad config leaves every dependency exactly as it was
// rather than reloading some values but not others.
//
// Everything Apply does not touch — the logger, the pid file, the socket, the
// systray, the hook worker limit — is restart-only, because nothing else in
// the daemon reads a fresh value for it after startup either. Apply returns
// the restart-only settings cfg changes, by TOML key, so the caller can say
// so instead of reporting a reload that quietly did nothing. Those keys are
// derived from the config type (config.RestartOnlyChanges); widening what
// Apply re-reads means retagging that field as reloadable in the same change.
//
// Apply is serialized: the whole of it runs under rl.mu, so a file save and a
// SIGHUP arriving together apply one config after the other rather than
// interleaving.
func (rl *reloader) Apply(cfg *config.Config) ([]string, error) {
	if cfg == nil {
		return nil, derrors.New(derrors.CodeInvalidInput, "nil config")
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	err := rl.reg.Reload(cfg)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeInvalidConfig, "reloading hooks")
	}

	rl.executor.UpdateSettings(&cfg.Settings)
	native.UpdateObservers(getObserverConfig(cfg))
	rl.router.SetDebounceWindow(time.Duration(cfg.Settings.ResizeDebounceMS) * time.Millisecond)

	perm := permissions.Check()
	rl.axTracker.Update(perm.Accessibility && hasWindowEvents(cfg))

	return config.RestartOnlyChanges(rl.running, cfg), nil
}
