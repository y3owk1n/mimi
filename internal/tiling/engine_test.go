package tiling_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	// noState is what a layout is handed for a space it has not run for.
	noState = "null"
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
	// spaceIDs is which space is in front per display, when a test names
	// one; otherwise an id is derived from the space's index, so distinct
	// spaces stay distinct without every test naming ids.
	spaceIDs map[uint32]uint64
	// fullScreen is the displays FullScreenDisplays reports.
	fullScreen map[uint32]bool
	// late is a window Windows lists only after lateAfter reads, the way
	// the window server lists a window a little after Accessibility does.
	late        action.WindowEntry
	lateAfter   int
	windowReads int
	applied     [][]action.WindowFrame
	// animations is the animation each Apply was asked for, nil for none.
	animations []*action.Animation
	applyErr   error
	focused    []uint32
	// calls is the order of Apply and Focus, by name.
	calls []string
	// onApply, when set, runs inside each Apply.
	onApply func()
}

func (d *fakeDesktop) Focus(number uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.focused = append(d.focused, number)
	d.calls = append(d.calls, "focus")

	return nil
}

func (d *fakeDesktop) Windows() (action.WindowsInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.windowReads++
	if d.late.Number != 0 && d.windowReads > d.lateAfter {
		windows := d.windows
		windows.Windows = append(slices.Clone(windows.Windows), d.late)

		return windows, nil
	}

	return d.windows, nil
}
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

func (d *fakeDesktop) ActiveSpaceIDs() (map[uint32]uint64, error) {
	if d.spaceIDs != nil {
		return d.spaceIDs, nil
	}

	ids := map[uint32]uint64{}
	for _, display := range d.displays {
		index := d.space
		if d.spaces != nil {
			index = d.spaces[display.ID]
		}

		ids[display.ID] = fakeSpaceID(index)
	}

	return ids, nil
}

// fakeSpaceID is the identifier the fake gives the space at a Mission Control
// index, when a test has not named one itself. It is offset so an id is never
// mistaken for an index in a failure message.
func fakeSpaceID(index int) uint64 { return uint64(index) + 1000 }

func (d *fakeDesktop) FullScreenDisplays() (map[uint32]bool, error) { return d.fullScreen, nil }

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
	d.calls = append(d.calls, "apply")

	if d.onApply != nil {
		d.onApply()
	}

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

// easing is the animation curve the tests name.
const easing = "ease-in-out"

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

	if string(inputs[0].State) != noState {
		t.Fatalf("state on a fresh space = %s, want null", inputs[0].State)
	}
}

// TestEngine_Pass_KeepsStateWithTheSpaceNotItsPlaceInMissionControl pins that
// the state a layout built belongs to the space it was built for. Adding or
// removing a space changes the number of every space after it, so that number
// does not identify a space. The state follows its space through such a
// change, and another space at the same number still starts from null.
func TestEngine_Pass_KeepsStateWithTheSpaceNotItsPlaceInMissionControl(t *testing.T) {
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

	want := `{"kind":"` + created + `","was":null}`

	// A space added before this one renumbers it from 2 to 5. It is the
	// same space, so it is the same state.
	desktop.space = 5

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() after a renumbering error = %v", err)
	}

	if got := string(inputs[0].State); got != want {
		t.Fatalf("state after a renumbering = %s, want %s", got, want)
	}

	// Another space that happens to sit where the first one did is still
	// another space.
	desktop.space = 2
	desktop.spaceIDs = map[uint32]uint64{7: 6000}

	inputs, _, err = engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() on another space error = %v", err)
	}

	if got := string(inputs[0].State); got != noState {
		t.Fatalf("state on another space at the same index = %s, want null", got)
	}
}

