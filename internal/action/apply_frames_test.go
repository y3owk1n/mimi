package action_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

func applyFramesCommandFor(t *testing.T, frames ...action.WindowFrame) action.Command {
	t.Helper()

	cmd, err := action.NewApplyFramesCommand(frames)
	if err != nil {
		t.Fatalf("NewApplyFramesCommand() error = %v, want nil", err)
	}

	return cmd
}

func TestExecutor_ApplyFrames_WritesEveryFrameByWindowNumber(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	cmd := applyFramesCommandFor(
		t,
		action.WindowFrame{
			Number: 4243,
			Frame:  action.Frame{X: 0, Y: 25, Width: 640, Height: 1055},
		},
		action.WindowFrame{
			Number: 4242,
			Frame:  action.Frame{X: 640, Y: 25, Width: 1280, Height: 1055},
		},
	)

	err := action.NewExecutor(desktop).ExecuteCommand(cmd)
	if err != nil {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want nil", err)
	}

	if got, want := desktop.windows[0].frame, (geometry.Rect{X: 640, Y: 25, W: 1280, H: 1055}); got != want {
		t.Fatalf("window 4242 frame = %v, want %v", got, want)
	}

	if got, want := desktop.windows[1].frame, (geometry.Rect{X: 0, Y: 25, W: 640, H: 1055}); got != want {
		t.Fatalf("window 4243 frame = %v, want %v", got, want)
	}
}

// TestExecutor_ApplyFrames_AppliesTheRestWhenOneFails pins the best-effort
// rule: a failure is reported, and named, but does not stop the frames after
// it from landing.
func TestExecutor_ApplyFrames_AppliesTheRestWhenOneFails(t *testing.T) {
	t.Parallel()

	desktop := desktopWithListedWindows()
	desktop.windows[0].setFrameErr = derrors.New(derrors.CodeAccessibilityFailed, "refused")
	cmd := applyFramesCommandFor(t,
		action.WindowFrame{Number: 4242, Frame: action.Frame{X: 0, Y: 0, Width: 100, Height: 100}},
		action.WindowFrame{Number: 9999, Frame: action.Frame{X: 0, Y: 0, Width: 100, Height: 100}},
		action.WindowFrame{Number: 4243, Frame: action.Frame{X: 0, Y: 0, Width: 100, Height: 100}},
	)

	err := action.NewExecutor(desktop).ExecuteCommand(cmd)
	if !derrors.IsCode(err, derrors.CodeActionFailed) {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want CodeActionFailed", err)
	}

	for _, fragment := range []string{"2 of 3", "window 4242: refused", "window 9999 is not on the active space"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not mention %q", err.Error(), fragment)
		}
	}

	if got, want := desktop.windows[1].frame, (geometry.Rect{X: 0, Y: 0, W: 100, H: 100}); got != want {
		t.Fatalf(
			"window 4243 frame = %v, want %v (the frame after a failure still lands)",
			got,
			want,
		)
	}
}

func TestExecutor_ApplyFrames_ErrorPaths(t *testing.T) {
	t.Parallel()

	failure := derrors.New(derrors.CodeAccessibilityDenied, "denied")

	cases := []struct {
		name    string
		breakIt func(*fakeDesktop)
		code    derrors.Code
	}{
		{
			name:    deniedCase,
			breakIt: func(d *fakeDesktop) { d.accessibilityErr = failure },
			code:    derrors.CodeAccessibilityDenied,
		},
		{
			name:    "windows cannot be enumerated",
			breakIt: func(d *fakeDesktop) { d.enumerateErr = failure },
			code:    derrors.CodeActionFailed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithListedWindows()
			testCase.breakIt(desktop)

			cmd := applyFramesCommandFor(t,
				action.WindowFrame{Number: 4242, Frame: action.Frame{Width: 100, Height: 100}},
			)

			err := action.NewExecutor(desktop).ExecuteCommand(cmd)
			if !derrors.IsCode(err, testCase.code) {
				t.Fatalf(
					"ExecuteCommand(apply_frames) error = %v, want code %s",
					err,
					testCase.code,
				)
			}
		})
	}
}

