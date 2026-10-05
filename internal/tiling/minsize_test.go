//nolint:testpackage // sets the resize grace, which is not configuration
package tiling

import (
	"context"
	"fmt"
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
	// passingReads is how many reads after the next apply report window 2
	// at passing instead, the way an application still resizing on its own
	// shows a size on the way. Apply arms it from passingAfterApply.
	passingReads      int
	passingAfterApply int
	passing           action.Frame
}

func (d *clampingDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	frames := d.frames
	if d.lateReads > 0 && d.previous != nil {
		d.lateReads--
		frames = d.previous
	}

	if d.passingReads > 0 {
		d.passingReads--
		frames = maps.Clone(frames)
		frames[2] = d.passing
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
	d.passingReads, d.passingAfterApply = d.passingAfterApply, 0

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

// waitForApplies waits until the engine has applied the desktop want times,
// then a little longer so a pass that should not come has the chance to.
func (d *clampingDesktop) waitForApplies(t *testing.T, want int, why string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for d.count() < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	if got := d.count(); got != want {
		t.Fatalf("%s: applied %d times, want %d", why, got, want)
	}
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

	// Startup asks for two halves, the window refuses, and the engine
	// replays with the minimum known. Then nothing more.
	desktop.waitForApplies(t, 2, "startup and the pass that learned the minimum")

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

	desktop.waitForApplies(t, 3, "a pass after the minimum dropped")

	if got := desktop.widthAsked(2, 2); got != 600 {
		t.Fatalf("third pass asked width %v for window 2, want the remembered 600", got)
	}

	cancel()
	<-done
}

// TestEngine_Run_ForgetsAMinimumTheWindowNoLongerHas pins that a window which
// later takes a frame smaller than the one it refused loses its minimum, so
// the engine stops handing it to the layout. That holds on a pass where no
// other window refuses anything.
func TestEngine_Run_ForgetsAMinimumTheWindowNoLongerHas(t *testing.T) {
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
		// The layout asks for two halves whatever the windows report, so a
		// later pass asks below the minimum the second window refused at first.
		Layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 500, height: 1000}}, ` +
			`{number: 2, frame: {x: 500, y: 0, width: 500, height: 1000}}], state: null}'`,
	}, "/bin/sh")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := make(events.Subscriber, 8)
	done := make(chan struct{})

	go func() {
		engine.Run(ctx, sub)
		close(done)
	}()

	desktop.waitForApplies(t, 2, "startup and the pass that learned the minimum")

	if got := minSizeOfSecond(t, engine); got == nil || got.Width != 600 {
		t.Fatalf("minSize = %+v, want width 600", got)
	}

	desktop.mu.Lock()
	desktop.minWidth = 300
	desktop.mu.Unlock()

	sub <- events.Event{Kind: events.WindowFocus, PID: 10}

	desktop.waitForApplies(t, 3, "a pass after the minimum dropped")

	// The engine forgets only after its confirming read, a while after the
	// apply.
	got := minSizeOfSecond(t, engine)
	for deadline := time.Now().Add(3 * time.Second); got != nil && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)

		got = minSizeOfSecond(t, engine)
	}

	if got != nil {
		t.Fatalf("minSize = %+v after the window took less, want none", got)
	}

	cancel()
	<-done
}

// TestEngine_Run_KeepsAMinimumThroughASizeOnTheWay pins that the engine reads
// again before it forgets a minimum. Terminal, while it changes its font,
// reports a size far below its minimum for a moment after an apply.
// Forgetting the minimum from that read would cost a refusal on the next pass
// to learn it back.
func TestEngine_Run_KeepsAMinimumThroughASizeOnTheWay(t *testing.T) {
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
		Layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 500, height: 1000}}, ` +
			`{number: 2, frame: {x: 500, y: 0, width: 500, height: 1000}}], state: null}'`,
	}, "/bin/sh")

	sub := make(events.Subscriber, 8)

	go engine.Run(t.Context(), sub)

	desktop.waitForApplies(t, 2, "startup and the pass that learned the minimum")

	desktop.mu.Lock()
	desktop.passingAfterApply = 1
	desktop.passing = action.Frame{X: 500, Width: 200, Height: 200}
	desktop.mu.Unlock()

	sub <- events.Event{Kind: events.WindowFocus, PID: 10}

	desktop.waitForApplies(t, 3, "a pass that reads a size on the way")
	time.Sleep(confirmRefusal + 500*time.Millisecond)

	if got := minSizeOfSecond(t, engine); got == nil || got.Width != 600 {
		t.Fatalf("minSize = %+v after a size on the way, want width 600 kept", got)
	}

	if got := desktop.count(); got != 3 {
		t.Fatalf("applied %d times, want 3: nothing was learned again", got)
	}
}

