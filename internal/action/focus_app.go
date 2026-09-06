package action

import (
	"slices"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// FocusApp brings an application's window to the front, switching to the
// space it is on first so that macOS has nothing left to animate.
//
// Which window depends on where focus is now. When the application is not in
// front, its most recently used window is the one the user last left, and is
// chosen. When the application is already in front, the command was run
// again to move on: the next window in a stable order is chosen, wrapping at
// the end, so repeated presses visit every window of the application once.
// That order is by space and then by the window server's number, which is
// creation order, so a cycle walks the spaces from left to right and never
// depends on which window was used last; see
// docs/adr/0004-focus-app-cycles-in-desktop-order.md.
//
// An application with no window is reopened, as a Dock click would, so it
// opens one and comes to the front.
func (e *Executor) FocusApp(query string) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	pid, err := e.desktop.FindApplication(query)
	if err != nil {
		return err
	}

	// The window in front is read before the application's windows are, so
	// that it is still a value of its own when the list replaces it; only its
	// application and its number are needed. An error here means nothing is
	// in front, which is a state, not a failure.
	current, currentErr := e.desktop.FrontmostWindow()

	windows, err := e.desktop.ApplicationWindows(pid)
	if err != nil {
		return derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to list the application's windows",
		)
	}

	if len(windows) == 0 {
		err = e.desktop.ReopenApplication(pid)
		if err != nil {
			return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to reopen application")
		}

		return nil
	}

	target := windows[0]

	if currentErr == nil && current.PID == pid {
		ordered := inDesktopOrder(windows)

		at := slices.IndexFunc(ordered, func(win AppWindow) bool {
			return win.Number != 0 && win.Number == current.Number
		})
		if at >= 0 {
			target = ordered[(at+1)%len(ordered)]
		}
	}

	switched, err := e.bringSpaceForward(target.SpaceIndex)
	if err != nil {
		return err
	}

	// The switch comes first because the window can only be raised once its
	// space is in front: Accessibility lists an application's windows on the
	// active space alone.
	err = e.desktop.RaiseWindow(pid, target.Number)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to raise window")
	}

	if switched {
		e.desktop.RefreshWorkspaceTitle()
	}

	return nil
}

// inDesktopOrder sorts an application's windows the way a cycle visits them:
// by the space they are on, left to right, then by the window server's
// number within a space. Windows on every space or none (index 0) come last.
// The sort is stable, so windows the desktop cannot number keep the order
// they arrived in.
func inDesktopOrder(windows []AppWindow) []AppWindow {
	ordered := slices.Clone(windows)

	slices.SortStableFunc(ordered, func(first, second AppWindow) int {
		switch {
		case first.SpaceIndex == second.SpaceIndex:
			return int(first.Number) - int(second.Number)
		case first.SpaceIndex == 0:
			return 1
		case second.SpaceIndex == 0:
			return -1
		default:
			return first.SpaceIndex - second.SpaceIndex
		}
	})

	return ordered
}

// bringSpaceForward switches to the space at index when it is not already in
// front, on the same terms as the space action, and reports whether it did.
// Index 0 names no particular space and asks for nothing.
func (e *Executor) bringSpaceForward(index int) (bool, error) {
	if index == 0 {
		return false, nil
	}

	active, err := e.desktop.ActiveSpaceIndex()
	if err != nil {
		return false, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to resolve the active space",
		)
	}

	if active == index {
		return false, nil
	}

	if e.desktop.MissionControlActive() {
		return false, derrors.New(
			derrors.CodeActionFailed,
			"cannot switch spaces while Mission Control is active",
		)
	}

	err = e.desktop.FocusSpace(index)
	if err != nil {
		return false, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to switch to space %d",
			index,
		)
	}

	return true, nil
}
