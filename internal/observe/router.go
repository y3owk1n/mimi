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

const defaultResizeDebounceDuration = 250 * time.Millisecond

// axRetryDelays is how long the router waits between attempts to attach an
// AX observer to an application that refused the first one. An application
// answers Accessibility only once its run loop is up, which for a large one
// is seconds after launch; the first attempt is made the instant macOS
// reports the launch, and until this retry existed a refusal there was the
// end of window events from that application for its whole run.
//
//nolint:gochecknoglobals // a fixed schedule
var axRetryDelays = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2 * time.Second,
	4 * time.Second,
}

// Router receives native events and publishes hookable events to the bus.
type Router struct {
	bus    *events.Bus
	ax     *AXTracker
	logger *zap.SugaredLogger

	mu          sync.Mutex
	timers      map[string]*resizeState
	retries     map[int]*axRetry
	retryDelays []time.Duration
	// listRunning names the applications to attach to at startup; it is the
	// native enumeration, replaced in tests.
	listRunning    func() []int
	stopped        bool
	debounceWindow time.Duration
}

type resizeState struct {
	timer *time.Timer
	evt   events.Event
}

// axRetry is one application the router is still trying to observe.
type axRetry struct {
	timer   *time.Timer
	evt     events.Event
	attempt int
}

// NewRouter creates an event router for the hook daemon with the default
// resize debounce window (250ms). A nil logger is tolerated; the fallback to
// zap.NewNop() lives in NewRouterWithDebounce, which this delegates to.
func NewRouter(bus *events.Bus, tracker *AXTracker, logger *zap.SugaredLogger) *Router {
	return NewRouterWithDebounce(bus, tracker, logger, defaultResizeDebounceDuration)
}

// NewRouterWithDebounce creates an event router with a caller-specified
// resize debounce window. A zero duration is replaced with the default.
func NewRouterWithDebounce(
	bus *events.Bus,
	tracker *AXTracker,
	logger *zap.SugaredLogger,
	debounceWindow time.Duration,
) *Router {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	if debounceWindow <= 0 {
		debounceWindow = defaultResizeDebounceDuration
	}

	return &Router{
		bus:            bus,
		ax:             tracker,
		logger:         logger,
		timers:         make(map[string]*resizeState),
		retries:        make(map[int]*axRetry),
		retryDelays:    axRetryDelays,
		listRunning:    native.RegularApplicationPIDs,
		debounceWindow: debounceWindow,
	}
}

// SetDebounceWindow updates the debounce window used for resize events. A
// zero or negative value resets to the default. Existing in-flight timers
// are not affected; the new window applies to events arriving after this
// call.
func (r *Router) SetDebounceWindow(window time.Duration) {
	if window <= 0 {
		window = defaultResizeDebounceDuration
	}

	r.mu.Lock()
	r.debounceWindow = window
	r.mu.Unlock()
}

// DebounceWindow is the window resize events coalesce over.
func (r *Router) DebounceWindow() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.debounceWindow
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

// AttachRunning attaches an observer to every running application that has
// none yet, retrying the ones that refuse. The bridge's startup event calls
// it, and so does a reload that switches window observation on: until then
// an application already running when the daemon started went unobserved
// until the user next switched to it, and every window it opened, closed or
// resized in between was missed.
func (r *Router) AttachRunning() {
	for _, pid := range r.listRunning() {
		if !r.ax.Install(pid) {
			r.scheduleRetry(events.Event{Kind: events.AppLaunch, PID: pid}, 0)
		}
	}
}

