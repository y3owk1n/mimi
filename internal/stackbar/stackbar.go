// Package stackbar marks the places where a layout put several windows in
// one frame, so the ones behind the top one can be seen to be there.
//
// A stack is the layout's idea, not mimi's: the engine gives every member the
// frame the layout returned and changes no z-order, because macOS offers no
// way to raise one application's window above another's without also focusing
// it. What is left invisible is that there is more than one window in that
// place at all, and this is what says so.
package stackbar

import (
	"sync"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// Bar is one stack as it is drawn: the frame its windows share, how many
// there are, and which of them, counting from 0, is the one to mark.
type Bar struct {
	Frame  action.Frame
	Count  int
	Active int
}

// Drawer puts the indicators on screen. The real one is the native overlay;
// the tests use a fake.
type Drawer interface {
	Sync(bars []Bar, style Style)
	Clear()
}

// Style is how an indicator is drawn, with its colors already parsed.
type Style struct {
	Color       Color
	ActiveColor Color
	Height      float64
	Radius      float64
}

// Color is one parsed color, each part from 0 to 1.
type Color struct {
	Red, Green, Blue, Alpha float64
}

// Tracker draws the stacks the engine reports. It is safe to use from several
// goroutines: the engine reports from a pass, and a reload replaces the style
// from the signal loop.
type Tracker struct {
	draw   Drawer
	logger *zap.SugaredLogger

	mu      sync.Mutex
	enabled bool
	style   Style
	// shown is how many bars are on screen, so a report of none after none
	// draws nothing rather than calling into the window server again.
	shown int
}

// New returns a tracker that draws nothing until Update enables it.
func New(draw Drawer, logger *zap.SugaredLogger) *Tracker {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Tracker{draw: draw, logger: logger}
}

// Update applies the [tiling.stackbar] section. Switching it off takes every
// indicator off screen at once rather than waiting for the next pass, since
// there may not be one.
func (t *Tracker) Update(cfg config.StackbarConfig) {
	t.mu.Lock()

	style, err := styleOf(cfg)
	if err != nil {
		t.mu.Unlock()
		// The config was validated before it got here, so this is a
		// build that let an unparseable color through rather than
		// anything the user can fix.
		t.logger.Warnw("stack indicator disabled: its colors do not parse", "err", err)

		return
	}

	wasEnabled := t.enabled
	t.enabled, t.style = cfg.Enabled, style

	switchedOff := wasEnabled && !cfg.Enabled && t.shown > 0
	if switchedOff {
		t.shown = 0
	}

	t.mu.Unlock()

	if switchedOff {
		t.draw.Clear()
	}
}

// Sync draws exactly these stacks and no others. It is what the engine calls
// at the end of every pass, with every display's stacks at once.
func (t *Tracker) Sync(stacks []tiling.PlacedStack) {
	t.mu.Lock()

	if !t.enabled {
		t.mu.Unlock()

		return
	}

	bars := make([]Bar, 0, len(stacks))

	for _, stack := range stacks {
		bars = append(bars, Bar{
			Frame:  stack.Frame,
			Count:  len(stack.Windows),
			Active: activeIndex(stack.Stack),
		})
	}

	if len(bars) == 0 && t.shown == 0 {
		t.mu.Unlock()

		return
	}

	style := t.style
	t.shown = len(bars)

	t.mu.Unlock()

	t.draw.Sync(bars, style)
}

// activeIndex is where the member the layout means to be seen sits among the
// stack's windows, or 0 when it names one that is not in the stack.
func activeIndex(stack tiling.Stack) int {
	for index, number := range stack.Windows {
		if number == stack.Active {
			return index
		}
	}

	return 0
}

// styleOf parses the colors in the config once, so a pass never parses one.
func styleOf(cfg config.StackbarConfig) (Style, error) {
	color, err := config.ParseColor(cfg.Color)
	if err != nil {
		return Style{}, err
	}

	active, err := config.ParseColor(cfg.ActiveColor)
	if err != nil {
		return Style{}, err
	}

	return Style{
		Color:       Color(color),
		ActiveColor: Color(active),
		Height:      cfg.Height,
		Radius:      cfg.Radius,
	}, nil
}
