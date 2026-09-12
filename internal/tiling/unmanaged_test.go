//nolint:testpackage // sets the resize grace, as resize_test does
package tiling

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// pairDesktop is a desktop with two windows, where every applied frame lands
// exactly as written and a test can drag either window as the user would.
type pairDesktop struct {
	mu      sync.Mutex
	frames  map[uint32]action.Frame
	applies int
}

func newPairDesktop() *pairDesktop {
	return &pairDesktop{frames: map[uint32]action.Frame{
		1: {Y: 25, Width: 500, Height: 975},
		2: {X: 500, Y: 25, Width: 500, Height: 975},
	}}
}

func (d *pairDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: d.frames[1]},
		{Number: 2, PID: 11, App: "B", Frame: d.frames[2]},
	}}, nil
}

// Displays reports one display whose visible frame is smaller than the whole,
// as a real one is: the menu bar takes the top. A window filling the whole
// display frame is how the engine recognizes a full-screen space, so a
// maximized window has to stop short of it.
func (d *pairDesktop) Displays() ([]action.DisplayEntry, error) {
	return []action.DisplayEntry{{
		Index:   1,
		ID:      1,
		Frame:   action.Frame{Width: 1000, Height: 1000},
		Visible: action.Frame{Y: 25, Width: 1000, Height: 975},
	}}, nil
}

func (d *pairDesktop) ActiveSpaces() (map[uint32]int, error) { return map[uint32]int{1: 1}, nil }

func (d *pairDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	return map[uint32]uint64{1: 1001}, nil
}

func (d *pairDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return map[uint32]bool{}, nil
}

func (d *pairDesktop) Margins() (action.MarginsInfo, error) { return action.MarginsInfo{}, nil }

func (d *pairDesktop) Focus(uint32) error { return nil }

func (d *pairDesktop) Apply(frames []action.WindowFrame, _ *action.Animation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.applies++
	for _, frame := range frames {
		d.frames[frame.Number] = frame.Frame
	}

	return nil
}

// drag moves a window sideways, as the user would.
func (d *pairDesktop) drag(number uint32, dx float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	frame := d.frames[number]
	frame.X += dx
	d.frames[number] = frame
}

func (d *pairDesktop) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.applies
}

// maximizing is a layout that places both windows on its first run and only
// the first window on every run after it, which is what a temporary maximize
// returns. With unmanaged set, it also says the second window is one it has
// stopped managing.
func maximizing(unmanaged string) string {
	return `jq -c 'if .state == null then {frames: [` +
		`{number: 1, frame: {x: 0, y: 25, width: 500, height: 975}},` +
		`{number: 2, frame: {x: 500, y: 25, width: 500, height: 975}}], ` +
		`state: {run: "first"}} else {frames: [` +
		`{number: 1, frame: {x: 0, y: 25, width: 1000, height: 975}}]` +
		unmanaged + `, state: {run: "later"}} end'`
}

// runPairEngine starts an engine reading drags on the desktop, waits for its
// startup pass, and returns the events it listens to along with a check that
// the desktop was applied to exactly want times.
func runPairEngine(
	t *testing.T,
	desktop *pairDesktop,
	layout string,
) (events.Subscriber, func(int, string)) {
	t.Helper()

	engine := New(desktop, nil, nil)
	engine.resizeGrace = 50 * time.Millisecond
	engine.Update(config.TilingConfig{
		Enabled:        true,
		RelayoutOnDrag: true,
		DebounceMS:     10,
		TimeoutSecs:    5,
		Layout:         layout,
	}, "/bin/sh")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		engine.Wait()
	})

	sub := make(events.Subscriber, 8)
	go engine.Run(ctx, sub)

	waitFor := func(want int, why string) {
		t.Helper()

		deadline := time.Now().Add(3 * time.Second)
		for desktop.count() < want && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		// Give a pass that should not happen the chance to happen.
		time.Sleep(150 * time.Millisecond)

		if got := desktop.count(); got != want {
			t.Fatalf("%s: applied %d times, want %d", why, got, want)
		}
	}

	waitFor(1, "startup")

	return sub, waitFor
}

// TestEngine_Run_ReadsADragOfAWindowTheLayoutStoppedPlacing pins the case a
// temporary maximize creates. The layout returns one frame and leaves its
// other windows where they are, which does not mean it has given them up: a
// drag of one still has to raise a pass, or the layout can never put it back.
func TestEngine_Run_ReadsADragOfAWindowTheLayoutStoppedPlacing(t *testing.T) {
	t.Parallel()

	desktop := newPairDesktop()
	sub, waitFor := runPairEngine(t, desktop, maximizing(""))

	// A second pass, which maximizes the first window and returns no frame
	// for the second.
	sub <- events.Event{Kind: events.WindowFocus, PID: 10}

	waitFor(2, "the maximizing pass")

	time.Sleep(60 * time.Millisecond)

	desktop.drag(2, 120)

	sub <- events.Event{Kind: events.WindowMove, PID: 11}

	waitFor(3, "a drag of the window the layout stopped placing")
}

// TestEngine_Run_LeavesAWindowTheLayoutGaveUpAlone pins the other half. A
// layout that says it is no longer managing a window means it, so the engine
// stops watching that window and a drag of it raises nothing.
func TestEngine_Run_LeavesAWindowTheLayoutGaveUpAlone(t *testing.T) {
	t.Parallel()

	desktop := newPairDesktop()
	sub, waitFor := runPairEngine(t, desktop, maximizing(", unmanaged: [2]"))

	sub <- events.Event{Kind: events.WindowFocus, PID: 10}

	waitFor(2, "the pass that gives the second window up")

	time.Sleep(60 * time.Millisecond)

	desktop.drag(2, 120)

	sub <- events.Event{Kind: events.WindowMove, PID: 11}

	waitFor(2, "a drag of the window the layout gave up")
}
