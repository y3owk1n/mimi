package tiling_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// recordingHandBack is a layout that places nothing, gives up the windows of
// pid 10, and keeps what it was told was replaced and handed back.
const recordingHandBack = `jq -c '{unmanaged: [.windows[] | select(.pid == 10) | .number], ` +
	`state: {replaced: .replaced, unmanaged: .unmanaged}}'`

// handedBack is the state recordingHandBack keeps.
type handedBack struct {
	Replaced  []tiling.Replacement `json:"replaced"`
	Unmanaged []uint32             `json:"unmanaged"`
}

// tabbedDesktop is a desktop with a window of pid 10, the front tab of a
// group, beside a window of pid 11.
func tabbedDesktop() *fakeDesktop {
	desktop := newDesktop()
	desktop.windows = action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: action.Frame{Width: 500, Height: 500}},
		{Number: 2, PID: 11, App: "B", Frame: action.Frame{X: 500, Width: 500, Height: 500}},
	}}

	return desktop
}

// handedBackAfterSwap runs a pass on tabbedDesktop, puts front where window 1
// was, switching to space when it is not 0, runs another, and returns what
// the second pass handed the layout.
func handedBackAfterSwap(t *testing.T, front action.WindowEntry, space int) handedBack {
	t.Helper()

	desktop := tabbedDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(recordingHandBack), shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("first Pass() error = %v", err)
	}

	desktop.windows.Windows[0] = front
	if space != 0 {
		desktop.space = space
	}

	err = engine.Pass(ctx, tiling.Event{Kind: string(events.WindowFocus)})
	if err != nil {
		t.Fatalf("second Pass() error = %v", err)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	var got handedBack

	err = json.Unmarshal(inputs[0].State, &got)
	if err != nil {
		t.Fatalf("state %s: %v", inputs[0].State, err)
	}

	return got
}

// TestEngine_Pass_NamesAWindowThatTookAnothersPlace pins that a native tab
// coming to the front reaches the layout as the window it replaced, with
// whether the layout manages it carried over. It does not reach the layout as
// one window closing and another opening.
func TestEngine_Pass_NamesAWindowThatTookAnothersPlace(t *testing.T) {
	t.Parallel()

	got := handedBackAfterSwap(t, action.WindowEntry{
		Number: 3, PID: 10, App: "A", Frame: action.Frame{Width: 500, Height: 500},
	}, 0)

	want := []tiling.Replacement{{Window: 3, Was: 1}}
	if len(got.Replaced) != 1 || got.Replaced[0] != want[0] {
		t.Fatalf("replaced = %+v, want %+v", got.Replaced, want)
	}

	if len(got.Unmanaged) != 1 || got.Unmanaged[0] != 3 {
		t.Fatalf("unmanaged = %v, want [3]", got.Unmanaged)
	}
}

// TestEngine_Pass_NamesNoReplacementForANewWindow pins what does not count as
// taking a window's place: another application's window, a window elsewhere,
// or one on another space where the window that went had been.
func TestEngine_Pass_NamesNoReplacementForANewWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		front action.WindowEntry
		space int
	}{
		{
			name: "another application",
			front: action.WindowEntry{
				Number: 3,
				PID:    12,
				App:    "C",
				Frame:  action.Frame{Width: 500, Height: 500},
			},
		},
		{
			name: "another frame",
			front: action.WindowEntry{
				Number: 3,
				PID:    10,
				App:    "A",
				Frame:  action.Frame{Width: 400, Height: 500},
			},
		},
		{
			name: "another space",
			front: action.WindowEntry{
				Number: 3,
				PID:    10,
				App:    "A",
				Frame:  action.Frame{Width: 500, Height: 500},
			},
			space: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := handedBackAfterSwap(t, test.front, test.space)
			if len(got.Replaced) != 0 {
				t.Fatalf("replaced = %+v, want none", got.Replaced)
			}
		})
	}
}

// stackingAll is a layout that stacks every window it is given, the last one
// in front, and keeps the stacks it was handed back.
const stackingAll = `jq -c '{frames: [.windows[] | {number, frame: {x: 0, y: 0, width: 500, height: 500}}], ` +
	`stacks: [{windows: [.windows[].number], active: .windows[-1].number}], state: {stacks: .stacks}}'`

// TestEngine_Pass_KeepsAReplacedWindowInItsStack pins that a window taking a
// stack member's place is handed back as that member.
func TestEngine_Pass_KeepsAReplacedWindowInItsStack(t *testing.T) {
	t.Parallel()

	desktop := twoWindowDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(stackingAll), shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("first Pass() error = %v", err)
	}

	desktop.windows.Windows[1] = action.WindowEntry{
		Number: 3, PID: 11, App: "B", Frame: action.Frame{Width: 500, Height: 500},
	}

	err = engine.Pass(ctx, tiling.Event{Kind: string(events.WindowFocus)})
	if err != nil {
		t.Fatalf("second Pass() error = %v", err)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	var got struct {
		Stacks []tiling.Stack `json:"stacks"`
	}

	err = json.Unmarshal(inputs[0].State, &got)
	if err != nil {
		t.Fatalf("state %s: %v", inputs[0].State, err)
	}

	if len(got.Stacks) != 1 || got.Stacks[0].Active != 3 ||
		len(got.Stacks[0].Windows) != 2 || got.Stacks[0].Windows[1] != 3 {
		t.Fatalf("stacks handed back = %+v, want windows [1 3] active 3", got.Stacks)
	}
}
