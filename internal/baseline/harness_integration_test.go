//go:build integration

package baseline_test

import (
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/baseline"
	"github.com/y3owk1n/mimi/internal/native"
)

// How long macOS is given to agree with what the recorder asked for.
const (
	focusTimeout   = 5 * time.Second
	settleTimeout  = 3 * time.Second
	pollInterval   = 250 * time.Millisecond
	settleInterval = 60 * time.Millisecond
)

// harness owns the windows the recorder created and nothing else. Ownership is
// decided by process ID: the helper runs as its own process, so every window it
// owns is one the recorder opened and no window it does not own can be reached.
type harness struct {
	pids    map[int]bool
	windows []*native.Element
	visible baseline.Rect
	top     float64
}

// newHarness waits for the helper's windows and registers their teardown. It
// skips the test rather than falling back to pre-existing windows.
func newHarness(t *testing.T, display baseline.Display, pids map[int]bool) *harness {
	t.Helper()

	fixture := &harness{
		pids:    pids,
		visible: display.Visible,
		top:     display.PrimaryHeight - display.Visible.Y - display.Visible.H,
	}

	fixture.windows = fixture.waitForWindows(t)
	t.Cleanup(func() { native.ReleaseAll(fixture.windows) })

	return fixture
}

// waitForWindows waits until the helper's windows show up in the active space's
// window list, and returns exactly those.
func (h *harness) waitForWindows(t *testing.T) []*native.Element {
	t.Helper()

	deadline := time.Now().Add(launchTimeout)

	for {
		all, _ := enumerateFocused(t)

		var (
			mine   []*native.Element
			others []*native.Element
		)

		for _, candidate := range all {
			if h.owns(candidate) {
				mine = append(mine, candidate)

				continue
			}

			others = append(others, candidate)
		}

		native.ReleaseAll(others)

		if len(mine) == windowCount {
			return mine
		}

		native.ReleaseAll(mine)

		if time.Now().After(deadline) {
			t.Skipf(
				"the helper's %d windows did not appear on the active space within %s (a locked screen or a session without a desktop will do this); refusing to record against windows this test did not create",
				windowCount,
				launchTimeout,
			)
		}

		time.Sleep(pollInterval)
	}
}

// owns reports whether the window belongs to the helper instance the recorder
// started. A window whose process cannot be read is never treated as owned.
func (h *harness) owns(candidate *native.Element) bool {
	pid, err := candidate.PID()
	if err != nil {
		return false
	}

	return h.pids[pid]
}

// enumerateFocused returns the focusable windows on the active space along with
// the index of the focused one.
func enumerateFocused(t *testing.T) ([]*native.Element, int) {
	t.Helper()

	windows, focused, err := native.AllFocusableOnActiveSpaceWithFocused()
	if err != nil {
		t.Fatalf("failed to enumerate windows on the active space: %v", err)
	}

	return windows, focused
}

// indexOfElement returns the position of target within set, or -1.
func indexOfElement(set []*native.Element, target *native.Element) int {
	for index, element := range set {
		if element.Equal(target) {
			return index
		}
	}

	return -1
}

// bringToFront activates one of the recorder's own windows and confirms macOS
// agrees before any action runs, so an action that operates on "the frontmost
// window" can only ever reach a window this test created.
func bringToFront(t *testing.T, target *native.Element) {
	t.Helper()

	deadline := time.Now().Add(focusTimeout)
	activated := false

	for {
		if isFrontmost(target) {
			return
		}

		// Only ask once; the rest of the loop waits for macOS to agree.
		if !activated {
			err := target.Activate()
			if err != nil {
				t.Skipf("cannot activate the recorder's own window: %v", err)
			}

			activated = true
		}

		if time.Now().After(deadline) {
			t.Skipf(
				"the recorder's own window did not become frontmost within %s; refusing to drive a window this test did not create",
				focusTimeout,
			)
		}

		time.Sleep(settleInterval)
	}
}

// isFrontmost reports whether macOS considers the given window the frontmost
// one, which is the window every "frontmost window" action resolves to.
func isFrontmost(target *native.Element) bool {
	front := native.FrontmostWindow()
	if front == nil {
		return false
	}

	defer front.Release()

	return front.Equal(target)
}

// placeWindow moves one of the recorder's windows and returns the frame macOS
// settled on, which may differ from the request when the app clamps it.
func placeWindow(t *testing.T, target *native.Element, frame baseline.Rect) baseline.Rect {
	t.Helper()

	err := target.SetFrame(frame.X, frame.Y, frame.W, frame.H)
	if err != nil {
		t.Fatalf("failed to place the recorder's own window: %v", err)
	}

	return settledFrame(t, target)
}

// settledFrame reads a window frame until two consecutive reads agree, so a
// recording never captures a frame mid-flight.
func settledFrame(t *testing.T, target *native.Element) baseline.Rect {
	t.Helper()

	var (
		previous baseline.Rect
		seen     bool
	)

	deadline := time.Now().Add(settleTimeout)

	for {
		posX, posY, width, height, err := target.GetFrame()
		if err != nil {
			t.Fatalf("failed to read the recorder's own window frame: %v", err)
		}

		current := baseline.Rect{X: posX, Y: posY, W: width, H: height}
		if seen && current == previous {
			return current
		}

		previous, seen = current, true

		if time.Now().After(deadline) {
			return current
		}

		time.Sleep(settleInterval)
	}
}
