package observers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
)

const resizeDebounceDuration = 250 * time.Millisecond

// WorkspaceObserver receives CGO bridge events and routes them to the bus.
type WorkspaceObserver struct {
	bus    *events.Bus
	axMgr  *AccessibilityManager
	logger *zap.SugaredLogger

	mu      sync.Mutex
	timers  map[string]*resizeState
	stopped bool
}

// resizeState holds a pending debounce timer and the latest event snapshot for a window.
type resizeState struct {
	timer *time.Timer
	evt   events.Event
}

// NewWorkspaceObserver creates a new workspace observer.
func NewWorkspaceObserver(
	bus *events.Bus,
	axMgr *AccessibilityManager,
	logger *zap.SugaredLogger,
) *WorkspaceObserver {
	return &WorkspaceObserver{
		bus:    bus,
		axMgr:  axMgr,
		logger: logger,
		timers: make(map[string]*resizeState),
	}
}

// Run starts the observer loop, consuming events and routing them.
func (o *WorkspaceObserver) Run(ctx context.Context) {
	evChannel := cgo_bridge.EventCh()
	for {
		select {
		case <-ctx.Done():
			o.stopAllTimers()

			return
		case event, ok := <-evChannel:
			if !ok {
				o.stopAllTimers()

				return
			}

			o.handle(event)
		}
	}
}

func (o *WorkspaceObserver) handle(evt events.Event) {
	switch evt.Kind { //nolint:exhaustive
	case events.AppActivate, events.AppLaunch:
		if evt.PID > 0 {
			if ok := o.axMgr.Install(evt.PID); !ok {
				o.logger.Debugw("AX observer install failed",
					"pid", evt.PID, "app", evt.AppName)
			}
		}
	case events.AppQuit:
		if evt.PID > 0 {
			o.axMgr.Remove(evt.PID)
			o.cancelTimersForPID(evt.PID)
		}
	case events.WindowResizing:
		o.debounceResize(evt)

		return
	default:
	}

	o.logger.Debugw("event",
		"kind", evt.Kind,
		"app", evt.AppName,
		"bundle", evt.BundleID,
		"pid", evt.PID,
		"title", evt.WindowTitle,
		"vol", evt.VolumeName,
	)
	o.bus.Publish(evt)
}

// resizeKey returns a unique key for debouncing resize events per window.
func resizeKey(pid int, title string) string {
	return fmt.Sprintf("%d:%s", pid, title)
}

// debounceResize resets or starts a debounce timer for the given resize event.
// When the timer fires (after 250ms of no further resize events for the same
// window), a single WindowResize event is published.
func (o *WorkspaceObserver) debounceResize(evt events.Event) {
	key := resizeKey(evt.PID, evt.WindowTitle)

	o.mu.Lock()
	defer o.mu.Unlock()

	if o.stopped {
		return
	}

	if rState, ok := o.timers[key]; ok {
		// Stop the existing timer. Even if Stop returns false (timer already
		// fired or is firing), the callback checks whether the entry still
		// exists in the map before publishing, so we are safe to overwrite.
		rState.timer.Stop()
		rState.evt = evt
		rState.timer.Reset(resizeDebounceDuration)

		return
	}

	rState := &resizeState{evt: evt}
	rState.timer = time.AfterFunc(resizeDebounceDuration, func() {
		o.mu.Lock()
		// Guard: if the entry was already removed by cancelTimersForPID,
		// stopAllTimers, or a concurrent debounceResize that replaced it,
		// do nothing. Also bail if the observer has been stopped entirely.
		current, exists := o.timers[key]
		if !exists || current != rState || o.stopped {
			o.mu.Unlock()

			return
		}

		snapshot := rState.evt

		delete(o.timers, key)
		o.mu.Unlock()

		resizeEvt := events.Event{
			ID:          uuid.NewString(),
			Kind:        events.WindowResize,
			AppName:     snapshot.AppName,
			BundleID:    snapshot.BundleID,
			PID:         snapshot.PID,
			WindowTitle: snapshot.WindowTitle,
			At:          time.Now(),
		}
		o.logger.Debugw("event",
			"kind", resizeEvt.Kind,
			"app", resizeEvt.AppName,
			"bundle", resizeEvt.BundleID,
			"pid", resizeEvt.PID,
			"title", resizeEvt.WindowTitle,
		)
		o.bus.Publish(resizeEvt)
	})
	o.timers[key] = rState
}

// cancelTimersForPID stops and removes all pending resize timers for a given PID.
func (o *WorkspaceObserver) cancelTimersForPID(pid int) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for key, rs := range o.timers {
		if rs.evt.PID == pid {
			rs.timer.Stop()
			delete(o.timers, key)
		}
	}
}

// stopAllTimers stops all pending resize timers and marks the observer as
// stopped so that any timer callbacks that have already started waiting on
// the mutex will bail out instead of publishing after shutdown.
func (o *WorkspaceObserver) stopAllTimers() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.stopped = true

	for key, rs := range o.timers {
		rs.timer.Stop()
		delete(o.timers, key)
	}
}
