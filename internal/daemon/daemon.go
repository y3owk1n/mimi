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
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/observe"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/systray"
)

const (
	logSubBufSize  = 128
	hookSubBufSize = 256
)

// Run starts the mimi daemon: window/space observers, hooks executor, and config watcher.
func Run(cfg *config.Config, logger *zap.SugaredLogger, configPath string, version string) error {
	var (
		quitCh    <-chan struct{}
		component *systray.Component
	)

	runDone := make(chan error, 1)

	reload := func(ctx context.Context, path string) error {
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			return err
		}

		return process.Signal(syscall.SIGHUP)
	}

	if cfg.Systray.Enabled {
		quitChWritable := make(chan struct{})
		quitCh = quitChWritable

		requestQuit := func() {
			select {
			case <-quitChWritable:
			default:
				close(quitChWritable)
			}
		}

		component = systray.NewComponent(
			version,
			configPath,
			reload,
			requestQuit,
			cfg.Systray.ShowWorkspaceNumber,
			logger,
		)
	}

	go func() {
		err := runCore(cfg, logger, configPath, quitCh)

		systray.Quit()

		runDone <- err
	}()

	if cfg.Systray.Enabled {
		systray.Run(component.OnReady, component.OnExit)
		component.Close()
	} else {
		systray.RunHeadless(func() {}, func() {})
	}

	return <-runDone
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
	if !native.StartObservers(obsCfg, accessibilityPrompt) {
		return nil
	}

	perm = permissions.Check()

	axEnabled := perm.Accessibility && hasWindowEvents(cfg)
	if hasWindowEvents(cfg) && !perm.Accessibility {
		logger.Warn("accessibility permission not granted — window hooks disabled")
	}

	bus := events.NewBus()
	axTracker := observe.NewAXTracker(axEnabled)
	router := observe.NewRouter(bus, axTracker, logger)

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

	go router.Run(ctx)
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
		native.UpdateObservers(getObserverConfig(newCfg))

		perm := permissions.Check()
		axTracker.Update(perm.Accessibility && hasWindowEvents(newCfg))
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
			native.StopObservers()
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
				native.UpdateObservers(getObserverConfig(newCfg))

				perm := permissions.Check()
				axTracker.Update(perm.Accessibility && hasWindowEvents(newCfg))
				logger.Info("reloaded config via SIGHUP")

				continue
			}

			logger.Infow("shutting down", "signal", sig)
			cancel()
			native.StopObservers()
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

func hasWorkspaceEvents(cfg *config.Config) bool {
	return len(cfg.Hooks.WorkspaceChanged) > 0
}

func getObserverConfig(cfg *config.Config) native.ObserverConfig {
	return native.ObserverConfig{
		AppLifecycle: hasWindowEvents(cfg),
		Workspace:    hasWorkspaceEvents(cfg),
	}
}
