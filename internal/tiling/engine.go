package tiling

import (
	"context"
	"encoding/json"
	"fmt"
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
	ActiveSpaces() (map[uint32]int, error)
	Margins() (action.MarginsInfo, error)
	Apply(frames []action.WindowFrame) error
	Focus(number uint32) error
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
	// gap is tiling.gap when set; nil follows the macOS margin.
	gap *int
	// configured reports whether Update has run at all, which is what tells
	// the startup pass from a reload's.
	configured bool
	layout     Layout
	settle     time.Duration
	// states is what the layout returned last time, keyed by display and
	// the space in front on it (stateKey), so a space switched on one
	// display never touches what the other remembers.
	states map[string]json.RawMessage

	// onDrag is whether a window the user moved or resized runs a pass.
	onDrag bool
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
		states:      map[string]json.RawMessage{},
		applied:     map[uint32]action.Frame{},
		resizeGrace: defaultResizeGrace,
		wake:        make(chan Event, 1),
	}
}

// defaultResizeGrace covers the router's resize debounce plus an
// application's own settling after a write.
const defaultResizeGrace = time.Second

// half is the divisor that finds the center of a frame.
const half = 2

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
	e.gap = cfg.Gap
	e.configured = true
	e.onDrag = cfg.RelayoutOnDrag
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
	events.AppActivate:      true,
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
// kinds, moves and resizes too when relayout_on_drag is set, and only while
// the engine is enabled, so a disabled engine costs the bus no sends.
func (e *Engine) KindFilter() events.KindFilter {
	return func(kind events.EventKind) bool {
		e.mu.Lock()
		defer e.mu.Unlock()

		if !e.enabled {
			return false
		}

		if kind == events.WindowResize || kind == events.WindowMove {
			return e.onDrag
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

			arm(eventOf(evt))
		case event := <-e.wake:
			arm(event)
		case <-fire:
			timer = nil
			fire = nil

			// A drag settles into one pass, whatever mix of move and
			// resize events it raised on the way: what it was is decided
			// here, from where the windows ended up.
			if isDrag(pending.Kind) {
				kind, windows := e.userDragged()
				if len(windows) == 0 {
					continue
				}

				pending.Kind, pending.Windows = kind, windows
			}

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

// Pass runs the layout once per display for event and applies what they
// return, in one write. A disabled engine passes without doing anything.
func (e *Engine) Pass(ctx context.Context, event Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	inputs, err := e.inputsLocked(event)
	if err != nil {
		return err
	}

	var (
		frames []action.WindowFrame
		focus  uint32
	)

	for _, input := range inputs {
		out, reduceErr := e.layout.Reduce(ctx, input)
		if reduceErr != nil {
			return reduceErr
		}

		if out.State != nil {
			e.states[stateKey(input)] = out.State
		}

		frames = append(frames, out.Frames...)

		if out.Focus != 0 {
			focus = out.Focus
		}
	}

	if len(frames) == 0 && focus == 0 {
		return nil
	}

	// The layout ran on a snapshot; a space that changed under it would
	// refuse every frame, and the switch itself raises the event that lays
	// the new space out.
	if e.spacesChangedLocked(inputs) {
		e.logger.Debugw("tiling pass skipped: space changed")

		return nil
	}

	err = e.run(func() error {
		if len(frames) == 0 {
			return nil
		}

		return e.desktop.Apply(frames)
	})

	if focus != 0 {
		focusErr := e.run(func() error { return e.desktop.Focus(focus) })
		if focusErr != nil {
			e.logger.Debugw("tiling pass could not focus", "window", focus, "err", focusErr)
		}
	}

	// Whatever the apply reported, some frames may have landed: remember
	// them all as requested, then read back where they are.
	e.applied = make(map[uint32]action.Frame, len(frames))
	for _, frame := range frames {
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
		"displays",
		len(inputs),
		"frames",
		len(frames),
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

// Preview runs the layout the way Pass would, once per display, and returns
// what each run was given and what it would apply instead of applying it.
// It runs whether or not the engine is enabled, so a layout can be tried
// before it is switched on, and it keeps no state.
func (e *Engine) Preview(ctx context.Context, event Event) ([]Input, []Output, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	inputs, err := e.inputsLocked(event)
	if err != nil {
		return nil, nil, err
	}

	if e.layout == nil {
		return inputs, nil, derrors.New(
			derrors.CodeInvalidConfig,
			"no tiling layout configured",
		)
	}

	outputs := make([]Output, 0, len(inputs))

	for _, input := range inputs {
		out, reduceErr := e.layout.Reduce(ctx, input)
		if reduceErr != nil {
			return inputs, outputs, reduceErr
		}

		outputs = append(outputs, out)
	}

	return inputs, outputs, nil
}

// stateKey names the state for one display and the space in front on it.
func stateKey(input Input) string {
	return fmt.Sprintf("%d/%d", input.Display.ID, input.Space)
}

// inputsLocked reads the desktop into one Input per display that has a
// window on it, in display order. The caller holds the lock.
func (e *Engine) inputsLocked(event Event) ([]Input, error) {
	var (
		displays []action.DisplayEntry
		spaces   map[uint32]int
		margins  action.MarginsInfo
		windows  action.WindowsInfo
	)

	err := e.run(func() error {
		var err error

		displays, err = e.desktop.Displays()
		if err != nil {
			return err
		}

		spaces, err = e.desktop.ActiveSpaces()
		if err != nil {
			return err
		}

		margins, err = e.desktop.Margins()
		if err != nil {
			return err
		}

		windows, err = e.desktop.Windows()

		return err
	})
	if err != nil {
		return nil, err
	}

	focused := uint32(0)
	if windows.Focused >= 0 && windows.Focused < len(windows.Windows) {
		focused = windows.Windows[windows.Focused].Number
	}

	gap := 0.0
	if e.gap != nil {
		gap = float64(*e.gap)
	} else if margins.Enabled {
		gap = margins.Size
	}

	inputs := make([]Input, 0, len(displays))

	for _, display := range displays {
		input := Input{
			Version:  InputVersion,
			Event:    event,
			Display:  display,
			Space:    spaces[display.ID],
			Gap:      gap,
			Displays: displays,
			Focused:  -1,
			Windows:  []action.WindowEntry{},
		}

		for _, win := range windows.Windows {
			if displayOf(win.Frame, displays) != display.ID {
				continue
			}

			if win.Number == focused {
				input.Focused = len(input.Windows)
			}

			input.Windows = append(input.Windows, win)
		}

		if len(input.Windows) == 0 {
			continue
		}

		input.State = e.states[stateKey(input)]
		if input.State == nil {
			input.State = json.RawMessage("null")
		}

		inputs = append(inputs, input)
	}

	return inputs, nil
}

// displayOf is the id of the display whose frame holds the center of frame,
// or the first display's when none does.
func displayOf(frame action.Frame, displays []action.DisplayEntry) uint32 {
	centerX := frame.X + frame.Width/half
	centerY := frame.Y + frame.Height/half

	for _, display := range displays {
		bounds := display.Frame
		if centerX >= bounds.X && centerX < bounds.X+bounds.Width &&
			centerY >= bounds.Y && centerY < bounds.Y+bounds.Height {
			return display.ID
		}
	}

	if len(displays) > 0 {
		return displays[0].ID
	}

	return 0
}

// spacesChangedLocked reports whether any display the inputs were read on
// shows another space now. The caller holds the lock.
func (e *Engine) spacesChangedLocked(inputs []Input) bool {
	var spaces map[uint32]int

	err := e.run(func() error {
		var err error

		spaces, err = e.desktop.ActiveSpaces()

		return err
	})
	if err != nil {
		return false
	}

	for _, input := range inputs {
		if now, ok := spaces[input.Display.ID]; ok && now != input.Space {
			return true
		}
	}

	return false
}

func (e *Engine) settleWindow() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.settle
}

// isDrag reports whether kind is one a user's drag raises.
func isDrag(kind string) bool {
	return kind == string(events.WindowMove) || kind == string(events.WindowResize)
}

// userDragged decides what a settled drag was, reporting the windows the
// user dragged, none when the drag was not the user's, and whether it was a
// move or a resize. Within the grace after an apply it is the engine's own
// write landing, possibly snapped by the application, so the frames are
// read back again and remembered. After it, the user's windows are the
// placed ones that are no longer where they were placed.
//
// The kind comes from where the windows ended up, not from which
// notifications macOS sent: an application that snaps its size to a grid
// resizes itself a little on every move, and raises both. A drag that
// changed position more than it changed size is a move; anything else,
// which includes a dragged edge, is a resize.
func (e *Engine) userDragged() (string, []uint32) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.onDrag || len(e.applied) == 0 {
		return "", nil
	}

	if time.Since(e.appliedAt) < e.resizeGrace {
		e.rememberLocked()

		return "", nil
	}

	windows, err := e.readWindows()
	if err != nil {
		return "", nil
	}

	var (
		dragged []uint32
		resized bool
	)

	for _, win := range windows.Windows {
		placed, ok := e.applied[win.Number]
		if !ok || sameFrame(placed, win.Frame) {
			continue
		}

		dragged = append(dragged, win.Number)

		moved := math.Abs(win.Frame.X-placed.X) + math.Abs(win.Frame.Y-placed.Y)
		sized := math.Abs(win.Frame.Width-placed.Width) + math.Abs(win.Frame.Height-placed.Height)

		if sized >= moved {
			resized = true
		}
	}

	if resized {
		return string(events.WindowResize), dragged
	}

	return string(events.WindowMove), dragged
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

// run puts fn through the serializer when there is one.
func (e *Engine) run(fn func() error) error {
	if e.serialize == nil {
		return fn()
	}

	return e.serialize(fn)
}
