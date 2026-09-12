package action

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/y3owk1n/mimi/internal/geometry"
)

// stepInterval is how often the animation writes a frame: a hundred a
// second, as the window managers that animate this way do.
const stepInterval = 10 * time.Millisecond

// animator moves windows to their frames through their applications, a
// step every stepInterval along the animation's curve, in the background:
// an apply returns as the windows set off. Every application has a worker
// of its own, since an application answers its accessibility calls one at
// a time, and a window sent somewhere new while on its way turns from where
// it is.
type animator struct {
	mu      sync.Mutex
	workers map[int]*appWorker
}

func newAnimator() *animator {
	return &animator{workers: map[int]*appWorker{}}
}

// start sends the frames on their way and reports, at once, the frames
// that cannot go: a window not on the active space, or one whose frame
// could not be read. A frame the payload leaves out of the animation is
// written at once, before the rest set off. Writes that fail on the way
// are counted in the worker's report.
func (a *animator) start(
	stepper FrameStepper,
	desktop Desktop,
	frames []WindowFrame,
	windows []Window,
	animation Animation,
) []string {
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
			errs[index] = desktop.SetWindowFrame(win.ID, rectOfFrame(entry.Frame))

			continue
		}

		groups[win.PID] = append(groups[win.PID], index)
	}

	duration := time.Duration(animation.DurationMS) * time.Millisecond

	var readers sync.WaitGroup

	for pid, indexes := range groups {
		readers.Add(1)

		go func(pid int, indexes []int) {
			defer readers.Done()

			worker := a.worker(pid, stepper)

			for _, index := range indexes {
				entry := frames[index]
				errs[index] = worker.send(
					desktop,
					byNumber[entry.Number].ID,
					rectOfFrame(entry.Frame),
					duration,
					animation.Easing,
				)
			}

			worker.wake()
		}(pid, indexes)
	}

	readers.Wait()

	failures, _ := collectFailures(frames, messages, errs)

	return failures
}

// worker is the application's worker, made on first use.
func (a *animator) worker(pid int, stepper FrameStepper) *appWorker {
	a.mu.Lock()
	defer a.mu.Unlock()

	worker, ok := a.workers[pid]
	if !ok {
		worker = &appWorker{pid: pid, stepper: stepper, moving: map[WindowID]*stepping{}}
		a.workers[pid] = worker
	}

	return worker
}

// appWorker steps one application's windows, together, on a goroutine
// that runs while any of them is on its way.
type appWorker struct {
	pid     int
	stepper FrameStepper

	mu      sync.Mutex
	moving  map[WindowID]*stepping
	running bool
}

// stepping is one window on its way: where it set off from and when, where
// it goes, how, and the last frame written, so a step that lands where the
// last did is not written again.
type stepping struct {
	start    geometry.Rect
	finish   geometry.Rect
	last     geometry.Rect
	began    time.Time
	duration time.Duration
	easing   string
}

// send puts a window on its way to finish. A window already on its way to
// that very frame keeps going; one on its way elsewhere turns from where it
// is. A window at rest sets off from where its application has it, which is
// a round trip.
func (w *appWorker) send(
	desktop Desktop,
	windowID WindowID,
	finish geometry.Rect,
	duration time.Duration,
	easing string,
) error {
	w.mu.Lock()
	current, inFlight := w.moving[windowID]
	w.mu.Unlock()

	if inFlight && current.finish == finish {
		return nil
	}

	var start geometry.Rect

	if inFlight {
		start = current.last
	} else {
		read, err := desktop.WindowFrame(windowID)
		if err != nil {
			return err
		}

		start = read
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.moving[windowID] = &stepping{
		start:    start,
		finish:   finish,
		last:     start,
		began:    time.Now(),
		duration: duration,
		easing:   easing,
	}

	return nil
}

// wake runs the worker when it is not running already.
func (w *appWorker) wake() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running || len(w.moving) == 0 {
		return
	}

	w.running = true

	go w.run()
}

// run steps the windows until every one has landed, then reports.
func (w *appWorker) run() {
	// Under the enhanced interface some applications animate every move
	// they are given, which fights the steps.
	if was, ok := w.stepper.SetEnhancedUI(w.pid, false); ok && was {
		defer w.stepper.SetEnhancedUI(w.pid, true)
	}

	var report StepReport

	began := time.Now()
	seen := map[WindowID]bool{}

	for tick := 1; ; tick++ {
		w.mu.Lock()

		for windowID, window := range w.moving {
			if !seen[windowID] {
				seen[windowID] = true
				report.Windows++
			}

			progress := 1.0
			if window.duration > 0 {
				progress = math.Min(1, float64(time.Since(window.began))/float64(window.duration))
			}

			landed := progress >= 1

			frame := window.finish
			if !landed {
				frame = between(window.start, window.finish, ease(window.easing, progress))
			}

			if frame != window.last {
				wrote := time.Now()
				err := w.stepper.StepWindowFrame(windowID, frame)
				report.Slowest = max(report.Slowest, time.Since(wrote))
				report.Frames++

				if err != nil {
					report.Failed++
					landed = true
				}

				window.last = frame
			}

			if landed {
				delete(w.moving, windowID)
			}
		}

		if len(w.moving) == 0 {
			w.running = false
			w.mu.Unlock()

			break
		}

		w.mu.Unlock()

		time.Sleep(time.Until(began.Add(time.Duration(tick) * stepInterval)))
	}

	report.Elapsed = time.Since(began)
	w.stepper.FinishSteps(report)
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
