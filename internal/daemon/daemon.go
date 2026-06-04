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
func Run(cfg *config.Config, logger *zap.SugaredLogger, configPath string, version string) error {
	return runWithHost(cfg, logger, configPath, version)
}

func runCore(
	cfg *config.Config,
	logger *zap.SugaredLogger,
	configPath string,
	quitCh <-chan struct{},
) error {
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

	obsCfg := getObserverConfig(cfg)
	if !cgo_bridge.Start(obsCfg, accessibilityPrompt) {
		return nil
	}

	perm = permissions.Check()

	axEnabled := perm.Accessibility && hasWindowEvents(cfg)
	if hasWindowEvents(cfg) && !perm.Accessibility {
		logger.Warn("accessibility permission not granted — window events disabled")
	}

	bus := events.NewBus()
	axMgr := observers.NewAccessibilityManager(axEnabled)
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
		cgo_bridge.UpdateObservers(getObserverConfig(newCfg))

		perm := permissions.Check()
		axMgr.Update(perm.Accessibility && hasWindowEvents(newCfg))
		logger.Info("hooks reloaded from config")
	}, logger)
	go func() { _ = watcher.Run(ctx) }()

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-quitCh:
			logger.Info("shutting down from systray")
			cancel()
			cgo_bridge.Stop()
			bus.Unsubscribe(logSub)
			bus.Unsubscribe(hookSub)

			return nil
		case sig := <-sigCh:
			if sig == syscall.SIGHUP {
				newCfg, err := config.Load(configPath)
				if err != nil {
					logger.Warnw("SIGHUP reload failed", "err", err)

					continue
				}

				_ = reg.Reload(newCfg)
				executor.UpdateSettings(&newCfg.Settings)
				cgo_bridge.UpdateObservers(getObserverConfig(newCfg))

				perm := permissions.Check()
				axMgr.Update(perm.Accessibility && hasWindowEvents(newCfg))
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
	}
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

func hasWindowEvents(cfg *config.Config) bool {
	return len(cfg.Hooks.WindowFocus) > 0 ||
		len(cfg.Hooks.WindowTitleChange) > 0 ||
		len(cfg.Hooks.WindowCreated) > 0 ||
		len(cfg.Hooks.WindowClosed) > 0 ||
		len(cfg.Hooks.WindowResize) > 0
}

func getObserverConfig(cfg *config.Config) cgo_bridge.ObserverConfig {
	return cgo_bridge.ObserverConfig{
		Power: len(cfg.Hooks.PowerAdapterConnected) > 0 ||
			len(cfg.Hooks.PowerAdapterDisconnected) > 0 ||
			len(cfg.Hooks.BatteryLow) > 0 ||
			len(cfg.Hooks.BatteryCritical) > 0,

		Audio: len(cfg.Hooks.AudioDeviceChanged) > 0,

		Clipboard: len(cfg.Hooks.ClipboardChanged) > 0,

		USB: len(cfg.Hooks.USBDeviceConnected) > 0 ||
			len(cfg.Hooks.USBDeviceDisconnected) > 0,

		Network: len(cfg.Hooks.NetworkUp) > 0 ||
			len(cfg.Hooks.NetworkDown) > 0,

		Display: len(cfg.Hooks.ExternalDisplayConnected) > 0 ||
			len(cfg.Hooks.ExternalDisplayDisconnected) > 0,

		AppLifecycle: len(cfg.Hooks.AppActivate) > 0 ||
			len(cfg.Hooks.AppDeactivate) > 0 ||
			len(cfg.Hooks.AppLaunch) > 0 ||
			len(cfg.Hooks.AppQuit) > 0 ||
			len(cfg.Hooks.AppHide) > 0 ||
			len(cfg.Hooks.AppUnhide) > 0 ||
			hasWindowEvents(cfg),

		SystemState: len(cfg.Hooks.SystemSleep) > 0 ||
			len(cfg.Hooks.SystemWake) > 0 ||
			len(cfg.Hooks.ScreenLock) > 0 ||
			len(cfg.Hooks.ScreenUnlock) > 0 ||
			len(cfg.Hooks.SystemShutdown) > 0 ||
			len(cfg.Hooks.UserSessionEnd) > 0,

		Volume: len(cfg.Hooks.VolumeMount) > 0 ||
			len(cfg.Hooks.VolumeUnmount) > 0,

		Workspace: len(cfg.Hooks.WorkspaceChanged) > 0,

		Appearance: len(cfg.Hooks.AppearanceChanged) > 0,
	}
}
