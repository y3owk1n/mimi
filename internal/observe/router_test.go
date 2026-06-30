//nolint:testpackage
package observe

import (
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
)

func newTestRouter(t *testing.T) (*Router, <-chan events.Event) {
	t.Helper()

	bus := events.NewBus()
	sub := bus.Subscribe(16)
	ax := NewAXTracker(false)
	logger := zap.NewNop().Sugar()
	router := NewRouter(bus, ax, logger)

	return router, sub
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
	case <-time.After(2 * time.Second):
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
		time.Sleep(50 * time.Millisecond)
	}

	select {
	case e := <-sub:
		if e.Kind != events.WindowResize {
			t.Errorf("expected WindowResize, got %s", e.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced resize event")
	}

	select {
	case e := <-sub:
		t.Errorf("unexpected second event: %+v", e)
	case <-time.After(500 * time.Millisecond):
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
		case <-time.After(2 * time.Second):
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
	case <-time.After(500 * time.Millisecond):
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
	case <-time.After(500 * time.Millisecond):
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
	case <-time.After(500 * time.Millisecond):
	}
}
