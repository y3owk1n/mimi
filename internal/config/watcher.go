package config

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/paths"
)

const debounceDelay = 300 * time.Millisecond

// Watcher monitors a config file and triggers a callback on changes.
type Watcher struct {
	path     string
	onChange func(*Config)
	logger   *zap.SugaredLogger
}

// NewWatcher creates a new config file watcher.
func NewWatcher(path string, onChange func(*Config), logger *zap.SugaredLogger) *Watcher {
	return &Watcher{path: paths.ExpandHome(path), onChange: onChange, logger: logger}
}

// Run starts the config file watcher loop. It blocks until the context is canceled.
func (w *Watcher) Run(ctx context.Context) error {
	fileWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	defer func() { _ = fileWatcher.Close() }()

	err = fileWatcher.Add(w.path)
	if err != nil {
		err2 := fileWatcher.Add(filepath.Dir(w.path))
		if err2 != nil {
			return err
		}
	}

	// Use a single resettable timer for debouncing instead of spawning a
	// new goroutine via time.AfterFunc on every fsnotify event. Saves
	// goroutine churn during rapid editor saves.
	var debounce *time.Timer

	stopDebounce := func() {
		if debounce != nil {
			debounce.Stop()
		}
	}
	defer stopDebounce()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fileWatcher.Events:
			if !ok {
				return nil
			}

			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) {
				if debounce != nil {
					// Reset extends the window; if it has already
					// fired or stopped, create a fresh timer.
					if debounce.Stop() {
						debounce.Reset(debounceDelay)
					} else {
						debounce = time.AfterFunc(debounceDelay, w.reload)
					}
				} else {
					debounce = time.AfterFunc(debounceDelay, w.reload)
				}
			}
		case err, ok := <-fileWatcher.Errors:
			if !ok {
				return nil
			}

			w.logger.Warnw("config watcher error", "err", err)
		}
	}
}

// reload parses the watched file and, on success, hands it to onChange.
//
// It does not log a success line: onChange is where the hook registry
// actually reloads and where a bad hook (an invalid regex, for example) is
// caught, so only the caller can know whether the reload as a whole
// succeeded. The daemon's applyReload is that caller, and it logs the one
// authoritative "config reloaded" / "config reload failed" line for every
// trigger. Logging a debug line here — rather than nothing — still records
// that the file parsed, which is useful when a later onChange failure needs
// to be told apart from a parse failure.
func (w *Watcher) reload() {
	cfg, err := Load(w.path)
	if err != nil {
		w.logger.Warnw("config reload failed", "err", err)

		return
	}

	w.logger.Debug("config file parsed")
	w.onChange(cfg)
}
