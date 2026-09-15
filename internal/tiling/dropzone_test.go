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

func TestEngine_DropPreview_MarksTheTargetTheLayoutNames(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.windows.Windows = append(desktop.windows.Windows, action.WindowEntry{
		Number: 2, PID: 11, App: "B", Frame: action.Frame{X: 500, Width: 500, Height: 500},
	})
	engine := tiling.New(desktop, nil, nil)
	cfg := enabled(
		`jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 100, height: 100}},` +
			`{number: 2, frame: {x: 500, y: 0, width: 100, height: 100}}], state: null,` +
			` target: (if .event.kind == "window_move" then {window: 2, action: "swap"} else null end)}'`,
	)
	cfg.RelayoutOnDrag = true
	engine.Update(cfg, shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	desktop.mu.Lock()
	desktop.windows.Windows[0].Frame = action.Frame{X: 400, Y: 300, Width: 100, Height: 100}
	desktop.mu.Unlock()

	target, ok, err := engine.DropPreview(ctx)
	if err != nil || !ok {
		t.Fatalf("DropPreview() = ok %v, err %v, want the layout's answer", ok, err)
	}

	want := &tiling.Highlight{
		Number: 2,
		Frame:  action.Frame{X: 500, Y: 0, Width: 100, Height: 100},
		Action: "swap",
	}
	if target.Target == nil || *target.Target != *want {
		t.Fatalf("target = %+v, want %+v", target.Target, want)
	}
}

func TestEngine_DropPreview_TellsTheLayoutWhichModifiersAreHeld(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	// The frame's x says which modifiers the layout was told: 50 for an
	// option-drag, 0 otherwise, so the first pass still places the window.
	cfg := enabled(
		`jq -c '{frames: [{number: 1, frame: {x: (if .event.modifiers == ["option"] then 50 else 0 end),` +
			` y: 0, width: 100, height: 100}}]}'`,
	)
	cfg.RelayoutOnDrag = true
	engine.Update(cfg, shell)
	engine.SetModifiers(func() []string { return []string{"option"} })

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	desktop.mu.Lock()
	desktop.windows.Windows[0].Frame = action.Frame{X: 400, Y: 300, Width: 100, Height: 100}
	desktop.mu.Unlock()

	target, ok, err := engine.DropPreview(ctx)
	if err != nil || !ok {
		t.Fatalf("DropPreview() = ok %v, err %v, want the layout's answer", ok, err)
	}

	if target.Frame.X != 50 {
		t.Fatalf("frame x = %v, want 50 for an option-drag", target.Frame.X)
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

// titledDesktop is a fake desktop that can take titles as given, and
// counts how each listing was asked for.
type titledDesktop struct {
	*fakeDesktop

	withTitles int
	known      map[uint32]string
}

func (d *titledDesktop) WindowsWithTitles(known map[uint32]string) (action.WindowsInfo, error) {
	d.withTitles++
	d.known = known

	return d.Windows()
}

func TestEngine_DropPreview_ReusesTheTitlesOfTheLastPass(t *testing.T) {
	t.Parallel()

	desktop := &titledDesktop{fakeDesktop: newDesktop()}
	desktop.windows.Windows[0].Title = "notes.md"
	engine := tiling.New(desktop, nil, nil)
	cfg := enabled(echoLayout)
	cfg.RelayoutOnDrag = true
	engine.Update(cfg, shell)

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
	if err != nil || !found {
		t.Fatalf("DropPreview() = ok %v, err %v, want the layout's answer", found, err)
	}

	if desktop.withTitles != 1 {
		t.Errorf(
			"the preview listed windows with given titles %d times, want once",
			desktop.withTitles,
		)
	}

	if desktop.known[1] != "notes.md" {
		t.Errorf(
			"the preview was handed titles %v, want the pass's title for window 1",
			desktop.known,
		)
	}
}
