package tiling

import (
	"context"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// DropTarget is where the window the user is dragging would land if they
// let go now: the window, by number, and the frame the layout would give
// it.
type DropTarget struct {
	Number uint32
	Frame  action.Frame
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

	if !e.enabled || !e.onDrag || e.layout == nil || len(e.applied) == 0 {
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

	kind, dragged := e.draggedLocked(read.windows)
	if len(dragged) == 0 {
		return DropTarget{}, false, nil
	}

	inputs := e.buildInputsLocked(Event{Kind: kind, Windows: dragged}, read)

	layoutStart := time.Now()

	outputs, err := e.reduceAll(ctx, inputs)
	if err != nil {
		return DropTarget{}, false, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"previewing the drop",
		)
	}

	e.logger.Debugw("drop preview",
		"read_ms", readFor.Milliseconds(),
		"layout_ms", time.Since(layoutStart).Milliseconds(),
		"windows", len(read.windows.Windows),
	)

	for _, output := range outputs {
		for _, frame := range output.Frames {
			if frame.Number == dragged[0] {
				return DropTarget{Number: frame.Number, Frame: frame.Frame}, true, nil
			}
		}
	}

	return DropTarget{}, false, nil
}
