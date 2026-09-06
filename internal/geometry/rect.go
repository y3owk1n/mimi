package geometry

// Rect is a rectangle in points: an origin plus a size.
//
// Window rectangles use the Accessibility coordinate system — y-down, origin at
// the primary display's top-left — which is the system every frame the action
// layer reads from or writes to a window is expressed in.
type Rect struct {
	X float64
	Y float64
	W float64
	H float64
}

// SameFrame reports whether two frames are the same to within the slack a
// display can show: macOS stores window frames in whole points, so two frames
// half a point apart are one frame.
func SameFrame(first, second Rect) bool {
	return adjacent(first.X, second.X) && adjacent(first.Y, second.Y) &&
		adjacent(first.W, second.W) && adjacent(first.H, second.H)
}

// Right returns the x coordinate of the rectangle's right edge.
func (r Rect) Right() float64 {
	return r.X + r.W
}

// Bottom returns the y coordinate of the rectangle's bottom edge.
func (r Rect) Bottom() float64 {
	return r.Y + r.H
}

// CenterX returns the x coordinate of the rectangle's center.
func (r Rect) CenterX() float64 {
	return r.X + r.W/2
}

// CenterY returns the y coordinate of the rectangle's center.
func (r Rect) CenterY() float64 {
	return r.Y + r.H/2
}
