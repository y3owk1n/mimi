package tiling

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
)

// Desktop is what the engine needs from the machine: the three reads and the
// one write a layout is built on. The real one is action's queries and
// apply_frames; the tests use a fake.
type Desktop interface {
	Windows() (action.WindowsInfo, error)
	Displays() ([]action.DisplayEntry, error)
	ActiveSpace() (int, error)
	Apply(frames []action.WindowFrame) error
}

// Serializer runs fn where the desktop is safe to drive. The daemon hands in
// the IPC server's action worker, so a pass never interleaves with an action
// arriving over the socket; nil runs fn where it stands.
type Serializer func(fn func() error) error

// Engine runs the layout on window events. It is safe to use from several
// goroutines: Update replaces the configuration under a lock, and Run and
// Pass take the same lock for each pass.
type Engine struct {
	desktop   Desktop
	serialize Serializer
	logger    *zap.SugaredLogger

	mu      sync.Mutex
	enabled bool
	command string
	// configured reports whether Update has run at all, which is what tells
	// the startup pass from a reload's.
	configured bool
	layout     Layout
	settle     time.Duration
	// states is what the layout returned last time, keyed by space index.
	states map[int]json.RawMessage

	// onResize is whether a window the user resized runs a pass.
	onResize bool
	// applied is where every window the engine last wrote actually landed,
	// read back rather than as requested, keyed by window number; and
	// appliedAt is when. Together they tell the engine's own resizes from
	// the user's.
	applied   map[uint32]action.Frame
	appliedAt time.Time
	// resizeGrace is how long after an apply a resize event is taken to be
	// the engine's own, and used to refresh applied rather than compared
	// against it: an application that snaps its frame does so a moment
	// after the write, and the frame it snapped to is the one to remember.
	resizeGrace time.Duration

	// wake carries the passes the engine asks of itself, on startup and on
	// a reload that switches it on; Run drains it alongside the bus. It
	// holds one, so an Update before Run starts is not lost.
	wake chan Event
}

// New returns an engine that is disabled until Update enables it.
func New(desktop Desktop, serialize Serializer, logger *zap.SugaredLogger) *Engine {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Engine{
		desktop:     desktop,
		serialize:   serialize,
		logger:      logger,
		states:      map[int]json.RawMessage{},
		applied:     map[uint32]action.Frame{},
		resizeGrace: defaultResizeGrace,
		wake:        make(chan Event, 1),
	}
}

// defaultResizeGrace covers the router's resize debounce plus an
// application's own settling after a write.
const defaultResizeGrace = time.Second

// samePoint is how far two frames may differ and still be the same
// placement: macOS stores frames in whole points, and rounds.
const samePoint = 1.0

// Update applies the [tiling] section and the shell it runs under. It is
// what the daemon calls at startup and on every reload.
//
// Enabling the engine, at startup or on a reload, or handing it another
// layout while enabled, asks for a pass, so the windows already open are laid
// out without waiting for one of them to change. Any other change asks for
// none: a reload that touched a hook should not reshuffle the desktop.
func (e *Engine) Update(cfg config.TilingConfig, shell string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	wasEnabled, hadCommand, hadConfig := e.enabled, e.command, e.configured

	e.enabled = cfg.Enabled
	e.command = cfg.Layout
	e.configured = true
	e.onResize = cfg.RelayoutOnResize
	e.settle = time.Duration(cfg.DebounceMS) * time.Millisecond
	e.layout = Program{
		Shell:   shell,
		Command: cfg.Layout,
		Timeout: time.Duration(cfg.TimeoutSecs) * time.Second,
	}

	if !cfg.Enabled || (wasEnabled && hadCommand == cfg.Layout) {
		return
	}

	kind := EventReload
	if !hadConfig {
		kind = EventStartup
	}

	select {
	case e.wake <- Event{Kind: kind}:
	default:
	}
}

// Enabled reports whether a pass would run anything.
func (e *Engine) Enabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.enabled
}

