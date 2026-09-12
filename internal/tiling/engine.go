package tiling

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"slices"
	"strings"
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
	FullScreenDisplays() (map[uint32]bool, error)
	Margins() (action.MarginsInfo, error)
	Apply(frames []action.WindowFrame, animation *action.Animation) error
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
	// animation is how the frames move, or nil to move them at once.
	animation *action.Animation
	// configured reports whether Update has run at all, which is what tells
	// the startup pass from a reload's.
	configured bool
	layout     Layout
	// resident is the layout when it is kept running between passes, so
	// a reload or shutdown can stop it; nil otherwise.
	resident *Resident
	// pending is each command a pass is waiting to run for, by name. A
	// command that arrives while one of its name already waits is dropped:
	// a held key sends them faster than passes run, and without this they
	// queue up and keep scrolling after the key is released.
	pendingMu sync.Mutex
	pending   map[string]bool
	settle    time.Duration
	// states is what the layout returned last time, keyed by display and
	// the space in front on it (stateKey), so a space switched on one
	// display never touches what the other remembers.
	states map[string]json.RawMessage
	// seen is every window number the last pass read, nil before the
	// first, so a window_created pass can tell whether the window it was
	// raised for has reached the window server's on-screen list yet.
	seen map[uint32]bool
	// titles is every window's title as the last full read had it, by
	// number, for a preview that would rather not ask the applications.
	titles map[uint32]string

	// onDrag is whether a window the user moved or resized runs a pass.
	onDrag bool
	// displays names the displays the last pass read, so the windows macOS
	// moves when one is plugged in or unplugged are not taken for a drag.
	displays string
	// mouseDown reports whether the left button is held, which is when a
	// settled drag is still going on; nil never is.
	mouseDown func() bool
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

	// shell is what Before and After command lines run under, as
	// settings.hook_shell, and commandTimeout bounds each line.
	shell          string
	commandTimeout time.Duration
	// background counts what a pass left running past its return, the
	// After command runs and the read-back of where the frames landed, so
	// a one-shot engine can wait for them before its process ends.
	background sync.WaitGroup

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
		pending:     map[string]bool{},
		applied:     map[uint32]action.Frame{},
		titles:      map[uint32]string{},
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

	e.animation = nil
	if cfg.Animation.Enabled {
		e.animation = &action.Animation{
			DurationMS: cfg.Animation.DurationMS,
			Easing:     cfg.Animation.Easing,
		}
	}

	e.configured = true
	e.shell = shell
	e.commandTimeout = time.Duration(cfg.CommandTimeoutSecs) * time.Second
	e.onDrag = cfg.RelayoutOnDrag
	e.settle = time.Duration(cfg.DebounceMS) * time.Millisecond

	// A resident layout survives a reload that leaves it as it was: what it
	// runs, and how. Anything else stops it, and the next pass starts what
	// the config names now.
	timeout := time.Duration(cfg.TimeoutSecs) * time.Second
	resident := cfg.Enabled && cfg.LayoutMode == config.LayoutModeResident

	keep := e.resident != nil && resident && e.resident.Shell == shell &&
		e.resident.Command == cfg.Layout && e.resident.Timeout == timeout
	if !keep {
		e.stopResidentLocked()
	}

	switch {
	case keep:
	case resident:
		e.resident = NewResident(shell, cfg.Layout, timeout, e.logger)
		e.layout = e.resident
	default:
		e.layout = Program{Shell: shell, Command: cfg.Layout, Timeout: timeout}
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

// SetMouse names the function that reports whether the left mouse button
// is down. A drag that settles while it is, because the user paused, waits
// for the release rather than laying the desktop out under their hand.
func (e *Engine) SetMouse(down func() bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.mouseDown = down
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
	events.WindowMinimize:   true,
	events.WindowUnminimize: true,
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

			e.Close()

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
			// here, from where the windows ended up. One the user is
			// still holding has not settled, however long they pause,
			// and is looked at again a little later.
			if isDrag(pending.Kind) {
				if e.dragHeld() {
					arm(pending)

					continue
				}

				kind, windows := e.userDragged()
				if kind == "" {
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

	return e.passLocked(ctx, event)
}

// Command is a Pass a user asked for, by hotkey or by hand: unlike an event
// from the bus, it is refused rather than dropped while the engine is
// disabled, so the user learns why nothing moved. Commands of one name are
// run one at a time with at most one more waiting; the rest are dropped, so
// a key held down scrolls in step with the screen and stops when released.
func (e *Engine) Command(ctx context.Context, event Event) error {
	key := event.Kind + " " + event.Name

	e.pendingMu.Lock()
	if e.pending[key] {
		e.pendingMu.Unlock()

		return nil
	}

	e.pending[key] = true
	e.pendingMu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.pendingMu.Lock()
	delete(e.pending, key)
	e.pendingMu.Unlock()

	if !e.enabled {
		return derrors.New(
			derrors.CodeActionFailed,
			"tiling is disabled (set tiling.enabled = true, and grant Accessibility)",
		)
	}

	return e.passLocked(ctx, event)
}

// Wait blocks until everything a pass left running has finished, its After
// lines and the read-back of where its frames landed. The CLI calls it
// before it exits, because the lines would die with the process. The daemon
// never needs to.
func (e *Engine) Wait() {
	e.background.Wait()
}

// Close stops a resident layout. The engine is not used after it.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stopResidentLocked()
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

	outputs, err := e.reduceAll(ctx, inputs)

	return inputs, outputs, err
}

// dragHeld reports whether the user is still holding a drag.
func (e *Engine) dragHeld() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.mouseDown != nil && e.mouseDown()
}

// reduceAll runs the layout on every input at once, one goroutine each,
// and returns the outputs in input order. A one-shot layout on two displays
// starts twice in the time of one start. A resident layout answers one
// input at a time either way. The first failure cancels the rest, and
// reduceAll returns it with the outputs that came back before it.
func (e *Engine) reduceAll(ctx context.Context, inputs []Input) ([]Output, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	outputs := make([]Output, len(inputs))
	errs := make([]error, len(inputs))

	var runs sync.WaitGroup

	for index, input := range inputs {
		runs.Go(func() {
			outputs[index], errs[index] = e.layout.Reduce(ctx, input)
			if errs[index] != nil {
				cancel()
			}
		})
	}

	runs.Wait()

	for index, err := range errs {
		if err != nil {
			return outputs[:index], err
		}
	}

	return outputs, nil
}

// passLocked is Pass under the lock.
func (e *Engine) passLocked(ctx context.Context, event Event) error {
	if !e.enabled {
		return nil
	}

	inputs, err := e.settledInputsLocked(ctx, event)
	if err != nil {
		return err
	}

	var (
		frames []action.WindowFrame
		focus  uint32
		before []string
		after  []string
	)

	outputs, err := e.reduceAll(ctx, inputs)
	if err != nil {
		return err
	}

	for index, out := range outputs {
		input := inputs[index]

		if out.State != nil {
			e.states[stateKey(input)] = out.State
		}

		frames = append(frames, out.Frames...)

		if out.Focus != 0 {
			focus = out.Focus
		}

		before = append(before, out.Before...)
		after = append(after, out.After...)
	}

	if len(frames) == 0 && focus == 0 && len(before) == 0 && len(after) == 0 {
		return nil
	}

	// The layout ran on a snapshot; a space that changed under it would
	// refuse every frame, and the switch itself raises the event that lays
	// the new space out.
	if e.spacesChangedLocked(inputs) {
		e.logger.Debugw("tiling pass skipped: space changed")

		return nil
	}

	// A window the user just dragged is placed where the layout puts it at
	// once rather than animated there.
	if e.animation != nil {
		for index := range frames {
			if slices.Contains(event.Windows, frames[index].Number) {
				frames[index].Animate = new(bool)
			}
		}
	}

	e.runBefore(ctx, before)

	// Focus goes first: it is what the user pressed a key for, and it is
	// one round trip, while the frames wait on a screen capture when they
	// animate. A window focuses as well off screen as on it.
	if focus != 0 {
		focusErr := e.run(func() error { return e.desktop.Focus(focus) })
		if focusErr != nil {
			e.logger.Debugw("tiling pass could not focus", "window", focus, "err", focusErr)
		}
	}

	applyStart := time.Now()

	err = e.run(func() error {
		if len(frames) == 0 {
			return nil
		}

		return e.desktop.Apply(frames, e.animation)
	})

	// Whatever the apply reported, some frames may have landed: remember
	// them all as requested, then read back where they are.
	e.applied = make(map[uint32]action.Frame, len(frames))
	for _, frame := range frames {
		e.applied[frame.Number] = frame.Frame
	}

	e.appliedAt = time.Now()
	e.rememberLater(e.appliedAt)

	if err != nil {
		return err
	}

	e.runAfterLocked(after, e.animationDelay(frames))

	e.logger.Debugw(
		"tiling pass applied",
		"kind",
		event.Kind,
		"displays",
		len(inputs),
		"frames",
		len(frames),
		"apply_ms",
		time.Since(applyStart).Milliseconds(),
	)

	return nil
}

// animationDelay is how long after the write the frames reach where the
// layout put them. It is the animation's length when one moved them, and
// zero otherwise.
func (e *Engine) animationDelay(frames []action.WindowFrame) time.Duration {
	if e.animation == nil || len(frames) == 0 {
		return 0
	}

	return time.Duration(e.animation.DurationMS) * time.Millisecond
}

// runBefore starts every Before line at once and waits for all of them, so
// the pass waits as long as the slowest line. It kills a line past the
// command timeout and logs a failure without reporting it, so the frames
// still apply.
func (e *Engine) runBefore(ctx context.Context, lines []string) {
	var runs sync.WaitGroup

	for _, line := range lines {
		runs.Go(func() {
			err := runLine(ctx, e.shell, line, e.commandTimeout)
			if err != nil {
				e.logger.Debugw("tiling before command failed", "err", err)
			}
		})
	}

	runs.Wait()
}

// runLine runs one command line through the shell and kills it past
// timeout, when there is one.
func runLine(ctx context.Context, shell, line string, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return exec.CommandContext(ctx, shell, "-c", line).Run()
}

// runAfterLocked starts every After line once delay has passed, in order
// and detached, so the pass does not wait. It kills a line past the command
// timeout and logs a failure without reporting it. The caller holds the
// lock.
func (e *Engine) runAfterLocked(lines []string, delay time.Duration) {
	if len(lines) == 0 {
		return
	}

	shell, timeout, logger := e.shell, e.commandTimeout, e.logger

	e.background.Go(func() {
		time.Sleep(delay)

		for _, line := range lines {
			// The command outlives the pass, so the pass's context
			// must not bound it.
			err := runLine(context.Background(), shell, line, timeout)
			if err != nil {
				logger.Debugw("tiling after command failed", "err", err)
			}
		}
	})
}

// stopResidentLocked ends the resident layout, if there is one. The caller
// holds the lock.
func (e *Engine) stopResidentLocked() {
	if e.resident == nil {
		return
	}

	e.resident.Stop()
	e.resident = nil
	e.layout = nil
}

// stateKey names the state for one display and the space in front on it.
func stateKey(input Input) string {
	return fmt.Sprintf("%d/%d", input.Display.ID, input.Space)
}

// newWindowWait is how long a window_created pass waits for the window
// server to list the window it was raised for, and newWindowPoll how often
// it looks. Accessibility reports a window created before the window server
// shows it on screen, by a couple of hundred milliseconds when the Dock
// reopens an application, and a pass that read the desktop in between
// would lay out everything but the new window, with nothing to run again.
const (
	newWindowWait = 500 * time.Millisecond
	newWindowPoll = 50 * time.Millisecond
)

// settledInputsLocked is inputsLocked, re-read for a window_created event
// until the application the event names shows a window the last pass did
// not, or newWindowWait is up. The caller holds the lock.
func (e *Engine) settledInputsLocked(ctx context.Context, event Event) ([]Input, error) {
	inputs, err := e.inputsLocked(event)
	if err != nil {
		return nil, err
	}

	if event.Kind == string(events.WindowCreated) && event.PID != 0 && e.seen != nil {
		deadline := time.Now().Add(newWindowWait)

		for !e.newWindowOf(inputs, event.PID) && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(newWindowPoll):
			}

			inputs, err = e.inputsLocked(event)
			if err != nil {
				return nil, err
			}
		}
	}

	e.seen = map[uint32]bool{}

	for _, input := range inputs {
		for _, win := range input.Windows {
			e.seen[win.Number] = true
		}
	}

	return inputs, nil
}

