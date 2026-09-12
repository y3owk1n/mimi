package tiling_test

import (
	"context"
	"testing"

	"github.com/y3owk1n/mimi/internal/tiling"
)

// TestEngine_State_ReportsWhatEachSpaceIsHolding pins what the state read
// answers with: one entry per display and space the layout has run for, naming
// that space both ways, and the windows the layout is not managing.
func TestEngine_State_ReportsWhatEachSpaceIsHolding(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.spaceIDs = map[uint32]uint64{7: 5000}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`jq -c '{frames: [], state: {kept: true}, unmanaged: [1]}'`), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	held := engine.State()

	if len(held.Spaces) != 1 {
		t.Fatalf("State() held %d spaces, want 1: %+v", len(held.Spaces), held.Spaces)
	}

	space := held.Spaces[0]
	if space.Display != 7 || space.SpaceID != 5000 {
		t.Fatalf("State() named display %d space %d, want 7 and 5000", space.Display, space.SpaceID)
	}

	// The space is in front, so it is also reported by the number the user
	// counts it by.
	if space.Space != 2 {
		t.Fatalf("State() reported Mission Control index %d, want 2", space.Space)
	}

	if got := string(space.State); got != `{"kept":true}` {
		t.Fatalf("State() held %s, want {\"kept\":true}", got)
	}

	if len(held.Unmanaged) != 1 || held.Unmanaged[0] != 1 {
		t.Fatalf("State() unmanaged = %v, want [1]", held.Unmanaged)
	}
}

// TestEngine_State_IsEmptyBeforeAnyPass pins that a read before the layout has
// run answers with nothing rather than failing, which is what a CLI with no
// daemon behind it prints.
func TestEngine_State_IsEmptyBeforeAnyPass(t *testing.T) {
	t.Parallel()

	engine := tiling.New(newDesktop(), nil, nil)

	if held := engine.State(); len(held.Spaces) != 0 || len(held.Unmanaged) != 0 {
		t.Fatalf("State() before any pass = %+v, want nothing held", held)
	}
}

// TestEngine_Reset_StartsTheSpaceInFrontOver pins the way out of a layout
// whose state has gone wrong: the space in front is forgotten and starts from
// null, and the other space the engine remembers is left alone.
func TestEngine_Reset_StartsTheSpaceInFrontOver(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.spaceIDs = map[uint32]uint64{7: 5000}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	// A second space on the same display, so the reset has something it
	// should not touch.
	desktop.space = 3
	desktop.spaceIDs = map[uint32]uint64{7: 6000}

	err = engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() on the second space error = %v", err)
	}

	if held := engine.State(); len(held.Spaces) != 2 {
		t.Fatalf("State() held %d spaces before the reset, want 2", len(held.Spaces))
	}

	dropped, err := engine.Reset(false)
	if err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if dropped != 1 {
		t.Fatalf("Reset() dropped %d spaces, want 1", dropped)
	}

	// The space in front starts over.
	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if got := string(inputs[0].State); got != noState {
		t.Fatalf("state on the reset space = %s, want null", got)
	}

	// The other one is where it was.
	desktop.space = 2
	desktop.spaceIDs = map[uint32]uint64{7: 5000}

	inputs, _, err = engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() on the untouched space error = %v", err)
	}

	if string(inputs[0].State) == noState {
		t.Fatal("the space that was not in front was reset too")
	}
}

// TestEngine_Reset_All pins that --all forgets every space rather than the
// ones in front.
func TestEngine_Reset_All(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.spaceIDs = map[uint32]uint64{7: 5000}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx := context.Background()

	for _, space := range []struct {
		index int
		id    uint64
	}{{2, 5000}, {3, 6000}} {
		desktop.space = space.index
		desktop.spaceIDs = map[uint32]uint64{7: space.id}

		err := engine.Pass(ctx, tiling.Event{Kind: created})
		if err != nil {
			t.Fatalf("Pass() on space %d error = %v", space.index, err)
		}
	}

	dropped, err := engine.Reset(true)
	if err != nil {
		t.Fatalf("Reset(all) error = %v", err)
	}

	if dropped != 2 {
		t.Fatalf("Reset(all) dropped %d spaces, want 2", dropped)
	}

	if held := engine.State(); len(held.Spaces) != 0 {
		t.Fatalf("State() after Reset(all) held %+v, want nothing", held.Spaces)
	}
}

// TestEngine_State_LeavesOutASpaceItCannotName pins that a space the window
// server would not identify, whose state is never kept, is never reported
// either.
func TestEngine_State_LeavesOutASpaceItCannotName(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.spaceIDs = map[uint32]uint64{}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if held := engine.State(); len(held.Spaces) != 0 {
		t.Fatalf("State() held %+v for an unnamable space, want nothing", held.Spaces)
	}
}
