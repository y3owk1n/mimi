package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
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
