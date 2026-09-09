package native

/*
#include "border.h"
*/
import "C"

// Color is a color with each channel from 0 to 1.
type Color struct {
	Red, Green, Blue, Alpha float64
}

// BorderStyle is how window borders are drawn: how wide, the radius of the
// window corner the border follows on its inside, or FollowWindowRadius to
// follow each window's own corner, and the colors for the focused window
// and for every other.
type BorderStyle struct {
	Width    float64
	Radius   float64
	Active   Color
	Inactive Color
}

// FollowWindowRadius is the Radius that follows each window's own corner
// as the window server reports it, which macOS 26 added; on earlier
// releases every window is drawn with the radius of a document window.
const FollowWindowRadius = -1

// SetBorderStyle draws borders under every real window on the spaces in
// front, in the given style, from now on. It restyles the borders already
// shown.
func SetBorderStyle(style BorderStyle) {
	cStyle := C.MimiBorderStyle{
		width:    C.double(style.Width),
		radius:   C.double(style.Radius),
		active:   cColor(style.Active),
		inactive: cColor(style.Inactive),
	}
	C.MimiBordersSetStyle(&cStyle)
}

// SyncBorders brings the borders up to date with the windows. refocus asks
// Accessibility which window is focused first; a drag leaves focus alone,
// so a caller reporting one passes false. Calls made before the last has
// drawn fold into it.
func SyncBorders(refocus bool) {
	C.MimiBordersSync(boolToInt(refocus))
}

// ClearBorders takes every border off the screen and draws none until the
// next SetBorderStyle.
func ClearBorders() {
	C.MimiBordersClear()
}

func cColor(color Color) C.MimiColor {
	return C.MimiColor{
		red:   C.double(color.Red),
		green: C.double(color.Green),
		blue:  C.double(color.Blue),
		alpha: C.double(color.Alpha),
	}
}
