package action_test

import (
	"sync"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// steppingDesktop is a fake desktop that can step frames: it records every
// step per window, how the enhanced interface was switched, and signals
// each application's report as it lands.
type steppingDesktop struct {
	*fakeDesktop

	mu       sync.Mutex
	steps    map[action.WindowID][]geometry.Rect
	enhanced []bool
	reports  []action.StepReport
	landed   chan action.StepReport
}

func newSteppingDesktop() *steppingDesktop {
	return &steppingDesktop{
		fakeDesktop: desktopWithListedWindows(),
		steps:       map[action.WindowID][]geometry.Rect{},
		landed:      make(chan action.StepReport, 16),
	}
}

func (d *steppingDesktop) StepWindowFrame(id action.WindowID, frame geometry.Rect) error {
	d.mu.Lock()
	d.steps[id] = append(d.steps[id], frame)
	d.mu.Unlock()

	return d.SetWindowFrame(id, frame)
}

func (d *steppingDesktop) FinishSteps(report action.StepReport) {
	d.mu.Lock()
	d.reports = append(d.reports, report)
	d.mu.Unlock()

	d.landed <- report
}

func (d *steppingDesktop) SetEnhancedUI(_ int, enabled bool) (bool, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.enhanced = append(d.enhanced, enabled)

	return true, true
}

// waitLanded waits for count applications to report, failing the test past
// the deadline.
func (d *steppingDesktop) waitLanded(t *testing.T, count int) {
	t.Helper()

	deadline := time.After(2 * time.Second)

	for range count {
		select {
		case <-d.landed:
		case <-deadline:
			t.Fatal("the animation never landed")
		}
	}
}

func (d *steppingDesktop) stepsOf(id action.WindowID) []geometry.Rect {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]geometry.Rect(nil), d.steps[id]...)
}

func (d *steppingDesktop) frameOf(index int) geometry.Rect {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.windows[index].frame
}

func TestExecutor_ApplyFrames_StepsEveryWindowHomeInTheBackground(t *testing.T) {
	t.Parallel()

	desktop := newSteppingDesktop()
	desktop.windows = append(desktop.windows, fakeWindow{id: 3, pid: 101, number: 4244})
	still := false
	animation := action.Animation{DurationMS: 60, Easing: "ease-out"}
	targets := []action.WindowFrame{
		{Number: 4242, Frame: action.Frame{X: 300, Y: 25, Width: 640, Height: 1055}},
		{Number: 4243, Frame: action.Frame{X: 940, Y: 25, Width: 640, Height: 1055}},
		{Number: 4244, Frame: action.Frame{Width: 100, Height: 100}, Animate: &still},
	}

	started := time.Now()

	err := action.NewExecutor(desktop).
		ExecuteCommand(animatedApplyFramesCommand(&animation, targets...))
	if err != nil {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want nil", err)
	}

	if returned := time.Since(started); returned >= 60*time.Millisecond {
		t.Errorf("apply_frames returned after %s, want before the animation ends", returned)
	}

	if desktop.frameOf(2) != rectOf(targets[2].Frame) {
		t.Errorf(
			"the window left out of the animation is at %+v, want placed at once",
			desktop.frameOf(2),
		)
	}

	desktop.waitLanded(t, 2)

	for index, win := range desktop.windows {
		want := rectOf(targets[index].Frame)
		if got := desktop.frameOf(index); got != want {
			t.Errorf("window %d landed at %+v, want %+v", win.number, got, want)
		}
	}

	for _, windowID := range []action.WindowID{1, 2} {
		steps := desktop.stepsOf(windowID)
		if len(steps) < 2 {
			t.Errorf("window %d moved in %d steps, want several", windowID, len(steps))
		}

		finish := desktop.frameOf(int(windowID) - 1)
		for index := 1; index < len(steps); index++ {
			if remaining(steps[index], finish) > remaining(steps[index-1], finish) {
				t.Errorf(
					"window %d stepped backwards from %+v to %+v",
					windowID,
					steps[index-1],
					steps[index],
				)
			}
		}
	}

	if len(desktop.stepsOf(3)) != 0 {
		t.Errorf(
			"the window left out of the animation was stepped %d times",
			len(desktop.stepsOf(3)),
		)
	}

	// Two applications: each has its interface switched off, then back on.
	switchedOff, switchedOn := 0, 0
	for _, enabled := range desktop.enhanced {
		if enabled {
			switchedOn++
		} else {
			switchedOff++
		}
	}

	if switchedOff != 2 || switchedOn != 2 {
		t.Errorf(
			"enhanced interface switched off %d and on %d times, want 2 and 2",
			switchedOff,
			switchedOn,
		)
	}
}

