//nolint:testpackage // tests reloader, an unexported type
package daemon

import (
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
	"github.com/y3owk1n/mimi/internal/observe"
)

const (
	reloaderHookRunOld   = "echo old"
	reloaderHookRunNew   = "echo new"
	reloaderInvalidRegex = "["
)

// newTestReloader wires up a reloader against real (non-nil) collaborators,
// mirroring what setupEventPipeline hands to the daemon at startup, so
// Apply's tests exercise the same objects every trigger shares in
// production.
func newTestReloader(
	t *testing.T,
	initialCfg *config.Config,
) (*reloader, *hooks.Registry, *events.Bus) {
	t.Helper()

	logger := zap.NewNop().Sugar()

	reg := hooks.NewRegistry()

	err := reg.Reload(initialCfg)
	if err != nil {
		t.Fatalf("failed to seed registry with initial config: %v", err)
	}

	bus := events.NewBus()
	axTracker := observe.NewAXTracker(false)
	router := observe.NewRouterWithDebounce(
		bus,
		axTracker,
		logger,
		time.Duration(initialCfg.Settings.ResizeDebounceMS)*time.Millisecond,
	)
	executor := hooks.NewExecutor(reg, &initialCfg.Settings, logger)

	cfgReloader := newReloader(initialCfg, reg, executor, axTracker, router)

	return cfgReloader, reg, bus
}

// TestReloader_Apply_ReportsRestartOnlySettingsThatChanged pins what a
// trigger learns from a reload: the restart-only settings the new config
// changes, so the caller can say a restart is needed instead of reporting a
// success that changed nothing.
func TestReloader_Apply_ReportsRestartOnlySettingsThatChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*config.Config)
		want   []string
	}{
		{
			name:   "only reloadable settings changed",
			change: func(c *config.Config) { c.Settings.HookShell = "/bin/bash" },
			want:   nil,
		},
		{
			name: "restart-only settings changed",
			change: func(c *config.Config) {
				c.Settings.LogLevel = "debug"
				c.Settings.MaxHookWorkers = 8
			},
			want: []string{keyLogLevel, keyMaxHookWorkers},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			oldCfg := &config.Config{Settings: baseSettings()}
			oldCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunOld}}

			cfgReloader, _, _ := newTestReloader(t, oldCfg)

			newCfg := &config.Config{Settings: baseSettings()}
			newCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunNew}}
			testCase.change(newCfg)

			restartOnly, err := cfgReloader.Apply(newCfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(restartOnly, testCase.want) {
				t.Errorf("Apply() restart-only settings = %v, want %v", restartOnly, testCase.want)
			}
		})
	}
}

// TestReloader_Apply_ComparesAgainstTheConfigTheDaemonIsRunning pins which
// config a restart-only change is measured against: the one the daemon
// started with, not the one it reloaded last. A restart-only setting only
// ever takes effect at startup, so putting the old value back leaves nothing
// out of step and must stop asking for a restart — which it would not do if
// each reload became the next reload's baseline.
func TestReloader_Apply_ComparesAgainstTheConfigTheDaemonIsRunning(t *testing.T) {
	t.Parallel()

	oldCfg := &config.Config{Settings: baseSettings()}
	oldCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunOld}}

	cfgReloader, _, _ := newTestReloader(t, oldCfg)

	changedCfg := &config.Config{Settings: baseSettings()}
	changedCfg.Settings.LogLevel = "debug"

	restartOnly, err := cfgReloader.Apply(changedCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(restartOnly, []string{keyLogLevel}) {
		t.Fatalf("Apply() restart-only settings = %v, want %v", restartOnly, []string{keyLogLevel})
	}

	revertedCfg := &config.Config{Settings: baseSettings()}

	restartOnly, err = cfgReloader.Apply(revertedCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if restartOnly != nil {
		t.Errorf(
			"putting a restart-only setting back to the running value still asks for a restart: %v",
			restartOnly,
		)
	}
}

// TestReloader_Apply_WaitsForAReloadAlreadyInProgress pins the serialization
// ADR 0002 calls for. The config file watcher and the signal loop are separate
// goroutines, so a file save and a SIGHUP can arrive together; without a lock
// held across the whole of Apply the two interleave into hooks from one config
// and executor settings from another.
//
// Holding the reloader's own lock is how the test stands in for "a reload is
// in progress": a second Apply must make no change at all until the first has
// finished, and must then apply its config whole. Reaching for the lock is
// white-box, but the lock is the mechanism the ADR decided on, and the
// alternative — racing two reloads and hoping to catch a tear — passes just as
// happily with no lock at all.
//
// What this proves is that none of Apply's writes happen before the lock is
// taken, and that a blocked reload lands whole once it is released. That the
// lock is also still held at Apply's last write is not observable from here;
// it is visible in Apply's `defer rl.mu.Unlock()`.
func TestReloader_Apply_WaitsForAReloadAlreadyInProgress(t *testing.T) {
	t.Parallel()

	const (
		debounceOld = 100
		debounceNew = 900
		// How long a blocked Apply is given to wrongly make progress. Only a
		// lower bound on the test's patience: it never waits this long unless
		// the serialization is broken.
		blockedFor = 100 * time.Millisecond
	)

	oldCfg := &config.Config{Settings: baseSettings()}
	oldCfg.Settings.ResizeDebounceMS = debounceOld
	oldCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunOld}}

	cfgReloader, reg, bus := newTestReloader(t, oldCfg)

	newCfg := &config.Config{Settings: baseSettings()}
	newCfg.Settings.ResizeDebounceMS = debounceNew
	newCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunNew}}

	logger := zap.NewNop().Sugar()

	applied := make(chan struct{})

	// Stand in for a reload that is mid-flight, then start a second one.
	cfgReloader.mu.Lock()

	go func() {
		defer close(applied)

		_, err := cfgReloader.Apply(newCfg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}()

	select {
	case <-applied:
		t.Fatal("a second reload ran while one was already in progress")
	case <-time.After(blockedFor):
	}

	got := reg.HooksFor(events.WindowFocus)
	if len(got) != 1 || got[0].Entry.Run != reloaderHookRunOld {
		t.Errorf("a reload in progress was overwritten by a second one, got %+v", got)
	}

	cfgReloader.mu.Unlock()
	<-applied

	// Whole, not partial: the hooks and the debounce window both come from
	// the config the second reload applied.
	got = reg.HooksFor(events.WindowFocus)
	if len(got) != 1 || got[0].Entry.Run != reloaderHookRunNew {
		t.Errorf("expected the registry to hold the second reload's hook, got %+v", got)
	}

	wantRouter := observe.NewRouterWithDebounce(
		bus,
		cfgReloader.axTracker,
		logger,
		debounceNew*time.Millisecond,
	)
	if !reflect.DeepEqual(wantRouter, cfgReloader.router) {
		t.Error(
			"the second reload applied its hooks but not its debounce window; the reload tore",
		)
	}
}