// newWindowOf reports whether inputs hold a window of pid the last pass
// did not see.
func (e *Engine) newWindowOf(inputs []Input, pid int) bool {
	for _, input := range inputs {
		for _, win := range input.Windows {
			if win.PID == pid && !e.seen[win.Number] {
				return true
			}
		}
	}

	return false
}

// inputsLocked reads the desktop into one Input per display that has a
// window on it and is not showing a full-screen space, in display order. The caller holds the lock.
// titledWindower is a desktop that can list windows with titles it is
// handed rather than read, which the live one can.
type titledWindower interface {
	WindowsWithTitles(known map[uint32]string) (action.WindowsInfo, error)
}

// desktopRead is everything one pass reads of the desktop.
type desktopRead struct {
	displays   []action.DisplayEntry
	spaces     map[uint32]int
	fullScreen map[uint32]bool
	margins    action.MarginsInfo
	windows    action.WindowsInfo
}

// readInputsLocked reads what the inputs are built from. With quick set,
// and a desktop that can, the windows keep the titles of the last full
// read rather than asking the applications; every full read refreshes
// them. The caller holds the lock.
func (e *Engine) readInputsLocked(quick bool) (desktopRead, error) {
	var read desktopRead

	titled, canReuse := e.desktop.(titledWindower)

	err := e.run(func() error {
		var err error

		read.displays, err = e.desktop.Displays()
		if err != nil {
			return err
		}

		read.spaces, err = e.desktop.ActiveSpaces()
		if err != nil {
			return err
		}

		read.fullScreen, err = e.desktop.FullScreenDisplays()
		if err != nil {
			return err
		}

		read.margins, err = e.desktop.Margins()
		if err != nil {
			return err
		}

		if quick && canReuse {
			read.windows, err = titled.WindowsWithTitles(e.titles)

			return err
		}

		read.windows, err = e.desktop.Windows()

		return err
	})
	if err != nil {
		return desktopRead{}, err
	}

	if !quick || !canReuse {
		e.titles = make(map[uint32]string, len(read.windows.Windows))
		for _, win := range read.windows.Windows {
			e.titles[win.Number] = win.Title
		}
	}

	if !quick {
		e.displays = displaySignature(read.displays)
	}

	return read, nil
}

