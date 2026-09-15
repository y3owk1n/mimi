//nolint:testpackage // sets the resize grace, which is not configuration
package tiling

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// clampingDesktop is a desktop of two windows, the second of which refuses
// any width under its minimum the way a window with a minimum size does.
type clampingDesktop struct {
	mu       sync.Mutex
	frames   map[uint32]action.Frame
	minWidth float64
	applies  [][]action.WindowFrame
	// lateReads is how many reads after an apply still report the frames
	// from before it, the way an application that has not finished
	// resizing does. previous holds those frames.
	lateReads int
	previous  map[uint32]action.Frame
}

func (d *clampingDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	frames := d.frames
	if d.lateReads > 0 && d.previous != nil {
		d.lateReads--
		frames = d.previous
	}

	return action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Display: 1, Frame: frames[1]},
		{Number: 2, PID: 20, App: "B", Display: 1, Frame: frames[2]},
	}}, nil
}

func (d *clampingDesktop) Displays() ([]action.DisplayEntry, error) {
	return []action.DisplayEntry{{
		Index:   1,
		ID:      1,
		Frame:   action.Frame{Width: 1000, Height: 1000},
		Visible: action.Frame{Width: 1000, Height: 1000},
	}}, nil
}

func (d *clampingDesktop) ActiveSpaces() (map[uint32]int, error) { return map[uint32]int{1: 1}, nil }

func (d *clampingDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	return map[uint32]uint64{1: 1001}, nil
}

func (d *clampingDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return map[uint32]bool{}, nil
}

func (d *clampingDesktop) Margins() (action.MarginsInfo, error) { return action.MarginsInfo{}, nil }

func (d *clampingDesktop) Focus(uint32) error { return nil }

func (d *clampingDesktop) Apply(frames []action.WindowFrame, _ *action.Animation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.applies = append(d.applies, frames)

	d.previous = map[uint32]action.Frame{}
	maps.Copy(d.previous, d.frames)

	for _, frame := range frames {
		landed := frame.Frame
		if frame.Number == 2 && landed.Width < d.minWidth {
			landed.Width = d.minWidth
		}

		d.frames[frame.Number] = landed
	}

	return nil
}

func (d *clampingDesktop) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.applies)
}

func (d *clampingDesktop) widthAsked(pass int, number uint32) float64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, frame := range d.applies[pass] {
		if frame.Number == number {
			return frame.Frame.Width
		}
	}

	return -1
}

