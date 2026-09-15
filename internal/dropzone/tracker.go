package dropzone

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// Previewer answers where the dragged window would land now. The tiling
// engine is the real one.
type Previewer interface {
	DropPreview(ctx context.Context) (tiling.DropTarget, bool, error)
}

// Drawer puts the zone on screen and takes it off, and marks the window a
// drop would act on inside it.
type Drawer interface {
	Show(frame geometry.Rect, style Style)
	Hide()
	// ShowTarget marks a frame in the shown zone. Hide takes it down with
	// the zone. HideTarget takes it down alone.
	ShowTarget(frame geometry.Rect, style Style)
	HideTarget()
}

// Mouse reports whether the left button is down, which is what makes a
// window's movement a drag.
type Mouse interface {
	LeftButtonDown() bool
}

// Style is how the zone is drawn.
type Style struct {
	Fill    config.Color
	Outline config.Color
	Width   float64
	Radius  float64
}

// Tracker turns the raw moves of a drag into previews, throttled so a fast
// drag asks the layout no more than every previewInterval, and hides the
// zone once the button is up.
type Tracker struct {
	previewer Previewer
	draw      Drawer
	mouse     Mouse
	logger    *zap.SugaredLogger

	mu      sync.Mutex
	enabled bool
	style   Style
	// mark is how the window a drop acts on is marked.
	mark Style
	// previewing is whether a preview is running; nudges meanwhile are
	// dropped, since the next nudge comes before the drag has moved far.
	previewing bool
	// shown is whether the zone is on screen, and so whether the watcher
	// that hides it on release is running.
	shown bool
	last  time.Time
}

// previewInterval is the least time between two previews: a layout run
// and the reads before it cost some ten milliseconds, and a drag at a
// hundred previews a second would show nothing more.
const previewInterval = 40 * time.Millisecond

// releasePoll is how often the watcher looks for the button coming up.
const releasePoll = 30 * time.Millisecond

// New returns a tracker that is disabled until Update enables it.
func New(previewer Previewer, draw Drawer, mouse Mouse, logger *zap.SugaredLogger) *Tracker {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Tracker{previewer: previewer, draw: draw, mouse: mouse, logger: logger}
}

// Update applies the [tiling.dropzone] section. It is what the daemon
// calls at startup and on every reload. Disabling takes the zone down.
func (t *Tracker) Update(cfg config.DropzoneConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.enabled = cfg.Enabled
	t.style = styleOf(cfg)
	t.mark = targetStyleOf(cfg)

	if !cfg.Enabled && t.shown {
		t.shown = false
		t.draw.Hide()
	}
}

// Nudge is called for every raw move and resize, ahead of the debounce. It
// previews the drop when the button is down, no more than every
// previewInterval, and hides the zone when the preview finds no drag.
func (t *Tracker) Nudge() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled || t.previewing || time.Since(t.last) < previewInterval {
		return
	}

	if !t.mouse.LeftButtonDown() {
		t.logger.Debugw("dropzone: window moved with the button up")

		return
	}

	t.previewing = true
	t.last = time.Now()

	go t.preview()
}

// Close takes the zone down. The tracker is not used after it.
func (t *Tracker) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.enabled = false
	t.hideLocked()
}

// preview asks where the drag would land and draws the answer.
func (t *Tracker) preview() {
	target, found, err := t.previewer.DropPreview(context.Background())

	t.mu.Lock()
	defer t.mu.Unlock()

	t.previewing = false

	if err != nil {
		t.logger.Debugw("dropzone preview failed", "err", err)
	}

	if !t.enabled || !found || !t.mouse.LeftButtonDown() {
		t.logger.Debugw("dropzone: nothing to show", "enabled", t.enabled, "found", found)
		t.hideLocked()

		return
	}

	t.logger.Debugw("dropzone: showing", "window", target.Number)

	frame := target.Frame
	t.draw.Show(geometry.Rect{X: frame.X, Y: frame.Y, W: frame.Width, H: frame.Height}, t.style)

	if target.Target != nil {
		t.logger.Debugw("dropzone: marking target",
			"window", target.Target.Number, "action", target.Target.Action)

		frame := target.Target.Frame
		t.draw.ShowTarget(
			geometry.Rect{X: frame.X, Y: frame.Y, W: frame.Width, H: frame.Height},
			t.mark,
		)
	} else {
		t.draw.HideTarget()
	}

	if !t.shown {
		t.shown = true

		go t.watchRelease()
	}
}

// watchRelease hides the zone once the button comes up.
func (t *Tracker) watchRelease() {
	for {
		time.Sleep(releasePoll)

		t.mu.Lock()

		if !t.shown {
			t.mu.Unlock()

			return
		}

		if !t.mouse.LeftButtonDown() {
			t.hideLocked()
			t.mu.Unlock()

			return
		}

		t.mu.Unlock()
	}
}

// hideLocked takes the zone down when it is up. The caller holds the lock.
func (t *Tracker) hideLocked() {
	if t.shown {
		t.shown = false
		t.draw.Hide()
	}
}

// styleOf is the style a validated section describes.
func styleOf(cfg config.DropzoneConfig) Style {
	fill, _ := config.ParseColor(cfg.Color)
	outline, _ := config.ParseColor(cfg.OutlineColor)

	return Style{Fill: fill, Outline: outline, Width: cfg.OutlineWidth, Radius: cfg.Radius}
}

// targetStyleOf is how a validated section marks the window a drop acts
// on: its own colors, the same width and radius as the zone.
func targetStyleOf(cfg config.DropzoneConfig) Style {
	fill, _ := config.ParseColor(cfg.TargetColor)
	outline, _ := config.ParseColor(cfg.TargetOutlineColor)

	return Style{Fill: fill, Outline: outline, Width: cfg.OutlineWidth, Radius: cfg.Radius}
}
