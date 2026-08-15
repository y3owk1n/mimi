package daemon

import (
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
// return value, and the caller (applyReload, in daemon.go) is the single
// place that turns that into a log line, so every trigger logs identically.
type reloader struct {
	reg       *hooks.Registry
	executor  *hooks.Executor
	axTracker *observe.AXTracker
	router    *observe.Router
}

// newReloader bundles the dependencies a reload touches — the hook
// registry, its executor, the AX tracker, and the router's debounce window —
// so callers pass them once at construction instead of threading the same
// four pointers by hand through every reload call site.
func newReloader(
	reg *hooks.Registry,
	executor *hooks.Executor,
	axTracker *observe.AXTracker,
	router *observe.Router,
) *reloader {
	return &reloader{
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
// Settings outside [hooks] and the fields read here — systray, log_file,
// log_level, pid_file, socket_file — are restart-only: Apply never touches
// them, and nothing else in the daemon reads a fresh value for them after
// startup.
func (rl *reloader) Apply(cfg *config.Config) error {
	if cfg == nil {
		return derrors.New(derrors.CodeInvalidInput, "nil config")
	}

	err := rl.reg.Reload(cfg)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeInvalidConfig, "reloading hooks")
	}

	rl.executor.UpdateSettings(&cfg.Settings)
	native.UpdateObservers(getObserverConfig(cfg))
	rl.router.SetDebounceWindow(time.Duration(cfg.Settings.ResizeDebounceMS) * time.Millisecond)

	perm := permissions.Check()
	rl.axTracker.Update(perm.Accessibility && hasWindowEvents(cfg))

	return nil
}
