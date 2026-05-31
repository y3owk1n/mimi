package config

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	path     string
	onChange func(*Config)
	logger   *slog.Logger
}

func NewWatcher(path string, onChange func(*Config), logger *slog.Logger) *Watcher {
	return &Watcher{path: expandHome(path), onChange: onChange, logger: logger}
}

func (w *Watcher) Run(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	if err := fw.Add(w.path); err != nil {
		if err2 := fw.Add(filepath.Dir(w.path)); err2 != nil {
			return err
		}
	}

	var debounce *time.Timer
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(300*time.Millisecond, func() {
					cfg, err := Load(w.path)
					if err != nil {
						w.logger.Warn("config reload failed", "err", err)
						return
					}
					w.logger.Info("config reloaded")
					w.onChange(cfg)
				})
			}
		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.logger.Warn("config watcher error", "err", err)
		}
	}
}
