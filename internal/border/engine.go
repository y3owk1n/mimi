package border

import (
	"context"
	"sync"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/native"
)

// Drawer is what the engine needs from the screen. It sets the style the
// borders are drawn in, brings them up to date with the windows, and takes
// them down. The daemon uses the native drawer and the tests use a fake.
type Drawer struct {
	SetStyle func(native.BorderStyle)
	Sync     func(refocus bool)
	Clear    func()
}

// NativeDrawer draws on the screen.
func NativeDrawer() Drawer {
	return Drawer{
		SetStyle: native.SetBorderStyle,
		Sync:     native.SyncBorders,
		Clear:    native.ClearBorders,
	}
}

// Engine keeps the borders in step with the windows. It is safe to use from
// several goroutines: Update replaces the configuration under a lock, and
// Run and Nudge read it under the same lock.
type Engine struct {
	draw Drawer

	mu      sync.Mutex
	enabled bool
	style   native.BorderStyle
}

// New returns an engine that draws nothing until Update enables it.
func New(draw Drawer) *Engine {
	return &Engine{draw: draw}
}

// Update applies the [border] section. It is what the daemon calls at
// startup and on every reload. Enabling draws the borders at once;
// disabling takes them down; a change of style redraws them.
func (e *Engine) Update(cfg config.BorderConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	wasEnabled, hadStyle := e.enabled, e.style

	e.enabled = cfg.Enabled
	e.style = styleOf(cfg)

	switch {
	case !cfg.Enabled && wasEnabled:
		e.draw.Clear()
	case cfg.Enabled && (!wasEnabled || hadStyle != e.style):
		e.draw.SetStyle(e.style)
	}
}

// styleOf is the config as the drawer takes it. The colors were validated
// with the config, so a color that fails to parse here is a programming
// error and draws as clear.
func styleOf(cfg config.BorderConfig) native.BorderStyle {
	active, _ := config.ParseColor(cfg.ActiveColor)
	inactive, _ := config.ParseColor(cfg.InactiveColor)

	return native.BorderStyle{
		Width:    cfg.Width,
		Radius:   cfg.CornerRadius(),
		Active:   native.Color(active),
		Inactive: native.Color(inactive),
	}
}

// Enabled reports whether borders are drawn.
func (e *Engine) Enabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.enabled
}

// wakingKinds are the events after which the windows, their stacking or
// the focus may differ from what the borders show.
//
//nolint:gochecknoglobals // a fixed set
var wakingKinds = map[events.EventKind]bool{
	events.Startup:          true,
	events.AppActivate:      true,
	events.AppDeactivate:    true,
	events.AppHide:          true,
	events.AppUnhide:        true,
	events.AppQuit:          true,
	events.WindowFocus:      true,
	events.WindowCreated:    true,
	events.WindowClosed:     true,
	events.WindowMove:       true,
	events.WindowResize:     true,
	events.WorkspaceChanged: true,
	events.AXAttached:       true,
}

// KindFilter is the bus filter for the engine's subscription: the waking
// kinds, and only while the engine is enabled, so a disabled engine costs
// the bus no sends.
func (e *Engine) KindFilter() events.KindFilter {
	return func(kind events.EventKind) bool {
		return wakingKinds[kind] && e.Enabled()
	}
}

// Run drains sub until ctx is done, bringing the borders up to date after
// every event, and takes them down at the end.
func (e *Engine) Run(ctx context.Context, sub events.Subscriber) {
	for {
		select {
		case <-ctx.Done():
			e.Close()

			return
		case _, ok := <-sub:
			if !ok {
				return
			}

			if e.Enabled() {
				e.draw.Sync(true)
			}
		}
	}
}

// Nudge follows a window being dragged: the borders are brought up to date
// with the frames alone, since a drag moves no focus. The router calls it
// for every raw move and resize, ahead of the debounce.
func (e *Engine) Nudge() {
	if e.Enabled() {
		e.draw.Sync(false)
	}
}

// Close takes the borders down. The engine is not used after it.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enabled {
		e.enabled = false
		e.draw.Clear()
	}
}
