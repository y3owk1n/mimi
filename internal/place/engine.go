package place

import (
	"context"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/native"
)

// Window is one window as the window server lists it, enough to match a
// rule against and to name it to a move.
type Window struct {
	PID    int
	Number uint32
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// Desktop is what the engine reads and drives: every window on every space
// by number, one application's real windows, its name and bundle
// identifier, and the two moves.
type Desktop struct {
	// Windows is every window the window server lists, on any space, by
	// number. It is what the engine marks seen when the rules come on.
	Windows func() []uint32
	// Applications is every running application with a regular activation
	// policy, by pid, which is what a sweep walks.
	Applications func() []int
	// Located is the space and the display a window is on now, as the
	// actions count them, 0 for one the desktop cannot place.
	Located func(win Window) (space, display int)
	// WindowsOf is an application's real windows, the ones focus_app
	// visits, on any space, with their frames. The window server lists
	// helper windows for an application too, and those are never placed.
	WindowsOf     func(pid int) []Window
	Application   func(pid int) (name, bundleID string)
	MoveToSpace   func(pid int, number uint32, space int, follow bool) error
	MoveToDisplay func(pid int, number uint32, display int, follow bool) error
}

// NativeDesktop is the desktop mimi runs on. serialize is where the moves
// run, the daemon's action worker.
func NativeDesktop(serialize func(func() error) error) Desktop {
	if serialize == nil {
		serialize = func(fn func() error) error { return fn() }
	}

	return Desktop{
		Windows: func() []uint32 {
			listed := native.WindowList(false)
			numbers := make([]uint32, 0, len(listed))

			for _, win := range listed {
				numbers = append(numbers, win.Number)
			}

			return numbers
		},
		WindowsOf: func(pid int) []Window {
			listed, err := native.ApplicationWindows(pid)
			if err != nil || len(listed) == 0 {
				return nil
			}

			frames := make(map[uint32]native.Frame, len(listed))

			for _, win := range native.WindowList(false) {
				frames[win.Number] = win.Frame
			}

			windows := make([]Window, 0, len(listed))

			for _, win := range listed {
				frame := frames[win.Number]
				windows = append(windows, Window{
					PID:    pid,
					Number: win.Number,
					X:      frame.X,
					Y:      frame.Y,
					Width:  frame.W,
					Height: frame.H,
				})
			}

			return windows
		},
		Applications: native.RegularApplicationPIDs,
		Located: func(win Window) (int, int) {
			space := native.SpaceIndexes()[native.WindowSpaceID(win.Number)]

			displays, err := action.QueryDisplays()
			if err != nil {
				return space, 0
			}

			frame := action.Frame{X: win.X, Y: win.Y, Width: win.Width, Height: win.Height}

			display, found := action.DisplayOf(frame, displays)
			if !found {
				return space, 0
			}

			return space, display.Index
		},
		Application: func(pid int) (string, string) {
			info, err := native.LookupApplication(pid)
			if err != nil {
				return "", ""
			}

			return info.Name, info.BundleID
		},
		MoveToSpace: func(pid int, number uint32, space int, follow bool) error {
			return serialize(func() error {
				return action.MoveWindowNumberToSpace(pid, number, space, follow)
			})
		},
		MoveToDisplay: func(_ int, number uint32, display int, follow bool) error {
			return serialize(func() error {
				return action.MoveWindowNumberToDisplay(
					number,
					action.DisplayArg{Index: display},
					follow,
				)
			})
		},
	}
}

// The window server lists a window a little after Accessibility reports it
// created, so a creation is looked for this long, this often.
const (
	appearWait = 500 * time.Millisecond
	appearPoll = 50 * time.Millisecond
)

// Engine places windows as they are created. It is safe to use from
// several goroutines: Update replaces the rules under a lock.
type Engine struct {
	desktop Desktop
	logger  *zap.SugaredLogger

	mu    sync.Mutex
	rules []config.Rule
	// written is the rules as the config wrote them, to tell a reload that
	// changed them from one that did not.
	written []config.TilingRule
	// seen is every window number the engine has looked at, so a window
	// is placed once, on creation, and never again.
	seen map[uint32]bool
	// active reports whether any rule places anything.
	active bool
	// wake asks Run for a sweep of every open window, on startup and on a
	// reload that changed the placing rules.
	wake chan struct{}
}

// New builds an engine over desktop.
func New(desktop Desktop, logger *zap.SugaredLogger) *Engine {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Engine{
		desktop: desktop,
		logger:  logger,
		seen:    map[uint32]bool{},
		wake:    make(chan struct{}, 1),
	}
}

// Update applies the rules. When they come on, and when a reload changes
// them, Run sweeps every open window into place, so a rule takes effect
// on the windows already open and not only on the ones opened later.
func (e *Engine) Update(rules []config.TilingRule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	compiled, err := config.CompileRules(rules)
	if err != nil {
		e.logger.Warnw("placement rules rejected", "err", err)

		compiled = nil
	}

	changed := !reflect.DeepEqual(e.written, rules)
	e.rules = compiled
	e.written = rules
	e.active = config.AnyPlaces(rules)

	for _, number := range e.desktop.Windows() {
		e.seen[number] = true
	}

	if e.active && changed {
		select {
		case e.wake <- struct{}{}:
		default:
		}
	}
}

// KindFilter is the bus filter for the engine's subscription. It admits
// window creations, and the moment the daemon's observer reaches an
// application it could not watch when it launched, since that application's
// first windows were created unseen. It admits nothing while no rule places
// anything.
func (e *Engine) KindFilter() events.KindFilter {
	return func(kind events.EventKind) bool {
		e.mu.Lock()
		defer e.mu.Unlock()

		return e.active && (kind == events.WindowCreated || kind == events.AXAttached)
	}
}

// Run places the windows the events on sub report created, and sweeps
// every open window when Update asks, until ctx ends.
func (e *Engine) Run(ctx context.Context, sub events.Subscriber) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
			e.sweep()
		case evt := <-sub:
			if evt.Kind == events.WindowCreated || evt.Kind == events.AXAttached {
				e.created(ctx, evt)
			}
		}
	}
}

