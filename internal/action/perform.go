package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/space"
	"github.com/y3owk1n/mimi/internal/window"
)

func ensureAccessibility() error {
	return permissions.FriendlyError(permissions.Check())
}

// FocusWindow cycles keyboard focus through focusable windows on the active space.
// When direction is set ("up", "down", "left", "right"), it moves focus spatially
// to the nearest window in that direction. Otherwise it cycles forward or backward
// through the sorted window list.
func FocusWindow(backward bool, direction string) error {
	err := ensureAccessibility()
	if err != nil {
		return err
	}

	windows, focusedIndex, err := window.AllFocusableOnActiveSpaceWithFocused()
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

	if direction != "" {
		dir, ok := directionFromFlag(direction)
		if !ok {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"unknown direction %q (use up, down, left, or right)",
				direction,
			)
		}

		return focusDirectional(windows, focusedIndex, dir)
	}

	var targetIndex int
	switch {
	case focusedIndex < 0:
		// No focused window found in the list (e.g. the frontmost app
		// has no focusable AX window). Default to the first window.
		targetIndex = 0
	case backward:
		targetIndex = focusedIndex - 1
		if targetIndex < 0 {
			targetIndex = len(windows) - 1
		}
	default:
		targetIndex = focusedIndex + 1
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

// directionFromFlag maps a focus_window direction flag onto the geometry
// direction it navigates in.
func directionFromFlag(flag string) (geometry.Direction, bool) {
	switch flag {
	case "up":
		return geometry.Up, true
	case "down":
		return geometry.Down, true
	case "left":
		return geometry.Left, true
	case "right":
		return geometry.Right, true
	default:
		return 0, false
	}
}

// focusDirectional moves focus to the window nearest windows[currentIndex] in
// the given direction. currentIndex may be -1 when no window is focused.
func focusDirectional(
	windows []*window.Element,
	currentIndex int,
	dir geometry.Direction,
) error {
	// A window whose frame cannot be read stays nil, which keeps every index
	// here an index into windows.
	frames := make([]*geometry.Rect, len(windows))

	for index, win := range windows {
		posX, posY, winW, winH, err := win.GetFrame()
		if err != nil {
			continue
		}

		frames[index] = &geometry.Rect{X: posX, Y: posY, W: winW, H: winH}
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

	return windows[target].Activate()
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

// ResizeWindow resizes and repositions the frontmost window to satisfy req.
//
// It reads the window and its screen from macOS, hands both to the pure
// geometry, and writes back the frame it returns.
func ResizeWindow(req geometry.Request) error {
	err := ensureAccessibility()
	if err != nil {
		return err
	}

	win := window.Frontmost()
	if win == nil {
		return derrors.New(derrors.CodeActionFailed, "no active window found")
	}
	defer win.Release()

	curX, curY, curW, curH, err := win.GetFrame()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get window frame")
	}

	current := geometry.Rect{X: curX, Y: curY, W: curW, H: curH}

	screen, err := screenAt(current)
	if err != nil {
		return err
	}

	target := geometry.Resize(current, screen, req)

	return win.SetFrame(target.X, target.Y, target.W, target.H)
}

// screenAt reads everything the geometry has to know about the display the
// given window is on.
func screenAt(current geometry.Rect) (geometry.Screen, error) {
	// The visible frame of the screen containing the window, in NSScreen y-up
	// coordinates.
	visX, visY, visW, visH, err := window.ScreenVisibleFrame(current.X, current.Y)
	if err != nil {
		return geometry.Screen{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get screen frame",
		)
	}

	// The primary screen's height, which is the constant relating the y-up
	// screen coordinates to the y-down window ones. Resize applies the
	// conversion itself.
	primaryH, err := window.PrimaryScreenHeight()
	if err != nil {
		return geometry.Screen{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get primary screen height",
		)
	}

	return geometry.Screen{
		Visible:        geometry.Rect{X: visX, Y: visY, W: visW, H: visH},
		PrimaryHeight:  primaryH,
		MarginsEnabled: window.TiledWindowMarginsEnabled(),
		MarginSize:     window.TiledWindowMarginSize(),
	}, nil
}
