package action

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// WindowFrame is one entry of apply_frames' payload: a window, by the number
// "mimi query windows" reported for it, and the frame to give it, in window
// coordinates. Animate, when the payload animates at all, excludes one
// window from the animation. It is placed at its frame as the others begin
// to move, for a window with no sensible starting position on screen.
type WindowFrame struct {
	Number  uint32 `json:"number"`
	Frame   Frame  `json:"frame"`
	Animate *bool  `json:"animate,omitempty"`
}

// Animation is how apply_frames moves the windows when it is asked to: over
// how long, in milliseconds, along which curve, named as in Easings, and by
// which driver, named as in Drivers; "" is the capture driver.
type Animation struct {
	DurationMS int    `json:"durationMs"`
	Easing     string `json:"easing"`
	Driver     string `json:"driver,omitempty"`
}

// Easings are the curves an Animation names, in the order native numbers
// them.
var Easings = []string{"linear", "ease-in", "ease-out", "ease-in-out"}

// MaxAnimationMS is the longest an animation may run. Past a second the
// windows are unusable for too long after every pass.
const MaxAnimationMS = 1000

// ApplyFramesArgs is apply_frames' typed payload: the frames to apply, in the
// order they are applied, and how to animate them, or nil to move them at
// once.
type ApplyFramesArgs struct {
	Frames    []WindowFrame `json:"frames"`
	Animation *Animation    `json:"animation,omitempty"`
}

// FrameStepper is what a Desktop offers when it can move windows a step at
// a time through their applications, which is how the accessibility driver
// animates. A desktop that cannot moves the frames at once.
type FrameStepper interface {
	// StepWindowFrame writes one step of a window's frame. Unlike
	// SetWindowFrame it leaves the desktop's listing alone: a step is one
	// of a hundred a second.
	StepWindowFrame(id WindowID, frame geometry.Rect) error
	// FinishSteps runs once the last window has landed, with what the
	// steps cost.
	FinishSteps(report StepReport)
	// SetEnhancedUI turns an application's enhanced accessibility interface
	// on or off, under which some applications animate every move they are
	// given. It reports whether the setting was on, and ok false when the
	// application has no such setting.
	SetEnhancedUI(pid int, enabled bool) (was bool, ok bool)
}

// StepReport is what a stepped animation cost: how many windows moved, how
// many frames were written over all of them, how long the whole took, and
// the slowest single write.
type StepReport struct {
	Windows int
	Frames  int
	Elapsed time.Duration
	Slowest time.Duration
}

// FrameAnimator is what a Desktop offers when it can animate frames. A
// desktop that cannot is used as it is, and the frames move at once.
type FrameAnimator interface {
	// BeginFrameAnimation prepares to move the given windows to their
	// frames over the animation, hiding the move the frame writes make, and
	// reports how many windows it will animate.
	BeginFrameAnimation(targets []WindowFrame, animation Animation) (int, error)
	// StartFrameAnimation runs the prepared animation, leaving out the
	// windows whose frames did not land.
	StartFrameAnimation(dropped []uint32)
}

// NewApplyFramesCommand builds apply_frames' command from the decoded
// payload, rejecting one the action would refuse.
func NewApplyFramesCommand(frames []WindowFrame) (Command, error) {
	args := ApplyFramesArgs{Frames: frames}

	err := validateApplyFramesArgs(args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameApplyFrames, ApplyFrames: args}, nil
}

// validateApplyFramesArgs is the one rule an apply_frames payload is held to,
// on both paths: at least one frame, every window named once, and every
// frame with a positive size. A window number of 0 is what a window without
// one reports, and names nothing.
func validateApplyFramesArgs(args ApplyFramesArgs) error {
	if len(args.Frames) == 0 {
		return derrors.New(derrors.CodeInvalidInput, "apply_frames needs at least one frame")
	}

	if args.Animation != nil {
		err := validateAnimation(*args.Animation)
		if err != nil {
			return err
		}
	}

	seen := make(map[uint32]struct{}, len(args.Frames))

	for index, entry := range args.Frames {
		if entry.Number == 0 {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"frames[%d]: window number is required",
				index,
			)
		}

		if _, dup := seen[entry.Number]; dup {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"frames[%d]: window %d is listed more than once",
				index,
				entry.Number,
			)
		}

		seen[entry.Number] = struct{}{}

		if entry.Frame.Width <= 0 || entry.Frame.Height <= 0 {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"frames[%d]: window %d needs a positive width and height",
				index,
				entry.Number,
			)
		}
	}

	return nil
}

// validateAnimation holds an Animation to a duration that is positive and
// bounded, and a curve that has a name.
func validateAnimation(animation Animation) error {
	if animation.DurationMS <= 0 || animation.DurationMS > MaxAnimationMS {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"animation.durationMs must be between 1 and %d",
			MaxAnimationMS,
		)
	}

	if !slices.Contains(Easings, animation.Easing) {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"animation.easing must be one of %s",
			strings.Join(Easings, ", "),
		)
	}

	if animation.Driver != "" && !slices.Contains(Drivers, animation.Driver) {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"animation.driver must be one of %s",
			strings.Join(Drivers, ", "),
		)
	}

	return nil
}

// FocusWindowNumber gives keyboard focus to the window with the given
// number on the active space, the way a layout asks for it after moving
// focus along its own structure.
func (e *Executor) FocusWindowNumber(number uint32) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	windows, err := e.windowsNamed([]uint32{number})
	if err != nil {
		return err
	}

	for _, win := range windows {
		if win.Number == number {
			return e.desktop.ActivateWindow(win.ID)
		}
	}

	return derrors.Newf(derrors.CodeActionFailed, "window %d is not on the active space", number)
}