func TestExecutor_ApplyFrames_SendsAWindowOnItsWayOnToItsNewFrame(t *testing.T) {
	t.Parallel()

	desktop := newSteppingDesktop()
	animation := action.Animation{DurationMS: 80, Easing: "linear"}
	executor := action.NewExecutor(desktop)

	first := action.WindowFrame{
		Number: 4242,
		Frame:  action.Frame{X: 500, Y: 25, Width: 960, Height: 1055},
	}
	second := action.WindowFrame{
		Number: 4242,
		Frame:  action.Frame{X: 100, Y: 25, Width: 960, Height: 1055},
	}

	err := executor.ExecuteCommand(animatedApplyFramesCommand(&animation, first))
	if err != nil {
		t.Fatalf("first apply_frames error = %v", err)
	}

	time.Sleep(25 * time.Millisecond)

	err = executor.ExecuteCommand(animatedApplyFramesCommand(&animation, second))
	if err != nil {
		t.Fatalf("second apply_frames error = %v", err)
	}

	// One application, one run: the second frame joined the first's worker.
	desktop.waitLanded(t, 1)

	if got := desktop.frameOf(0); got != rectOf(second.Frame) {
		t.Errorf("window landed at %+v, want the later frame %+v", got, rectOf(second.Frame))
	}

	steps := desktop.stepsOf(1)
	if len(steps) < 3 {
		t.Fatalf("window moved in %d steps, want several", len(steps))
	}

	// It set off towards the first frame, then turned back without ever
	// reaching it.
	turned := false
	for index := 1; index < len(steps); index++ {
		if steps[index].X < steps[index-1].X {
			turned = true
		}

		if steps[index].X >= 500 {
			t.Errorf("window reached the first frame at %+v before turning", steps[index])
		}
	}

	if !turned {
		t.Error("window never turned towards the later frame")
	}

	select {
	case <-desktop.landed:
		t.Error("the later frame ran as a second animation, want it to join the first")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestExecutor_ApplyFrames_ReportsTheWindowThatRefused(t *testing.T) {
	t.Parallel()

	desktop := newSteppingDesktop()
	desktop.windows[1].setFrameErr = derrors.New(derrors.CodeAccessibilityFailed, "refused")
	desktop.windows[1].frameErr = derrors.New(derrors.CodeAccessibilityFailed, "gone")
	animation := action.Animation{DurationMS: 20, Easing: "linear"}

	err := action.NewExecutor(desktop).ExecuteCommand(animatedApplyFramesCommand(
		&animation,
		action.WindowFrame{
			Number: 4242,
			Frame:  action.Frame{X: 10, Y: 25, Width: 960, Height: 1055},
		},
		action.WindowFrame{
			Number: 4243,
			Frame:  action.Frame{X: 970, Y: 25, Width: 960, Height: 1055},
		},
		action.WindowFrame{Number: 9999, Frame: action.Frame{Width: 100, Height: 100}},
	))
	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want CodeActionFailed", err)
	}

	desktop.waitLanded(t, 1)

	if got := desktop.frameOf(0); got.X != 10 {
		t.Errorf("the window that answered landed at %+v, want x 10", got)
	}
}

// remaining is how far a frame still has to go to finish, along every edge.
func remaining(frame, finish geometry.Rect) float64 {
	abs := func(value float64) float64 {
		if value < 0 {
			return -value
		}

		return value
	}

	return abs(
		finish.X-frame.X,
	) + abs(
		finish.Y-frame.Y,
	) + abs(
		finish.W-frame.W,
	) + abs(
		finish.H-frame.H,
	)
}

func rectOf(frame action.Frame) geometry.Rect {
	return geometry.Rect{X: frame.X, Y: frame.Y, W: frame.Width, H: frame.Height}
}
