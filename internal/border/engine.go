package border

import (
	"context"
	"sync"
	"time"

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
	events.WindowMinimize:   true,
	events.WindowUnminimize: true,
	events.WorkspaceChanged: true,
	events.AXAttached:       true,
}

// settlingKinds are the events that can arrive before the window server lists
// the window they are about. An application reports a new window to
// Accessibility, and activates, before the window server has finished making
// it, so the sync the event wakes finds nothing to draw. The layout writes a
// tiled window's frame a moment later and that move syncs again. A window no
// layout places has no later event, and stays without a border until
// something else changes.
//
//nolint:gochecknoglobals // a fixed set
var settlingKinds = map[events.EventKind]bool{
	events.AppActivate:   true,
	events.AppUnhide:     true,
	events.WindowCreated: true,
	events.AXAttached:    true,
}

// settleAfter is when the engine syncs the borders again after a settling
// event. Each delay counts from the event, and a later settling event starts
// them over. A cold launch of Activity Monitor took most of a second.
//
//nolint:gochecknoglobals // a fixed schedule
var settleAfter = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
	700 * time.Millisecond,
	1500 * time.Millisecond,
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
// every event, and again a few times after a settling one, and takes them
// down at the end.
func (e *Engine) Run(ctx context.Context, sub events.Subscriber) {
	settle := time.NewTimer(time.Hour)
	settle.Stop()

	var (
		settledAt time.Time
		pending   []time.Duration
	)

	for {
		select {
		case <-ctx.Done():
			e.Close()

			return
		case evt, ok := <-sub:
			if !ok {
				return
			}

			// A settled move or resize is a drag's, and a drag moves no
			// focus; asking the application for its focused window would
			// only hold it up mid-drag.
			if e.Enabled() {
				e.draw.Sync(evt.Kind != events.WindowMove && evt.Kind != events.WindowResize)
			}

			if settlingKinds[evt.Kind] {
				settledAt = time.Now()
				pending = settleAfter
				settle.Reset(pending[0])
			}
		case <-settle.C:
			if e.Enabled() {
				e.draw.Sync(true)
			}

			pending = pending[1:]
			if len(pending) > 0 {
				settle.Reset(time.Until(settledAt.Add(pending[0])))
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
