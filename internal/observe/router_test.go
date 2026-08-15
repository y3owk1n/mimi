//nolint:testpackage
package observe

import (
	"slices"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
)

const (
	testAppName     = "TestApp"
	testAppShort    = "App"
	testWindowTitle = "Win"
	testBundleID    = "com.test.app"

	// testDebounceWindow keeps the debounce suite's wall-clock down: short
	// enough that a real fire is fast, long enough that scheduling jitter
	// under `-race` doesn't produce a flake.
	testDebounceWindow = 50 * time.Millisecond
	// testFireTimeout is how long a test waits for a debounced event to
	// arrive; comfortably above testDebounceWindow.
	testFireTimeout = 500 * time.Millisecond
	// testNoFireWait is how long a test waits to confirm nothing arrives;
	// comfortably above testDebounceWindow so a wrongly-still-running timer
	// would be caught.
	testNoFireWait = 200 * time.Millisecond
	// testCoalesceSpacing is the gap between successive resize events in the
	// coalescing test; it must stay well under testDebounceWindow so each
	// event resets the timer instead of letting it fire early.
	testCoalesceSpacing = 15 * time.Millisecond
)

func newTestRouter(t *testing.T) (*Router, <-chan events.Event) {
	t.Helper()

	bus := events.NewBus()
	sub := bus.Subscribe(16)
	ax := NewAXTracker(false)
	logger := zap.NewNop().Sugar()
	router := NewRouterWithDebounce(bus, ax, logger, testDebounceWindow)

	return router, sub
}

// newHandleTestRouter returns a Router wired to an enabled AXTracker whose
// native calls are faked (via newFakeTracker, shared with ax_test.go), for
// exercising Router.handle's AX install/remove branches.
func newHandleTestRouter(t *testing.T) (*Router, <-chan events.Event, *AXTracker, *fakeAXCalls) {
	t.Helper()

	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axTracker, fake := newFakeTracker(true, true)
	logger := zap.NewNop().Sugar()
	router := NewRouterWithDebounce(bus, axTracker, logger, testDebounceWindow)

	return router, sub, axTracker, fake
}

// hookableKinds is a hardcoded mirror of buildMap's literal kind→config map
// (internal/hooks/registry.go), kept deliberately independent of
// events.AllKinds: if a future internal kind were mistakenly added to
// AllKinds, a filter built from AllKinds would pick up the same mistake and
// this test would stop catching it. Hardcoding the list here means the test
// only passes if `_`-prefixed and unrecognized kinds are excluded on their
// own merits.
var hookableKinds = []events.EventKind{
	events.AppActivate,
	events.AppDeactivate,
	events.AppLaunch,
	events.AppQuit,
	events.AppHide,
	events.AppUnhide,
	events.WindowFocus,
	events.WindowTitleChange,
	events.WindowCreated,
	events.WindowClosed,
	events.WindowResize,
	events.WorkspaceChanged,
}

// isHookableKind is a hookSub-style filter built from hookableKinds, mirroring
// registry.KindFilter() (internal/hooks/registry.go) without depending on it.
func isHookableKind(kind events.EventKind) bool {
	return slices.Contains(hookableKinds, kind)
}

func TestDebounceResize_SingleEvent(t *testing.T) {
	router, sub := newTestRouter(t)

	evt := events.Event{
		Kind:        events.WindowResizing,
		AppName:     testAppName,
		BundleID:    testBundleID,
		PID:         42,
		WindowTitle: "Test Window",
		At:          time.Now(),
	}

	router.debounceResize(evt)

	select {
	case _evt := <-sub:
		if _evt.Kind != events.WindowResize {
			t.Errorf("expected WindowResize, got %s", _evt.Kind)
		}

		if _evt.AppName != testAppName {
			t.Errorf("expected AppName TestApp, got %s", _evt.AppName)
		}

		if _evt.PID != 42 {
			t.Errorf("expected PID 42, got %d", _evt.PID)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for debounced resize event")
	}

	router.mu.Lock()
	remaining := len(router.timers)
	router.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining timers, got %d", remaining)
	}
}

func TestDebounceResize_MultipleEventsCoalesce(t *testing.T) {
	router, sub := newTestRouter(t)

	for index := range 5 {
		evt := events.Event{
			Kind:        events.WindowResizing,
			AppName:     testAppName,
			BundleID:    testBundleID,
			PID:         42,
			WindowTitle: "Test Window",
			At:          time.Now(),
			Extra:       map[string]string{"seq": string(rune('0' + index))},
		}
		router.debounceResize(evt)
		time.Sleep(testCoalesceSpacing)
	}

	select {
	case e := <-sub:
		if e.Kind != events.WindowResize {
			t.Errorf("expected WindowResize, got %s", e.Kind)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for debounced resize event")
	}

	select {
	case e := <-sub:
		t.Errorf("unexpected second event: %+v", e)
	case <-time.After(testNoFireWait):
	}
}

func TestDebounceResize_DifferentWindows(t *testing.T) {
	router, sub := newTestRouter(t)

	router.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 1, WindowTitle: "Win A", AppName: "AppA",
	})
	router.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 2, WindowTitle: "Win B", AppName: "AppB",
	})

	received := make(map[string]bool)

	for range 2 {
		select {
		case e := <-sub:
			received[e.AppName] = true
		case <-time.After(testFireTimeout):
			t.Fatal("timed out waiting for debounced resize events")
		}
	}

	if !received["AppA"] || !received["AppB"] {
		t.Errorf("expected events from both windows, got: %v", received)
	}
}

