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
