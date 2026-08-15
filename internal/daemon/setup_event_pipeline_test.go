//nolint:testpackage // tests setupEventPipeline, an unexported function
package daemon

import (
	"context"
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
	pipelineHookRun    = "echo"
	pipelineInvalidRe  = "["
	pipelineWaitWindow = time.Second
)

// pipelineResult names setupEventPipeline's ten-value return so test bodies
// can address the piece they care about instead of a wall of blanks.
type pipelineResult struct {
	bus       *events.Bus
	axTracker *observe.AXTracker
	router    *observe.Router
	reg       *hooks.Registry
	executor  *hooks.Executor
	logSub    events.Subscriber
	hookSub   events.Subscriber
	ctx       context.Context //nolint:containedctx // captured from setupEventPipeline for assertions
	cancel    context.CancelFunc
	err       error
}

func runSetupEventPipeline(
	cfg *config.Config,
	logger *zap.SugaredLogger,
	accessibilityGranted bool,
) pipelineResult {
	bus, axTracker, router, reg, executor, logSub, hookSub, ctx, cancel, err := setupEventPipeline(
		cfg,
		logger,
		accessibilityGranted,
	)

	return pipelineResult{
		bus:       bus,
		axTracker: axTracker,
		router:    router,
		reg:       reg,
		executor:  executor,
		logSub:    logSub,
		hookSub:   hookSub,
		ctx:       ctx,
		cancel:    cancel,
		err:       err,
	}
}

// mustSetupPipeline runs setupEventPipeline, fails the test on error, and
// registers the returned cancel func for cleanup. It exists so the tests
// below that expect success don't each repeat the same error-check and
// cleanup boilerplate.
func mustSetupPipeline(
	t *testing.T,
	cfg *config.Config,
	logger *zap.SugaredLogger,
	accessibilityGranted bool,
) pipelineResult {
	t.Helper()

	result := runSetupEventPipeline(cfg, logger, accessibilityGranted)
	if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}

	t.Cleanup(result.cancel)

	return result
}

// baseSettings returns a SettingsConfig with the fields setupEventPipeline
// and its collaborators need populated to avoid degenerate zero values
// (e.g. an unbuffered executor semaphore).
func baseSettings() config.SettingsConfig {
	return config.SettingsConfig{
		HookShell:       "/bin/sh",
		HookTimeoutSecs: 5,
		MaxHookWorkers:  1,
	}
}

func TestSetupEventPipeline_AXTrackerEnabledState(t *testing.T) {
	scenarios := []struct {
		name                 string
		windowHooks          bool
		accessibilityGranted bool
		wantEnabled          bool
	}{
		{
			name:                 "window hooks and accessibility granted enables AX",
			windowHooks:          true,
			accessibilityGranted: true,
			wantEnabled:          true,
		},
		{
			name:                 "window hooks without accessibility disables AX",
			windowHooks:          true,
			accessibilityGranted: false,
			wantEnabled:          false,
		},
		{
			name:                 "accessibility without window hooks disables AX",
			windowHooks:          false,
			accessibilityGranted: true,
			wantEnabled:          false,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := &config.Config{Settings: baseSettings()}
			if scenario.windowHooks {
				cfg.Hooks.WindowFocus = []config.HookEntry{{Run: pipelineHookRun}}
			}

			logger := zap.NewNop().Sugar()

			result := mustSetupPipeline(t, cfg, logger, scenario.accessibilityGranted)

			// AXTracker exposes no getter for its enabled state, so compare
			// the returned tracker against a freshly constructed one: both
			// are unused (empty tracked map, zero-value mutex), so the
			// comparison isolates the enabled flag setupEventPipeline chose.
			want := observe.NewAXTracker(scenario.wantEnabled)
			if !reflect.DeepEqual(want, result.axTracker) {
				t.Errorf("AX tracker enabled state mismatch: want enabled=%v", scenario.wantEnabled)
			}
		})
	}
}

func TestSetupEventPipeline_ResizeDebounceReachesRouter(t *testing.T) {
	scenarios := []struct {
		name         string
		debounceMS   int
		wantDebounce time.Duration
	}{
		{
			name:         "explicit debounce is threaded through",
			debounceMS:   500,
			wantDebounce: 500 * time.Millisecond,
		},
		{
			name:         "zero debounce falls back to the router default",
			debounceMS:   0,
			wantDebounce: 0, // NewRouterWithDebounce(..., 0) applies its own default
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := &config.Config{Settings: baseSettings()}
			cfg.Settings.ResizeDebounceMS = scenario.debounceMS

			logger := zap.NewNop().Sugar()

			result := mustSetupPipeline(t, cfg, logger, false)

			// Router exposes no getter for its debounce window. Build a
			// comparison router from the exact same bus/AX-tracker pointers
			// and logger so every other field is trivially identical, which
			// isolates the debounceWindow field in the comparison.
			want := observe.NewRouterWithDebounce(
				result.bus,
				result.axTracker,
				logger,
				scenario.wantDebounce,
			)
			if !reflect.DeepEqual(want, result.router) {
				t.Errorf(
					"router debounce window was not built from settings.resize_debounce_ms=%d",
					scenario.debounceMS,
				)
			}
		})
	}
}