func TestReloader_Apply_ValidConfigReloadsHooksAndDependencies(t *testing.T) {
	oldCfg := &config.Config{Settings: baseSettings()}
	oldCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunOld}}

	cfgReloader, reg, bus := newTestReloader(t, oldCfg)

	newCfg := &config.Config{Settings: baseSettings()}
	newCfg.Settings.ResizeDebounceMS = 750
	newCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunNew}}

	_, err := cfgReloader.Apply(newCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := reg.HooksFor(events.WindowFocus)
	if len(got) != 1 || got[0].Entry.Run != reloaderHookRunNew {
		t.Errorf("expected registry to hold the new hook, got %+v", got)
	}

	logger := zap.NewNop().Sugar()

	wantRouter := observe.NewRouterWithDebounce(
		bus,
		cfgReloader.axTracker,
		logger,
		750*time.Millisecond,
	)
	if !reflect.DeepEqual(wantRouter, cfgReloader.router) {
		t.Errorf("expected router debounce window to be updated to 750ms")
	}

	// A whole-struct reflect.DeepEqual against a freshly built
	// observe.NewAXTracker(true) doesn't work here: AXTracker carries
	// unexported installAX/removeAX func fields, and Go only treats two func
	// values as deeply equal when both are nil, so two independently
	// constructed trackers are never DeepEqual regardless of their enabled
	// state. axTrackerEnabled (setup_event_pipeline_test.go) reads the
	// unexported field directly instead.
	const wantEnabled = true
	if got := axTrackerEnabled(cfgReloader.axTracker); got != wantEnabled {
		t.Errorf(
			"expected AX tracker enabled state to follow the new config's window hooks, got enabled=%v want %v",
			got,
			wantEnabled,
		)
	}
}

func TestReloader_Apply_InvalidHookRegexLeavesPreviousStateUntouched(t *testing.T) {
	oldCfg := &config.Config{Settings: baseSettings()}
	oldCfg.Settings.ResizeDebounceMS = 250
	oldCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunOld}}

	cfgReloader, reg, bus := newTestReloader(t, oldCfg)

	logger := zap.NewNop().Sugar()
	wantRouterBefore := observe.NewRouterWithDebounce(
		bus,
		cfgReloader.axTracker,
		logger,
		250*time.Millisecond,
	)
	wantEnabledBefore := axTrackerEnabled(cfgReloader.axTracker)

	newCfg := &config.Config{Settings: baseSettings()}
	newCfg.Settings.ResizeDebounceMS = 999
	newCfg.Hooks.WindowFocus = []config.HookEntry{
		{Run: reloaderHookRunNew, Title: reloaderInvalidRegex},
	}

	restartOnly, err := cfgReloader.Apply(newCfg)
	if err == nil {
		t.Fatal("expected an error for an invalid hook regex, got nil")
	}

	if restartOnly != nil {
		t.Errorf(
			"a failed reload reported %v as needing a restart; nothing was applied, so nothing does",
			restartOnly,
		)
	}

	if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
		t.Errorf("expected CodeInvalidConfig, got %v", derrors.GetCode(err))
	}

	got := reg.HooksFor(events.WindowFocus)
	if len(got) != 1 || got[0].Entry.Run != reloaderHookRunOld {
		t.Errorf("expected registry to still hold the old hook after a failed reload, got %+v", got)
	}

	if !reflect.DeepEqual(wantRouterBefore, cfgReloader.router) {
		t.Error("expected router debounce window to be left untouched by a failed reload")
	}

	// See the comment on the first axTrackerEnabled call above: AXTracker's
	// unexported func fields make whole-struct reflect.DeepEqual against a
	// freshly built tracker always false, so compare the enabled flag
	// directly instead — captured before Apply so this asserts "unchanged",
	// not a hardcoded value.
	if got := axTrackerEnabled(cfgReloader.axTracker); got != wantEnabledBefore {
		t.Errorf(
			"expected AX tracker state to be left untouched by a failed reload, got enabled=%v want %v",
			got,
			wantEnabledBefore,
		)
	}
}