// sweep moves every open window its rules name a destination for and that
// is not there already. Focus stays where it is. Following the one window
// the user just opened is useful. Following a dozen being sorted is not.
func (e *Engine) sweep() {
	e.mu.Lock()
	rules := e.rules
	e.mu.Unlock()

	for _, pid := range e.desktop.Applications() {
		name, bundleID := e.desktop.Application(pid)

		for _, win := range e.desktop.WindowsOf(pid) {
			target := config.RuleWindow{
				App:      name,
				BundleID: bundleID,
				Width:    win.Width,
				Height:   win.Height,
			}

			placement, found := config.PlacementFor(rules, target)
			if !found {
				continue
			}

			space, display := e.desktop.Located(win)
			if placement.Space != 0 && space == placement.Space {
				continue
			}

			if placement.Space == 0 && display == placement.Display {
				continue
			}

			placement.Follow = false
			e.place(win, placement)
		}
	}
}

// created finds the windows of the event's application the engine has not
// seen, and places each one its rules name a destination for.
func (e *Engine) created(ctx context.Context, evt events.Event) {
	fresh := e.freshWindows(ctx, evt.PID)
	if len(fresh) == 0 {
		return
	}

	name, bundleID := e.desktop.Application(evt.PID)

	e.mu.Lock()
	rules := e.rules
	e.mu.Unlock()

	for _, win := range fresh {
		target := config.RuleWindow{
			App:      name,
			BundleID: bundleID,
			Title:    evt.WindowTitle,
			Width:    win.Width,
			Height:   win.Height,
		}

		placement, found := config.PlacementFor(rules, target)
		if !found {
			continue
		}

		e.place(win, placement)
	}
}

// freshWindows is the application's windows the engine has not seen,
// waiting for the window server to list the one just created, and marks
// every one of the application's windows seen.
func (e *Engine) freshWindows(ctx context.Context, pid int) []Window {
	deadline := time.Now().Add(appearWait)

	for {
		var fresh []Window

		e.mu.Lock()

		for _, win := range e.desktop.WindowsOf(pid) {
			if !e.seen[win.Number] {
				fresh = append(fresh, win)
			}
		}

		if len(fresh) > 0 || time.Now().After(deadline) {
			for _, win := range fresh {
				e.seen[win.Number] = true
			}

			e.mu.Unlock()

			return fresh
		}

		e.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(appearPoll):
		}
	}
}

// place sends one window where its rule says. A space belongs to one
// display, so when a rule sets both, the space move alone decides where the
// window ends up.
func (e *Engine) place(win Window, placement config.Placement) {
	if placement.Display != 0 && placement.Space == 0 {
		err := e.desktop.MoveToDisplay(win.PID, win.Number, placement.Display, placement.Follow)
		if err != nil {
			e.logger.Warnw(
				"could not place window on display",
				"window",
				win.Number,
				"display",
				placement.Display,
				"err",
				err,
			)
		}

		return
	}

	err := e.desktop.MoveToSpace(win.PID, win.Number, placement.Space, placement.Follow)
	if err != nil {
		e.logger.Warnw(
			"could not place window on space",
			"window",
			win.Number,
			"space",
			placement.Space,
			"err",
			err,
		)
	}
}
