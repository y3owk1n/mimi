package tiling_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/tiling"
)

const (
	shell   = "/bin/sh"
	created = "window_created"
	// echoLayout prints one frame for window 1 and records the input's
	// event kind and previous state as its new state.
	echoLayout = `jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 100, height: 100}}], ` +
		`state: {kind: .event.kind, was: .state}}'`
)

// fakeDesktop is a desktop made of values; every apply lands input applied.
type fakeDesktop struct {
	mu       sync.Mutex
	space    int
	windows  action.WindowsInfo
	displays []action.DisplayEntry
	spaces   map[uint32]int
	applied  [][]action.WindowFrame
	// animations is the animation each Apply was asked for, nil for none.
	animations []*action.Animation
	applyErr   error
	focused    []uint32
}

func (d *fakeDesktop) Focus(number uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.focused = append(d.focused, number)

	return nil
}

func (d *fakeDesktop) Windows() (action.WindowsInfo, error)     { return d.windows, nil }
func (d *fakeDesktop) Displays() ([]action.DisplayEntry, error) { return d.displays, nil }
func (d *fakeDesktop) ActiveSpaces() (map[uint32]int, error) {
	if d.spaces != nil {
		return d.spaces, nil
	}

	spaces := map[uint32]int{}
	for _, display := range d.displays {
		spaces[display.ID] = d.space
	}

	return spaces, nil
}

func (d *fakeDesktop) Margins() (action.MarginsInfo, error) {
	return action.MarginsInfo{Enabled: true, Size: 8}, nil
}

func (d *fakeDesktop) Apply(frames []action.WindowFrame, animation *action.Animation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.applyErr != nil {
		return d.applyErr
	}

	d.applied = append(d.applied, frames)
	d.animations = append(d.animations, animation)

	return nil
}

func (d *fakeDesktop) appliedCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.applied)
}

func newDesktop() *fakeDesktop {
	return &fakeDesktop{
		space: 2,
		windows: action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
			{Number: 1, PID: 10, App: "A", Frame: action.Frame{Width: 500, Height: 500}},
		}},
		displays: []action.DisplayEntry{
			{Index: 1, ID: 7, Visible: action.Frame{Width: 1000, Height: 1000}},
		},
	}
}

func enabled(layout string) config.TilingConfig {
	return config.TilingConfig{Enabled: true, Layout: layout, DebounceMS: 10, TimeoutSecs: 5}
}

func TestEngine_Pass_RunsTheLayoutAndAppliesItsFrames(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v, want nil", err)
	}

	want := []action.WindowFrame{{Number: 1, Frame: action.Frame{Width: 100, Height: 100}}}
	if len(desktop.applied) != 1 || len(desktop.applied[0]) != 1 ||
		desktop.applied[0][0] != want[0] {
		t.Fatalf("applied = %+v, want %+v", desktop.applied, want)
	}
}

// TestEngine_Pass_HandsTheLayoutItsOwnStateBack pins the state contract: what
// the layout returned for a space is what it reads on the next pass for that
// space, and another space starts from null.
func TestEngine_Pass_HandsTheLayoutItsOwnStateBack(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx := context.Background()

	for _, kind := range []string{created, "window_focus"} {
		err := engine.Pass(ctx, tiling.Event{Kind: kind})
		if err != nil {
			t.Fatalf("Pass(%s) error = %v", kind, err)
		}
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if got, want := string(
		inputs[0].State,
	), `{"kind":"window_focus","was":{"kind":"`+created+`","was":null}}`; got != want {
		t.Fatalf("state after two passes = %s, want %s", got, want)
	}

	desktop.space = 3

	inputs, _, err = engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() on another space error = %v", err)
	}

	if string(inputs[0].State) != "null" {
		t.Fatalf("state on a fresh space = %s, want null", inputs[0].State)
	}
}

