package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/space"
	"github.com/y3owk1n/mimi/internal/window"
)

func ensureAccessibility() error {
	return permissions.FriendlyError(permissions.Check())
}

// FocusWindow cycles keyboard focus through focusable windows on the active space.
func FocusWindow(backward bool) error {
	err := ensureAccessibility()
	if err != nil {
		return err
	}

	windows, err := window.AllFocusableOnActiveSpace()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get focusable windows")
	}

	if len(windows) == 0 {
		return derrors.New(
			derrors.CodeActionFailed,
			"no focusable windows found on the active space",
		)
	}

	defer window.ReleaseAll(windows)

	frontmost := window.Frontmost()

	currentIndex := -1
	if frontmost != nil {
		for i, w := range windows {
			if w.Equal(frontmost) {
				currentIndex = i

				break
			}
		}

		frontmost.Release()
	}

	var targetIndex int
	if backward {
		targetIndex = currentIndex - 1
		if targetIndex < 0 {
			targetIndex = len(windows) - 1
		}
	} else {
		targetIndex = currentIndex + 1
		if targetIndex >= len(windows) {
			targetIndex = 0
		}
	}

	err = windows[targetIndex].Activate()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to activate window")
	}

	return nil
}

// FocusSpace focuses the Mission Control space at the given 1-based index.
func FocusSpace(index int) error {
	err := ensureAccessibility()
	if err != nil {
		return err
	}

	if window.MissionControlActive() {
		return derrors.New(
			derrors.CodeActionFailed,
			"cannot switch spaces while Mission Control is active",
		)
	}

	err = space.Focus(index)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to focus space")
	}

	return nil
}

// MoveWindowToSpace moves the frontmost window to the space at the given 1-based index.
func MoveWindowToSpace(index int) error {
	err := ensureAccessibility()
	if err != nil {
		return err
	}

	if window.MissionControlActive() {
		return derrors.New(
			derrors.CodeActionFailed,
			"cannot move window while Mission Control is active",
		)
	}

	err = space.MoveWindow(index)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to move window")
	}

	return nil
}