// TestEngine_Pass_KeepsNoStateForASpaceItCannotName pins what happens when the
// window server will not say which space a display shows. The layout still
// runs, and the engine keeps nothing for it, so the next space it cannot name
// is not handed the state the last one built.
func TestEngine_Pass_KeepsNoStateForASpaceItCannotName(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	// An empty map is a display whose space did not resolve.
	desktop.spaceIDs = map[uint32]uint64{}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(echoLayout), shell)

	ctx := context.Background()

	err := engine.Pass(ctx, tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if got := string(inputs[0].State); got != noState {
		t.Fatalf("state for an unnamable space = %s, want null", got)
	}

	// Another space the window server will not name either. Without an
	// identity to tell the two apart, neither may inherit the other's.
	desktop.space = 9

	inputs, _, err = engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() on a second unnamable space error = %v", err)
	}

	if got := string(inputs[0].State); got != noState {
		t.Fatalf("state leaked to a second unnamable space = %s, want null", got)
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

// TestEngine_Pass_LeavesAFullScreenDisplayAlone pins that a display showing
// a full-screen space gets no run: macOS lays that window out itself.
func TestEngine_Pass_LeavesAFullScreenDisplayAlone(t *testing.T) {
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
	desktop.windows = action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{Number: 1, PID: 10, App: "A", Frame: action.Frame{Width: 1000, Height: 1000}},
		{
			Number: 2,
			PID:    11,
			App:    "B",
			Frame:  action.Frame{X: 1100, Y: 100, Width: 500, Height: 500},
		},
	}}
	desktop.fullScreen = map[uint32]bool{7: true}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`jq -c '{frames: [], state: null}'`), shell)

	inputs, _, err := engine.Preview(context.Background(), tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(inputs) != 1 || inputs[0].Display.ID != 8 || len(inputs[0].Windows) != 1 {
		t.Fatalf("inputs = %+v; want display 8 alone with its one window", inputs)
	}
}

// TestEngine_Pass_WaitsForACreatedWindowToBeListed pins that a window_created
// pass does not lay out until the window server lists the new window, which
// it does a little after Accessibility reports it.
func TestEngine_Pass_WaitsForACreatedWindowToBeListed(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	desktop.late = action.WindowEntry{
		Number: 2,
		PID:    20,
		App:    "B",
		Frame:  action.Frame{X: 500, Width: 400, Height: 400},
	}
	desktop.lateAfter = 3

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`jq -c '{frames: [.windows[] | {number, frame}], state: null}'`), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventStartup})
	if err != nil {
		t.Fatalf("startup Pass() error = %v", err)
	}

	err = engine.Pass(
		context.Background(),
		tiling.Event{Kind: string(events.WindowCreated), PID: 20},
	)
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	desktop.mu.Lock()
	defer desktop.mu.Unlock()

	if len(desktop.applied) != 2 || len(desktop.applied[1]) != 2 {
		t.Fatalf("applied = %v; want the second pass to place both windows", desktop.applied)
	}
}

// TestEngine_KindFilter_WakesOnMinimize pins that minimizing a window, and
// restoring it, each run a pass: the window leaves or rejoins the layout.
func TestEngine_KindFilter_WakesOnMinimize(t *testing.T) {
	t.Parallel()

	engine := tiling.New(newDesktop(), nil, nil)
	engine.Update(enabled(`jq -c '{frames: [], state: null}'`), shell)

	filter := engine.KindFilter()
	if !filter(events.WindowMinimize) || !filter(events.WindowUnminimize) {
		t.Fatal(
			"KindFilter() refuses window_minimize or window_unminimize; want both to wake a pass",
		)
	}
}

