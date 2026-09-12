package stackbar_test

import (
	"sync"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/stackbar"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// fakeDrawer records what it was asked to draw.
type fakeDrawer struct {
	mu     sync.Mutex
	syncs  [][]stackbar.Bar
	clears int
}

func (d *fakeDrawer) Sync(bars []stackbar.Bar, _ stackbar.Style) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.syncs = append(d.syncs, bars)
}

func (d *fakeDrawer) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.clears++
}

func (d *fakeDrawer) last() []stackbar.Bar {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.syncs) == 0 {
		return nil
	}

	return d.syncs[len(d.syncs)-1]
}

func (d *fakeDrawer) counts() (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.syncs), d.clears
}

func enabled() config.StackbarConfig {
	return config.StackbarConfig{
		Enabled:  true,
		Step:     10,
		Taper:    6,
		Radius:   -1,
		Color:    "#b0636366",
		FarColor: "#30636366",
	}
}

func stackOf(active uint32, windows ...uint32) tiling.PlacedStack {
	return tiling.PlacedStack{
		Stack: tiling.Stack{Windows: windows, Active: active},
		Frame: action.Frame{X: 10, Y: 20, Width: 300, Height: 400},
	}
}

// TestTracker_Show_DrawsOneBarPerStack pins what reaches the drawer: the
// frame the stack's windows share, how many windows are in that place, and
// which of them the cards are drawn under.
func TestTracker_Show_DrawsOneBarPerStack(t *testing.T) {
	t.Parallel()

	draw := &fakeDrawer{}
	tracker := stackbar.New(draw, nil)
	tracker.Update(enabled())

	tracker.Show([]tiling.PlacedStack{stackOf(4243, 4242, 4243, 4244)})

	bars := draw.last()
	if len(bars) != 1 {
		t.Fatalf("drew %d bars, want 1", len(bars))
	}

	if bars[0].Count != 3 || bars[0].Front != 4243 {
		t.Fatalf("bar = %+v, want 3 windows with 4243 in front", bars[0])
	}

	if bars[0].Frame.Width != 300 {
		t.Fatalf("bar frame = %+v, want the frame the stack shares", bars[0].Frame)
	}
}

// TestTracker_Show_DrawsUnderTheFirstWhenActiveIsNotAMember pins that a
// layout naming a window that is not in the stack still puts the cards under
// one of its windows, since cards under nothing would float over the desktop.
func TestTracker_Show_DrawsUnderTheFirstWhenActiveIsNotAMember(t *testing.T) {
	t.Parallel()

	draw := &fakeDrawer{}
	tracker := stackbar.New(draw, nil)
	tracker.Update(enabled())

	tracker.Show([]tiling.PlacedStack{stackOf(9999, 4242, 4243)})

	if bars := draw.last(); len(bars) != 1 || bars[0].Front != 4242 {
		t.Fatalf("bars = %+v, want the cards under the first window", bars)
	}
}

// TestTracker_Show_DrawsNothingWhileDisabled pins that the indicator is off by
// default and costs a pass nothing when it is.
func TestTracker_Show_DrawsNothingWhileDisabled(t *testing.T) {
	t.Parallel()

	draw := &fakeDrawer{}
	tracker := stackbar.New(draw, nil)

	tracker.Show([]tiling.PlacedStack{stackOf(4242, 4242, 4243)})

	if syncs, clears := draw.counts(); syncs != 0 || clears != 0 {
		t.Fatalf("drew %d times and cleared %d while disabled, want neither", syncs, clears)
	}
}

// TestTracker_Update_ClearsWhatIsDrawnWhenSwitchedOff pins that switching the
// indicator off takes it off screen at once, rather than leaving it until a
// pass that may never come.
func TestTracker_Update_ClearsWhatIsDrawnWhenSwitchedOff(t *testing.T) {
	t.Parallel()

	draw := &fakeDrawer{}
	tracker := stackbar.New(draw, nil)
	tracker.Update(enabled())
	tracker.Show([]tiling.PlacedStack{stackOf(4242, 4242, 4243)})

	off := enabled()
	off.Enabled = false
	tracker.Update(off)

	if _, clears := draw.counts(); clears != 1 {
		t.Fatalf("cleared %d times, want 1", clears)
	}

	// And again is not another call into the window server.
	tracker.Update(off)

	if _, clears := draw.counts(); clears != 1 {
		t.Fatalf("cleared %d times after a second off, want 1", clears)
	}
}

// TestTracker_Show_StopsDrawingWhenTheStacksGo pins that a pass with no stack
// takes the last one off screen, and that the pass after it calls nothing.
func TestTracker_Show_StopsDrawingWhenTheStacksGo(t *testing.T) {
	t.Parallel()

	draw := &fakeDrawer{}
	tracker := stackbar.New(draw, nil)
	tracker.Update(enabled())

	tracker.Show([]tiling.PlacedStack{stackOf(4242, 4242, 4243)})
	tracker.Show(nil)

	syncs, _ := draw.counts()
	if syncs != 2 {
		t.Fatalf("drew %d times, want the stack and then nothing", syncs)
	}

	if bars := draw.last(); len(bars) != 0 {
		t.Fatalf("last draw = %+v, want no bars", bars)
	}

	tracker.Show(nil)

	if syncs, _ := draw.counts(); syncs != 2 {
		t.Fatalf("drew %d times, want no call for a second pass with no stack", syncs)
	}
}
