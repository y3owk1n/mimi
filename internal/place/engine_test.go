package place_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/place"
)

// move is one move the fake desktop was asked for.
type move struct {
	kind   string
	number uint32
	target int
	follow bool
}

// fakeDesktop lists whatever windows a test puts in it and records moves.
type fakeDesktop struct {
	mu      sync.Mutex
	windows []place.Window
	moves   []move
	// spaces and displays are where each window is now, by number; 1
	// when unset.
	spaces   map[uint32]int
	displays map[uint32]int
}

func (d *fakeDesktop) desktop() place.Desktop {
	return place.Desktop{
		Windows: func() []uint32 {
			d.mu.Lock()
			defer d.mu.Unlock()

			numbers := make([]uint32, 0, len(d.windows))
			for _, win := range d.windows {
				numbers = append(numbers, win.Number)
			}

			return numbers
		},
		WindowsOf: func(pid int) []place.Window {
			d.mu.Lock()
			defer d.mu.Unlock()

			var mine []place.Window

			for _, win := range d.windows {
				if win.PID == pid {
					mine = append(mine, win)
				}
			}

			return mine
		},
		Applications: func() []int {
			d.mu.Lock()
			defer d.mu.Unlock()

			seen := map[int]bool{}

			var pids []int

			for _, win := range d.windows {
				if !seen[win.PID] {
					seen[win.PID] = true
					pids = append(pids, win.PID)
				}
			}

			return pids
		},
		Located: func(win place.Window) (int, int) {
			d.mu.Lock()
			defer d.mu.Unlock()

			space, display := 1, 1

			if known, ok := d.spaces[win.Number]; ok {
				space = known
			}

			if known, ok := d.displays[win.Number]; ok {
				display = known
			}

			return space, display
		},
		Application: func(pid int) (string, string) {
			if pid == 10 {
				return "Slack", "com.tinyspeck.slackmacgap"
			}

			return "Other", "com.example.other"
		},
		MoveToSpace: func(_ int, number uint32, space int, follow bool) error {
			d.record(move{toSpace, number, space, follow})

			return nil
		},
		MoveToDisplay: func(_ int, number uint32, display int, follow bool) error {
			d.record(move{"display", number, display, follow})

			return nil
		},
	}
}

func (d *fakeDesktop) record(m move) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.moves = append(d.moves, m)
}

func (d *fakeDesktop) open(win place.Window) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.windows = append(d.windows, win)
}

func (d *fakeDesktop) recorded() []move {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]move(nil), d.moves...)
}

// runEngine starts an engine with rules over a desktop that already holds
// one Slack window on its rule's space, and returns the channel to raise
// events on and the engine for a reload.
func runEngine(
	t *testing.T,
	desktop *fakeDesktop,
	rules []config.TilingRule,
) (events.Subscriber, *place.Engine) {
	t.Helper()

	desktop.open(place.Window{PID: 10, Number: 1, Width: 800, Height: 600})
	desktop.mu.Lock()
	desktop.spaces = map[uint32]int{1: 3}
	desktop.mu.Unlock()

	engine := place.New(desktop.desktop(), nil)
	engine.Update(rules)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	sub := make(events.Subscriber, 4)
	go engine.Run(ctx, sub)

	settle()

	return sub, engine
}

const (
	toSpace   = "space"
	slackGlob = "com.tinyspeck.*"
)

func settle() { time.Sleep(150 * time.Millisecond) }

func TestEngine_PlacesANewWindowOnceWhereItsRuleSays(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{}
	sub, _ := runEngine(t, desktop, []config.TilingRule{{BundleID: slackGlob, Space: 3}})

	// The window already open is left alone.
	sub <- events.Event{Kind: events.WindowCreated, PID: 10}

	settle()

	desktop.open(place.Window{PID: 10, Number: 2, Width: 800, Height: 600})

	sub <- events.Event{Kind: events.WindowCreated, PID: 10}

	settle()

	// The same window reported again is not moved again.
	sub <- events.Event{Kind: events.WindowCreated, PID: 10}

	settle()

	want := []move{{toSpace, 2, 3, true}}
	if got := desktop.recorded(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("moves = %v, want %v", got, want)
	}
}

func TestEngine_PlacesTheWindowsOfAnApplicationItReachedLate(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{}
	sub, _ := runEngine(t, desktop, []config.TilingRule{{BundleID: slackGlob, Space: 3}})

	desktop.open(place.Window{PID: 10, Number: 2, Width: 800, Height: 600})

	sub <- events.Event{Kind: events.AXAttached, PID: 10}

	settle()

	want := move{toSpace, 2, 3, true}
	if got := desktop.recorded(); len(got) != 1 || got[0] != want {
		t.Fatalf("moves = %v, want %v", got, want)
	}
}

func TestEngine_LeavesAWindowNoRuleNames(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{}
	sub, _ := runEngine(t, desktop, []config.TilingRule{{BundleID: slackGlob, Space: 3}})

	desktop.open(place.Window{PID: 11, Number: 5, Width: 800, Height: 600})

	sub <- events.Event{Kind: events.WindowCreated, PID: 11}

	settle()

	if got := desktop.recorded(); len(got) != 0 {
		t.Fatalf("moves = %v, want none", got)
	}
}

func TestEngine_MovesToADisplayWithoutFollowingWhenTold(t *testing.T) {
	t.Parallel()

	// The window open at startup is on display 2 already, so only the new
	// one moves.
	desktop := &fakeDesktop{displays: map[uint32]int{1: 2}}
	sub, _ := runEngine(
		t,
		desktop,
		[]config.TilingRule{{App: "Slack", Display: 2, Follow: new(false)}},
	)

	desktop.open(place.Window{PID: 10, Number: 2, Width: 800, Height: 600})

	sub <- events.Event{Kind: events.WindowCreated, PID: 10}

	settle()

	want := move{"display", 2, 2, false}
	if got := desktop.recorded(); len(got) != 1 || got[0] != want {
		t.Fatalf("moves = %v, want %v", got, want)
	}
}

func TestEngine_SweepsOpenWindowsIntoPlaceOnStartAndOnRuleChange(t *testing.T) {
	t.Parallel()

	desktop := &fakeDesktop{}
	// A second Slack window is open on the wrong space before the engine
	// starts.
	desktop.open(place.Window{PID: 10, Number: 2, Width: 800, Height: 600})

	_, engine := runEngine(t, desktop, []config.TilingRule{{BundleID: slackGlob, Space: 3}})

	want := []move{{toSpace, 2, 3, false}}
	if got := desktop.recorded(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("moves after start = %v, want %v, without follow", got, want)
	}

	// A reload with the same rules sweeps nothing.
	engine.Update([]config.TilingRule{{BundleID: slackGlob, Space: 3}})
	settle()

	if got := desktop.recorded(); len(got) != 1 {
		t.Fatalf("moves after an unchanged reload = %v, want the one from startup", got)
	}

	// A reload that changes the destination sweeps again.
	engine.Update([]config.TilingRule{{BundleID: slackGlob, Space: 4}})
	settle()

	if got := desktop.recorded(); len(got) != 3 || got[1].target != 4 || got[2].target != 4 {
		t.Fatalf("moves after a changed reload = %v, want both windows sent to space 4", got)
	}
}
