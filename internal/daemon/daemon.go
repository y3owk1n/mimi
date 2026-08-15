package daemon

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
	"github.com/y3owk1n/mimi/internal/ipc"
	"github.com/y3owk1n/mimi/internal/logging"
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/observe"
	"github.com/y3owk1n/mimi/internal/paths"
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

	warnUnknownHookKeys(cfg, logger)

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

	obsCfg, accessibilityGranted := setupObservers(cfg, logger)
	if obsCfg == nil {
		return nil
	}

	pipeline, ctx, cancel, err := setupEventPipeline(cfg, logger, accessibilityGranted)
	if err != nil {
		return err
	}
	defer cancel()

	go pipeline.router.Run(ctx)
	go pipeline.executor.Run(ctx, pipeline.hookSub)
	go logging.WriteEventLog(ctx, pipeline.logSub, cfg.Settings.LogFile, logger)

	cfgReloader := newReloader(
		cfg,
		pipeline.reg,
		pipeline.executor,
		pipeline.axTracker,
		pipeline.router,
	)

	onChange := func() {
		reloadConfig(configPath, cfgReloader, reloadTriggerFsnotify, logger)
	}

	watcher := config.NewWatcher(configPath, onChange, logger)
	go func() { _ = watcher.Run(ctx) }()

	ipcServer := ipc.NewServer(cfg.Settings.SocketFile)
	defer ipcServer.Shutdown()

	go func() {
		err := ipcServer.Run(ctx)
		if err != nil && ctx.Err() == nil {
			logger.Warnw("IPC server stopped", "err", err)
		}
	}()

	runSignalLoop(cancel, quitCh, cfgReloader, pipeline, logger, configPath)

	return nil
}

func setupObservers(cfg *config.Config, logger *zap.SugaredLogger) (*native.ObserverConfig, bool) {
	perm := permissions.Check()

	accessibilityGranted := perm.Accessibility

	var accessibilityPrompt func() bool
	if !accessibilityGranted {
		accessibilityPrompt = func() bool {
			choice := permissions.ShowAccessibilityStartupAlert()

			return choice == permissions.AccessibilityStartupGranted
		}
	}

	obsCfg := getObserverConfig(cfg)
	if !native.StartObservers(obsCfg, accessibilityPrompt) {
		return nil, false
	}

	perm = permissions.Check()
	accessibilityGranted = perm.Accessibility

	if hasWindowEvents(cfg) && !accessibilityGranted {
		logger.Warn("accessibility permission not granted — window hooks disabled")
	}

	return &obsCfg, accessibilityGranted
}

// eventPipeline bundles the dependencies setupEventPipeline wires together:
// the event bus, the hook registry and its executor, the AX tracker and
// router that react to window state, and the two subscribers that drain the
// bus. Packaging them here means callers — runCore and, in turn, the
// reloader — pass the bundle once instead of threading the same handful of
// pointers by hand through every call site, which is how the fsnotify and
// SIGHUP reload paths drifted from each other in the first place.
type eventPipeline struct {
	bus       *events.Bus
	axTracker *observe.AXTracker
	router    *observe.Router
	reg       *hooks.Registry
	executor  *hooks.Executor
	logSub    events.Subscriber
	hookSub   events.Subscriber
}

func setupEventPipeline(
	cfg *config.Config,
	logger *zap.SugaredLogger,
	accessibilityGranted bool,
) (*eventPipeline, context.Context, context.CancelFunc, error) {
	axEnabled := accessibilityGranted && hasWindowEvents(cfg)

	bus := events.NewBus()
	axTracker := observe.NewAXTracker(axEnabled)
	router := observe.NewRouterWithDebounce(
		bus,
		axTracker,
		logger,
		time.Duration(cfg.Settings.ResizeDebounceMS)*time.Millisecond,
	)

	reg := hooks.NewRegistry()

	err := reg.Reload(cfg)
	if err != nil {
		return nil, nil, nil, derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading hooks")
	}

	executor := hooks.NewExecutor(reg, &cfg.Settings, logger)

	// Subscribe the executor with a kind filter so the bus can drop events
	// for which no hooks are registered, avoiding a channel send on the
	// hot path of high-frequency events.
	hookSub := bus.SubscribeWithFilter(hookSubBufSize, reg.KindFilter())

	// The event log is opt-in via [settings].log_file; when present, write
	// every event so the user can replay what happened. When disabled, the
	// always-false filter prevents the bus from sending into a channel
	// nobody reads, eliminating a source of silent drops.
	logPath := cfg.Settings.LogFile

	var logSub events.Subscriber

	if logPath != "" {
		logSub = bus.Subscribe(logSubBufSize)
	} else {
		logSub = bus.SubscribeWithFilter(
			hookSubBufSize,
			func(_ events.EventKind) bool { return false },
		)
	}

	ctx, cancel := context.WithCancel(context.Background())

	pipeline := &eventPipeline{
		bus:       bus,
		axTracker: axTracker,
		router:    router,
		reg:       reg,
		executor:  executor,
		logSub:    logSub,
		hookSub:   hookSub,
	}

	return pipeline, ctx, cancel, nil
}

// reloadTrigger names which route into the daemon fired a reload. It closes
// the set of triggers that can call reloadConfig — fsnotify and SIGHUP today
// — over a bare string so a new call site can't drift in an ad hoc label.
type reloadTrigger string

