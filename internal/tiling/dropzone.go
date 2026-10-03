package tiling

import (
	"context"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// slowPreview is how long a drop preview's read and layout may take before
// the drag lags behind the pointer: the drop zone asks for one every 40 ms.
const slowPreview = 40 * time.Millisecond

// DropTarget is where the window the user is dragging would land if they
// let go now: the window, by number, the frame the layout would give it,
// and the window the drop would act on when the layout named one.
type DropTarget struct {
	Number uint32
	Frame  action.Frame
	// Target is the window the layout said the drop acts on, where it is
	// now, or nil when the layout named none or named the dragged window.
	Target *Highlight
}

// Highlight is a window the drop zone marks while a drag is held: which,
// its frame on screen now, and the layout's word for what the drop does.
type Highlight struct {
	Number uint32
	Frame  action.Frame
	Action string
}

// DropPreview asks the layout where the window the user is dragging would
// land if dropped where it is now. It finds the drag the way a settled one
// is found, by the windows no longer where the engine placed them, runs the
// layout with the event the drop would raise, and reports the frame it
// returns for the dragged window. Nothing is applied and no state is kept.
// It reports false when nothing is being dragged, when relayout_on_drag is
// off, or when the layout leaves the window out.
func (e *Engine) DropPreview(ctx context.Context) (DropTarget, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled || !e.onDrag || len(e.programs) == 0 || len(e.applied) == 0 {
		return DropTarget{}, false, nil
	}

	// One read serves both the drag and the layout's input, with the
	// titles of the last full read: at every step of a drag, a round trip
	// into each application for a title, the dragged one's included, is
	// what made the drag stutter.
	readStart := time.Now()

	read, err := e.readInputsLocked(true)
	if err != nil {
		return DropTarget{}, false, err
	}

	readFor := time.Since(readStart)

	if inFullScreenTransition(read.windows.Windows, read.displays, read.fullScreen) {
		return DropTarget{}, false, nil
	}

	e.sampleModifiersLocked()

	kind, dragged := e.draggedLocked(read.windows)
	if len(dragged) == 0 {
		return DropTarget{}, false, nil
	}

	inputs := e.buildInputsLocked(Event{Kind: kind, Windows: dragged, Modifiers: e.dragMods}, read)

	layoutStart := time.Now()

	outputs, err := e.reduceAll(ctx, inputs)
	if err != nil {
		return DropTarget{}, false, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"previewing the drop",
		)
	}

	// A preview runs up to 25 times a second through a drag, so only one
	// slow enough to lag the drag is worth a line.
	if layoutFor := time.Since(layoutStart); readFor+layoutFor > slowPreview {
		e.logger.Debugw("drop preview slow",
			"kind", kind,
			"modifiers", e.dragMods,
			"read_ms", readFor.Milliseconds(),
			"layout_ms", layoutFor.Milliseconds(),
			"windows", len(read.windows.Windows),
		)
	}

	for _, output := range outputs {
		for _, frame := range output.Frames {
			if frame.Number == dragged[0] {
				target := DropTarget{Number: frame.Number, Frame: frame.Frame}
				target.Target = highlightOf(output.Target, dragged[0], read.windows.Windows)

				return target, true, nil
			}
		}
	}

	return DropTarget{}, false, nil
}

// highlightOf is the target a layout named, placed where that window is
// now, or nil when it named none, named the dragged window, or named a
// window the read did not list.
func highlightOf(target *Target, dragged uint32, windows []action.WindowEntry) *Highlight {
	if target == nil || target.Window == dragged {
		return nil
	}

	for _, win := range windows {
		if win.Number == target.Window {
			return &Highlight{Number: win.Number, Frame: win.Frame, Action: target.Action}
		}
	}

	return nil
}