// TestEngine_Run_DoesNotLearnFromAFrameTheWindowServerClamped pins that a
// window landing larger than asked has not always refused anything. A window
// as large as its display took a frame that never applied, since no minimum
// is that big. The window server clamps a window asked for a frame past the
// display's edge, since it keeps part of every window on screen. The engine
// learns no minimum from either and runs no extra pass.
func TestEngine_Run_DoesNotLearnFromAFrameTheWindowServerClamped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		minWidth float64
		secondX  int
	}{
		{name: "a window as large as its display", minWidth: 1000, secondX: 500},
		{name: "a frame asked past the display's edge", minWidth: 600, secondX: 600},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := &clampingDesktop{
				frames: map[uint32]action.Frame{
					1: {Width: 500, Height: 500},
					2: {X: 500, Width: 500, Height: 500},
				},
				minWidth: testCase.minWidth,
			}
			engine := New(desktop, nil, nil)
			engine.resizeGrace = 50 * time.Millisecond
			engine.Update(config.TilingConfig{
				Enabled:     true,
				DebounceMS:  10,
				TimeoutSecs: 5,
				Layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 500, height: 1000}}, ` +
					fmt.Sprintf(
						`{number: 2, frame: {x: %d, y: 0, width: 500, height: 1000}}], state: null}'`,
						testCase.secondX,
					),
			}, "/bin/sh")

			sub := make(events.Subscriber, 8)

			go engine.Run(t.Context(), sub)

			// Long enough for the read-back, the confirming read, and the
			// pass a wrongly learned minimum would have asked for.
			desktop.waitForApplies(t, 1, "startup alone")
			time.Sleep(confirmRefusal + 500*time.Millisecond)

			if got := desktop.count(); got != 1 {
				t.Fatalf("applied %d times, want 1: a clamped frame is not a refusal", got)
			}

			if got := minSizeOfSecond(t, engine); got != nil {
				t.Fatalf("minSize = %+v, want none", got)
			}
		})
	}
}

// minSizeOfSecond is the minimum the engine would hand the layout for the
// clamping desktop's second window, or nil for none.
func minSizeOfSecond(t *testing.T, engine *Engine) *action.MinSize {
	t.Helper()

	inputs, _, err := engine.Preview(t.Context(), Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	return inputs[0].Windows[1].MinSize
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

// TestEngine_Run_KeepsASpaceSwitchPassUnderItsMoveEchoes pins that the
// engine lays out a space switch even when the windows it slides raise moves
// before the debounce ends, as activating an application on another space
// does.
func TestEngine_Run_KeepsASpaceSwitchPassUnderItsMoveEchoes(t *testing.T) {
	t.Parallel()

	desktop := &clampingDesktop{
		frames: map[uint32]action.Frame{
			1: {Width: 500, Height: 500},
			2: {X: 500, Width: 500, Height: 500},
		},
	}
	engine := New(desktop, nil, nil)
	engine.resizeGrace = time.Second
	engine.Update(config.TilingConfig{
		Enabled:        true,
		RelayoutOnDrag: true,
		DebounceMS:     50,
		TimeoutSecs:    5,
		Layout:         `jq -c '{frames: [.windows[] | {number, frame}], state: null}'`,
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

	sub <- events.Event{Kind: events.WorkspaceChanged}

	time.Sleep(10 * time.Millisecond)

	sub <- events.Event{Kind: events.WindowMove, PID: 20}

	deadline = time.Now().Add(3 * time.Second)
	for desktop.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if got := desktop.count(); got != 2 {
		t.Fatalf("applied %d times, want the space switch laid out under the move", got)
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