// inputsLocked is one input per display with windows, built from a full
// read of the desktop. The caller holds the lock.
func (e *Engine) inputsLocked(event Event) ([]Input, error) {
	read, err := e.readInputsLocked(false)
	if err != nil {
		return nil, err
	}

	return e.buildInputsLocked(event, read), nil
}

// buildInputsLocked is one input per display with windows, from read. The
// caller holds the lock.
func (e *Engine) buildInputsLocked(event Event, read desktopRead) []Input {
	displays, spaces, fullScreen, margins, windows := read.displays, read.spaces, read.fullScreen, read.margins, read.windows

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
		// macOS lays out a full-screen space itself, one window or a
		// split-view pair, and rejects frames written to it.
		if fullScreen[display.ID] {
			continue
		}

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

	return inputs
}

// displayOf is the id of the display whose frame holds the center of frame,
// or, when none does, the one that center is nearest: a column a strip
// parks off the edge of a display stays that display's.
func displayOf(frame action.Frame, displays []action.DisplayEntry) uint32 {
	centerX := frame.X + frame.Width/half
	centerY := frame.Y + frame.Height/half

	var (
		nearest  uint32
		distance = math.Inf(1)
	)

	for _, display := range displays {
		bounds := display.Frame
		outsideX := math.Max(bounds.X-centerX, math.Max(0, centerX-(bounds.X+bounds.Width)))
		outsideY := math.Max(bounds.Y-centerY, math.Max(0, centerY-(bounds.Y+bounds.Height)))

		if outsideX == 0 && outsideY == 0 {
			return display.ID
		}

		if d := outsideX*outsideX + outsideY*outsideY; d < distance {
			nearest, distance = display.ID, d
		}
	}

	return nearest
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

// animationSettle is how long after an animated apply the windows have
// landed, with a margin for a slow application; 0 without an animation.
func (e *Engine) animationSettle() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.animation == nil {
		return 0
	}

	const margin = 2

	return time.Duration(e.animation.DurationMS) * time.Millisecond * margin
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
//
// A window entering or leaving full screen raises the same events, and
// nothing is a drag then. macOS animates the space switch and reports every
// window on the display at a frame along the way, so those frames are
// neither compared nor remembered. The switch raises the event that lays
// the space out.
func (e *Engine) userDragged() (string, []uint32) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.onDrag || len(e.applied) == 0 {
		return "", nil
	}

	windows, displays, fullScreen, err := e.readDesktop()
	if err != nil {
		return "", nil
	}

	if inFullScreenTransition(windows.Windows, displays, fullScreen) {
		return "", nil
	}

	// A display plugged in or unplugged moves every window macOS has to
	// find a new home for, all at once and none of it the user's doing.
	// The desktop is laid out afresh instead, which places those windows
	// by the layout's own reckoning rather than by where they landed.
	if e.displays != "" && displaySignature(displays) != e.displays {
		return EventRelayout, nil
	}

	if time.Since(e.appliedAt) < e.resizeGrace {
		e.rememberFrames(windows)

		return "", nil
	}

	return e.draggedLocked(windows)
}

