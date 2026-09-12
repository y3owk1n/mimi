package stackbar

import (
	"github.com/y3owk1n/mimi/internal/native"
)

// nativeDrawer draws the indicators with the window server.
type nativeDrawer struct{}

// NativeDrawer draws the indicators on the desktop mimi runs on.
func NativeDrawer() Drawer {
	return nativeDrawer{}
}

func (nativeDrawer) Sync(bars []Bar, style Style) {
	native.SyncStackbars(barsOf(bars), native.StackbarStyle{
		Color:       native.Color(style.Color),
		ActiveColor: native.Color(style.ActiveColor),
		Height:      style.Height,
		Radius:      style.Radius,
	})
}

func (nativeDrawer) Clear() {
	native.ClearStackbars()
}

// barsOf is the bars in the shape the window server takes them.
func barsOf(bars []Bar) []native.Stackbar {
	drawn := make([]native.Stackbar, len(bars))
	for index, bar := range bars {
		drawn[index] = native.Stackbar{
			Left:   bar.Frame.X,
			Top:    bar.Frame.Y,
			Width:  bar.Frame.Width,
			Height: bar.Frame.Height,
			Count:  bar.Count,
			Active: bar.Active,
		}
	}

	return drawn
}
