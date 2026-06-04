//nolint:testpackage
package observers

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
)

func TestDebounceResize_SingleEvent(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axMgr := NewAccessibilityManager(false)
	logger := zap.NewNop().Sugar()

	obs := NewWorkspaceObserver(bus, axMgr, logger)

	evt := events.Event{
		Kind:        events.WindowResizing,
		AppName:     "TestApp",
		BundleID:    "com.test.app",
		PID:         42,
		WindowTitle: "Test Window",
		At:          time.Now(),
	}

	obs.debounceResize(evt)

	select {
	case _evt := <-sub:
		if _evt.Kind != events.WindowResize {
			t.Errorf("expected WindowResize, got %s", _evt.Kind)
		}

		if _evt.AppName != "TestApp" {
			t.Errorf("expected AppName TestApp, got %s", _evt.AppName)
		}

		if _evt.PID != 42 {
			t.Errorf("expected PID 42, got %d", _evt.PID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced resize event")
	}

	// Verify no stale timers remain.
	obs.mu.Lock()
	remaining := len(obs.timers)
	obs.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining timers, got %d", remaining)
	}
}

func TestDebounceResize_MultipleEventsCoalesce(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axMgr := NewAccessibilityManager(false)
	logger := zap.NewNop().Sugar()

	obs := NewWorkspaceObserver(bus, axMgr, logger)

	// Simulate rapid resize events — only the last should produce an event.
	for index := range 5 {
		evt := events.Event{
			Kind:        events.WindowResizing,
			AppName:     "TestApp",
			BundleID:    "com.test.app",
			PID:         42,
			WindowTitle: "Test Window",
			At:          time.Now(),
			Extra:       map[string]string{"seq": string(rune('0' + index))},
		}
		obs.debounceResize(evt)
		time.Sleep(50 * time.Millisecond) // well within the 250ms debounce
	}

	// Should get exactly one event.
	select {
	case e := <-sub:
		if e.Kind != events.WindowResize {
			t.Errorf("expected WindowResize, got %s", e.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced resize event")
	}

	// Verify no second event arrives.
	select {
	case e := <-sub:
		t.Errorf("unexpected second event: %+v", e)
	case <-time.After(500 * time.Millisecond):
		// Expected — no more events.
	}
}

func TestDebounceResize_DifferentWindows(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axMgr := NewAccessibilityManager(false)
	logger := zap.NewNop().Sugar()

	obs := NewWorkspaceObserver(bus, axMgr, logger)

	// Two different windows should produce two separate events.
	obs.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 1, WindowTitle: "Win A", AppName: "AppA",
	})
	obs.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 2, WindowTitle: "Win B", AppName: "AppB",
	})

	received := make(map[string]bool)

	for range 2 {
		select {
		case e := <-sub:
			received[e.AppName] = true
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for debounced resize events")
		}
	}

	if !received["AppA"] || !received["AppB"] {
		t.Errorf("expected events from both windows, got: %v", received)
	}
}

func TestDebounceResize_CancelTimersForPID(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axMgr := NewAccessibilityManager(false)
	logger := zap.NewNop().Sugar()

	obs := NewWorkspaceObserver(bus, axMgr, logger)

	obs.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 99, WindowTitle: "Win", AppName: "App",
	})

	// Cancel before the timer fires.
	obs.cancelTimersForPID(99)

	select {
	case e := <-sub:
		t.Errorf("expected no event after cancel, got: %+v", e)
	case <-time.After(500 * time.Millisecond):
		// Expected — timer was canceled.
	}

	obs.mu.Lock()
	remaining := len(obs.timers)
	obs.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining timers after cancel, got %d", remaining)
	}
}

func TestDebounceResize_StopAllTimers(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axMgr := NewAccessibilityManager(false)
	logger := zap.NewNop().Sugar()

	obs := NewWorkspaceObserver(bus, axMgr, logger)

	// Queue up several resize events for different windows.
	for i := range 3 {
		obs.debounceResize(events.Event{
			Kind: events.WindowResizing, PID: i + 1, WindowTitle: "Win", AppName: "App",
		})
	}

	obs.stopAllTimers()

	select {
	case e := <-sub:
		t.Errorf("expected no events after stopAllTimers, got: %+v", e)
	case <-time.After(500 * time.Millisecond):
		// Expected — all timers were stopped.
	}

	obs.mu.Lock()
	remaining := len(obs.timers)
	stopped := obs.stopped
	obs.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining timers, got %d", remaining)
	}

	if !stopped {
		t.Error("expected stopped flag to be true")
	}
}

func TestDebounceResize_NoPublishAfterStop(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(16)
	axMgr := NewAccessibilityManager(false)
	logger := zap.NewNop().Sugar()

	obs := NewWorkspaceObserver(bus, axMgr, logger)

	// Stop immediately — any subsequent debounce calls should be no-ops.
	obs.stopAllTimers()

	obs.debounceResize(events.Event{
		Kind: events.WindowResizing, PID: 1, WindowTitle: "Win", AppName: "App",
	})

	select {
	case e := <-sub:
		t.Errorf("expected no events after stop, got: %+v", e)
	case <-time.After(500 * time.Millisecond):
		// Expected.
	}
}
