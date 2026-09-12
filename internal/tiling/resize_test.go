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
	// displays are the displays it reports, or nil for the one display
	// below.
	displays []action.DisplayEntry
}

func (d *snappingDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "Term", Frame: d.frame},
	}}, nil
}

func (d *snappingDesktop) Displays() ([]action.DisplayEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.displays != nil {
		return d.displays, nil
	}

	return []action.DisplayEntry{{
		Index:   1,
		ID:      1,
		Frame:   action.Frame{Width: 1000, Height: 1000},
		Visible: action.Frame{Width: 1000, Height: 1000},
	}}, nil
}

func (d *snappingDesktop) ActiveSpaces() (map[uint32]int, error) { return map[uint32]int{1: 1}, nil }

func (d *snappingDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	return map[uint32]uint64{1: 1001}, nil
}

func (d *snappingDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return map[uint32]bool{}, nil
}

func (d *snappingDesktop) Margins() (action.MarginsInfo, error) { return action.MarginsInfo{}, nil }

func (d *snappingDesktop) Focus(uint32) error { return nil }

func (d *snappingDesktop) Apply(frames []action.WindowFrame, _ *action.Animation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.applies++
	d.frame = frames[0].Frame
	d.frame.Width -= d.snapBy

	return nil
}

// setDisplays reports these displays from now on, as macOS does when one is
// plugged in or unplugged.
func (d *snappingDesktop) setDisplays(displays []action.DisplayEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.displays = displays
}

func (d *snappingDesktop) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.applies
}