func TestEngine_Pass_DoesNothingWhileDisabled(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(config.TilingConfig{Enabled: false, Layout: echoLayout, TimeoutSecs: 5}, shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil || len(desktop.applied) != 0 {
		t.Fatalf(
			"Pass() while disabled = %v, applied %d, want nil and 0",
			err,
			len(desktop.applied),
		)
	}

	if engine.KindFilter()(events.WindowCreated) {
		t.Fatal("KindFilter admits events while disabled")
	}
}

// TestEngine_Preview_RunsWithoutApplying: the input a layout sees and the
// output it gives, and nothing moves.
func TestEngine_Preview_RunsWithoutApplying(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(config.TilingConfig{Enabled: false, Layout: echoLayout, TimeoutSecs: 5}, shell)

	inputs, outs, err := engine.Preview(
		context.Background(),
		tiling.Event{Kind: tiling.EventPreview},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(inputs) != 1 || inputs[0].Version != tiling.InputVersion || inputs[0].Space != 2 ||
		len(inputs[0].Windows) != 1 || len(inputs[0].Displays) != 1 || inputs[0].Display.ID != 7 ||
		inputs[0].Gap != 8 {
		t.Fatalf(
			"Preview() inputs = %+v, want one for display 7, version %d, space 2, one window",
			inputs,
			tiling.InputVersion,
		)
	}

	if len(outs) != 1 || len(outs[0].Frames) != 1 || len(desktop.applied) != 0 {
		t.Fatalf(
			"Preview() outputs = %+v applied = %d, want one frame and 0",
			outs,
			len(desktop.applied),
		)
	}
}

func TestEngine_Pass_LayoutFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		layout string
		cfg    func(config.TilingConfig) config.TilingConfig
		want   string
	}{
		{name: "non-zero exit", layout: `echo boom >&2; exit 3`, want: "layout failed: boom"},
		{name: "not json", layout: `echo left-half`, want: "decoding layout output"},
		{
			name:   "timeout",
			layout: `sleep 5`,
			cfg: func(c config.TilingConfig) config.TilingConfig {
				c.TimeoutSecs = 1

				return c
			},
			want: "layout timed out",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := enabled(testCase.layout)
			if testCase.cfg != nil {
				cfg = testCase.cfg(cfg)
			}

			desktop := newDesktop()
			engine := tiling.New(desktop, nil, nil)
			engine.Update(cfg, shell)

			err := engine.Pass(context.Background(), tiling.Event{Kind: created})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Pass() error = %v, want one containing %q", err, testCase.want)
			}

			if len(desktop.applied) != 0 {
				t.Fatal("a failed layout applied frames")
			}
		})
	}
}

// TestEngine_Pass_AnEmptyOutputChangesNothing: a layout that has nothing to
// say prints nothing, and that is neither an error nor an apply.
func TestEngine_Pass_AnEmptyOutputChangesNothing(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`cat >/dev/null`), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil || len(desktop.applied) != 0 {
		t.Fatalf("Pass() = %v, applied %d, want nil and 0", err, len(desktop.applied))
	}
}

func TestEngine_Pass_ReportsAnApplyFailure(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.applyErr = derrors.New(derrors.CodeActionFailed, "1 of 1 frames not applied")
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Fatalf("Pass() error = %v, want CodeActionFailed", err)
	}
}

