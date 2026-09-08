//nolint:testpackage // reaches the router's retry schedule and the tracker's seams
package observe

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
)

// daemonAppName is the application name the bridge puts on its own startup
// event.
const daemonAppName = "mimi"

// flakyAX is an installAX that refuses the first failures attempts and
// accepts after, recording every call. It is safe from the retry timers.
type flakyAX struct {
	mu       sync.Mutex
	failures int
	calls    int
}

func (f *flakyAX) install(int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return f.calls > f.failures
}

func (f *flakyAX) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

// newRetryTestRouter wires a router whose retry schedule is short and whose
// tracker refuses the first failures installs.
func newRetryTestRouter(t *testing.T, failures int) (*Router, <-chan events.Event, *flakyAX) {
	t.Helper()

	bus := events.NewBus()
	sub := bus.Subscribe(16)
	fake := &flakyAX{failures: failures}

	tracker := NewAXTracker(true)
	tracker.installAX = fake.install
	tracker.removeAX = func(int) {}

	router := NewRouterWithDebounce(bus, tracker, zap.NewNop().Sugar(), testDebounceWindow)
	router.retryDelays = []time.Duration{
		10 * time.Millisecond,
		10 * time.Millisecond,
		10 * time.Millisecond,
	}

	t.Cleanup(router.stopAllTimers)

	return router, sub, fake
}

// drain reads events off sub until an AXAttached arrives or the timeout
// passes, reporting whether it arrived.
func drain(
	sub <-chan events.Event,
	timeout time.Duration,
) (events.Event, bool) {
	deadline := time.After(timeout)

	for {
		select {
		case evt := <-sub:
			if evt.Kind == events.AXAttached {
				return evt, true
			}
		case <-deadline:
			return events.Event{}, false
		}
	}
}

// TestHandle_AppLaunch_RetriesARefusedInstallAndAnnouncesIt pins the fix for
// an application that is not accessible the instant it launches: the router
// keeps trying, and says so on the bus when it gets through, so the tiling
// engine can lay out the windows it could not see open.
func TestHandle_AppLaunch_RetriesARefusedInstallAndAnnouncesIt(t *testing.T) {
	router, sub, fake := newRetryTestRouter(t, 2)

	router.handle(
		events.Event{Kind: events.AppLaunch, PID: 77, AppName: testAppName, BundleID: testBundleID},
	)

	attached, ok := drain(sub, testFireTimeout)
	if !ok {
		t.Fatalf("no %s within %s; installs = %d", events.AXAttached, testFireTimeout, fake.count())
	}

	if attached.PID != 77 || attached.AppName != testAppName || attached.BundleID != testBundleID {
		t.Fatalf("attached = %+v, want pid 77 with the application's name and bundle", attached)
	}

	if got := fake.count(); got != 3 {
		t.Fatalf(
			"installs = %d, want 3 (one refused at launch, one refused on retry, one accepted)",
			got,
		)
	}

	if _, tracked := router.ax.tracked[77]; !tracked {
		t.Fatal("pid 77 is not tracked after the accepted install")
	}
}

func TestHandle_AppQuit_CancelsAPendingRetry(t *testing.T) {
	router, sub, fake := newRetryTestRouter(t, 100)

	router.handle(events.Event{Kind: events.AppLaunch, PID: 78, AppName: testAppName})
	router.handle(events.Event{Kind: events.AppQuit, PID: 78, AppName: testAppName})

	before := fake.count()

	if _, ok := drain(sub, testNoFireWait); ok {
		t.Fatal("an application that quit was attached to")
	}

	if got := fake.count(); got != before {
		t.Fatalf("installs went from %d to %d after the application quit", before, got)
	}
}

func TestHandle_AppLaunch_GivesUpAfterTheSchedule(t *testing.T) {
	router, sub, fake := newRetryTestRouter(t, 100)

	router.handle(events.Event{Kind: events.AppLaunch, PID: 79, AppName: testAppName})

	if _, ok := drain(sub, testNoFireWait); ok {
		t.Fatal("attached to an application that never accepts")
	}

	// One at launch plus one per delay on the schedule, and no more.
	if got, want := fake.count(), 1+len(router.retryDelays); got != want {
		t.Fatalf("installs = %d, want %d", got, want)
	}
}

// TestHandle_Startup_AttachesToEveryRunningApplication pins that the
// applications already running when the daemon starts are observed from the
// start, the ones that refuse on the retry schedule.
func TestHandle_Startup_AttachesToEveryRunningApplication(t *testing.T) {
	router, sub, fake := newRetryTestRouter(t, 1)
	router.listRunning = func() []int { return []int{201, 202} }

	router.handle(events.Event{Kind: events.Startup, AppName: daemonAppName})

	// 201 was refused once and retried; 202 was accepted first time.
	if _, ok := drain(sub, testFireTimeout); !ok {
		t.Fatal("the refused application was never attached to")
	}

	for _, pid := range []int{201, 202} {
		if _, tracked := router.ax.tracked[pid]; !tracked {
			t.Errorf("pid %d is not tracked after startup", pid)
		}
	}

	if got := fake.count(); got != 3 {
		t.Fatalf("installs = %d, want 3", got)
	}
}

// TestHandle_WithWindowObservationOff_NeitherInstallsNorRetries pins that a
// daemon with no window hooks and no tiling does what it did before the
// retry existed: nothing. No install attempt, no retry timer, no
// enumeration of running applications, and so no warning about giving up.
func TestHandle_WithWindowObservationOff_NeitherInstallsNorRetries(t *testing.T) {
	router, sub, fake := newRetryTestRouter(t, 100)
	router.ax.Update(false)

	enumerated := false
	router.listRunning = func() []int {
		enumerated = true

		return []int{301}
	}

	router.handle(events.Event{Kind: events.Startup, AppName: daemonAppName})
	router.handle(events.Event{Kind: events.AppLaunch, PID: 302, AppName: testAppName})
	router.handle(events.Event{Kind: events.AppActivate, PID: 302, AppName: testAppName})

	if _, ok := drain(sub, testNoFireWait); ok {
		t.Fatal("attached with window observation off")
	}

	if enumerated {
		t.Error("running applications were enumerated with window observation off")
	}

	if got := fake.count(); got != 0 {
		t.Errorf("installs = %d, want 0", got)
	}

	router.mu.Lock()
	pending := len(router.retries)
	router.mu.Unlock()

	if pending != 0 {
		t.Errorf("retries pending = %d, want 0", pending)
	}
}
