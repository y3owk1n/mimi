package daemon

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
	"github.com/y3owk1n/mimi/internal/logging"
	"github.com/y3owk1n/mimi/internal/observers"
	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
	"github.com/y3owk1n/mimi/internal/permissions"
)

const (
	logSubBufSize  = 128
	hookSubBufSize = 256
)

// Run starts the mimi daemon: event observers, hooks executor, and config watcher.
func Run(cfg *config.Config, logger *zap.SugaredLogger, configPath string) error {
	err := writePID(cfg.Settings.PIDFile)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing pid file")
	}
	defer removePID(cfg.Settings.PIDFile)

	perm := permissions.Check()

	var accessibilityPrompt func() bool
	if !perm.Accessibility {
		accessibilityPrompt = func() bool {
			choice := permissions.ShowAccessibilityStartupAlert()

			return choice != permissions.AccessibilityStartupQuit
		}
	}

	if !cgo_bridge.Start(accessibilityPrompt) {
		return nil
	}

	perm = permissions.Check()
	if !perm.Accessibility {
		logger.Warn("accessibility permission not granted — window events disabled")
	}

	bus := events.NewBus()
	axMgr := observers.NewAccessibilityManager(perm.Accessibility)
	wsObs := observers.NewWorkspaceObserver(bus, axMgr, logger)

	reg := hooks.NewRegistry()

	err = reg.Reload(cfg)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading hooks")
	}

	executor := hooks.NewExecutor(reg, &cfg.Settings, logger)

	logSub := bus.Subscribe(logSubBufSize)
	hookSub := bus.Subscribe(hookSubBufSize)

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
			logger.Warnw("hook registry reload failed", "err", err)

			return
		}

		executor.UpdateSettings(&newCfg.Settings)
		logger.Info("hooks reloaded from config")
	}, logger)
	go func() { _ = watcher.Run(ctx) }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	for sig := range sigCh {
		if sig == syscall.SIGHUP {
			newCfg, err := config.Load(configPath)
			if err != nil {
				logger.Warnw("SIGHUP reload failed", "err", err)

				continue
			}

			_ = reg.Reload(newCfg)
			executor.UpdateSettings(&newCfg.Settings)
			logger.Info("reloaded config via SIGHUP")

			continue
		}

		logger.Infow("shutting down", "signal", sig)
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

	err := os.MkdirAll(filepath.Dir(path), 0o755) //nolint:mnd
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644) //nolint:mnd
}

func removePID(path string) {
	_ = os.Remove(expandHome(path))
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
}
