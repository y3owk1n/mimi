package mousefocus

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/native"
)

// Desktop is what the engine reads and drives: where a point lands, which
// window is in front, whether a button is held, and how to focus a window.
type Desktop struct {
	WindowAt   func(native.Point) (int, uint32, bool)
	Frontmost  func() uint32
	ButtonDown func() bool
	Focus      func(number uint32) error
	Start      func() bool
	Stop       func()
}

// NativeDesktop is the desktop mimi runs on.
func NativeDesktop() Desktop {
	return Desktop{
		WindowAt:   native.WindowAtPoint,
		Frontmost:  native.FrontWindowNumber,
		ButtonDown: native.LeftMouseButtonDown,
		Focus:      action.FocusWindowNumber,
		Start:      native.StartMouseMonitor,
		Stop:       native.StopMouseMonitor,
	}
}

// rest is how long the pointer has to stay over a window before it is
// focused, so a pass across a window on the way somewhere else focuses
// nothing.
const rest = 40 * time.Millisecond

// Engine focuses the window under the pointer. It is safe to use from
// several goroutines: Update replaces the configuration under a lock.
type Engine struct {
	desktop   Desktop
	serialize func(func() error) error
	logger    *zap.SugaredLogger

	mu      sync.Mutex
	enabled bool
	running bool
	// last is the window the pointer was last seen over, so crossing into
	// the same window twice focuses it once.
	last uint32
}

// New builds an engine over desktop. serialize is where the focus runs,
// the daemon's action worker; nil runs it where the engine stands.
func New(desktop Desktop, serialize func(func() error) error, logger *zap.SugaredLogger) *Engine {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	if serialize == nil {
		serialize = func(fn func() error) error { return fn() }
	}

	return &Engine{desktop: desktop, serialize: serialize, logger: logger}
}

// Update applies the [mouse] section. The pointer tap runs only while
// focus_follows_mouse is on, so the setting costs nothing while off.
func (e *Engine) Update(cfg config.MouseConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.enabled = cfg.FocusFollowsMouse

	switch {
	case e.enabled && !e.running:
		e.running = e.desktop.Start()
		if !e.running {
			e.logger.Warnw("focus_follows_mouse is on but the pointer cannot be watched")
		}
	case !e.enabled && e.running:
		e.desktop.Stop()
		e.running = false
	}
}

// Run reads pointer positions from moves until ctx ends, focusing the
// window under the pointer once it has rested there.
func (e *Engine) Run(ctx context.Context, moves <-chan native.Point) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()

	var pending native.Point

	for {
		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case point := <-moves:
			pending = point

			timer.Reset(rest)
		case <-timer.C:
			e.settle(pending)
		}
	}
}

// settle focuses the window under point when the pointer has newly arrived
// on it and no button is down.
func (e *Engine) settle(point native.Point) {
	e.mu.Lock()
	enabled := e.enabled
	e.mu.Unlock()

	if !enabled || e.desktop.ButtonDown() {
		return
	}

	_, number, found := e.desktop.WindowAt(point)
	if !found {
		return
	}

	e.mu.Lock()
	crossed := number != e.last
	e.last = number
	e.mu.Unlock()

	if !crossed || number == e.desktop.Frontmost() {
		return
	}

	err := e.serialize(func() error { return e.desktop.Focus(number) })
	if err != nil {
		e.logger.Debugw("focus follows mouse", "window", number, "err", err)
	}
}