// TestEngine_Run_SettlesABurstIntoOnePass pins the debounce: a flurry of
// events costs one layout run, reporting the last of them.
func TestEngine_Run_SettlesABurstIntoOnePass(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()

	var (
		passesMu sync.Mutex
		passes   []func() error
	)

	serialize := func(work func() error) error {
		passesMu.Lock()
		defer passesMu.Unlock()

		passes = append(passes, work)

		return work()
	}

	engine := tiling.New(desktop, serialize, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := make(events.Subscriber, 8)
	done := make(chan struct{})

	go func() {
		engine.Run(ctx, sub)
		close(done)
	}()

	// The engine starts enabled, so its startup pass comes first; let it
	// land before the burst so the two cannot merge into one.
	deadline := time.Now().Add(3 * time.Second)
	for desktop.appliedCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	for _, kind := range []events.EventKind{
		events.WindowCreated,
		events.WindowFocus,
		events.WindowClosed,
	} {
		sub <- events.Event{Kind: kind}
	}

	deadline = time.Now().Add(3 * time.Second)
	for desktop.appliedCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Give a further, unwanted pass the chance to show up.
	time.Sleep(100 * time.Millisecond)

	if got := desktop.appliedCount(); got != 2 {
		t.Fatalf("startup plus a burst of three events applied %d times, want 2", got)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	var state struct {
		Kind string `json:"kind"`
	}

	_ = json.Unmarshal(inputs[0].State, &state)

	if state.Kind != string(events.WindowClosed) {
		t.Fatalf(
			"the pass reported %q, want the last event of the burst %q",
			state.Kind,
			events.WindowClosed,
		)
	}

	passesMu.Lock()
	serialized := len(passes)
	passesMu.Unlock()

	if serialized == 0 {
		t.Fatal("desktop reads and writes bypassed the serializer")
	}

	cancel()
	<-done
}

// TestEngine_Run_PassesOnStartupAndOnEnablingReloads pins the passes the
// engine asks of itself: one as it starts enabled, one when a reload switches
// it on or names another layout, and none for a reload that changes neither.
func TestEngine_Run_PassesOnStartupAndOnEnablingReloads(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := make(events.Subscriber)
	done := make(chan struct{})

	go func() {
		engine.Run(ctx, sub)
		close(done)
	}()

	waitForApplied := func(want int, why string) {
		t.Helper()

		deadline := time.Now().Add(3 * time.Second)
		for desktop.appliedCount() < want && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)

		if got := desktop.appliedCount(); got != want {
			t.Fatalf("%s: applied %d times, want %d", why, got, want)
		}
	}

	waitForApplied(1, "startup")

	lastKind := func() string {
		inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
		if err != nil {
			t.Fatalf("Preview() error = %v", err)
		}

		var state struct {
			Kind string `json:"kind"`
		}

		_ = json.Unmarshal(inputs[0].State, &state)

		return state.Kind
	}

	if got := lastKind(); got != tiling.EventStartup {
		t.Fatalf("startup pass reported %q, want %q", got, tiling.EventStartup)
	}

	// The same config again, and a debounce change: neither is a reason.
	engine.Update(enabled(echoLayout), shell)

	same := enabled(echoLayout)
	same.DebounceMS = 20
	engine.Update(same, shell)
	waitForApplied(1, "reload that changes nothing the layout sees")

	// Another layout while enabled.
	engine.Update(enabled(echoLayout+" | jq -c ."), shell)
	waitForApplied(2, "reload naming another layout")

	if got := lastKind(); got != tiling.EventReload {
		t.Fatalf("reload pass reported %q, want %q", got, tiling.EventReload)
	}

	// Off, then on again.
	off := enabled(echoLayout)
	off.Enabled = false
	engine.Update(off, shell)
	waitForApplied(2, "reload switching tiling off")

	engine.Update(enabled(echoLayout), shell)
	waitForApplied(3, "reload switching tiling on")

	cancel()
	<-done
}

// TestEngine_Pass_RunsOncePerDisplayWithStateOfItsOwn pins the multi-display
// contract: a display gets a run of its own with only its windows, its
// state is keyed by its own space, and a space switched on one display
// leaves the other's state untouched.
func TestEngine_Pass_RunsOncePerDisplayWithStateOfItsOwn(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.displays = []action.DisplayEntry{
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
	}
	desktop.windows = action.WindowsInfo{Focused: 1, Windows: []action.WindowEntry{
		{
			Number: 1,
			PID:    10,
			App:    "A",
			Frame:  action.Frame{X: 100, Y: 100, Width: 500, Height: 500},
		},
		{
			Number: 2,
			PID:    11,
			App:    "B",
			Frame:  action.Frame{X: 1100, Y: 100, Width: 500, Height: 500},
		},
	}}
	desktop.spaces = map[uint32]int{7: 2, 8: 5}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		enabled(
			`jq -c '{frames: [], state: {display: .display.id, space: .space, n: (.windows|length), focused: .focused, was: .state}}'`,
		),
		shell,
	)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(inputs) != 2 {
		t.Fatalf("got %d inputs, want one per display", len(inputs))
	}

	want := []string{
		`{"display":7,"space":2,"n":1,"focused":-1,"was":null}`,
		`{"display":8,"space":5,"n":1,"focused":0,"was":null}`,
	}

	for index, input := range inputs {
		if len(input.Windows) != 1 || string(input.State) != want[index] {
			t.Fatalf("display %d input = %d windows, state %s; want one window and %s",
				input.Display.ID, len(input.Windows), input.State, want[index])
		}
	}

	// The second display switches space: its state starts over, the first
	// display's is exactly where it was.
	desktop.spaces = map[uint32]int{7: 2, 8: 6}

	inputs, _, err = engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if string(inputs[0].State) != want[0] || string(inputs[1].State) != "null" {
		t.Fatalf("after a switch on display 8: states %s and %s; want %s and null",
			inputs[0].State, inputs[1].State, want[0])
	}
}

// TestEngine_Input_GapFollowsTheConfigThenTheMargin pins where the gap comes
// from: tiling.gap when set, 0 included, else the macOS margin when it is
// on, else nothing.
func TestEngine_Input_GapFollowsTheConfigThenTheMargin(t *testing.T) {
	t.Parallel()

	zero, twelve := 0, 12

	cases := []struct {
		name string
		gap  *int
		want float64
	}{
		{name: "unset follows the margin", gap: nil, want: 8},
		{name: "set replaces it", gap: &twelve, want: 12},
		{name: "set to zero removes it", gap: &zero, want: 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			engine := tiling.New(newDesktop(), nil, nil)
			cfg := enabled(echoLayout)
			cfg.Gap = testCase.gap
			engine.Update(cfg, shell)

			inputs, _, err := engine.Preview(
				context.Background(),
				tiling.Event{Kind: tiling.EventPreview},
			)
			if err != nil {
				t.Fatalf("Preview() error = %v", err)
			}

			if inputs[0].Gap != testCase.want {
				t.Fatalf("gap = %v, want %v", inputs[0].Gap, testCase.want)
			}
		})
	}
}

