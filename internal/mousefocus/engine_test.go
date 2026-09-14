package mousefocus_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/mousefocus"
	"github.com/y3owk1n/mimi/internal/native"
)

// fakeDesktop has two windows side by side, window 1 on the left and
// window 2 on the right, and records every focus.
type fakeDesktop struct {
	mu        sync.Mutex
	front     uint32
	focused   []uint32
	buttonDwn bool
	started   int
	stopped   int
}

func (d *fakeDesktop) desktop() mousefocus.Desktop {
	return mousefocus.Desktop{
		WindowAt: func(p native.Point) (int, uint32, bool) {
			switch {
			case p.X < 500:
				return 10, 1, true
			case p.X < 1000:
				return 11, 2, true
			default:
				return 0, 0, false
			}
		},
		Frontmost: func() uint32 {
			d.mu.Lock()
			defer d.mu.Unlock()

			return d.front
		},
		ButtonDown: func() bool {
			d.mu.Lock()
			defer d.mu.Unlock()

			return d.buttonDwn
		},
		Focus: func(number uint32) error {
			d.mu.Lock()
			defer d.mu.Unlock()

			d.front = number
			d.focused = append(d.focused, number)

			return nil
		},
		Start: func() bool {
			d.mu.Lock()
			defer d.mu.Unlock()

			d.started++

			return true
		},
		Stop: func() {
			d.mu.Lock()
			defer d.mu.Unlock()

			d.stopped++
		},
	}
}

func (d *fakeDesktop) focusedWindows() []uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]uint32(nil), d.focused...)
}

// runEngine starts an enabled engine and returns the channel to move the
// pointer on, and a wait that gives a settle the time to happen.
func runEngine(t *testing.T, desktop *fakeDesktop, enabled bool) (chan native.Point, func()) {
	t.Helper()

	engine := mousefocus.New(desktop.desktop(), nil, nil)
	engine.Update(config.MouseConfig{FocusFollowsMouse: enabled})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	moves := make(chan native.Point, 1)
	go engine.Run(ctx, moves)

	return moves, func() { time.Sleep(120 * time.Millisecond) }
}

func TestEngine_FocusesTheWindowThePointerRestsOn(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{front: 1}
	moves, settled := runEngine(t, desktop, true)

	moves <- native.Point{X: 700, Y: 100}

	settled()

	// Resting on the same window again focuses nothing more.
	moves <- native.Point{X: 800, Y: 200}

	settled()

	if got := desktop.focusedWindows(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("focused = %v, want [2]", got)
	}
}

func TestEngine_LeavesFocusAloneWhileAButtonIsDownOrOverNothing(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{front: 1, buttonDwn: true}
	moves, settled := runEngine(t, desktop, true)

	moves <- native.Point{X: 700, Y: 100}

	settled()

	desktop.mu.Lock()
	desktop.buttonDwn = false
	desktop.mu.Unlock()

	moves <- native.Point{X: 1500, Y: 100}

	settled()

	if got := desktop.focusedWindows(); len(got) != 0 {
		t.Fatalf("focused = %v, want none", got)
	}
}

func TestEngine_Update_StartsAndStopsThePointerTap(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{front: 1}
	engine := mousefocus.New(desktop.desktop(), nil, nil)

	engine.Update(config.MouseConfig{FocusFollowsMouse: true})
	engine.Update(config.MouseConfig{FocusFollowsMouse: true})
	engine.Update(config.MouseConfig{FocusFollowsMouse: false})

	desktop.mu.Lock()
	defer desktop.mu.Unlock()

	if desktop.started != 1 || desktop.stopped != 1 {
		t.Fatalf("started %d, stopped %d, want 1 and 1", desktop.started, desktop.stopped)
	}
}