func (r *Router) handle(evt events.Event) {
	switch evt.Kind { //nolint:exhaustive
	case events.Startup:
		r.AttachRunning()
	case events.AppActivate, events.AppLaunch:
		if evt.PID > 0 {
			if ok := r.ax.Install(evt.PID); ok {
				r.cancelRetry(evt.PID)
			} else {
				r.logger.Debugw("AX observer install failed; will retry",
					"pid", evt.PID, "app", evt.AppName)
				r.scheduleRetry(evt, 0)
			}
		}
	case events.AppQuit:
		if evt.PID > 0 {
			r.ax.Remove(evt.PID)
			r.cancelTimersForPID(evt.PID)
			r.cancelRetry(evt.PID)
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
		"title_present", evt.WindowTitle != "",
	)
	r.bus.Publish(evt)
}

// resizeKey identifies the debounce bucket a resize event coalesces into.
// It keys on the window id when the native layer supplied one, so two
// windows of the same app with identical titles debounce independently;
// it falls back to the title only when no id is available.
func resizeKey(evt events.Event) string {
	if evt.WindowID != 0 {
		return fmt.Sprintf("%d:%d", evt.PID, evt.WindowID)
	}

	return fmt.Sprintf("%d:%s", evt.PID, evt.WindowTitle)
}

func (r *Router) debounceResize(evt events.Event) {
	key := resizeKey(evt)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return
	}

	if rState, ok := r.timers[key]; ok {
		if rState.timer.Stop() {
			rState.evt = evt
			rState.timer.Reset(r.debounceWindow)

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
	rState.timer = time.AfterFunc(r.debounceWindow, func() {
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
			WindowID:    snapshot.WindowID,
			At:          time.Now(),
		}
		r.logger.Debugw("event",
			"kind", resizeEvt.Kind,
			"app", resizeEvt.AppName,
			"bundle", resizeEvt.BundleID,
			"pid", resizeEvt.PID,
			"title_present", resizeEvt.WindowTitle != "",
		)
		r.bus.Publish(resizeEvt)
	})

	return rState
}

// scheduleRetry arms the next attempt to observe evt's application, or gives
// up once the schedule is spent. A retry already pending for the pid stands.
func (r *Router) scheduleRetry(evt events.Event, attempt int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return
	}

	if _, pending := r.retries[evt.PID]; pending {
		return
	}

	if attempt >= len(r.retryDelays) {
		r.logger.Warnw(
			"AX observer install gave up; window events from this application will not fire",
			"pid",
			evt.PID,
			"app",
			evt.AppName,
			"attempts",
			attempt,
		)

		return
	}

	retry := &axRetry{evt: evt, attempt: attempt}
	retry.timer = time.AfterFunc(r.retryDelays[attempt], func() { r.retryInstall(evt.PID, retry) })
	r.retries[evt.PID] = retry
}

// retryInstall is one attempt off the timer. On success it publishes
// AXAttached, so a subscriber that missed the application's first windows
// knows to look now.
func (r *Router) retryInstall(pid int, retry *axRetry) {
	r.mu.Lock()

	current, pending := r.retries[pid]
	if !pending || current != retry || r.stopped {
		r.mu.Unlock()

		return
	}

	delete(r.retries, pid)
	r.mu.Unlock()

	if !r.ax.Install(pid) {
		r.scheduleRetry(retry.evt, retry.attempt+1)

		return
	}

	attached := events.Event{
		ID:       uuid.NewString(),
		Kind:     events.AXAttached,
		AppName:  retry.evt.AppName,
		BundleID: retry.evt.BundleID,
		PID:      pid,
		At:       time.Now(),
	}
	r.logger.Debugw("event",
		"kind", attached.Kind,
		"app", attached.AppName,
		"bundle", attached.BundleID,
		"pid", attached.PID,
		"attempt", retry.attempt+1,
	)
	r.bus.Publish(attached)
}

// cancelRetry drops any pending attempt for pid.
func (r *Router) cancelRetry(pid int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if retry, pending := r.retries[pid]; pending {
		retry.timer.Stop()
		delete(r.retries, pid)
	}
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

	for pid, retry := range r.retries {
		retry.timer.Stop()
		delete(r.retries, pid)
	}
}