// TestEngine_Pass_FocusesTheWindowTheLayoutAsksFor pins the one thing a
// layout may ask for beyond frames: keyboard focus on a window, applied
// after the frames, and alone when there are no frames.
func TestEngine_Pass_FocusesTheWindowTheLayoutAsksFor(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`jq -c '{frames: [], state: null, focus: 1}'`), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventCommand, Name: "focus"})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if len(desktop.applied) != 0 || len(desktop.focused) != 1 || desktop.focused[0] != 1 {
		t.Fatalf(
			"applied %d, focused %v; want nothing applied and window 1 focused",
			len(desktop.applied),
			desktop.focused,
		)
	}
}

// TestEngine_Pass_AnimatesTheFramesButNotADraggedWindow pins how the
// [tiling.animation] section reaches apply_frames: as the animation every
// pass asks for, with the window the user just dragged left out of it.
func TestEngine_Pass_AnimatesTheFramesButNotADraggedWindow(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	cfg := enabled(echoLayout)
	cfg.Animation = config.AnimationConfig{Enabled: true, DurationMS: 120, Easing: "ease-in-out"}
	engine.Update(cfg, shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v, want nil", err)
	}

	err = engine.Pass(context.Background(), tiling.Event{Kind: "window_move", Windows: []uint32{1}})
	if err != nil {
		t.Fatalf("Pass(window_move) error = %v, want nil", err)
	}

	want := action.Animation{DurationMS: 120, Easing: "ease-in-out"}
	for index, got := range desktop.animations {
		if got == nil || *got != want {
			t.Fatalf("animations[%d] = %+v, want %+v", index, got, want)
		}
	}

	if len(desktop.animations) != 2 {
		t.Fatalf("len(animations) = %d, want 2", len(desktop.animations))
	}

	if desktop.applied[0][0].Animate != nil {
		t.Fatal("a window nobody dragged was left out of the animation")
	}

	if dragged := desktop.applied[1][0].Animate; dragged == nil || *dragged {
		t.Fatal("the window the user dragged was animated")
	}

	cfg.Animation.Enabled = false
	engine.Update(cfg, shell)

	err = engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v, want nil", err)
	}

	if desktop.animations[2] != nil {
		t.Fatal("animation off in the config still animated")
	}

	// Off, a drag is not marked either: the frames go out untouched.
	err = engine.Pass(context.Background(), tiling.Event{Kind: "window_move", Windows: []uint32{1}})
	if err != nil {
		t.Fatalf("Pass(window_move) error = %v, want nil", err)
	}

	if desktop.applied[3][0].Animate != nil {
		t.Fatal("animation off in the config still marked the dragged window")
	}
}

// TestEngine_Command_DropsRepeatsWhileOneWaits pins the key-repeat rule: with
// a pass running and one command of the same name already waiting, further
// copies are dropped rather than queued, so a held key never scrolls on
// after it is released.
func TestEngine_Command_DropsRepeatsWhileOneWaits(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled("sleep 0.3; "+echoLayout), shell)

	scroll := tiling.Event{Kind: tiling.EventCommand, Name: "scroll", Args: []string{"right"}}

	var commands sync.WaitGroup

	commands.Go(func() {
		_ = engine.Command(context.Background(), scroll)
	})

	time.Sleep(100 * time.Millisecond)

	for range 4 {
		commands.Go(func() {
			_ = engine.Command(context.Background(), scroll)
		})
	}

	commands.Wait()

	if got := desktop.appliedCount(); got != 2 {
		t.Fatalf("passes applied = %d, want 2: the running one and one waiting", got)
	}
}

// TestEngine_Update_KeepsAResidentLayoutUnlessItChanges pins what a reload
// does to a resident layout: the same command keeps its process, a
// different one gets a fresh one.
func TestEngine_Update_KeepsAResidentLayoutUnlessItChanges(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()

	engine := tiling.New(desktop, nil, nil)
	defer engine.Close()

	cfg := enabled(countingLayout)
	cfg.LayoutMode = config.LayoutModeResident
	engine.Update(cfg, shell)

	pass := func(want int) {
		t.Helper()

		_, outs, err := engine.Preview(context.Background(), tiling.Event{Kind: created})
		if err != nil {
			t.Fatalf("Preview() error = %v, want nil", err)
		}

		if got := stateOf(t, outs[0]); got != want {
			t.Fatalf("state = %d, want %d", got, want)
		}
	}

	pass(1)
	pass(2)

	engine.Update(cfg, shell)
	pass(3)

	cfg.Layout = "true; " + countingLayout
	engine.Update(cfg, shell)
	pass(1)
}
