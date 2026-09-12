package tiling //nolint:testpackage // reads the states the passes kept

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// TestEngine_Run_UnpluggingADisplayLaysOutRatherThanDrags pins that the
// windows macOS moves when a display goes away are not taken for a drag:
// the desktop is laid out afresh instead, so a layout that drops a window
// where it was dropped does not pile them all where macOS put them.
func TestEngine_Run_UnpluggingADisplayLaysOutRatherThanDrags(t *testing.T) {
	t.Parallel()

	both := []action.DisplayEntry{
		{
			Index:   1,
			ID:      1,
			Frame:   action.Frame{Width: 1000, Height: 1000},
			Visible: action.Frame{Width: 1000, Height: 1000},
		},
		{
			Index:   2,
			ID:      2,
			Frame:   action.Frame{X: 1000, Width: 1000, Height: 1000},
			Visible: action.Frame{X: 1000, Width: 1000, Height: 1000},
		},
	}

	desktop := &snappingDesktop{frame: action.Frame{X: 1000, Width: 500, Height: 500}}
	desktop.setDisplays(both)

	engine := New(desktop, nil, nil)
	engine.resizeGrace = 50 * time.Millisecond
	engine.Update(config.TilingConfig{
		Enabled:        true,
		RelayoutOnDrag: true,
		DebounceMS:     10,
		TimeoutSecs:    5,
		Layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 800, height: 800}}], ` +
			`state: {last: .event.kind}}'`,
	}, "/bin/sh")

	var held atomic.Bool

	engine.SetMouse(held.Load)

	ctx := t.Context()

	sub := make(chan events.Event, 4)

	go engine.Run(ctx, sub)

	waitForCount(t, desktop, 1)
	time.Sleep(engine.resizeGrace)

	// The display goes away and macOS moves the window onto the one left.
	desktop.setDisplays(both[:1])
	desktop.move(-900)

	sub <- events.Event{Kind: events.WindowMove, PID: 10}

	waitForCount(t, desktop, 2)

	kinds := engine.lastKinds(t)
	if !strings.Contains(kinds, EventRelayout) {
		t.Errorf("the layout was run for %q, want a %q pass", kinds, EventRelayout)
	}

	if strings.Contains(kinds, string(events.WindowMove)) {
		t.Errorf("the layout was run for %q, want the moved window not taken for a drag", kinds)
	}
}

// lastKinds is every event kind the layout recorded in its state, joined.
func (e *Engine) lastKinds(t *testing.T) string {
	t.Helper()

	e.mu.Lock()
	defer e.mu.Unlock()

	kinds := make([]string, 0, len(e.states))
	for _, state := range e.states {
		kinds = append(kinds, string(state))
	}

	return strings.Join(kinds, " ")
}
