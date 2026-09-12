package native

/*
#include <stdlib.h>
#include "stackbar.h"
*/
import "C"

// StackbarStyle is how a stack indicator is drawn: the color of a member
// that is not the active one, the color of the active one, the height of the
// bar and the corner radius of each segment, in points.
type StackbarStyle struct {
	Color       Color
	ActiveColor Color
	Height      float64
	Radius      float64
}

// Stackbar is one stack to mark: the frame its windows share, in window
// coordinates, how many windows are in it, and which of them, counting from
// 0, the layout means to be seen.
type Stackbar struct {
	Left   float64
	Top    float64
	Width  float64
	Height float64
	Count  int
	Active int
}

// SyncStackbars draws exactly these stacks and no others. Passing none takes
// every indicator off screen.
func SyncStackbars(bars []Stackbar, style StackbarStyle) {
	cStyle := C.MimiStackbarStyle{
		color:       cColor(style.Color),
		activeColor: cColor(style.ActiveColor),
		height:      C.double(style.Height),
		radius:      C.double(style.Radius),
	}

	if len(bars) == 0 {
		C.MimiStackbarsSync(nil, 0, &cStyle)

		return
	}

	// A MimiStackbar holds only numbers, so this array carries no Go
	// pointer and may cross into C as it is. The native side copies every
	// bar before it hands the work to the main thread, so nothing of Go's
	// is held past the call.
	cBars := make([]C.MimiStackbar, len(bars))
	for index, bar := range bars {
		cBars[index] = C.MimiStackbar{
			x:      C.double(bar.Left),
			y:      C.double(bar.Top),
			width:  C.double(bar.Width),
			height: C.double(bar.Height),
			count:  C.int(bar.Count),
			active: C.int(bar.Active),
		}
	}

	C.MimiStackbarsSync(&cBars[0], C.int(len(cBars)), &cStyle)
}

// ClearStackbars takes every indicator off screen.
func ClearStackbars() {
	C.MimiStackbarsClear()
}
