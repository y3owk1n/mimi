package stackbar

import (
	"github.com/y3owk1n/mimi/internal/native"
)

// nativeDrawer draws the cards with the window server.
type nativeDrawer struct{}

// NativeDrawer draws the cards on the desktop mimi runs on.
func NativeDrawer() Drawer {
	return nativeDrawer{}
}

func (nativeDrawer) Sync(bars []Bar, style Style) {
	native.SyncStackbars(barsOf(bars), nativeStyle(style))
}

// nativeStyle is the style in the shape the window server side takes it.
func nativeStyle(style Style) native.StackbarStyle {
	return native.StackbarStyle{
		Step:     style.Step,
		Taper:    style.Taper,
		Radius:   style.Radius,
		Color:    native.Color(style.Color),
		FarColor: native.Color(style.FarColor),
	}
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
			Front:  bar.Front,
			Count:  bar.Count,
			Active: bar.Active,
		}
	}

	return drawn
}
