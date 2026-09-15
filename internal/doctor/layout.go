package doctor

import (
	"context"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// LayoutRun is one layout command run on the sample input: how many frames
// it returned for how many windows, whether it printed nothing at all, and
// why it failed when it did.
type LayoutRun struct {
	Command string
	Frames  int
	Windows int
	Empty   bool
	Err     error
}

// RunLayout runs one layout command on a sample input of two windows on
// one display, as a one-shot program through shell, without reading the
// desktop. It says whether the layout answers before the daemon runs it.
func RunLayout(ctx context.Context, shell, command string, timeout time.Duration) LayoutRun {
	input := sampleInput()
	program := tiling.Program{Shell: shell, Command: command, Timeout: timeout}

	out, err := program.Reduce(ctx, input)
	run := LayoutRun{Command: command, Windows: len(input.Windows), Err: err}

	if err != nil {
		return run
	}

	run.Frames = len(out.Frames)
	run.Empty = out.Frames == nil && out.State == nil

	return run
}

// The sample desktop: one display of a MacBook's size with the menu bar
// taken off the top, and two windows splitting the rest.
const (
	sampleWidth   = 1440
	sampleHeight  = 900
	sampleMenuBar = 25
	sampleWindows = 2
)

// sampleInput is a preview event for two windows side by side on one
// display, the shape the daemon's first pass on a space sends.
func sampleInput() tiling.Input {
	display := action.DisplayEntry{
		Index: 1,
		ID:    1,
		Frame: action.Frame{Width: sampleWidth, Height: sampleHeight},
		Visible: action.Frame{
			Y:      sampleMenuBar,
			Width:  sampleWidth,
			Height: sampleHeight - sampleMenuBar,
		},
	}

	width := display.Visible.Width / sampleWindows
	windows := make([]action.WindowEntry, 0, sampleWindows)

	for index := range sampleWindows {
		windows = append(windows, action.WindowEntry{
			Number:   uint32(index + 1),
			PID:      index + 1,
			App:      "Sample",
			BundleID: "com.example.sample",
			Title:    "Window",
			Order:    index,
			Space:    1,
			Display:  1,
			Frame: action.Frame{
				X:      float64(index) * width,
				Y:      display.Visible.Y,
				Width:  width,
				Height: display.Visible.Height,
			},
		})
	}

	return tiling.Input{
		Version:  tiling.InputVersion,
		Event:    tiling.Event{Kind: tiling.EventPreview},
		Display:  display,
		Space:    1,
		Displays: []action.DisplayEntry{display},
		Windows:  windows,
		State:    []byte("null"),
	}
}
