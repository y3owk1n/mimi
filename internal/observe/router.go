package observe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/native"
)

const resizeDebounceDuration = 250 * time.Millisecond

// Router receives native events and publishes hookable events to the bus.
type Router struct {
	bus    *events.Bus
	ax     *AXTracker
	logger *zap.SugaredLogger

	mu      sync.Mutex
	timers  map[string]*resizeState
	stopped bool
}

type resizeState struct {
	timer *time.Timer
	evt   events.Event
}

// NewRouter creates an event router for the hook daemon.
func NewRouter(bus *events.Bus, ax *AXTracker, logger *zap.SugaredLogger) *Router {
	return &Router{
		bus:    bus,
		ax:     ax,
		logger: logger,
		timers: make(map[string]*resizeState),
	}
}

// Run consumes native events until the context is canceled.
func (r *Router) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			r.stopAllTimers()

			return
		case evt, ok := <-native.Events():
			if !ok {
				r.stopAllTimers()

				return
			}

			r.handle(evt)
		}
	}
}

func (r *Router) handle(evt events.Event) {
	switch evt.Kind { //nolint:exhaustive
	case events.AppActivate, events.AppLaunch:
		if evt.PID > 0 {
			if ok := r.ax.Install(evt.PID); !ok {
				r.logger.Debugw("AX observer install failed",
					"pid", evt.PID, "app", evt.AppName)
			}
		}
	case events.AppQuit:
		if evt.PID > 0 {
			r.ax.Remove(evt.PID)
			r.cancelTimersForPID(evt.PID)
		}
	case events.WindowResizing:
		r.debounceResize(evt)

		return
	default:
	}

	r.logger.Debugw("event",
		"kind", evt.Kind,
		"app", evt.AppName,
		"bundle", evt.BundleID,
		"pid", evt.PID,
		"title", evt.WindowTitle,
	)
	r.bus.Publish(evt)
}

func resizeKey(pid int, title string) string {
	return fmt.Sprintf("%d:%s", pid, title)
}

func (r *Router) debounceResize(evt events.Event) {
	key := resizeKey(evt.PID, evt.WindowTitle)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return
	}

	if rState, ok := r.timers[key]; ok {
		if rState.timer.Stop() {
			rState.evt = evt
			rState.timer.Reset(resizeDebounceDuration)

			return
		}

		// Timer already fired — create a fresh entry to avoid resetting
		// an AfterFunc timer whose callback may still be running.
		delete(r.timers, key)
	}

	rState := r.newDebounceEntry(key, evt)
	r.timers[key] = rState
}

func (r *Router) newDebounceEntry(key string, evt events.Event) *resizeState {
	rState := &resizeState{evt: evt}
	rState.timer = time.AfterFunc(resizeDebounceDuration, func() {
		r.mu.Lock()

		current, exists := r.timers[key]
		if !exists || current != rState || r.stopped {
			r.mu.Unlock()

			return
		}

		snapshot := rState.evt

		delete(r.timers, key)
		r.mu.Unlock()

		resizeEvt := events.Event{
			ID:          uuid.NewString(),
			Kind:        events.WindowResize,
			AppName:     snapshot.AppName,
			BundleID:    snapshot.BundleID,
			PID:         snapshot.PID,
			WindowTitle: snapshot.WindowTitle,
			At:          time.Now(),
		}
		r.logger.Debugw("event",
			"kind", resizeEvt.Kind,
			"app", resizeEvt.AppName,
			"bundle", resizeEvt.BundleID,
			"pid", resizeEvt.PID,
			"title", resizeEvt.WindowTitle,
		)
		r.bus.Publish(resizeEvt)
	})

	return rState
}

func (r *Router) cancelTimersForPID(pid int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, rs := range r.timers {
		if rs.evt.PID == pid {
			rs.timer.Stop()
			delete(r.timers, key)
		}
	}
}

func (r *Router) stopAllTimers() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopped = true

	for key, rs := range r.timers {
		rs.timer.Stop()
		delete(r.timers, key)
	}
}