func (d *snappingDesktop) move(dx float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.frame.X += dx
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
		Enabled:        true,
		RelayoutOnDrag: true,
		DebounceMS:     10,
		TimeoutSecs:    5,
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

	if !engine.KindFilter()(events.WindowResize) || !engine.KindFilter()(events.WindowMove) {
		t.Fatal("KindFilter refuses moves or resizes with relayout_on_drag set")
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

	inputs, _, err := engine.Preview(ctx, Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if string(inputs[0].State) != `{"last":"window_resize","windows":[1]}` {
		t.Fatalf(
			"state = %s, want the pass to have reported window_resize for window 1",
			inputs[0].State,
		)
	}

	// The user drags the window somewhere else, and the application snaps
	// its width a little on the way, raising a resize too. It is a move.
	time.Sleep(engine.resizeGrace)
	desktop.move(300)
	desktop.drag(3)

	sub <- events.Event{Kind: events.WindowResize, PID: 10}

	sub <- events.Event{Kind: events.WindowMove, PID: 10}

	waitFor(3, "a move by the user")

	inputs, _, err = engine.Preview(ctx, Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if string(inputs[0].State) != `{"last":"window_move","windows":[1]}` {
		t.Fatalf(
			"state = %s, want the pass to have reported window_move for window 1",
			inputs[0].State,
		)
	}

	cancel()
	<-done
}

func TestEngine_KindFilter_RefusesDragsUnlessAsked(t *testing.T) {
	t.Parallel()

	engine := New(&snappingDesktop{}, nil, nil)
	engine.Update(config.TilingConfig{Enabled: true, Layout: "true", TimeoutSecs: 1}, "/bin/sh")

	if engine.KindFilter()(events.WindowResize) || engine.KindFilter()(events.WindowMove) {
		t.Fatal("KindFilter admits moves or resizes without relayout_on_drag")
	}
}

// transitionDesktop is a desktop of several windows whose frames, and
// whether its display shows a full-screen space, a test sets as macOS would
// report them mid-way through a full-screen switch.
type transitionDesktop struct {
	mu         sync.Mutex
	windows    []action.WindowEntry
	fullScreen bool
	applies    int
}

func (d *transitionDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return action.WindowsInfo{
		Focused: 0,
		Windows: append([]action.WindowEntry(nil), d.windows...),
	}, nil
}

func (d *transitionDesktop) Displays() ([]action.DisplayEntry, error) {
	return []action.DisplayEntry{{
		Index:   1,
		ID:      1,
		Frame:   action.Frame{Width: 1000, Height: 1000},
		Visible: action.Frame{Y: 30, Width: 1000, Height: 970},
	}}, nil
}

func (d *transitionDesktop) ActiveSpaces() (map[uint32]int, error) { return map[uint32]int{1: 1}, nil }

func (d *transitionDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	return map[uint32]uint64{1: 1001}, nil
}

func (d *transitionDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return map[uint32]bool{1: d.fullScreen}, nil
}

func (d *transitionDesktop) Margins() (action.MarginsInfo, error) { return action.MarginsInfo{}, nil }

func (d *transitionDesktop) Focus(uint32) error { return nil }

func (d *transitionDesktop) Apply(frames []action.WindowFrame, _ *action.Animation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.applies++

	for _, frame := range frames {
		for index := range d.windows {
			if d.windows[index].Number == frame.Number {
				d.windows[index].Frame = frame.Frame
			}
		}
	}

	return nil
}

func (d *transitionDesktop) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.applies
}

func (d *transitionDesktop) set(fullScreen bool, windows ...action.WindowEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.fullScreen = fullScreen
	d.windows = windows
}

// TestEngine_Run_FullScreenSwitchIsNotADrag pins the guard. While a window
// enters or leaves full screen, macOS reports every window on the display
// somewhere along the animation. Those frames run no pass, and the engine
// does not remember them, so it judges the next drag against where it
// placed the windows.
func TestEngine_Run_FullScreenSwitchIsNotADrag(t *testing.T) {
	t.Parallel()

	tiled := []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: action.Frame{X: 0, Y: 30, Width: 500, Height: 970}},
		{Number: 2, PID: 20, App: "B", Frame: action.Frame{X: 500, Y: 30, Width: 500, Height: 970}},
		{
			Number: 3,
			PID:    30,
			App:    "C",
			Frame:  action.Frame{X: 1000, Y: 30, Width: 500, Height: 970},
		},
	}

	desktop := &transitionDesktop{}
	desktop.set(false, tiled...)

	engine := New(desktop, nil, nil)
	engine.resizeGrace = 50 * time.Millisecond
	engine.Update(config.TilingConfig{
		Enabled:        true,
		RelayoutOnDrag: true,
		DebounceMS:     10,
		TimeoutSecs:    5,
		Layout: `jq -c '{frames: [.windows[] | {number, frame}], ` +
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

	waitFor(1, "startup")
	time.Sleep(engine.resizeGrace)

	// Window 4 grows to the display's whole frame, menu bar included, on
	// its way to a full-screen space of its own. macOS reports the desktop
	// behind it part way through the animation, with every window shifted.
	shifted := []action.WindowEntry{
		{Number: 4, PID: 10, App: "A", Frame: action.Frame{Width: 1000, Height: 1000}},
		{
			Number: 1,
			PID:    10,
			App:    "A",
			Frame:  action.Frame{X: -800, Y: 30, Width: 500, Height: 970},
		},
		{Number: 2, PID: 20, App: "B", Frame: action.Frame{X: 150, Y: 30, Width: 500, Height: 970}},
		{Number: 3, PID: 30, App: "C", Frame: action.Frame{X: 150, Y: 30, Width: 500, Height: 970}},
	}
	desktop.set(false, shifted...)

	sub <- events.Event{Kind: events.WindowResize, PID: 10}

	waitFor(1, "a window entering full screen")

	// The display shows the full-screen space now, and the frames behind
	// it are whatever macOS says.
	desktop.set(true, shifted[1:]...)

	sub <- events.Event{Kind: events.WindowMove, PID: 20}

	waitFor(1, "a display showing a full-screen space")

	// Back on the desktop, the user drags window 2 rightwards. The engine
	// did not remember the frames mid-switch, so window 2 is the one that
	// moved.
	dragged := append([]action.WindowEntry(nil), tiled...)
	dragged[1].Frame.X += 300
	desktop.set(false, dragged...)

	sub <- events.Event{Kind: events.WindowMove, PID: 20}

	waitFor(2, "a move by the user after the switch")

	inputs, _, err := engine.Preview(ctx, Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if string(inputs[0].State) != `{"last":"window_move","windows":[2]}` {
		t.Fatalf(
			"state = %s, want the pass to have reported window_move for window 2 alone",
			inputs[0].State,
		)
	}

	cancel()
	<-done
}