// displaySignature names a set of displays by their ids and frames, so a
// display added, removed, or moved in the arrangement reads as a change.
func displaySignature(displays []action.DisplayEntry) string {
	ids := make([]string, 0, len(displays))
	for _, display := range displays {
		ids = append(ids, fmt.Sprintf("%d:%v", display.ID, display.Frame))
	}

	slices.Sort(ids)

	return strings.Join(ids, " ")
}

// draggedLocked is the windows in windows that are no longer where the
// engine placed them, and whether, taken together, they were moved or
// resized. The caller holds the lock.
func (e *Engine) draggedLocked(windows action.WindowsInfo) (string, []uint32) {
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

	if len(dragged) == 0 {
		return "", nil
	}

	if resized {
		return string(events.WindowResize), dragged
	}

	return string(events.WindowMove), dragged
}

// rememberLater reads back where the windows the engine placed actually
// are, and keeps that. Nothing in the pass needs the answer, so the read
// runs after the pass has returned rather than holding it, and it keeps
// what it read only while the apply it was started for is the latest.
func (e *Engine) rememberLater(appliedAt time.Time) {
	e.background.Go(func() {
		// An animated apply returns as the windows set off, so the read
		// waits for them to land.
		if settle := e.animationSettle(); settle > 0 {
			time.Sleep(settle)
		}

		windows, err := e.readWindows()
		if err != nil {
			return
		}

		e.mu.Lock()
		defer e.mu.Unlock()

		if e.appliedAt.Equal(appliedAt) {
			e.rememberFrames(windows)
		}
	})
}