// wakingKinds are the events a pass runs for. Resizes are deliberately not
// among them: every frame the engine writes is one, and the loop that would
// make is the failure a layout engine exists to avoid. AXAttached is: the
// application's first windows opened before the daemon could see them, and
// this is the moment it can.
//
//nolint:gochecknoglobals // a fixed set
var wakingKinds = map[events.EventKind]bool{
	events.WindowCreated:    true,
	events.WindowClosed:     true,
	events.WindowFocus:      true,
	events.WorkspaceChanged: true,
	events.AppHide:          true,
	events.AppUnhide:        true,
	events.AppQuit:          true,
	events.AXAttached:       true,
}

// KindFilter is the bus filter for the engine's subscription: the waking
// kinds, resizes too when relayout_on_resize is set, and only while the
// engine is enabled, so a disabled engine costs the bus no sends.
func (e *Engine) KindFilter() events.KindFilter {
	return func(kind events.EventKind) bool {
		e.mu.Lock()
		defer e.mu.Unlock()

		if !e.enabled {
			return false
		}

		if kind == events.WindowResize {
			return e.onResize
		}

		return wakingKinds[kind]
	}
}

// Run drains sub, and the engine's own wake-ups, until ctx is done, settling
// each burst into one pass that reports the last event of the burst.
func (e *Engine) Run(ctx context.Context, sub events.Subscriber) {
	var (
		timer   *time.Timer
		fire    <-chan time.Time
		pending Event
	)

	arm := func(event Event) {
		pending = event

		settle := e.settleWindow()
		if timer == nil {
			timer = time.NewTimer(settle)
			fire = timer.C

			return
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		timer.Reset(settle)
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}

			return
		case evt, ok := <-sub:
			if !ok {
				return
			}

			event := eventOf(evt)

			if evt.Kind == events.WindowResize {
				event.Windows = e.userResized()
				if len(event.Windows) == 0 {
					continue
				}
			}

			arm(event)
		case event := <-e.wake:
			arm(event)
		case <-fire:
			timer = nil
			fire = nil

			err := e.Pass(ctx, pending)
			if err != nil {
				e.logger.Warnw("tiling pass failed", "kind", pending.Kind, "err", err)
			}
		}
	}
}

// eventOf is the bus event as the layout hears of it: the kind and the
// application, never the title.
func eventOf(evt events.Event) Event {
	return Event{Kind: string(evt.Kind), App: evt.AppName, BundleID: evt.BundleID, PID: evt.PID}
}

// Pass runs the layout once for event and applies what it returns. A
// disabled engine passes without doing anything.
func (e *Engine) Pass(ctx context.Context, event Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	out, space, err := e.reduceLocked(ctx, event)
	if err != nil {
		return err
	}

	if out.State != nil {
		e.states[space] = out.State
	}

	if len(out.Frames) == 0 {
		return nil
	}

	err = e.run(func() error { return e.desktop.Apply(out.Frames) })

	// Whatever the apply reported, some frames may have landed: remember
	// them all as requested, then read back where they are.
	e.applied = make(map[uint32]action.Frame, len(out.Frames))
	for _, frame := range out.Frames {
		e.applied[frame.Number] = frame.Frame
	}

	e.appliedAt = time.Now()
	e.rememberLocked()

	if err != nil {
		return err
	}

	e.logger.Debugw(
		"tiling pass applied",
		"kind",
		event.Kind,
		"space",
		space,
		"frames",
		len(out.Frames),
	)

	return nil
}

// Command is a Pass a user asked for, by hotkey or by hand: unlike an event
// from the bus, it is refused rather than dropped while the engine is
// disabled, so the user learns why nothing moved.
func (e *Engine) Command(ctx context.Context, event Event) error {
	if !e.Enabled() {
		return derrors.New(
			derrors.CodeActionFailed,
			"tiling is disabled (set tiling.enabled = true, and grant Accessibility)",
		)
	}

	return e.Pass(ctx, event)
}