func TestSetupEventPipeline_InvalidHookRegexSurfacesInvalidConfig(t *testing.T) {
	cfg := &config.Config{Settings: baseSettings()}
	cfg.Hooks.WindowFocus = []config.HookEntry{{Run: pipelineHookRun, Title: pipelineInvalidRe}}

	logger := zap.NewNop().Sugar()

	result := runSetupEventPipeline(cfg, logger, true)
	if result.err == nil {
		t.Fatal("expected an error for an invalid hook regex, got nil")
	}

	if !derrors.IsCode(result.err, derrors.CodeInvalidConfig) {
		t.Errorf("expected CodeInvalidConfig, got %v", derrors.GetCode(result.err))
	}

	if result.bus != nil || result.axTracker != nil || result.router != nil ||
		result.reg != nil || result.executor != nil {
		t.Error("expected all pointer results to be nil on error")
	}

	if result.logSub != nil || result.hookSub != nil {
		t.Error("expected both subscribers to be nil on error")
	}

	if result.ctx != nil || result.cancel != nil {
		t.Error("expected context and cancel to be nil on error")
	}
}

func TestSetupEventPipeline_LogSubscriber(t *testing.T) {
	// This event kind has no registered hook in either subtest's config, so
	// hookSub's registry filter would reject it — a useful control to make
	// sure the log subscriber's own filter (not the hook registry's) is
	// what's under test.
	const unhookedKind = events.AppQuit

	t.Run("log_file set subscribes a real subscriber", func(t *testing.T) {
		cfg := &config.Config{Settings: baseSettings()}
		cfg.Settings.LogFile = "/tmp/mimi-test-event-log.jsonl"

		logger := zap.NewNop().Sugar()

		result := mustSetupPipeline(t, cfg, logger, false)

		result.bus.Publish(events.Event{Kind: unhookedKind, At: time.Now()})

		select {
		case evt := <-result.logSub:
			if evt.Kind != unhookedKind {
				t.Errorf("expected kind %s, got %s", unhookedKind, evt.Kind)
			}
		case <-time.After(pipelineWaitWindow):
			t.Fatal("expected log subscriber to receive the published event")
		}
	})

	t.Run("log_file unset subscribes a subscriber that rejects everything", func(t *testing.T) {
		cfg := &config.Config{Settings: baseSettings()}

		logger := zap.NewNop().Sugar()

		result := mustSetupPipeline(t, cfg, logger, false)

		for _, kind := range events.AllKinds {
			result.bus.Publish(events.Event{Kind: kind, At: time.Now()})
		}

		select {
		case evt := <-result.logSub:
			t.Fatalf("expected no-op log subscriber to receive nothing, got kind %s", evt.Kind)
		case <-time.After(100 * time.Millisecond):
		}
	})
}

func TestSetupEventPipeline_HookSubUsesRegistryKindFilter(t *testing.T) {
	cfg := &config.Config{Settings: baseSettings()}
	cfg.Hooks.WindowFocus = []config.HookEntry{{Run: pipelineHookRun}}

	logger := zap.NewNop().Sugar()

	result := mustSetupPipeline(t, cfg, logger, true)

	// WindowFocus has a registered hook: the registry's KindFilter should
	// let it through.
	result.bus.Publish(events.Event{Kind: events.WindowFocus, At: time.Now()})

	select {
	case evt := <-result.hookSub:
		if evt.Kind != events.WindowFocus {
			t.Errorf("expected kind %s, got %s", events.WindowFocus, evt.Kind)
		}
	case <-time.After(pipelineWaitWindow):
		t.Fatal("expected hook subscriber to receive an event kind with a registered hook")
	}

	// AppQuit has no registered hook: the registry's KindFilter should drop
	// it before it reaches the channel.
	result.bus.Publish(events.Event{Kind: events.AppQuit, At: time.Now()})

	select {
	case evt := <-result.hookSub:
		t.Fatalf("expected hook subscriber to reject an unregistered kind, got %s", evt.Kind)
	case <-time.After(100 * time.Millisecond):
	}
}
