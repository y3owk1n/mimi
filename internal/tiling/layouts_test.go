//nolint:testpackage // reads the programs and states the engine holds
package tiling

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
)

// namedLayout is a layout that records its own name and the state it was
// handed, so a test can tell which program ran for which display.
func namedLayout(name string) string {
	return `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 100, height: 100}}], ` +
		`state: {name: "` + name + `", was: .state}}'`
}

// twoDisplayDesktop is a desktop of two displays, each with one window, on
// the same space, on which every apply lands.
type twoDisplayDesktop struct{}

func (twoDisplayDesktop) Windows() (action.WindowsInfo, error) {
	return action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: action.Frame{Width: 500, Height: 500}},
		{Number: 2, PID: 11, App: "B", Frame: action.Frame{X: 1200, Width: 500, Height: 500}},
	}}, nil
}

func (twoDisplayDesktop) Displays() ([]action.DisplayEntry, error) {
	return []action.DisplayEntry{
		{
			Index:   1,
			ID:      7,
			Frame:   action.Frame{Width: 1000, Height: 1000},
			Visible: action.Frame{Width: 1000, Height: 1000},
		},
		{
			Index:   2,
			ID:      8,
			Frame:   action.Frame{X: 1000, Width: 1000, Height: 1000},
			Visible: action.Frame{X: 1000, Width: 1000, Height: 1000},
		},
	}, nil
}

func (twoDisplayDesktop) ActiveSpaces() (map[uint32]int, error) {
	return map[uint32]int{7: 2, 8: 2}, nil
}

func (twoDisplayDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	return map[uint32]uint64{7: 1002, 8: 1002}, nil
}

func (twoDisplayDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return map[uint32]bool{}, nil
}

func (twoDisplayDesktop) Margins() (action.MarginsInfo, error) { return action.MarginsInfo{}, nil }

func (twoDisplayDesktop) Apply([]action.WindowFrame, *action.Animation) error { return nil }

func (twoDisplayDesktop) Focus(uint32) error { return nil }

// enabledWith is a [tiling] section running layout on every display.
func enabledWith(layout string) config.TilingConfig {
	return config.TilingConfig{Enabled: true, Layout: layout, DebounceMS: 10, TimeoutSecs: 5}
}

const (
	layoutShell = "/bin/sh"
	nullState   = "null"
	// passThrough is a layout that prints its input back, enough for a
	// test of which programs exist.
	passThrough = "cat"
)

func stateName(t *testing.T, state json.RawMessage) string {
	t.Helper()

	var got struct {
		Name string `json:"name"`
	}

	err := json.Unmarshal(state, &got)
	if err != nil {
		t.Fatalf("state %s: %v", state, err)
	}

	return got.Name
}

func TestEngine_Preview_RunsTheLayoutNamedForEachDisplay(t *testing.T) {
	t.Parallel()

	engine := New(twoDisplayDesktop{}, nil, nil)
	cfg := enabledWith(namedLayout("default"))
	cfg.Layouts = []config.LayoutTarget{{Display: 2, Layout: namedLayout("second")}}
	engine.Update(cfg, layoutShell)

	_, outs, err := engine.Preview(context.Background(), Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(outs) != 2 || stateName(t, outs[0].State) != "default" ||
		stateName(t, outs[1].State) != "second" {
		t.Fatalf("Preview() outputs = %+v, want default on display 1 and second on display 2", outs)
	}
}

func TestEngine_Pass_KeepsStatePerProgram(t *testing.T) {
	t.Parallel()

	engine := New(twoDisplayDesktop{}, nil, nil)
	cfg := enabledWith(namedLayout("default"))
	cfg.Layouts = []config.LayoutTarget{{Space: 2, Layout: namedLayout("bySpace")}}
	engine.Update(cfg, layoutShell)

	err := engine.Pass(context.Background(), Event{Kind: "window_created"})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	// Space 2 is in front on both displays, so the by-space layout ran on
	// both and each display holds its own state for it.
	held := engine.State()
	if len(held.Spaces) != 2 {
		t.Fatalf("State() = %+v, want two entries", held)
	}

	for _, space := range held.Spaces {
		if space.Layout != namedLayout("bySpace") || stateName(t, space.State) != "bySpace" {
			t.Fatalf("state %+v, want the by-space layout's", space)
		}
	}

	// Switching the space to the default layout starts it from null, and
	// leaves the other program's state where it was.
	cfg.Layouts = nil
	engine.Update(cfg, layoutShell)

	engine.mu.Lock()
	inputs, err := engine.inputsLocked(Event{Kind: EventRelayout})
	engine.mu.Unlock()

	if err != nil {
		t.Fatalf("inputsLocked() error = %v", err)
	}

	for _, input := range inputs {
		if string(input.State) != nullState {
			t.Fatalf(
				"input state = %s, want %s for a program that has not run here",
				input.State,
				nullState,
			)
		}
	}

	if got := engine.State(); len(got.Spaces) != 2 {
		t.Fatalf("State() after switching = %+v, want the old program's two entries kept", got)
	}
}

func TestEngine_Update_KeepsOneResidentPerProgram(t *testing.T) {
	t.Parallel()

	engine := New(twoDisplayDesktop{}, nil, nil)
	cfg := enabledWith(passThrough)
	cfg.LayoutMode = config.LayoutModeResident
	cfg.Layouts = []config.LayoutTarget{
		{Display: 2, Layout: "tee"},
		{Space: 9, Layout: passThrough},
	}
	engine.Update(cfg, layoutShell)

	if len(engine.residents) != 2 {
		t.Fatalf("residents = %d, want 2, one per distinct command", len(engine.residents))
	}

	first := engine.residents["tee"]

	cfg.Layouts = cfg.Layouts[:1]
	engine.Update(cfg, layoutShell)

	if len(engine.residents) != 2 || engine.residents["tee"] != first {
		t.Fatalf(
			"residents = %v, want the same two, untouched by a reload that kept them",
			engine.residents,
		)
	}

	cfg.Layouts = nil
	engine.Update(cfg, layoutShell)

	if _, still := engine.residents["tee"]; still || len(engine.residents) != 1 {
		t.Fatalf("residents = %v, want only the default after its entry went", engine.residents)
	}
}

func TestEngine_Inputs_SkipADisplayNoLayoutIsNamedFor(t *testing.T) {
	t.Parallel()

	engine := New(twoDisplayDesktop{}, nil, nil)
	cfg := enabledWith("")
	cfg.Layouts = []config.LayoutTarget{{Display: 2, Layout: namedLayout("second")}}
	engine.Update(cfg, layoutShell)

	inputs, outs, err := engine.Preview(context.Background(), Event{Kind: EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(inputs) != 1 || inputs[0].Display.Index != 2 || len(outs) != 1 {
		t.Fatalf("Preview() = %d inputs, want only display 2's", len(inputs))
	}
}
