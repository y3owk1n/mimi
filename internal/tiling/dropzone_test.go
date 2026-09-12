package tiling_test

import (
	"context"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/tiling"
)

func TestEngine_DropPreview_AsksTheLayoutWhereTheDraggedWindowLands(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	cfg := enabled(echoLayout)
	cfg.RelayoutOnDrag = true
	engine.Update(cfg, shell)

	ctx := context.Background()

	_, early, err := engine.DropPreview(ctx)
	if early || err != nil {
		t.Fatalf("DropPreview() before any pass = ok %v, err %v, want nothing dragged", early, err)
	}

	err = engine.Pass(ctx, tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	// The user drags window 1 away from where the layout put it.
	desktop.mu.Lock()
	desktop.windows.Windows[0].Frame = action.Frame{X: 400, Y: 300, Width: 100, Height: 100}
	desktop.mu.Unlock()

	target, ok, err := engine.DropPreview(ctx)
	if err != nil || !ok {
		t.Fatalf("DropPreview() = ok %v, err %v, want the layout's answer", ok, err)
	}

	want := tiling.DropTarget{Number: 1, Frame: action.Frame{X: 0, Y: 0, Width: 100, Height: 100}}
	if target != want {
		t.Errorf("DropPreview() = %+v, want %+v", target, want)
	}

	// Nothing was applied or remembered: the pass that follows the drop
	// still sees the drag.
	_, again, _ := engine.DropPreview(ctx)
	if !again {
		t.Error("a preview changed what the engine remembers")
	}
}

func TestEngine_DropPreview_NeedsRelayoutOnDrag(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	desktop.mu.Lock()
	desktop.windows.Windows[0].Frame = action.Frame{X: 400, Y: 300, Width: 100, Height: 100}
	desktop.mu.Unlock()

	_, found, err := engine.DropPreview(ctx)
	if found || err != nil {
		t.Errorf(
			"DropPreview() = ok %v, err %v, want nothing with relayout_on_drag off",
			found,
			err,
		)
	}
}
