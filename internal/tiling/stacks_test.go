package tiling_test

import (
	"context"
	"sync"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// twoWindowDesktop is newDesktop with a second window, which is the fewest a
// stack can be made of.
func twoWindowDesktop() *fakeDesktop {
	desktop := newDesktop()
	desktop.windows = action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: action.Frame{Width: 500, Height: 500}},
		{Number: 2, PID: 11, App: "B", Frame: action.Frame{X: 500, Width: 500, Height: 500}},
	}}

	return desktop
}

// heard collects what the engine tells about stacks.
type heard struct {
	mu     sync.Mutex
	stacks [][]tiling.PlacedStack
}

func (h *heard) note(stacks []tiling.PlacedStack) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.stacks = append(h.stacks, stacks)
}

func (h *heard) last() []tiling.PlacedStack {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.stacks) == 0 {
		return nil
	}

	return h.stacks[len(h.stacks)-1]
}

// stacking is a layout that puts both windows in one place and names that a
// stack, which is what a stacking layout does.
const stacking = `jq -c '{frames: [` +
	`{number: 1, frame: {x: 0, y: 0, width: 500, height: 500}},` +
	`{number: 2, frame: {x: 0, y: 0, width: 500, height: 500}}], ` +
	`stacks: [{windows: [1, 2], active: 2}], state: {stacked: true}}'`

// TestEngine_Pass_TellsTheStacksALayoutNamed pins that a stack reaches
// whatever draws it, with the frame its windows share, since that frame is
// the one thing the layout does not repeat in the stack itself.
func TestEngine_Pass_TellsTheStacksALayoutNamed(t *testing.T) {
	t.Parallel()

	desktop := twoWindowDesktop()
	told := &heard{}

	engine := tiling.New(desktop, nil, nil)
	engine.SetStacks(told.note)
	engine.Update(enabled(stacking), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	stacks := told.last()
	if len(stacks) != 1 {
		t.Fatalf("told %d stacks, want 1: %+v", len(stacks), stacks)
	}

	stack := stacks[0]
	if len(stack.Windows) != 2 || stack.Active != 2 {
		t.Fatalf("stack = %+v, want windows [1 2] active 2", stack.Stack)
	}

	want := action.Frame{Width: 500, Height: 500}
	if stack.Frame != want {
		t.Fatalf("stack frame = %+v, want %+v", stack.Frame, want)
	}
}

// TestEngine_Pass_HandsTheStacksBack pins that a layout is told the stacks it
// named last time, so one that lost its own state can pick them up again.
func TestEngine_Pass_HandsTheStacksBack(t *testing.T) {
	t.Parallel()

	desktop := twoWindowDesktop()

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(stacking), shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(inputs[0].Stacks) != 1 {
		t.Fatalf("input carried %d stacks, want 1", len(inputs[0].Stacks))
	}

	if got := inputs[0].Stacks[0]; len(got.Windows) != 2 || got.Active != 2 {
		t.Fatalf("input stack = %+v, want windows [1 2] active 2", got)
	}
}

// TestEngine_Pass_DropsAStackItCannotDraw pins the two a stack is refused
// for: a member with no frame, whose mark would sit where nothing is, and a
// stack of one, which is every window in every layout.
func TestEngine_Pass_DropsAStackItCannotDraw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		layout string
	}{
		{
			name: "a member with no frame",
			layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 500, height: 500}}], ` +
				`stacks: [{windows: [1, 2], active: 1}], state: {}}'`,
		},
		{
			name: "a stack of one",
			layout: `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 500, height: 500}}], ` +
				`stacks: [{windows: [1], active: 1}], state: {}}'`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := twoWindowDesktop()
			told := &heard{}

			engine := tiling.New(desktop, nil, nil)
			engine.SetStacks(told.note)
			engine.Update(enabled(testCase.layout), shell)

			// The frames still apply; only the stack is refused.
			err := engine.Pass(context.Background(), tiling.Event{Kind: created})
			if err != nil {
				t.Fatalf("Pass() error = %v", err)
			}

			if stacks := told.last(); len(stacks) != 0 {
				t.Fatalf("told %+v, want no stack", stacks)
			}

			if len(desktop.applied) != 1 {
				t.Fatalf(
					"applied %d times, want the frames to have been written",
					len(desktop.applied),
				)
			}
		})
	}
}