func TestDebounceResize_CancelTimersForPID(t *testing.T) {
	router, sub := newTestRouter(t)

	router.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 99, WindowTitle: testWindowTitle, AppName: testAppShort,
	})

	router.cancelTimersForPID(99)

	select {
	case e := <-sub:
		t.Errorf("expected no event after cancel, got: %+v", e)
	case <-time.After(testNoFireWait):
	}

	router.mu.Lock()
	remaining := len(router.timers)
	router.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining timers after cancel, got %d", remaining)
	}
}

func TestDebounceResize_StopAllTimers(t *testing.T) {
	router, sub := newTestRouter(t)

	for i := range 3 {
		router.debounceResize(events.Event{
			Kind:        events.WindowResizing,
			PID:         i + 1,
			WindowTitle: testWindowTitle,
			AppName:     testAppShort,
		})
	}

	router.stopAllTimers()

	select {
	case e := <-sub:
		t.Errorf("expected no events after stopAllTimers, got: %+v", e)
	case <-time.After(testNoFireWait):
	}

	router.mu.Lock()
	remaining := len(router.timers)
	stopped := router.stopped
	router.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining timers, got %d", remaining)
	}

	if !stopped {
		t.Error("expected stopped flag to be true")
	}
}

func TestDebounceResize_NoPublishAfterStop(t *testing.T) {
	router, sub := newTestRouter(t)

	router.stopAllTimers()

	router.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 1, WindowTitle: testWindowTitle, AppName: testAppShort,
	})

	select {
	case e := <-sub:
		t.Errorf("expected no events after stop, got: %+v", e)
	case <-time.After(testNoFireWait):
	}
}

