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

	cfgReloader := newReloader(reg, executor, axTracker, router)

	return cfgReloader, reg, bus
}

func TestReloader_Apply_ValidConfigReloadsHooksAndDependencies(t *testing.T) {
	oldCfg := &config.Config{Settings: baseSettings()}
	oldCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunOld}}

	cfgReloader, reg, bus := newTestReloader(t, oldCfg)

	newCfg := &config.Config{Settings: baseSettings()}
	newCfg.Settings.ResizeDebounceMS = 750
	newCfg.Hooks.WindowFocus = []config.HookEntry{{Run: reloaderHookRunNew}}

	err := cfgReloader.Apply(newCfg)
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

	wantTracker := observe.NewAXTracker(true)
	if !reflect.DeepEqual(wantTracker, cfgReloader.axTracker) {
		t.Errorf("expected AX tracker enabled state to follow the new config's window hooks")
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
	wantTrackerBefore := observe.NewAXTracker(false)

	newCfg := &config.Config{Settings: baseSettings()}
	newCfg.Settings.ResizeDebounceMS = 999
	newCfg.Hooks.WindowFocus = []config.HookEntry{
		{Run: reloaderHookRunNew, Title: reloaderInvalidRegex},
	}

	err := cfgReloader.Apply(newCfg)
	if err == nil {
		t.Fatal("expected an error for an invalid hook regex, got nil")
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

	if !reflect.DeepEqual(wantTrackerBefore, cfgReloader.axTracker) {
		t.Error("expected AX tracker state to be left untouched by a failed reload")
	}
}
