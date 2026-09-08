//nolint:testpackage // sets the resize grace, which is not configuration
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

// snappingDesktop is a desktop whose one window lands where it is put, then
// snaps a few points narrower the way a terminal does, and whose frame a test
// can move as the user would.
type snappingDesktop struct {
	mu      sync.Mutex
	frame   action.Frame
	applies int
	snapBy  float64
}

func (d *snappingDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "Term", Frame: d.frame},
	}}, nil
}

func (d *snappingDesktop) Displays() ([]action.DisplayEntry, error) {
	return []action.DisplayEntry{{Index: 1, Visible: action.Frame{Width: 1000, Height: 1000}}}, nil
}

func (d *snappingDesktop) ActiveSpace() (int, error) { return 1, nil }

func (d *snappingDesktop) Apply(frames []action.WindowFrame) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.applies++
	d.frame = frames[0].Frame
	d.frame.Width -= d.snapBy

	return nil
}

func (d *snappingDesktop) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.applies
}

func (d *snappingDesktop) drag(dx float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.frame.Width += dx
}

// TestEngine_Run_ResizesByTheUserPassAndOwnResizesDoNot pins the guard: the
// engine's own write, even snapped by the application, runs no pass; a
// window the user then resizes does, as a window_resize event.
func TestEngine_Run_ResizesByTheUserPassAndOwnResizesDoNot(t *testing.T) {
	t.Parallel()

	desktop := &snappingDesktop{frame: action.Frame{Width: 500, Height: 500}, snapBy: 7}
	engine := New(desktop, nil, nil)
	engine.resizeGrace = 50 * time.Millisecond
	engine.Update(config.TilingConfig{
		Enabled:          true,
		RelayoutOnResize: true,
		DebounceMS:       10,
		TimeoutSecs:      5,
		Layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 800, height: 800}}], ` +
			`state: {last: .event.kind, windows: .event.windows}}'`,
	}, "/bin/sh")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := make(events.Subscriber, 8)
	done := make(chan struct{})

	go func() {
		engine.Run(ctx, sub)
		close(done)
	}()

	waitFor := func(want int, why string) {
		t.Helper()

		deadline := time.Now().Add(3 * time.Second)
		for desktop.count() < want && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)

		if got := desktop.count(); got != want {
			t.Fatalf("%s: applied %d times, want %d", why, got, want)
		}
	}

	if !engine.KindFilter()(events.WindowResize) {
		t.Fatal("KindFilter refuses resizes with relayout_on_resize set")
	}

	waitFor(1, "startup")

	// The engine's own write comes back as a resize, inside the grace: it
	// refreshes what is remembered (the snapped width) and runs nothing.
	sub <- events.Event{Kind: events.WindowResize, PID: 10}

	waitFor(1, "own resize inside the grace")

	time.Sleep(engine.resizeGrace)

	// A late echo of the own write, outside the grace: the frame is still
	// the snapped one that was remembered, so still nothing.
	sub <- events.Event{Kind: events.WindowResize, PID: 10}

	waitFor(1, "own resize outside the grace")

	// The user drags the window wider.
	desktop.drag(120)

	sub <- events.Event{Kind: events.WindowResize, PID: 10}

	waitFor(2, "a resize by the user")

	input, _, err := engine.Preview(ctx, Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if string(input.State) != `{"last":"window_resize","windows":[1]}` {
		t.Fatalf(
			"state = %s, want the pass to have reported window_resize for window 1",
			input.State,
		)
	}

	cancel()
	<-done
}

func TestEngine_KindFilter_RefusesResizesUnlessAsked(t *testing.T) {
	t.Parallel()

	engine := New(&snappingDesktop{}, nil, nil)
	engine.Update(config.TilingConfig{Enabled: true, Layout: "true", TimeoutSecs: 1}, "/bin/sh")

	if engine.KindFilter()(events.WindowResize) {
		t.Fatal("KindFilter admits resizes without relayout_on_resize")
	}
}