// TestEngine_Run_LearnsAMinimumSizeAndReplaysOnce pins the contract: a
// window that lands wider than asked is reported to the layout with its
// minSize on the next input, that input comes at once as a relayout pass,
// and a layout that then honors the minimum raises no further pass.
func TestEngine_Run_LearnsAMinimumSizeAndReplaysOnce(t *testing.T) {
	t.Parallel()

	desktop := &clampingDesktop{
		frames: map[uint32]action.Frame{
			1: {Width: 500, Height: 500},
			2: {X: 500, Width: 500, Height: 500},
		},
		minWidth: 600,
	}
	engine := New(desktop, nil, nil)
	engine.resizeGrace = 50 * time.Millisecond
	engine.Update(config.TilingConfig{
		Enabled:     true,
		DebounceMS:  10,
		TimeoutSecs: 5,
		// Two columns: the second as wide as its minimum when one is known,
		// else half.
		Layout: `jq -c '(.windows[1].minSize.width // 500) as $w | {frames: [` +
			`{number: 1, frame: {x: 0, y: 0, width: (1000 - $w), height: 1000}}, ` +
			`{number: 2, frame: {x: (1000 - $w), y: 0, width: $w, height: 1000}}], state: null}'`,
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

		time.Sleep(200 * time.Millisecond)

		if got := desktop.count(); got != want {
			t.Fatalf("%s: applied %d times, want %d", why, got, want)
		}
	}

	// Startup asks for two halves, the window refuses, and the engine
	// replays with the minimum known. Then nothing more.
	waitFor(2, "startup and the pass that learned the minimum")

	if got := desktop.widthAsked(0, 2); got != 500 {
		t.Fatalf("first pass asked width %v for window 2, want 500", got)
	}

	if got := desktop.widthAsked(1, 2); got != 600 {
		t.Fatalf("second pass asked width %v for window 2, want its minimum 600", got)
	}

	if got := desktop.widthAsked(1, 1); got != 400 {
		t.Fatalf("second pass asked width %v for window 1, want the rest 400", got)
	}

	inputs, _, err := engine.Preview(ctx, Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if inputs[0].Windows[1].MinSize == nil || inputs[0].Windows[1].MinSize.Width != 600 ||
		inputs[0].Windows[1].MinSize.Height != 0 {
		t.Fatalf("minSize = %+v, want width 600 and no height", inputs[0].Windows[1].MinSize)
	}

	if inputs[0].Windows[0].MinSize != nil {
		t.Fatalf(
			"minSize = %+v for the window that took its frame, want none",
			inputs[0].Windows[0].MinSize,
		)
	}

	// The application lowers its minimum. The next pass writes the old
	// minimum, the window takes it, and the hint stays: taking what was
	// asked says nothing about anything smaller.
	desktop.mu.Lock()
	desktop.minWidth = 300
	desktop.mu.Unlock()

	sub <- events.Event{Kind: events.WindowFocus, PID: 10}

	waitFor(3, "a pass after the minimum dropped")

	if got := desktop.widthAsked(2, 2); got != 600 {
		t.Fatalf("third pass asked width %v for window 2, want the remembered 600", got)
	}

	cancel()
	<-done
}

func TestEngine_learnMinSize(t *testing.T) {
	t.Parallel()

	engine := New(&clampingDesktop{}, nil, nil)

	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{Width: 500, Height: 500},
		action.Frame{Width: 600, Height: 500},
	); !grew {
		t.Fatal("landing wider than asked did not report a new minimum")
	}

	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{Width: 500, Height: 500},
		action.Frame{Width: 600, Height: 500},
	); grew {
		t.Fatal("landing at the known minimum again reported it as new")
	}

	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{Width: 600, Height: 500},
		action.Frame{Width: 600, Height: 500},
	); grew {
		t.Fatal("taking the minimum as asked reported it as new")
	}

	if got := engine.minSizes[1]; got != (action.MinSize{Width: 600}) {
		t.Fatalf("minSize = %+v, want width 600", got)
	}

	// Smaller than the minimum and taken: the minimum was wrong, forget it.
	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{Width: 400, Height: 500},
		action.Frame{Width: 400, Height: 500},
	); grew {
		t.Fatal("taking a smaller size reported a new minimum")
	}

	if _, ok := engine.minSizes[1]; ok {
		t.Fatalf("minSize = %+v after the window took less, want none", engine.minSizes[1])
	}
}

