package action_test

import (
	"sync"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// steppingDesktop is a fake desktop that can step frames: it records every
// step per window and how the enhanced interface was switched.
type steppingDesktop struct {
	*fakeDesktop

	mu       sync.Mutex
	steps    map[action.WindowID][]geometry.Rect
	enhanced []bool
	report   action.StepReport
	finished int
}

func newSteppingDesktop() *steppingDesktop {
	return &steppingDesktop{
		fakeDesktop: desktopWithListedWindows(),
		steps:       map[action.WindowID][]geometry.Rect{},
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
	defer d.mu.Unlock()

	d.report = report
	d.finished++
}

func (d *steppingDesktop) SetEnhancedUI(_ int, enabled bool) (bool, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.enhanced = append(d.enhanced, enabled)

	return true, true
}

func TestExecutor_ApplyFrames_AccessibilityDriverStepsEveryWindowHome(t *testing.T) {
	t.Parallel()

	desktop := newSteppingDesktop()
	desktop.windows = append(desktop.windows, fakeWindow{id: 3, pid: 101, number: 4244})
	still := false
	animation := action.Animation{
		DurationMS: 40,
		Easing:     "ease-out",
		Driver:     action.DriverAccessibility,
	}
	targets := []action.WindowFrame{
		{Number: 4242, Frame: action.Frame{X: 300, Y: 25, Width: 640, Height: 1055}},
		{Number: 4243, Frame: action.Frame{X: 940, Y: 25, Width: 640, Height: 1055}},
		{Number: 4244, Frame: action.Frame{Width: 100, Height: 100}, Animate: &still},
	}

	err := action.NewExecutor(desktop).
		ExecuteCommand(animatedApplyFramesCommand(&animation, targets...))
	if err != nil {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want nil", err)
	}

	for index, win := range desktop.windows {
		want := rectOf(targets[index].Frame)
		if win.frame != want {
			t.Errorf("window %d landed at %+v, want %+v", win.number, win.frame, want)
		}
	}

	if len(desktop.steps[1]) < 2 || len(desktop.steps[2]) < 2 {
		t.Errorf(
			"windows moved in %d and %d steps, want several each",
			len(desktop.steps[1]),
			len(desktop.steps[2]),
		)
	}

	if len(desktop.steps[3]) != 0 {
		t.Errorf("the window left out of the animation was stepped %d times", len(desktop.steps[3]))
	}

	for windowID, steps := range desktop.steps {
		finish := desktop.windows[windowID-1].frame
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

	if desktop.finished != 1 || desktop.report.Windows != 2 || desktop.report.Frames < 4 {
		t.Errorf(
			"finished %d times with report %+v, want once for 2 windows and several frames",
			desktop.finished,
			desktop.report,
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

func TestExecutor_ApplyFrames_AccessibilityDriverReportsTheWindowThatRefused(t *testing.T) {
	t.Parallel()

	desktop := newSteppingDesktop()
	desktop.windows[1].setFrameErr = derrors.New(derrors.CodeAccessibilityFailed, "refused")
	animation := action.Animation{
		DurationMS: 20,
		Easing:     "linear",
		Driver:     action.DriverAccessibility,
	}

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
	))
	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Fatalf(
			"ExecuteCommand(apply_frames) error = %v, want CodeActionFailed for the refused frame",
			err,
		)
	}

	if desktop.windows[0].frame.X != 10 {
		t.Errorf("the window that answered landed at %+v, want x 10", desktop.windows[0].frame)
	}
}

func TestApplyFrames_RejectsAnUnknownDriver(t *testing.T) {
	t.Parallel()

	animation := action.Animation{DurationMS: 20, Easing: "linear", Driver: "telepathy"}

	err := action.NewExecutor(desktopWithListedWindows()).ExecuteCommand(animatedApplyFramesCommand(
		&animation,
		action.WindowFrame{
			Number: 4242,
			Frame:  action.Frame{X: 10, Y: 25, Width: 960, Height: 1055},
		},
	))
	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want CodeInvalidInput", err)
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
