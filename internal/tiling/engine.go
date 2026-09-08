package tiling

import (
	"context"
	"encoding/json"
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
	layout  Layout
	settle  time.Duration
	// states is what the layout returned last time, keyed by space index.
	states map[int]json.RawMessage
}

// New returns an engine that is disabled until Update enables it.
func New(desktop Desktop, serialize Serializer, logger *zap.SugaredLogger) *Engine {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Engine{
		desktop:   desktop,
		serialize: serialize,
		logger:    logger,
		states:    map[int]json.RawMessage{},
	}
}

// Update applies the [tiling] section and the shell it runs under. It is
// what the daemon calls at startup and on every reload.
func (e *Engine) Update(cfg config.TilingConfig, shell string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.enabled = cfg.Enabled
	e.settle = time.Duration(cfg.DebounceMS) * time.Millisecond
	e.layout = Program{
		Shell:   shell,
		Command: cfg.Layout,
		Timeout: time.Duration(cfg.TimeoutSecs) * time.Second,
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
// make is the failure a layout engine exists to avoid.
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
}

// KindFilter is the bus filter for the engine's subscription: the waking
// kinds, and only while the engine is enabled, so a disabled engine costs the
// bus no sends.
func (e *Engine) KindFilter() events.KindFilter {
	return func(kind events.EventKind) bool {
		return wakingKinds[kind] && e.Enabled()
	}
}

// Run drains sub until ctx is done, settling each burst of events into one
// pass that reports the last event of the burst.
func (e *Engine) Run(ctx context.Context, sub events.Subscriber) {
	var (
		timer   *time.Timer
		fire    <-chan time.Time
		pending events.Event
	)

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

			pending = evt

			settle := e.settleWindow()
			if timer == nil {
				timer = time.NewTimer(settle)
				fire = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				timer.Reset(settle)
			}
		case <-fire:
			timer = nil
			fire = nil

			err := e.Pass(ctx, eventOf(pending))
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
