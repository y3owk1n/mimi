package action

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/y3owk1n/mimi/internal/geometry"
)

// DriverCapture and DriverAccessibility name how an Animation moves the
// windows. The capture driver takes a picture of the screen and slides
// pictures of the windows, which needs Screen Recording. The accessibility
// driver moves the real windows through their applications, a step at a
// time, and needs nothing beyond Accessibility.
const (
	DriverCapture       = "capture"
	DriverAccessibility = "accessibility"
)

// Drivers are the animation drivers, in the order the config documents them.
var Drivers = []string{DriverCapture, DriverAccessibility}

// stepInterval is how often the accessibility driver writes a frame: a
// hundred a second, as the window managers that animate this way do.
const stepInterval = 10 * time.Millisecond

// stepFrames moves the windows to their frames through their applications,
// a step every stepInterval along the animation's curve, every application
// on a thread of its own, and reports the failures the way writeFrames does.
// It returns once the last window has landed. A frame the payload leaves
// out of the animation is written at once, before the rest start.
func (e *Executor) stepFrames(
	stepper FrameStepper,
	frames []WindowFrame,
	windows []Window,
	animation Animation,
) ([]string, []uint32) {
	byNumber := make(map[uint32]Window, len(windows))
	for _, win := range windows {
		byNumber[win.Number] = win
	}

	messages := make([]string, len(frames))
	errs := make([]error, len(frames))
	groups := make(map[int][]int)

	for index, entry := range frames {
		win, ok := byNumber[entry.Number]
		if !ok {
			messages[index] = fmt.Sprintf("window %d is not on the active space", entry.Number)

			continue
		}

		if entry.Animate != nil && !*entry.Animate {
			errs[index] = e.desktop.SetWindowFrame(win.ID, rectOfFrame(entry.Frame))

			continue
		}

		groups[win.PID] = append(groups[win.PID], index)
	}

	var (
		steppers sync.WaitGroup
		reportMu sync.Mutex
		report   StepReport
	)

	duration := time.Duration(animation.DurationMS) * time.Millisecond
	began := time.Now()

	for pid, indexes := range groups {
		steppers.Add(1)

		go func(pid int, indexes []int) {
			defer steppers.Done()

			written, slowest := e.stepApplication(
				stepper,
				pid,
				indexes,
				frames,
				byNumber,
				errs,
				duration,
				animation.Easing,
			)

			reportMu.Lock()
			defer reportMu.Unlock()

			report.Windows += len(indexes)
			report.Frames += written
			report.Slowest = max(report.Slowest, slowest)
		}(pid, indexes)
	}

	steppers.Wait()

	report.Elapsed = time.Since(began)
	stepper.FinishSteps(report)

	return collectFailures(frames, messages, errs)
}

// stepping is one window on its way: where it started, where it goes, and
// the last frame written, so a step that lands where the last did is not
// written again.
type stepping struct {
	index  int
	id     WindowID
	start  geometry.Rect
	finish geometry.Rect
	last   geometry.Rect
}

// stepApplication steps one application's windows, together, since an
// application answers its accessibility calls one at a time. It reports how
// many frames it wrote and the slowest write.
func (e *Executor) stepApplication(
	stepper FrameStepper,
	pid int,
	indexes []int,
	frames []WindowFrame,
	byNumber map[uint32]Window,
	errs []error,
	duration time.Duration,
	easing string,
) (int, time.Duration) {
	// Under the enhanced interface some applications animate every move
	// they are given, which fights the steps.
	if was, ok := stepper.SetEnhancedUI(pid, false); ok && was {
		defer stepper.SetEnhancedUI(pid, true)
	}

	moving := make([]*stepping, 0, len(indexes))

	for _, index := range indexes {
		entry := frames[index]
		win := byNumber[entry.Number]

		start, err := e.desktop.WindowFrame(win.ID)
		if err != nil {
			errs[index] = err

			continue
		}

		moving = append(moving, &stepping{
			index:  index,
			id:     win.ID,
			start:  start,
			finish: rectOfFrame(entry.Frame),
			last:   start,
		})
	}

	var (
		written int
		slowest time.Duration
	)

	began := time.Now()

	for tick := 1; len(moving) > 0; tick++ {
		progress := 1.0
		if duration > 0 {
			progress = math.Min(1, float64(time.Since(began))/float64(duration))
		}

		eased := ease(easing, progress)
		landed := progress >= 1

		remaining := moving[:0]

		for _, window := range moving {
			frame := window.finish
			if !landed {
				frame = between(window.start, window.finish, eased)
			}

			if frame != window.last {
				wrote := time.Now()
				err := stepper.StepWindowFrame(window.id, frame)
				slowest = max(slowest, time.Since(wrote))
				written++

				if err != nil {
					errs[window.index] = err

					continue
				}

				window.last = frame
			}

			if !landed {
				remaining = append(remaining, window)
			}
		}

		moving = remaining

		time.Sleep(time.Until(began.Add(time.Duration(tick) * stepInterval)))
	}

	return written, slowest
}

// between is the frame progress of the way from start to finish, in whole
// points, as the window server places windows.
func between(start, finish geometry.Rect, progress float64) geometry.Rect {
	along := func(from, to float64) float64 {
		return math.Round(from + (to-from)*progress)
	}

	return geometry.Rect{
		X: along(start.X, finish.X),
		Y: along(start.Y, finish.Y),
		W: along(start.W, finish.W),
		H: along(start.H, finish.H),
	}
}

// ease maps progress through the animation's time onto progress along its
// way, by the curve the Easings name.
func ease(name string, progress float64) float64 {
	switch name {
	case "ease-in":
		return progress * progress
	case "ease-out":
		return 1 - (1-progress)*(1-progress)
	case "ease-in-out":
		const halfway = 0.5

		if progress < halfway {
			return progress * progress / halfway
		}

		tail := 1 - progress

		return 1 - tail*tail/halfway
	default:
		return progress
	}
}
