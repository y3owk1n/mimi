package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// defaultExecutor is what the package-level Execute runs on: the Executor
// bound to the desktop mimi is actually running on.
//
//nolint:gochecknoglobals // there is one machine, and one desktop on it
var defaultExecutor = NewExecutor(newNativeDesktop())

// Executor runs actions against one desktop.
//
// Every action goes through the desktop it holds, which is what lets the
// branch logic below be exercised without a Mac under it.
type Executor struct {
	desktop Desktop
}

// NewExecutor returns an Executor that drives the given desktop.
func NewExecutor(desktop Desktop) *Executor {
	return &Executor{desktop: desktop}
}

// FocusWindow cycles keyboard focus through focusable windows on the active
// space. When a direction is set ("up", "down", "left", "right"), it moves
// focus spatially to the nearest window in that direction. Otherwise it cycles
// forward or backward through the sorted window list. With SameApp, only the
// windows of the application owning the focused window take part, in either
// mode.
func (e *Executor) FocusWindow(args FocusWindowArgs) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	windows, focusedIndex, err := e.desktop.FocusableWindows()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get focusable windows")
	}

	if len(windows) == 0 {
		return derrors.New(
			derrors.CodeActionFailed,
			"no focusable windows found on the active space",
		)
	}

	if args.SameApp {
		windows, focusedIndex, err = sameApplicationOnly(windows, focusedIndex)
		if err != nil {
			return err
		}
	}

	if args.Direction != "" {
		dir, err := parseFocusDirection(args.Direction)
		if err != nil {
			return err
		}

		return e.focusDirectional(windows, focusedIndex, dir)
	}

	targetIndex := cycleTarget(focusedIndex, len(windows), args.Backward)

	err = e.desktop.ActivateWindow(windows[targetIndex].ID)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to activate window")
	}

	return nil
}

// FocusSpace focuses the Mission Control space at the given 1-based index.
func (e *Executor) FocusSpace(index int) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	if e.desktop.MissionControlActive() {
		return derrors.New(
			derrors.CodeActionFailed,
			"cannot switch spaces while Mission Control is active",
		)
	}

	err = e.ensureSpaceExists(index)
	if err != nil {
		return err
	}

	err = e.desktop.FocusSpace(index)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to focus space")
	}

	e.desktop.RefreshWorkspaceTitle()

	return nil
}

// MoveWindowToSpace moves the frontmost window to the space at the given
// 1-based index, and with follow set switches to that space afterwards.
//
// The switch is the same one the space action makes, so a window that was
// moved but could not be followed is reported as such and stays where it was
// moved to: the move landed, and undoing it would be a second surprise.
func (e *Executor) MoveWindowToSpace(index int, follow bool) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	if e.desktop.MissionControlActive() {
		return derrors.New(
			derrors.CodeActionFailed,
			"cannot move window while Mission Control is active",
		)
	}

	err = e.ensureSpaceExists(index)
	if err != nil {
		return err
	}

	err = e.desktop.MoveWindowToSpace(index)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to move window")
	}

	if follow {
		err = e.desktop.FocusSpace(index)
		if err != nil {
			e.desktop.RefreshWorkspaceTitle()

			return derrors.Wrapf(
				err,
				derrors.CodeActionFailed,
				"window moved, but failed to follow it to space %d",
				index,
			)
		}
	}

	e.desktop.RefreshWorkspaceTitle()

	return nil
}

// ResizeWindow resizes and repositions the frontmost window to satisfy req.
//
// It reads the window and its screen from the desktop, hands both to the pure
// geometry, and writes back the frame it returns.
func (e *Executor) ResizeWindow(req geometry.Request) error {
	err := e.desktop.EnsureAccessible()
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
	// the error along rather than wrapping that detail out of sight.
	screen, err := e.desktop.ScreenAt(current)
	if err != nil {
		return err
	}

	return e.desktop.SetWindowFrame(win.ID, geometry.Resize(current, screen, req))
}

// sameApplicationOnly narrows windows to those owned by the same application
// as windows[focusedIndex], keeping their order, and reports where the focused
// window now sits. It is the focused window that names the application: with
// nothing focused there is no application to stay within, and a window whose
// owner cannot be read (pid 0) names none either.
func sameApplicationOnly(windows []Window, focusedIndex int) ([]Window, int, error) {
	if focusedIndex < 0 || focusedIndex >= len(windows) {
		return nil, -1, derrors.New(
			derrors.CodeActionFailed,
			"no focused window, so no application to stay within",
		)
	}

	pid := windows[focusedIndex].PID
	if pid == 0 {
		return nil, -1, derrors.New(
			derrors.CodeActionFailed,
			"cannot tell which application owns the focused window",
		)
	}

	kept := make([]Window, 0, len(windows))
	keptFocused := -1

	for index, win := range windows {
		if win.PID != pid {
			continue
		}

		if index == focusedIndex {
			keptFocused = len(kept)
		}

		kept = append(kept, win)
	}

	return kept, keptFocused, nil
}

// ensureSpaceExists is the one range check both space actions share. A space
// number outside the Mission Control range is invalid input, and stays invalid
// input — neither caller may wrap it into something else.
func (e *Executor) ensureSpaceExists(index int) error {
	count := e.desktop.SpaceCount()
	if count == 0 {
		return derrors.New(derrors.CodeActionFailed, "failed to enumerate Mission Control spaces")
	}

	if index < 1 || index > count {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number %d is out of range; valid range is 1..%d",
			index,
			count,
		)
	}

	return nil
}

// focusDirectional moves focus to the window nearest windows[currentIndex] in
// the given direction. currentIndex may be -1 when no window is focused.
func (e *Executor) focusDirectional(
	windows []Window,
	currentIndex int,
	dir geometry.Direction,
) error {
	// A window whose frame cannot be read stays nil, which keeps every index
	// here an index into windows.
	frames := make([]*geometry.Rect, len(windows))

	for index, win := range windows {
		frame, err := e.desktop.WindowFrame(win.ID)
		if err != nil {
			continue
		}

		frames[index] = &frame
	}

	if currentIndex < 0 || currentIndex >= len(frames) || frames[currentIndex] == nil {
		return derrors.New(
			derrors.CodeActionFailed,
			"current window not found; cannot determine spatial navigation target",
		)
	}

	target, found := geometry.Nearest(frames, currentIndex, dir)
	if !found {
		return derrors.New(
			derrors.CodeActionFailed,
			"no window found in that direction",
		)
	}

	return e.desktop.ActivateWindow(windows[target].ID)
}

// cycleTarget is the index focus moves to when cycling through count windows
// from focusedIndex, which is -1 when no window holds focus.
func cycleTarget(focusedIndex, count int, backward bool) int {
	switch {
	case focusedIndex < 0:
		// No focused window found in the list (e.g. the frontmost app has no
		// focusable AX window). Default to the first window.
		return 0
	case backward:
		if focusedIndex-1 < 0 {
			return count - 1
		}

		return focusedIndex - 1
	default:
		if focusedIndex+1 >= count {
			return 0
		}

		return focusedIndex + 1
	}
}
