//nolint:testpackage // reads the engine's inputs, as engine_test does
package tiling

import (
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// TestEngine_Inputs_LeaveOutAWindowARuleKeepsFromTheLayout pins what a
// [[tiling.rules]] entry with manage = false does. The window never reaches
// the layout, and the focused index still names the window that has focus.
func TestEngine_Inputs_LeaveOutAWindowARuleKeepsFromTheLayout(t *testing.T) {
	t.Parallel()

	desktop := newPairDesktop()
	engine := New(desktop, nil, nil)

	manage := false
	engine.Update(config.TilingConfig{
		Enabled:     true,
		TimeoutSecs: 5,
		Layout:      "cat",
		Rules:       []config.TilingRule{{App: "A", Manage: &manage}},
	}, "/bin/sh")

	engine.mu.Lock()
	defer engine.mu.Unlock()

	inputs, err := engine.inputsLocked(Event{Kind: "preview"})
	if err != nil {
		t.Fatalf("inputsLocked() error = %v, want nil", err)
	}

	if len(inputs) != 1 || len(inputs[0].Windows) != 1 {
		t.Fatalf("inputs = %+v, want one input with one window", inputs)
	}

	if got := inputs[0].Windows[0].Number; got != 2 {
		t.Fatalf("window left = %d, want 2", got)
	}

	if got := inputs[0].Focused; got != -1 {
		t.Fatalf("focused = %d, want -1 since the focused window was kept out", got)
	}
}

// TestEngine_Run_IgnoresADragOfAWindowARuleKeepsOut pins that dragging a
// window a rule keeps out raises no pass.
func TestEngine_Run_IgnoresADragOfAWindowARuleKeepsOut(t *testing.T) {
	t.Parallel()

	manage := false
	desktop := newPairDesktop()
	_, sub, waitFor := runPairEngine(
		t,
		desktop,
		`jq -c '{frames: [.windows[] | {number, frame}]}'`,
		config.TilingRule{App: "B", Manage: &manage},
	)

	time.Sleep(60 * time.Millisecond)

	desktop.dragSecond()

	sub <- events.Event{Kind: events.WindowMove, PID: 11}

	waitFor(1, "a drag of the window a rule keeps out")
}