// TestApplyFramesCommand_RejectsAMalformedPayloadOnBothPaths pins the rule
// the constructor and the daemon-side dispatch share, in the same words.
func TestApplyFramesCommand_RejectsAMalformedPayloadOnBothPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		frames []action.WindowFrame
		want   string
	}{
		{name: "empty", frames: nil, want: "at least one frame"},
		{
			name:   "window number 0",
			frames: []action.WindowFrame{{Number: 0, Frame: action.Frame{Width: 1, Height: 1}}},
			want:   "frames[0]: window number is required",
		},
		{
			name: "duplicate window",
			frames: []action.WindowFrame{
				{Number: 7, Frame: action.Frame{Width: 1, Height: 1}},
				{Number: 7, Frame: action.Frame{Width: 1, Height: 1}},
			},
			want: "frames[1]: window 7 is listed more than once",
		},
		{
			name:   "zero height",
			frames: []action.WindowFrame{{Number: 7, Frame: action.Frame{Width: 1, Height: 0}}},
			want:   "frames[0]: window 7 needs a positive width and height",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, direct := action.NewApplyFramesCommand(testCase.frames)
			decoded := action.NewExecutor(desktopWithListedWindows()).ExecuteCommand(action.Command{
				Name:        action.NameApplyFrames,
				ApplyFrames: action.ApplyFramesArgs{Frames: testCase.frames},
			})

			for path, err := range map[string]error{"direct": direct, "daemon": decoded} {
				if !derrors.IsCode(err, derrors.CodeInvalidInput) {
					t.Fatalf("%s path: error = %v, want CodeInvalidInput", path, err)
				}

				if !strings.Contains(err.Error(), testCase.want) {
					t.Errorf(
						"%s path: error %q does not contain %q",
						path,
						err.Error(),
						testCase.want,
					)
				}
			}

			if direct.Error() != decoded.Error() {
				t.Errorf(
					"paths disagree:\n direct: %s\n daemon: %s",
					direct.Error(),
					decoded.Error(),
				)
			}
		})
	}
}

const linearEasing = "linear"

func animatedApplyFramesCommand(
	animation *action.Animation,
	frames ...action.WindowFrame,
) action.Command {
	return action.Command{
		Name:        action.NameApplyFrames,
		ApplyFrames: action.ApplyFramesArgs{Frames: frames, Animation: animation},
	}
}

func TestExecutor_ApplyFrames_RejectsAnAnimationItCannotRun(t *testing.T) {
	t.Parallel()

	frame := action.WindowFrame{Number: 4242, Frame: action.Frame{Width: 100, Height: 100}}

	cases := map[string]struct {
		animation action.Animation
		fragment  string
	}{
		"zero duration": {
			action.Animation{DurationMS: 0, Easing: linearEasing},
			"animation.durationMs",
		},
		"too long": {
			action.Animation{DurationMS: 5000, Easing: linearEasing},
			"animation.durationMs",
		},
		"unknown easing": {action.Animation{DurationMS: 100, Easing: "bouncy"}, "animation.easing"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			desktop := desktopWithListedWindows()
			cmd := animatedApplyFramesCommand(&testCase.animation, frame)

			err := action.NewExecutor(desktop).ExecuteCommand(cmd)
			if !derrors.IsCode(err, derrors.CodeInvalidInput) {
				t.Fatalf("error = %v, want CodeInvalidInput", err)
			}

			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Errorf("error %q does not mention %q", err.Error(), testCase.fragment)
			}

			if desktop.windows[0].setFrameWrites != 0 {
				t.Error("a rejected payload wrote a frame")
			}
		})
	}
}

// TestExecutor_ApplyFrames_WritesOneApplicationsFramesInPayloadOrder pins
// that the frames of one application land in the order the payload lists
// them, whatever the other applications' frames are doing at the time.
func TestExecutor_ApplyFrames_WritesOneApplicationsFramesInPayloadOrder(t *testing.T) {
	t.Parallel()

	desktop := desktopWithWindows(4, 1)
	for index := range desktop.windows {
		desktop.windows[index].pid = 100 + index%2
		desktop.windows[index].number = uint32(5000 + index)
	}

	frames := make([]action.WindowFrame, 0, 4)
	for _, number := range []uint32{5003, 5000, 5001, 5002} {
		frames = append(frames, action.WindowFrame{
			Number: number,
			Frame:  action.Frame{X: 0, Y: 0, Width: 100, Height: 100},
		})
	}

	err := action.NewExecutor(desktop).ExecuteCommand(applyFramesCommandFor(t, frames...))
	if err != nil {
		t.Fatalf("ExecuteCommand(apply_frames) error = %v, want nil", err)
	}

	var even, odd []action.WindowID

	for _, id := range desktop.frameWrites {
		if desktop.windows[id-1].pid == 100 {
			even = append(even, id)
		} else {
			odd = append(odd, id)
		}
	}

	if want := []action.WindowID{1, 3}; !slices.Equal(even, want) {
		t.Fatalf("pid 100 writes = %v, want %v", even, want)
	}

	if want := []action.WindowID{4, 2}; !slices.Equal(odd, want) {
		t.Fatalf("pid 101 writes = %v, want %v", odd, want)
	}
}
