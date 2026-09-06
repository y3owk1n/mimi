package action

import (
	"slices"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// MoveWindowToDisplay moves the frontmost window to the display target names,
// keeping the share of the visible frame it had, so a window tiled to the
// left half of one display is tiled to the left half of the other.
//
// Displays are counted left to right, then top to bottom, across every
// connected one. A window already on the destination stays where it is, which
// is also what "next" does on a machine with one display.
//
// The payload is checked first, as every branch of ExecuteCommand checks its
// own: on the direct path the constructor already did, on the daemon path
// this is the first thing that has looked at it.
func (e *Executor) MoveWindowToDisplay(target DisplayArg) error {
	err := validateDisplayArg(target)
	if err != nil {
		return err
	}

	err = e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	win, err := e.desktop.FrontmostWindow()
	if err != nil {
		return err
	}

	current, err := e.desktop.WindowFrame(win.ID)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get window frame")
	}

	// ScreenAt says which of the reads behind it failed, so this one passes
	// the error along rather than wrapping that detail out of sight. Only the
	// primary height is read from it here: the display list below is what
	// says where the window is.
	screen, err := e.desktop.ScreenAt(current)
	if err != nil {
		return err
	}

	displays, err := e.desktop.Displays()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to enumerate displays")
	}

	if len(displays) == 0 {
		return derrors.New(derrors.CodeActionFailed, "no displays found")
	}

	orderDisplays(displays)

	frames := make([]geometry.Rect, len(displays))
	for index, display := range displays {
		frames[index] = display.Frame
	}

	fromIndex, found := geometry.ScreenContaining(current, screen.PrimaryHeight, frames)
	if !found {
		return derrors.New(
			derrors.CodeActionFailed,
			"could not tell which display the frontmost window is on",
		)
	}

	toIndex, err := resolveDisplayArg(target, fromIndex, len(displays))
	if err != nil {
		return err
	}

	if toIndex == fromIndex {
		return nil
	}

	frame := geometry.MoveToScreen(
		current,
		screen.PrimaryHeight,
		displays[fromIndex].Visible,
		displays[toIndex].Visible,
	)

	err = e.desktop.SetWindowFrame(win.ID, frame)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to move window")
	}

	e.desktop.ActivateDisplay(displays[toIndex].ID)
	e.desktop.RefreshWorkspaceTitle()

	return nil
}

// orderDisplays sorts displays the way the display argument counts them: left
// to right by where each starts, and for two starting at the same x, top to
// bottom. Screen coordinates are y-up, so the higher y comes first.
func orderDisplays(displays []Display) {
	slices.SortStableFunc(displays, func(first, second Display) int {
		switch {
		case first.Frame.X < second.Frame.X:
			return -1
		case first.Frame.X > second.Frame.X:
			return 1
		case first.Frame.Y > second.Frame.Y:
			return -1
		case first.Frame.Y < second.Frame.Y:
			return 1
		default:
			return 0
		}
	})
}

// resolveDisplayArg turns a DisplayArg into the 0-based position, among count
// ordered displays, of the display it names. from is the 0-based position of
// the display the window is on, which "next" and "prev" step from, wrapping at
// both ends. An absolute number outside the range is invalid input, as an
// absolute space number is.
func resolveDisplayArg(target DisplayArg, from, count int) (int, error) {
	if target.Direction != 0 {
		return (from + target.Direction + count) % count, nil
	}

	if target.Index < 1 || target.Index > count {
		return 0, derrors.Newf(
			derrors.CodeInvalidInput,
			"display number %d is out of range; valid range is 1..%d",
			target.Index,
			count,
		)
	}

	return target.Index - 1, nil
}