// TestEngine_Pass_KeepsAnOffScreenWindowWithItsNearestDisplay pins where a
// window whose center is on no display is laid out: with the display it is
// nearest, as a column a strip parks off a secondary display's edge is,
// rather than with the first display.
func TestEngine_Pass_KeepsAnOffScreenWindowWithItsNearestDisplay(t *testing.T) {
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
			Frame:   action.Frame{X: 200, Y: 1000, Width: 1000, Height: 1000},
			Visible: action.Frame{X: 200, Y: 1000, Width: 1000, Height: 1000},
		},
	}
	desktop.windows = action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
		{
			Number: 1,
			PID:    10,
			App:    "A",
			Frame:  action.Frame{X: 300, Y: 1100, Width: 500, Height: 500},
		},
		// Parked left of the second display: its center is on no display.
		{
			Number: 2,
			PID:    11,
			App:    "B",
			Frame:  action.Frame{X: -400, Y: 1100, Width: 500, Height: 500},
		},
	}}

	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`jq -c '{frames: [], state: null}'`), shell)

	inputs, _, err := engine.Preview(context.Background(), tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(inputs) != 1 || inputs[0].Display.ID != 8 || len(inputs[0].Windows) != 2 {
		t.Fatalf("inputs = %+v; want both windows on display 8 alone", inputs)
	}
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

	if string(inputs[0].State) != want[0] || string(inputs[1].State) != noState {
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
// layout may ask for beyond frames: keyboard focus on a window, given
// before the frames, and alone when there are no frames.
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

// TestEngine_Pass_FocusesBeforeTheFramesMove pins that the focus a layout
// asks for lands before its frames do: the focus is what the user asked
// for, and the frames may wait on a screen capture.
func TestEngine_Pass_FocusesBeforeTheFramesMove(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		enabled(
			`jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 10, height: 10}}], state: null, focus: 1}'`,
		),
		shell,
	)

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventCommand, Name: "focus"})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if got, want := strings.Join(desktop.calls, ","), "focus,apply"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
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
	cfg.Animation = config.AnimationConfig{Enabled: true, DurationMS: 120, Easing: easing}
	engine.Update(cfg, shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: created})
	if err != nil {
		t.Fatalf("Pass() error = %v, want nil", err)
	}

	err = engine.Pass(context.Background(), tiling.Event{Kind: "window_move", Windows: []uint32{1}})
	if err != nil {
		t.Fatalf("Pass(window_move) error = %v, want nil", err)
	}

	want := action.Animation{DurationMS: 120, Easing: easing}
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

// TestEngine_Pass_RunsTheCommandsTheLayoutAsksForAfterApplying pins the
// after key: each line runs through the shell once the frames are applied,
// and a pass that returns only commands still runs them.
func TestEngine_Pass_RunsTheCommandsTheLayoutAsksForAfterApplying(t *testing.T) {
	t.Parallel()

	mark := filepath.Join(t.TempDir(), "ran")
	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(enabled(`jq -c '{frames: [], state: null, after: ["touch `+mark+`"]}'`), shell)

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	_, statErr := os.Stat(mark)
	if statErr != nil {
		t.Fatalf("after command never ran: %v", statErr)
	}
}

// TestEngine_Pass_RunsTheCommandsAfterTheAnimation pins that with
// [tiling.animation] on, the after lines wait for the animation to end, so a
// command that reads a window's frame sees where it stopped.
func TestEngine_Pass_RunsTheCommandsAfterTheAnimation(t *testing.T) {
	t.Parallel()

	mark := filepath.Join(t.TempDir(), "ran")
	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	cfg := enabled(
		`jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 10, height: 10}}], state: null, after: ["touch ` + mark + `"]}'`,
	)
	cfg.Animation = config.AnimationConfig{Enabled: true, DurationMS: 300, Easing: easing}
	engine.Update(cfg, shell)

	start := time.Now()

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	_, statErr := os.Stat(mark)
	if statErr == nil {
		t.Fatal("after command ran before the animation ended")
	}

	engine.Wait()

	_, statErr = os.Stat(mark)
	if statErr != nil {
		t.Fatalf("after command never ran: %v", statErr)
	}

	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("after command ran after %s, want the 300ms animation first", elapsed)
	}
}

// TestEngine_Pass_RunsTheBeforeCommandsAndWaitsForThem pins the before
// key: each line runs and finishes before the frames are applied.
func TestEngine_Pass_RunsTheBeforeCommandsAndWaitsForThem(t *testing.T) {
	t.Parallel()

	mark := filepath.Join(t.TempDir(), "ran")
	desktop := newDesktop()
	desktop.onApply = func() {
		_, statErr := os.Stat(mark)
		if statErr != nil {
			t.Errorf("frames applied before the before command finished: %v", statErr)
		}
	}
	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		enabled(
			`jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 10, height: 10}}], state: null, before: ["sleep 0.1; touch `+mark+`"]}'`,
		),
		shell,
	)

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if len(desktop.applied) != 1 {
		t.Fatalf("applied %d passes, want 1", len(desktop.applied))
	}
}

