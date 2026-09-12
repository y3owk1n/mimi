package tiling //nolint:testpackage // sets the resize grace, as resize_test does

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// TestEngine_Run_WaitsForTheMouseBeforeLayingOutADrag pins that a drag
// which settles while the button is still held runs no pass until the
// release, however long the user pauses.
func TestEngine_Run_WaitsForTheMouseBeforeLayingOutADrag(t *testing.T) {
	t.Parallel()

	desktop := &snappingDesktop{frame: action.Frame{Width: 500, Height: 500}}
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

	held.Store(true)
	engine.SetMouse(held.Load)

	ctx := t.Context()

	sub := make(chan events.Event, 4)

	go engine.Run(ctx, sub)

	// The startup pass places the window; past the grace, the user drags
	// it and pauses with the button down.
	waitForCount(t, desktop, 1)
	time.Sleep(engine.resizeGrace)
	desktop.move(300)

	sub <- events.Event{Kind: events.WindowMove, PID: 10}

	time.Sleep(100 * time.Millisecond)

	if applies := desktop.count(); applies != 1 {
		t.Fatalf("the drag was laid out %d times while the button was held, want none", applies-1)
	}

	held.Store(false)
	waitForCount(t, desktop, 2)
}

// waitForCount waits for the desktop to have been written count times.
func waitForCount(t *testing.T, desktop *snappingDesktop, count int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if desktop.count() >= count {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("waited for %d applies, have %d", count, desktop.count())
}
