package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/space"
	"github.com/y3owk1n/mimi/internal/window"
)

const (
	percentage100  = 100.0
	divisionFactor = 2.0
	marginDivisor  = 2
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

// ResizeWindow resizes and repositions the frontmost window according to the given args.
func ResizeWindow(args parsedResizeWindowArgs) error {
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

	// Get the visible frame of the screen containing the window (in NSScreen y-up coords)
	screenX, screenY, screenWidth, screenHeight, err := window.ScreenVisibleFrame(curX, curY)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get screen frame")
	}

	// Get the primary screen height for AX ↔ NSScreen coordinate conversion.
	// AX uses y-down (top-left origin at the primary screen's top). NSScreen uses
	// y-up (bottom-left origin at the primary screen's bottom). The primary screen's
	// height is the constant that relates the two systems.
	primaryH, err := window.PrimaryScreenHeight()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get primary screen height")
	}

	// Convert the visible frame's top edge from NSScreen y-up to AX y-down:
	//   y-down top = primaryScreenHeight - visibleFrameY - visibleFrameHeight
	syd := primaryH - screenY - screenHeight

	// Determine if margins should be applied
	useMargins := window.TiledWindowMarginsEnabled()
	if args.useMargin != nil {
		useMargins = *args.useMargin
	}

	// Compute target dimensions from screen visible frame (in y-down)
	newW := curW
	switch {
	case args.width > 0:
		newW = float64(args.width)
	case args.widthPct > 0:
		newW = screenWidth * args.widthPct / percentage100
	}

	newH := curH
	switch {
	case args.height > 0:
		newH = float64(args.height)
	case args.heightPct > 0:
		newH = screenHeight * args.heightPct / percentage100
	}

	// Compute target position from anchor (relative to visible frame in y-down)
	vert := args.anchor[0]  // 't', 'c', 'b'
	horiz := args.anchor[1] // 'l', 'c', 'r'

	var targetX, targetY float64

	if args.hasX {
		targetX = float64(args.x)
	} else {
		switch horiz {
		case 'l':
			targetX = screenX
		case 'c':
			targetX = screenX + (screenWidth-newW)/divisionFactor
		case 'r':
			targetX = screenX + screenWidth - newW
		}
	}

	if args.hasY {
		targetY = float64(args.y)
	} else {
		switch vert {
		case 't':
			targetY = syd
		case 'c':
			targetY = syd + (screenHeight-newH)/divisionFactor
		case 'b':
			targetY = syd + screenHeight - newH
		}
	}

	// Apply margins: full margin on edges that abut the visible frame boundary,
	// half margin on internal edges (split between windows). This matches macOS
	// behavior where the gap between two tiled windows is margin (not 2*margin).
	if useMargins {
		marginSize := window.TiledWindowMarginSize()

		leftExt := targetX == screenX
		rightExt := targetX+newW == screenX+screenWidth
		topExt := targetY == syd
		botExt := targetY+newH == syd+screenHeight

		leftMargin := marginSize
		if !leftExt {
			leftMargin = marginSize / marginDivisor
		}

		rightMargin := marginSize
		if !rightExt {
			rightMargin = marginSize / marginDivisor
		}

		topMargin := marginSize
		if !topExt {
			topMargin = marginSize / marginDivisor
		}

		bottomMargin := marginSize
		if !botExt {
			bottomMargin = marginSize / marginDivisor
		}

		targetX += leftMargin
		newW -= leftMargin + rightMargin
		targetY += topMargin
		newH -= topMargin + bottomMargin
	}

	return win.SetFrame(targetX, targetY, newW, newH)
}
