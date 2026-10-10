//nolint:testpackage // reads the engine's inputs, as engine_test does
package tiling

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// TestEngine_Inputs_LeaveOutAWindowARuleKeepsFromTheLayout pins what a
// [[tiling.rules]] entry with manage = false does. The window never reaches
// the layout, the focused index still names the window that has focus, and
// focusKeptOut says when that window is the one kept out.
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

	if !inputs[0].FocusKeptOut {
		t.Fatalf("focusKeptOut = false, want true since the focused window was kept out")
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

// TestEngine_Pass_LogsTheWindowsItLeftOutAndWhatItSpent pins the two debug
// lines docs/TILING.md sends a user to: the windows a pass left out, by
// reason, and how long each part of the pass took.
func TestEngine_Pass_LogsTheWindowsItLeftOutAndWhatItSpent(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	engine := New(newPairDesktop(), nil, zap.New(core).Sugar())

	manage := false
	engine.Update(config.TilingConfig{
		Enabled:     true,
		TimeoutSecs: 5,
		Layout:      `jq -c '{frames: [.windows[] | {number, frame}], state: null}'`,
		Rules:       []config.TilingRule{{App: "A", Manage: &manage}},
	}, "/bin/sh")

	err := engine.Pass(t.Context(), Event{Kind: EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	left := logs.FilterMessage("windows left out").All()
	if len(left) != 1 || fmt.Sprint(left[0].ContextMap()["rule"]) != "[1]" {
		t.Fatalf("windows left out = %+v, want window 1 under rule", left)
	}

	applied := logs.FilterMessage("pass applied").All()
	if len(applied) != 1 {
		t.Fatalf("got %d pass applied lines, want 1", len(applied))
	}

	for _, field := range []string{"read_ms", "layout_ms", "apply_ms", "total_ms"} {
		if _, ok := applied[0].ContextMap()[field]; !ok {
			t.Errorf("pass applied has no %s: %v", field, applied[0].ContextMap())
		}
	}
}
