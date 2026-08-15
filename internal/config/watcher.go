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

// Watcher monitors a config file and calls a callback once the writes to it
// have settled.
//
// It reports that the file changed and nothing more: it does not read the
// file, so it can neither parse it nor fail to. The daemon's reloadConfig is
// the one place that loads a config, applies it, and logs how that went, so
// a file that will not parse and a file that will not apply are reported the
// same way, by the same line — which they were not while this watcher parsed
// the file itself and logged its own failure.
type Watcher struct {
	path     string
	onChange func()
	logger   *zap.SugaredLogger
}

// NewWatcher creates a new config file watcher.
func NewWatcher(path string, onChange func(), logger *zap.SugaredLogger) *Watcher {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

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
						debounce = time.AfterFunc(debounceDelay, w.notifyChange)
					}
				} else {
					debounce = time.AfterFunc(debounceDelay, w.notifyChange)
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

// notifyChange tells onChange the watched file has changed.
//
// It logs at debug and claims nothing about the reload: onChange is where the
// file is loaded, where the hook registry reloads, and where a bad hook (an
// invalid regex, for example) is caught, so only it can know how the reload
// went. The debug line still records that the watcher fired, which is what
// tells "my editor's save was never noticed" apart from "it was noticed and
// the config was rejected".
func (w *Watcher) notifyChange() {
	w.logger.Debug("config file changed")
	w.onChange()
}