const (
	reloadTriggerFsnotify reloadTrigger = "fsnotify"
	reloadTriggerSighup   reloadTrigger = "sighup"
)

// The three things a reload can report. They are constants so that the tests
// that pin which outcome a given config produces name the outcome rather than
// repeating its wording.
const (
	reloadFailedMessage          = "config reload failed"
	reloadedMessage              = "config reloaded"
	reloadRestartRequiredMessage = "config reloaded; restart required for changed restart-only settings"
)

// reloadConfig loads the config at configPath, applies it, and logs the
// outcome. It is the one place all three of those happen: fsnotify's onChange
// and the SIGHUP handler both call it, so a config that will not parse and a
// config that will not apply are reported the same way, by the same line,
// naming the trigger that noticed — previously the watcher logged parse
// failures itself, without a trigger, while apply failures were logged here.
//
// A reload that changes a restart-only setting is not a plain success: the
// daemon has applied everything it can and is still running the old value for
// the rest, so it says so and names them. The names are mimi's own; the
// values the user gave them stay out of the log.
func reloadConfig(
	configPath string,
	cfgReloader *reloader,
	trigger reloadTrigger,
	logger *zap.SugaredLogger,
) {
	newCfg, err := config.Load(configPath)

	var restartOnly []string

	if err == nil {
		restartOnly, err = cfgReloader.Apply(newCfg)
	}

	if err != nil {
		logger.Warnw(reloadFailedMessage, "trigger", trigger, "err", err)

		return
	}

	warnUnknownHookKeys(newCfg, logger)

	if len(restartOnly) > 0 {
		logger.Warnw(
			reloadRestartRequiredMessage,
			"trigger", trigger,
			"restart_only", restartOnly,
		)

		return
	}

	logger.Infow(reloadedMessage, "trigger", trigger)
}

// warnUnknownHookKeys notes that the config asked for hook kinds mimi does not
// have. The daemon runs anyway on the hooks it did understand, so this is a
// warning rather than a refusal to start; `mimi config validate` is the one
// that fails.
//
// The keys themselves stay out of the log -- they are the user's config text.
// The count says something is wrong and the recognized set says what is
// allowed, which is enough to send someone to validate.
func warnUnknownHookKeys(cfg *config.Config, logger *zap.SugaredLogger) {
	if len(cfg.UnknownHookKeys) == 0 {
		return
	}

	logger.Warnw(
		"config names hook kinds that do not exist; those hooks will never fire",
		"count", len(cfg.UnknownHookKeys),
		"recognized", strings.Join(config.HookKindNames(), "|"),
	)
}

func runSignalLoop(
	cancel context.CancelFunc,
	quitCh <-chan struct{},
	cfgReloader *reloader,
	pipeline *eventPipeline,
	logger *zap.SugaredLogger,
	configPath string,
) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-quitCh:
			logger.Info("shutting down from systray")
			shutdown(cancel, pipeline, logger)

			return
		case sig := <-sigCh:
			if sig == syscall.SIGHUP {
				reloadConfig(configPath, cfgReloader, reloadTriggerSighup, logger)

				continue
			}

			logger.Infow("shutting down", "signal", sig)
			shutdown(cancel, pipeline, logger)

			return
		}
	}
}

func shutdown(cancel context.CancelFunc, pipeline *eventPipeline, logger *zap.SugaredLogger) {
	cancel()
	native.StopObservers()
	logEventDropCounts(native.EventDropCount(), pipeline.bus.DropCount(), logger)
	pipeline.bus.Unsubscribe(pipeline.logSub)
	pipeline.bus.Unsubscribe(pipeline.hookSub)
}

// logEventDropCounts logs the native observer's and the event bus's drop
// counters as distinct fields. Shutdown is the first — and, per #94, the
// only — place either count becomes observable: the native counter lives in
// the daemon process's own address space (mimi status runs in the CLI
// process and can never see it), and the bus has no other reader.
func logEventDropCounts(nativeDropped, busDropped int64, logger *zap.SugaredLogger) {
	logger.Infow(
		"event drop counts",
		"native_dropped", nativeDropped,
		"bus_dropped", busDropped,
	)
}

func writePID(path string) error {
	path = paths.ExpandHome(path)

	err := os.MkdirAll(filepath.Dir(path), 0o755) //nolint:mnd
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644) //nolint:mnd
}

func removePID(path string) {
	_ = os.Remove(paths.ExpandHome(path))
}

func hasWindowEvents(cfg *config.Config) bool {
	return cfg.Hooks.HasGroup(config.GroupWindow)
}

func hasAppEvents(cfg *config.Config) bool {
	return cfg.Hooks.HasGroup(config.GroupApp)
}

func hasWorkspaceEvents(cfg *config.Config) bool {
	return cfg.Hooks.HasGroup(config.GroupWorkspace)
}

func getObserverConfig(cfg *config.Config) native.ObserverConfig {
	return native.ObserverConfig{
		// Window hooks need the app-lifecycle observer too, not just the AX
		// observers: AX observers attach per process, so the daemon relies on
		// launch and terminate notifications to attach and detach them. This
		// union is policy, which is why it lives here rather than as a column
		// on config.HookKinds.
		AppLifecycle: hasWindowEvents(cfg) || hasAppEvents(cfg),
		Workspace:    hasWorkspaceEvents(cfg),
	}
}