// FocusWindowNumber focuses a window by number on the desktop mimi is
// running on.
func FocusWindowNumber(number uint32) error {
	return defaultExecutor.FocusWindowNumber(number)
}

// ApplyFrames moves and resizes several windows on the active space in one
// action. Every frame is attempted, in order, whatever happened to the ones
// before it: a layout is more useful mostly applied than abandoned at its
// first failure. The action fails when any frame did not land, naming each
// window it could not place and why.
func (e *Executor) ApplyFrames(args ApplyFramesArgs) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	numbers := make([]uint32, 0, len(args.Frames))
	for _, entry := range args.Frames {
		numbers = append(numbers, entry.Number)
	}

	windows, err := e.windowsNamed(numbers)
	if err != nil {
		return err
	}

	byNumber := make(map[uint32]WindowID, len(windows))
	for _, win := range windows {
		byNumber[win.Number] = win.ID
	}

	// The accessibility driver writes the frames itself, a step at a time,
	// and returns when the windows have landed.
	if args.Animation != nil && args.Animation.Driver == DriverAccessibility {
		if stepper, ok := e.desktop.(FrameStepper); ok {
			failures, _ := e.stepFrames(stepper, args.Frames, windows, *args.Animation)

			return framesError(failures, len(args.Frames))
		}
	}

	// The animation is prepared before the first write and started after
	// the last, so the writes happen while the animation hides them. A
	// desktop that cannot animate, or an animation that cannot begin, moves
	// the frames at once. Without an animation in the payload none of this
	// runs.
	var (
		animator  FrameAnimator
		animating bool
	)

	if args.Animation != nil {
		animator, animating = e.desktop.(FrameAnimator)
	}

	if animating {
		targets := make([]WindowFrame, 0, len(args.Frames))

		for _, entry := range args.Frames {
			if _, ok := byNumber[entry.Number]; ok && (entry.Animate == nil || *entry.Animate) {
				targets = append(targets, entry)
			}
		}

		count, beginErr := animator.BeginFrameAnimation(targets, *args.Animation)
		animating = beginErr == nil && count > 0
	}

	failures, dropped := e.writeFrames(args.Frames, windows)

	if animating {
		animator.StartFrameAnimation(dropped)
	}

	return framesError(failures, len(args.Frames))
}

// framesError is apply_frames' error for the frames that did not land, nil
// when every one did.
func framesError(failures []string, total int) error {
	if len(failures) == 0 {
		return nil
	}

	return derrors.Newf(
		derrors.CodeActionFailed,
		"%d of %d frames not applied: %s",
		len(failures),
		total,
		strings.Join(failures, "; "),
	)
}

// windowsNamed is the focusable windows, from the desktop's recent
// enumeration when it has every number asked for, else enumerated afresh.
func (e *Executor) windowsNamed(numbers []uint32) ([]Window, error) {
	known := e.desktop.KnownWindows()
	if len(known) > 0 {
		seen := make(map[uint32]bool, len(known))
		for _, win := range known {
			seen[win.Number] = true
		}

		complete := true
		for _, number := range numbers {
			if !seen[number] {
				complete = false

				break
			}
		}

		if complete {
			return known, nil
		}
	}

	windows, _, err := e.desktop.FocusableWindows()
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get focusable windows")
	}

	return windows, nil
}

// writeFrames writes every frame, one goroutine per owning application. A
// write is a synchronous round trip into the application, some ten
// milliseconds for a heavy one, and writes to different applications do not
// wait on each other, so a layout takes as long as its slowest application
// rather than the sum. Writes to one application stay in payload order, on
// one goroutine, since an application may apply concurrent requests out of
// order.
// The failures come back in payload order, and the dropped windows are the
// ones whose frame did not land.
func (e *Executor) writeFrames(frames []WindowFrame, windows []Window) ([]string, []uint32) {
	byNumber := make(map[uint32]Window, len(windows))
	for _, win := range windows {
		byNumber[win.Number] = win
	}

	// messages holds each frame's failure in payload order, empty for one
	// that landed. errs is what the writers report, one slot each, so they
	// share nothing.
	messages := make([]string, len(frames))
	errs := make([]error, len(frames))
	groups := make(map[int][]int)

	for index, entry := range frames {
		win, ok := byNumber[entry.Number]
		if !ok {
			messages[index] = fmt.Sprintf("window %d is not on the active space", entry.Number)

			continue
		}

		groups[win.PID] = append(groups[win.PID], index)
	}

	var writers sync.WaitGroup

	for _, indexes := range groups {
		writers.Add(1)

		go func(indexes []int) {
			defer writers.Done()

			for _, index := range indexes {
				entry := frames[index]
				errs[index] = e.desktop.SetWindowFrame(
					byNumber[entry.Number].ID,
					rectOfFrame(entry.Frame),
				)
			}
		}(indexes)
	}

	writers.Wait()

	return collectFailures(frames, messages, errs)
}

// collectFailures turns the per-frame messages and errors of a write into
// the failures to report, in payload order, and the windows whose frames did
// not land.
func collectFailures(frames []WindowFrame, messages []string, errs []error) ([]string, []uint32) {
	var (
		failures []string
		dropped  []uint32
	)

	for index, entry := range frames {
		if errs[index] != nil {
			dropped = append(dropped, entry.Number)
			messages[index] = fmt.Sprintf(
				"window %d: %s",
				entry.Number,
				derrors.Message(errs[index]),
			)
		}

		if messages[index] != "" {
			failures = append(failures, messages[index])
		}
	}

	return failures, dropped
}