// Preview runs the layout the way Pass would, and returns what it would
// apply instead of applying it. It runs whether or not the engine is enabled,
// so a layout can be tried before it is switched on, and it keeps no state.
func (e *Engine) Preview(ctx context.Context, event Event) (Input, Output, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	input, err := e.inputLocked(event)
	if err != nil {
		return Input{}, Output{}, err
	}

	if e.layout == nil {
		return input, Output{}, derrors.New(
			derrors.CodeInvalidConfig,
			"no tiling layout configured",
		)
	}

	out, err := e.layout.Reduce(ctx, input)

	return input, out, err
}

func (e *Engine) settleWindow() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.settle
}

// userResized decides what a resize event means, reporting the windows the
// user moved, none when the event was not the user's. Within the grace after
// an apply it is the engine's own write landing, possibly snapped by the
// application, so the frames are read back again and remembered. After it,
// the user's windows are the placed ones that are no longer where they were
// placed.
func (e *Engine) userResized() []uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.onResize || len(e.applied) == 0 {
		return nil
	}

	if time.Since(e.appliedAt) < e.resizeGrace {
		e.rememberLocked()

		return nil
	}

	windows, err := e.readWindows()
	if err != nil {
		return nil
	}

	var moved []uint32

	for _, win := range windows.Windows {
		placed, ok := e.applied[win.Number]
		if ok && !sameFrame(placed, win.Frame) {
			moved = append(moved, win.Number)
		}
	}

	return moved
}

// rememberLocked reads back where the windows the engine placed actually
// are, and keeps that. The caller holds the lock.
func (e *Engine) rememberLocked() {
	windows, err := e.readWindows()
	if err != nil {
		return
	}

	for _, win := range windows.Windows {
		if _, ok := e.applied[win.Number]; ok {
			e.applied[win.Number] = win.Frame
		}
	}
}

func (e *Engine) readWindows() (action.WindowsInfo, error) {
	var windows action.WindowsInfo

	err := e.run(func() error {
		var err error

		windows, err = e.desktop.Windows()

		return err
	})

	return windows, err
}

func sameFrame(first, second action.Frame) bool {
	return math.Abs(first.X-second.X) <= samePoint &&
		math.Abs(first.Y-second.Y) <= samePoint &&
		math.Abs(first.Width-second.Width) <= samePoint &&
		math.Abs(first.Height-second.Height) <= samePoint
}

// reduceLocked builds the input, runs the layout, and reports the space the
// state belongs to. The caller holds the lock.
func (e *Engine) reduceLocked(ctx context.Context, event Event) (Output, int, error) {
	input, err := e.inputLocked(event)
	if err != nil {
		return Output{}, 0, err
	}

	out, err := e.layout.Reduce(ctx, input)
	if err != nil {
		return Output{}, 0, err
	}

	return out, input.Space, nil
}

// inputLocked reads the desktop into an Input. The caller holds the lock.
func (e *Engine) inputLocked(event Event) (Input, error) {
	var input Input

	err := e.run(func() error {
		space, err := e.desktop.ActiveSpace()
		if err != nil {
			return err
		}

		displays, err := e.desktop.Displays()
		if err != nil {
			return err
		}

		windows, err := e.desktop.Windows()
		if err != nil {
			return err
		}

		input = Input{
			Version:  InputVersion,
			Event:    event,
			Space:    space,
			Displays: displays,
			Focused:  windows.Focused,
			Windows:  windows.Windows,
			State:    e.states[space],
		}

		return nil
	})
	if err != nil {
		return Input{}, err
	}

	if input.Windows == nil {
		input.Windows = []action.WindowEntry{}
	}

	if input.State == nil {
		input.State = json.RawMessage("null")
	}

	return input, nil
}

// run puts fn through the serializer when there is one.
func (e *Engine) run(fn func() error) error {
	if e.serialize == nil {
		return fn()
	}

	return e.serialize(fn)
}