// TestEngine_Reset_ForgetsMinimums pins that a reset drops learned
// minimums along with the state, and that a minimum survives a pass that
// does not list its window, which is what leaving its space does.
func TestEngine_Reset_ForgetsMinimums(t *testing.T) {
	t.Parallel()

	desktop := &clampingDesktop{
		frames: map[uint32]action.Frame{
			1: {Width: 500, Height: 500},
			2: {X: 500, Width: 500, Height: 500},
		},
		minWidth: 600,
	}
	engine := New(desktop, nil, nil)
	engine.minSizes[7] = action.MinSize{Width: 900}
	engine.Update(config.TilingConfig{
		Enabled:     true,
		TimeoutSecs: 5,
		Layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 500, height: 1000}}, ` +
			`{number: 2, frame: {x: 500, y: 0, width: 500, height: 1000}}], state: null}'`,
	}, "/bin/sh")

	err := engine.Pass(context.Background(), Event{Kind: EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	if got := engine.minSizes[7]; got != (action.MinSize{Width: 900}) {
		t.Fatalf("minSize for a window on another space = %+v after a pass, want kept", got)
	}

	if got := engine.minSizes[2]; got != (action.MinSize{Width: 600}) {
		t.Fatalf("minSize = %+v, want width 600", got)
	}

	_, err = engine.Reset(false)
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if len(engine.minSizes) != 0 {
		t.Fatalf("minSizes = %v after a reset, want none", engine.minSizes)
	}
}

// TestEngine_Run_KeepsAnAskedPassUnderAnOwnResizeEcho pins the loop: the
// relayout the engine asks for on learning a minimum still runs when one
// of its own writes echoes back as a resize event in the meantime.
func TestEngine_Run_KeepsAnAskedPassUnderAnOwnResizeEcho(t *testing.T) {
	t.Parallel()

	desktop := &clampingDesktop{
		frames: map[uint32]action.Frame{
			1: {Width: 500, Height: 500},
			2: {X: 500, Width: 500, Height: 500},
		},
		minWidth: 600,
	}
	engine := New(desktop, nil, nil)
	engine.resizeGrace = time.Second
	engine.Update(config.TilingConfig{
		Enabled:        true,
		RelayoutOnDrag: true,
		DebounceMS:     50,
		TimeoutSecs:    5,
		Layout: `jq -c '(.windows[1].minSize.width // 500) as $w | {frames: [` +
			`{number: 1, frame: {x: 0, y: 0, width: (1000 - $w), height: 1000}}, ` +
			`{number: 2, frame: {x: (1000 - $w), y: 0, width: $w, height: 1000}}], state: null}'`,
	}, "/bin/sh")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := make(events.Subscriber, 8)
	done := make(chan struct{})

	go func() {
		engine.Run(ctx, sub)
		close(done)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for desktop.count() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	// The startup write echoes back as a resize while the relayout the
	// readback asked for is still waiting out the debounce.
	time.Sleep(10 * time.Millisecond)

	sub <- events.Event{Kind: events.WindowResize, PID: 20}

	deadline = time.Now().Add(3 * time.Second)
	for desktop.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if got := desktop.count(); got != 2 {
		t.Fatalf("applied %d times, want the relayout to have run under the echo", got)
	}

	if got := desktop.widthAsked(1, 2); got != 600 {
		t.Fatalf("second pass asked width %v for window 2, want its minimum 600", got)
	}

	cancel()
	<-done
}

// TestEngine_SetStore_SeedsNewWindowsFromTheirApplication pins the store:
// a minimum learned for one window is kept for its application, written
// to the store, read back by the next engine, and handed to a window of
// that application that has refused nothing yet.
func TestEngine_SetStore_SeedsNewWindowsFromTheirApplication(t *testing.T) {
	t.Parallel()

	store := filepath.Join(t.TempDir(), "minsizes.json")

	first := New(&clampingDesktop{}, nil, nil)
	first.SetStore(store)

	if !first.learnMinSize(
		1,
		"com.example.app",
		1,
		action.Frame{Width: 500, Height: 500},
		action.Frame{Width: 600, Height: 500},
	) {
		t.Fatal("landing wider than asked did not report a new minimum")
	}

	first.saveMinSizes()

	desktop := &clampingDesktop{
		frames: map[uint32]action.Frame{
			1: {Width: 500, Height: 500},
			2: {X: 500, Width: 500, Height: 500},
		},
		minWidth: 600,
	}
	second := New(desktop, nil, nil)
	second.SetStore(store)
	second.Update(config.TilingConfig{Enabled: true, TimeoutSecs: 5, Layout: "cat"}, "/bin/sh")

	inputs, _, err := second.Preview(context.Background(), Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	// The fake's windows carry no bundle id, so only one named as the app
	// is seeded.
	for index := range inputs[0].Windows {
		inputs[0].Windows[index].BundleID = "com.example.app"
	}

	desktop.mu.Lock()
	desktop.frames[1] = action.Frame{Width: 500, Height: 500}
	desktop.mu.Unlock()

	if got := second.appMinSizes["com.example.app"]; got != (action.MinSize{Width: 600}) {
		t.Fatalf("appMinSizes = %+v after reading the store, want width 600", got)
	}

	_, err = second.Reset(true)
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	third := New(&clampingDesktop{}, nil, nil)
	third.SetStore(store)

	if len(third.appMinSizes) != 0 {
		t.Fatalf("appMinSizes = %v after a reset wrote the store, want none", third.appMinSizes)
	}
}

// TestEngine_Run_DoesNotLearnFromAWindowStillResizing pins the read-back
// against an application that applies a size late: the first read sees the
// old, larger frame, and a minimum learned from it would be wrong. The
// engine reads again before believing a refusal, and learns nothing.
func TestEngine_Run_DoesNotLearnFromAWindowStillResizing(t *testing.T) {
	t.Parallel()

	desktop := &clampingDesktop{
		frames: map[uint32]action.Frame{
			1: {Width: 500, Height: 1000},
			2: {X: 500, Width: 500, Height: 1000},
		},
		lateReads: 2,
	}
	engine := New(desktop, nil, nil)
	engine.resizeGrace = 50 * time.Millisecond
	engine.Update(config.TilingConfig{
		Enabled:     true,
		DebounceMS:  10,
		TimeoutSecs: 5,
		Layout: `jq -c '{frames: [` +
			`{number: 1, frame: {x: 0, y: 0, width: 500, height: 500}}, ` +
			`{number: 2, frame: {x: 500, y: 0, width: 500, height: 500}}], state: null}'`,
	}, "/bin/sh")

	ctx := t.Context()

	sub := make(events.Subscriber, 8)

	go engine.Run(ctx, sub)

	deadline := time.Now().Add(3 * time.Second)
	for desktop.count() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Long enough for the read-back, the confirming read, and the pass a
	// wrongly learned minimum would have asked for.
	time.Sleep(confirmRefusal + 500*time.Millisecond)

	if got := desktop.count(); got != 1 {
		t.Fatalf("applied %d times, want 1: a late resize is not a refusal", got)
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()

	if len(engine.minSizes) != 0 || len(engine.appMinSizes) != 0 {
		t.Fatalf("learned %v / %v, want nothing", engine.minSizes, engine.appMinSizes)
	}
}

// TestEngine_learnMinSize_IgnoresAWindowAsLargeAsItsDisplay pins that a
// window landed at its display's size did not refuse anything: no minimum
// is that big, and the frame written never applied.
func TestEngine_learnMinSize_IgnoresAWindowAsLargeAsItsDisplay(t *testing.T) {
	t.Parallel()

	engine := New(&clampingDesktop{}, nil, nil)
	engine.visible[1] = action.Frame{Width: 1920, Height: 1050}

	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{Width: 500, Height: 500},
		action.Frame{Width: 1920, Height: 1050},
	); grew {
		t.Fatal("a window as large as its display reported a minimum")
	}

	if _, ok := engine.minSizes[1]; ok {
		t.Fatalf("minSize = %+v, want none", engine.minSizes[1])
	}
}

// TestEngine_learnMinSize_IgnoresAFrameAskedOffTheDisplay pins that a
// window asked for a frame past the display's edge learns nothing from
// where it lands. The window server keeps part of every window on screen,
// and the size it settles on is the clamp's, not the application's.
func TestEngine_learnMinSize_IgnoresAFrameAskedOffTheDisplay(t *testing.T) {
	t.Parallel()

	engine := New(&clampingDesktop{}, nil, nil)
	engine.visible[1] = action.Frame{Y: 30, Width: 1920, Height: 1050}

	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{X: -948, Y: 38, Width: 948, Height: 1034},
		action.Frame{X: -955, Y: 38, Width: 959, Height: 1034},
	); grew {
		t.Fatal("a window asked for a frame off the display reported a minimum")
	}

	if _, ok := engine.minSizes[1]; ok {
		t.Fatalf("minSize = %+v, want none", engine.minSizes[1])
	}

	if grew := engine.learnMinSize(
		1,
		"app",
		1,
		action.Frame{X: 8, Y: 38, Width: 948, Height: 1034},
		action.Frame{X: 8, Y: 38, Width: 959, Height: 1034},
	); !grew {
		t.Fatal("a window asked for a frame on the display did not report its minimum")
	}
}

// TestEngine_SetStore_DiscardsAnOlderStore pins that what a build without
// the confirming read learned is not carried forward.
func TestEngine_SetStore_DiscardsAnOlderStore(t *testing.T) {
	t.Parallel()

	store := filepath.Join(t.TempDir(), "minsizes.json")

	err := os.WriteFile(store, []byte(`{"com.apple.Safari":{"width":574,"height":1034}}`), 0o600)
	if err != nil {
		t.Fatalf("writing store: %v", err)
	}

	engine := New(&clampingDesktop{}, nil, nil)
	engine.SetStore(store)

	if len(engine.appMinSizes) != 0 {
		t.Fatalf("appMinSizes = %v, want nothing from an unversioned store", engine.appMinSizes)
	}
}
