// Package stackbar draws the windows a layout stacked as a deck of cards, so
// that a place holding several of them does not look like one holding one.
//
// A stack is the layout's idea, not mimi's, and mimi changes no z-order to
// arrange it. macOS offers no way to raise one application's window above
// another's without also focusing it, so the window in front is whichever has
// focus. What is left invisible is that the others are there at all.
//
// The cards are drawn inside the frame the layout set aside, never around it.
// Reserve says how much of that frame they need, the engine takes it out of
// the window in front, and Show draws them in what is left, above the window
// for the members before it in the stack and below for the ones after. Where
// the window in front sits between them is where it sits in the stack.
package stackbar

import (
	"sync"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// Bar is one stack as it is drawn: the frame its windows share, how many
// there are, and which of them, counting from 0, is the one to mark.
type Bar struct {
	Frame action.Frame
	// Front is the window seen, which the cards are drawn under.
	Front uint32
	// Count is how many windows are in that place altogether, and Active
	// where the one in front sits among them, counting from 0. The windows
	// before it are drawn above and the ones after it below.
	Count  int
	Active int
}

// Drawer puts the cards on screen. The real one is the native overlay;
// the tests use a fake.
type Drawer interface {
	Sync(bars []Bar, style Style)
	Clear()
}

// Style is how the cards are drawn, with their colors already parsed.
type Style struct {
	Step     float64
	Taper    float64
	Radius   float64
	Color    Color
	FarColor Color
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
// cards off screen at once rather than waiting for the next pass, since
// there may not be one.
func (t *Tracker) Update(cfg config.StackbarConfig) {
	t.mu.Lock()

	style, err := styleOf(cfg)
	if err != nil {
		t.mu.Unlock()
		// The config was validated before it got here, so this is a
		// build that let an unparseable color through rather than
		// anything the user can fix.
		t.logger.Warnw("stack cards disabled: their colors do not parse", "err", err)

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

// Reserve is the height at the top and at the bottom of a stack's frame the
// cards need, which the engine takes out of the window in front. Nothing is
// reserved while the mark is switched off, so a desktop that does not draw
// stacks is laid out exactly as it was.
//
// How many cards fit is decided in the one place that draws them, so what is
// reserved here and what is drawn there can never disagree.
func (t *Tracker) Reserve(stack tiling.PlacedStack) (float64, float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled {
		return 0, 0
	}

	above, below := native.StackbarCards(
		len(stack.Windows), activeIndex(stack.Stack),
		stack.Frame.Width, stack.Frame.Height, nativeStyle(t.style),
	)

	return float64(above) * t.style.Step, float64(below) * t.style.Step
}

// Show draws exactly these stacks and no others. It is what the engine calls
// at the end of every pass, with every display's stacks at once.
func (t *Tracker) Show(stacks []tiling.PlacedStack) {
	t.mu.Lock()

	if !t.enabled {
		t.mu.Unlock()

		return
	}

	bars := make([]Bar, 0, len(stacks))

	for _, stack := range stacks {
		bars = append(bars, Bar{
			Frame:  stack.Frame,
			Front:  front(stack.Stack),
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

// front is the window the cards are drawn under: the one the layout means to
// be seen, or the first in the stack when it names one that is not in it.
func front(stack tiling.Stack) uint32 {
	return stack.Windows[activeIndex(stack)]
}

// activeIndex is where the window the layout means to be seen sits among the
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

	far, err := config.ParseColor(cfg.FarColor)
	if err != nil {
		return Style{}, err
	}

	return Style{
		Step:     cfg.Step,
		Taper:    cfg.Taper,
		Radius:   cfg.Radius,
		Color:    Color(color),
		FarColor: Color(far),
	}, nil
}
