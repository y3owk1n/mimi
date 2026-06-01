package config

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
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
	return &Watcher{path: expandHome(path), onChange: onChange, logger: logger}
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

	var debounce *time.Timer
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
					debounce.Stop()
				}

				debounce = time.AfterFunc(debounceDelay, func() {
					cfg, err := Load(w.path)
					if err != nil {
						w.logger.Warnw("config reload failed", "err", err)

						return
					}

					w.logger.Info("config reloaded")
					w.onChange(cfg)
				})
			}
		case err, ok := <-fileWatcher.Errors:
			if !ok {
				return nil
			}

			w.logger.Warnw("config watcher error", "err", err)
		}
	}
}
