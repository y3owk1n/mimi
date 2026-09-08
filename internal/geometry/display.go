package geometry

import "math"

// WindowBounds converts a frame from screen (y-up) coordinates to window
// (y-down) coordinates. The top edge is the one line that converts between
// the two systems:
//
//	y-down top = primaryHeight - visibleFrameY - visibleFrameHeight
func WindowBounds(visible Rect, primaryHeight float64) Rect {
	return Rect{
		X: visible.X,
		Y: primaryHeight - visible.Y - visible.H,
		W: visible.W,
		H: visible.H,
	}
}

// MoveToScreen returns the frame a window at cur takes when it moves from the
// display whose visible frame is from to the one whose visible frame is to,
// keeping the share of the visible frame it had: a window filling the left
// half of one display fills the left half of the other, whatever their sizes.
//
// cur and the returned frame are in window coordinates; from and to are in
// screen coordinates, as macOS reports them. The frame comes back in whole
// points, which is how macOS stores it: rounding here rather than leaving it
// to the application keeps a window that goes there and back from drifting a
// point each way.
func MoveToScreen(cur Rect, primaryHeight float64, from, to Rect) Rect {
	src := WindowBounds(from, primaryHeight)
	dst := WindowBounds(to, primaryHeight)

	return Rect{
		X: math.Round(dst.X + (cur.X-src.X)/src.W*dst.W),
		Y: math.Round(dst.Y + (cur.Y-src.Y)/src.H*dst.H),
		W: math.Round(cur.W / src.W * dst.W),
		H: math.Round(cur.H / src.H * dst.H),
	}
}

// ScreenContaining reports which of the given full frames holds the center of
// cur, and false when none does. Frames are in screen coordinates and cur in
// window coordinates; the center is what decides, so a window straddling two
// displays belongs to the one showing more of it.
func ScreenContaining(cur Rect, primaryHeight float64, frames []Rect) (int, bool) {
	centerX, centerY := cur.CenterX(), cur.CenterY()

	for index, frame := range frames {
		bounds := WindowBounds(frame, primaryHeight)
		if centerX >= bounds.X && centerX < bounds.Right() &&
			centerY >= bounds.Y && centerY < bounds.Bottom() {
			return index, true
		}
	}

	return 0, false
}
