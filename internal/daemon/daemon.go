package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
	"github.com/y3owk1n/mimi/internal/logging"
	"github.com/y3owk1n/mimi/internal/observers"
	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
	"github.com/y3owk1n/mimi/internal/permissions"
)

func Run(cfg *config.Config, logger *slog.Logger, configPath string) error {
	err := writePID(cfg.Settings.PIDFile)
	if err != nil {
		return fmt.Errorf("pid file: %w", err)
	}
	defer removePID(cfg.Settings.PIDFile)

	perm := permissions.Check()
	if !perm.Accessibility {
		logger.Warn("accessibility permission not granted — window events disabled")
	}

	cgo_bridge.Start()

	bus := events.NewBus()
	axMgr := observers.NewAccessibilityManager(perm.Accessibility)
	wsObs := observers.NewWorkspaceObserver(bus, axMgr, logger)

	reg := hooks.NewRegistry()

	err = reg.Reload(cfg)
	if err != nil {
		return fmt.Errorf("loading hooks: %w", err)
	}

	executor := hooks.NewExecutor(reg, &cfg.Settings, logger)

	logSub := bus.Subscribe(128)
	hookSub := bus.Subscribe(256)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go wsObs.Run(ctx)
	go executor.Run(ctx, hookSub)
	go logging.WriteEventLog(ctx, logSub, cfg.Settings.LogFile, logger)

	watcher := config.NewWatcher(configPath, func(newCfg *config.Config) {
		if newCfg == nil {
			return
		}

		err := reg.Reload(newCfg)
		if err != nil {
			logger.Warn("hook registry reload failed", "err", err)

			return
		}

		executor.UpdateSettings(&newCfg.Settings)
		logger.Info("hooks reloaded from config")
	}, logger)
	go watcher.Run(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	for sig := range sigCh {
		if sig == syscall.SIGHUP {
			newCfg, err := config.Load(config.DefaultConfigPath)
			if err != nil {
				logger.Warn("SIGHUP reload failed", "err", err)

				continue
			}

			_ = reg.Reload(newCfg)
			executor.UpdateSettings(&newCfg.Settings)
			logger.Info("reloaded config via SIGHUP")

			continue
		}

		logger.Info("shutting down", "signal", sig)
		cancel()
		cgo_bridge.Stop()
		bus.Unsubscribe(logSub)
		bus.Unsubscribe(hookSub)

		return nil
	}

	return nil
}

func writePID(path string) error {
	path = expandHome(path)

	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

func removePID(path string) {
	os.Remove(expandHome(path))
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
}
