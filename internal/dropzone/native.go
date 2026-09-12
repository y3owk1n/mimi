package dropzone

import (
	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/native"
)

// nativeDrawer draws the zone with the window server, and nativeMouse asks
// it for the button.
type (
	nativeDrawer struct{}
	nativeMouse  struct{}
)

// NativeDrawer draws the zone on the desktop mimi runs on.
func NativeDrawer() Drawer {
	return nativeDrawer{}
}

// NativeMouse reads the real mouse.
func NativeMouse() Mouse {
	return nativeMouse{}
}

func (nativeDrawer) Show(frame geometry.Rect, style Style) {
	native.ShowDropzone(native.DropzoneStyle{
		Fill:    native.Color(style.Fill),
		Outline: native.Color(style.Outline),
		Width:   style.Width,
		Radius:  style.Radius,
	}, frame.X, frame.Y, frame.W, frame.H)
}

func (nativeDrawer) Hide() {
	native.HideDropzone()
}

func (nativeMouse) LeftButtonDown() bool {
	return native.LeftMouseButtonDown()
}