func TestHandle_AppActivate_InstallsAXObserverAndPublishes(t *testing.T) {
	router, sub, _, fake := newHandleTestRouter(t)

	router.handle(events.Event{Kind: events.AppActivate, PID: 7, AppName: testAppName})

	if len(fake.installed) != 1 || fake.installed[0] != 7 {
		t.Errorf("installAX calls = %v, want [7]", fake.installed)
	}

	select {
	case evt := <-sub:
		if evt.Kind != events.AppActivate || evt.PID != 7 {
			t.Errorf("published event = %+v, want kind=app_activate pid=7", evt)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for app_activate to publish")
	}
}

func TestHandle_AppLaunch_InstallsAXObserverAndPublishes(t *testing.T) {
	router, sub, _, fake := newHandleTestRouter(t)

	router.handle(events.Event{Kind: events.AppLaunch, PID: 11, AppName: testAppName})

	if len(fake.installed) != 1 || fake.installed[0] != 11 {
		t.Errorf("installAX calls = %v, want [11]", fake.installed)
	}

	select {
	case evt := <-sub:
		if evt.Kind != events.AppLaunch || evt.PID != 11 {
			t.Errorf("published event = %+v, want kind=app_launch pid=11", evt)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for app_launch to publish")
	}
}

func TestHandle_AppActivate_ZeroPIDSkipsInstallButStillPublishes(t *testing.T) {
	router, sub, _, fake := newHandleTestRouter(t)

	router.handle(events.Event{Kind: events.AppActivate, PID: 0, AppName: testAppName})

	if len(fake.installed) != 0 {
		t.Errorf("installAX calls = %v, want none for PID 0", fake.installed)
	}

	select {
	case evt := <-sub:
		if evt.Kind != events.AppActivate {
			t.Errorf("published event kind = %s, want app_activate", evt.Kind)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for app_activate to publish")
	}
}

func TestHandle_AppQuit_RemovesAXObserverAndCancelsTimers(t *testing.T) {
	router, sub, ax, fake := newHandleTestRouter(t)

	const pid = 21

	if ok := ax.Install(pid); !ok {
		t.Fatalf("ax.Install(%d) = false, want true (test setup)", pid)
	}

	router.debounceResize(
		events.Event{Kind: events.WindowResizing, PID: pid, WindowTitle: testWindowTitle},
	)

	router.mu.Lock()
	before := len(router.timers)
	router.mu.Unlock()

	if before != 1 {
		t.Fatalf("timers before quit = %d, want 1 (test setup)", before)
	}

	router.handle(events.Event{Kind: events.AppQuit, PID: pid, AppName: testAppName})

	if len(fake.removed) != 1 || fake.removed[0] != pid {
		t.Errorf("removeAX calls = %v, want [%d]", fake.removed, pid)
	}

	router.mu.Lock()
	after := len(router.timers)
	router.mu.Unlock()

	if after != 0 {
		t.Errorf("timers after quit = %d, want 0 (cancelTimersForPID should have run)", after)
	}

	select {
	case evt := <-sub:
		if evt.Kind != events.AppQuit || evt.PID != pid {
			t.Errorf("published event = %+v, want kind=app_quit pid=%d", evt, pid)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for app_quit to publish")
	}

	// The canceled resize timer must never fire a debounced event.
	select {
	case evt := <-sub:
		t.Errorf("unexpected second event after quit, got: %+v", evt)
	case <-time.After(testNoFireWait):
	}
}

func TestHandle_AppQuit_ZeroPIDSkipsRemoveAndCancel(t *testing.T) {
	router, sub, _, fake := newHandleTestRouter(t)

	router.handle(events.Event{Kind: events.AppQuit, PID: 0, AppName: testAppName})

	if len(fake.removed) != 0 {
		t.Errorf("removeAX calls = %v, want none for PID 0", fake.removed)
	}

	select {
	case evt := <-sub:
		if evt.Kind != events.AppQuit {
			t.Errorf("published event kind = %s, want app_quit", evt.Kind)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for app_quit to publish")
	}
}

func TestHandle_WindowResizing_DebouncesWithoutImmediatePublish(t *testing.T) {
	router, sub := newTestRouter(t)

	router.handle(events.Event{
		Kind: events.WindowResizing, PID: 5, WindowTitle: testWindowTitle, AppName: testAppShort,
	})

	// handle() must route WindowResizing through debounceResize and return
	// before publishing anything immediately.
	select {
	case evt := <-sub:
		t.Errorf("unexpected immediate publish for _window_resizing, got: %+v", evt)
	case <-time.After(testCoalesceSpacing):
	}

	select {
	case evt := <-sub:
		if evt.Kind != events.WindowResize {
			t.Errorf("debounced event kind = %s, want window_resize", evt.Kind)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for the debounced resize to publish")
	}
}

func TestHandle_DefaultCase_LogsAndPublishesVerbatim(t *testing.T) {
	router, sub := newTestRouter(t)

	evt := events.Event{
		Kind:     events.WindowFocus,
		AppName:  testAppName,
		BundleID: testBundleID,
		PID:      3,
	}
	router.handle(evt)

	select {
	case got := <-sub:
		if got.Kind != events.WindowFocus || got.PID != 3 || got.AppName != testAppName {
			t.Errorf("published event = %+v, want %+v", got, evt)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for the default-case publish")
	}
}

// TestHandle_InternalKindReachesLogSubButNotHookFilteredSub pins the
// invariant that `_`-prefixed (and otherwise unrecognized) kinds never reach
// a hook subscriber. handle() itself applies no such guard — the default
// case logs and publishes every kind unconditionally, exactly like
// native.StartObservers' inline "_startup_" event (bridge.go:68). The
// invariant holds only because a hookSub-style filter (built from
// events.AllKinds, mirroring buildMap in internal/hooks/registry.go) never
// admits it, while an unfiltered logSub-style subscriber sees everything.
func TestHandle_InternalKindReachesLogSubButNotHookFilteredSub(t *testing.T) {
	bus := events.NewBus()
	logSub := bus.Subscribe(16)
	hookSub := bus.SubscribeWithFilter(16, isHookableKind)
	ax := NewAXTracker(false)
	logger := zap.NewNop().Sugar()
	router := NewRouterWithDebounce(bus, ax, logger, testDebounceWindow)

	router.handle(events.Event{Kind: events.EventKind("_startup_"), AppName: "mimi"})

	select {
	case evt := <-logSub:
		if evt.Kind != events.EventKind("_startup_") {
			t.Errorf("logSub got kind %s, want _startup_", evt.Kind)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for logSub to receive the internal-kind event")
	}

	select {
	case evt := <-hookSub:
		t.Errorf("hookSub received an internal-kind event, want it filtered out: %+v", evt)
	case <-time.After(testNoFireWait):
	}

	// Sanity check: the same filter does admit a real hookable kind, so the
	// prior assertion is proof of filtering, not of a wedged subscriber.
	router.handle(events.Event{Kind: events.AppActivate, PID: 1})

	select {
	case evt := <-hookSub:
		if evt.Kind != events.AppActivate {
			t.Errorf("hookSub got kind %s, want app_activate", evt.Kind)
		}
	case <-time.After(testFireTimeout):
		t.Fatal("timed out waiting for hookSub to receive a real hookable kind")
	}
}
