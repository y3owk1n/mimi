package dropzone_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/dropzone"
	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/tiling"
)

type fakes struct {
	mu     sync.Mutex
	target tiling.DropTarget
	ok     bool
	down   bool
	shown  []geometry.Rect
	hidden int
}

func (f *fakes) DropPreview(context.Context) (tiling.DropTarget, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.target, f.ok, nil
}

func (f *fakes) Show(frame geometry.Rect, _ dropzone.Style) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.shown = append(f.shown, frame)
}

func (f *fakes) Hide() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.hidden++
}

func (f *fakes) LeftButtonDown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.down
}

func (f *fakes) release() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.down = false
}

func (f *fakes) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.shown), f.hidden
}

func (f *fakes) shownCount() int {
	shown, _ := f.counts()

	return shown
}

func (f *fakes) hiddenCount() int {
	_, hidden := f.counts()

	return hidden
}

func enabledConfig() config.DropzoneConfig {
	return config.DropzoneConfig{
		Enabled: true, Color: "#30e2e2e3", OutlineColor: "#e2e2e3", OutlineWidth: 2, Radius: 12,
	}
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("waited a second for %s", what)
}

func TestTracker_ShowsWhereTheDragWouldLandAndHidesOnRelease(t *testing.T) {
	t.Parallel()

	fake := &fakes{
		target: tiling.DropTarget{
			Number: 4,
			Frame:  action.Frame{X: 10, Y: 20, Width: 300, Height: 400},
		},
		ok:   true,
		down: true,
	}
	tracker := dropzone.New(fake, fake, fake, nil)
	tracker.Update(enabledConfig())

	tracker.Nudge()
	waitFor(t, "the zone to show", func() bool { return fake.shownCount() == 1 })

	fake.mu.Lock()
	got := fake.shown[0]
	fake.mu.Unlock()

	want := geometry.Rect{X: 10, Y: 20, W: 300, H: 400}
	if got != want {
		t.Errorf("zone shown at %+v, want the layout's frame %+v", got, want)
	}

	// A burst of nudges within the interval asks the layout once.
	tracker.Nudge()
	tracker.Nudge()

	if shown, _ := fake.counts(); shown != 1 {
		t.Errorf("zone shown %d times after a burst, want 1", shown)
	}

	fake.release()
	waitFor(t, "the zone to hide", func() bool { return fake.hiddenCount() == 1 })
}

func TestTracker_DoesNothingUnlessEnabledAndTheButtonIsDown(t *testing.T) {
	t.Parallel()

	fake := &fakes{target: tiling.DropTarget{Number: 4}, ok: true, down: true}
	tracker := dropzone.New(fake, fake, fake, nil)

	tracker.Nudge()
	time.Sleep(20 * time.Millisecond)

	if shown, _ := fake.counts(); shown != 0 {
		t.Errorf("a disabled tracker showed the zone %d times", shown)
	}

	tracker.Update(enabledConfig())
	fake.release()
	tracker.Nudge()
	time.Sleep(20 * time.Millisecond)

	if shown, _ := fake.counts(); shown != 0 {
		t.Errorf("the zone showed %d times with the button up", shown)
	}
}

func TestTracker_HidesWhenNothingIsBeingDragged(t *testing.T) {
	t.Parallel()

	fake := &fakes{target: tiling.DropTarget{Number: 4}, ok: true, down: true}
	tracker := dropzone.New(fake, fake, fake, nil)
	tracker.Update(enabledConfig())

	tracker.Nudge()
	waitFor(t, "the zone to show", func() bool { return fake.shownCount() == 1 })

	fake.mu.Lock()
	fake.ok = false
	fake.mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	tracker.Nudge()
	waitFor(t, "the zone to hide", func() bool { return fake.hiddenCount() == 1 })
}