// commandTimeout is the before and after bound the tests run with.
func commandTimeout(cfg config.TilingConfig, secs int) config.TilingConfig {
	cfg.CommandTimeoutSecs = secs

	return cfg
}

// TestEngine_Pass_RunsTheBeforeCommandsAtOnce pins that the before lines
// start together. Two that each take 200ms hold the pass for one of them,
// not both.
func TestEngine_Pass_RunsTheBeforeCommandsAtOnce(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		enabled(`jq -c '{frames: [], state: null, before: ["sleep 0.2", "sleep 0.2"]}'`),
		shell,
	)

	start := time.Now()

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if elapsed := time.Since(start); elapsed >= 400*time.Millisecond {
		t.Fatalf("pass took %s, want the two before lines overlapped", elapsed)
	}
}

// TestEngine_Pass_KillsABeforeCommandPastTheCommandTimeout pins that a
// before line runs under tiling.command_timeout_secs, not the layout's
// timeout, and that the frames still apply once it is killed.
func TestEngine_Pass_KillsABeforeCommandPastTheCommandTimeout(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		commandTimeout(
			enabled(
				`jq -c '{frames: [{number: 1, frame: {x: 0, y: 0, width: 10, height: 10}}], state: null, before: ["sleep 5"]}'`,
			),
			1,
		),
		shell,
	)

	start := time.Now()

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if elapsed := time.Since(start); elapsed >= 3*time.Second {
		t.Fatalf("pass took %s, want the before line killed after 1s", elapsed)
	}

	if desktop.appliedCount() != 1 {
		t.Fatalf("applied %d passes, want 1", desktop.appliedCount())
	}
}

// TestEngine_Wait_ReturnsWhenAnAfterCommandHangs pins that the timeout
// covers an after line too, so a one-shot engine's Wait cannot hang on one.
func TestEngine_Wait_ReturnsWhenAnAfterCommandHangs(t *testing.T) {
	t.Parallel()

	desktop := newDesktop()
	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		commandTimeout(enabled(`jq -c '{frames: [], state: null, after: ["sleep 5"]}'`), 1),
		shell,
	)

	start := time.Now()

	err := engine.Pass(context.Background(), tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	engine.Wait()

	if elapsed := time.Since(start); elapsed >= 3*time.Second {
		t.Fatalf("Wait() returned after %s, want the after line killed after 1s", elapsed)
	}
}

// TestEngine_Pass_RunsTheLayoutOnEveryDisplayAtOnce pins that a one-shot
// layout runs for both displays together, and that the engine still keeps
// its outputs by display, so each display's state names its own id.
func TestEngine_Pass_RunsTheLayoutOnEveryDisplayAtOnce(t *testing.T) {
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
	desktop.windows = action.WindowsInfo{Focused: 0, Windows: []action.WindowEntry{
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

	engine := tiling.New(desktop, nil, nil)
	engine.Update(
		enabled(`sleep 0.2; jq -c '{frames: [], state: {display: .display.id}}'`),
		shell,
	)

	ctx := context.Background()
	start := time.Now()

	err := engine.Pass(ctx, tiling.Event{Kind: tiling.EventRelayout})
	if err != nil {
		t.Fatalf("Pass() error = %v", err)
	}

	if elapsed := time.Since(start); elapsed >= 400*time.Millisecond {
		t.Fatalf("pass took %s, want the two layout runs overlapped", elapsed)
	}

	inputs, _, err := engine.Preview(ctx, tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	for _, input := range inputs {
		want := `{"display":` + strconv.Itoa(int(input.Display.ID)) + `}`
		if got := string(input.State); got != want {
			t.Fatalf("display %d state = %s, want %s", input.Display.ID, got, want)
		}
	}
}