// rememberFrames keeps where the windows the engine placed are in windows.
// The caller holds the lock.
func (e *Engine) rememberFrames(windows action.WindowsInfo) {
	for _, win := range windows.Windows {
		if _, ok := e.applied[win.Number]; ok {
			e.applied[win.Number] = win.Frame
		}
	}
}

// inFullScreenTransition reports whether the desktop is switching to or from
// a full-screen space. Either a display shows one, or a window covers a
// display's whole frame, menu bar included. Only a full-screen window can do
// that, and it does while its space is not yet, or no longer, in front.
func inFullScreenTransition(
	windows []action.WindowEntry,
	displays []action.DisplayEntry,
	fullScreen map[uint32]bool,
) bool {
	for _, display := range displays {
		if fullScreen[display.ID] {
			return true
		}

		for _, win := range windows {
			if sameFrame(win.Frame, display.Frame) {
				return true
			}
		}
	}

	return false
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

// readDesktop reads the windows, the displays, and which displays show a
// full-screen space, in one turn of the serializer.
func (e *Engine) readDesktop() (action.WindowsInfo, []action.DisplayEntry, map[uint32]bool, error) {
	var (
		windows    action.WindowsInfo
		displays   []action.DisplayEntry
		fullScreen map[uint32]bool
	)

	err := e.run(func() error {
		var err error

		windows, err = e.desktop.Windows()
		if err != nil {
			return err
		}

		displays, err = e.desktop.Displays()
		if err != nil {
			return err
		}

		fullScreen, err = e.desktop.FullScreenDisplays()

		return err
	})

	return windows, displays, fullScreen, err
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
