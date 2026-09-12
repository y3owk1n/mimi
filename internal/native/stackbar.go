package native

/*
#include "stackbar.h"
*/
import "C"

// StackbarStyle is how the windows behind the one in front are drawn: how
// much of each shows above the one in front of it, how much narrower each is
// on either side, the corner radius to follow when the window's own cannot be
// read, and the colors of the nearest card and of the furthest. Sizes are in
// points.
type StackbarStyle struct {
	Step     float64
	Taper    float64
	Radius   float64
	Color    Color
	FarColor Color
}

// Stackbar is one stack to show: the frame its windows share, in window
// coordinates, the window seen, and how many windows are in that place.
type Stackbar struct {
	Left   float64
	Top    float64
	Width  float64
	Height float64
	Front  uint32
	Count  int
	// Active is where the window in front sits among them, counting from 0.
	Active int
}

// SyncStackbars shows exactly these stacks and no others. Passing none takes
// every one off screen.
func SyncStackbars(bars []Stackbar, style StackbarStyle) {
	cStyle := C.MimiStackbarStyle{
		step:     C.double(style.Step),
		taper:    C.double(style.Taper),
		radius:   C.double(style.Radius),
		color:    cColor(style.Color),
		farColor: cColor(style.FarColor),
	}

	if len(bars) == 0 {
		C.MimiStackbarsSync(nil, 0, &cStyle)

		return
	}

	// Every field is a number, so this array holds no Go pointer and may
	// cross as it is. The native side copies each bar before it hands the
	// drawing to the main thread.
	cBars := make([]C.MimiStackbar, len(bars))
	for index, bar := range bars {
		cBars[index] = C.MimiStackbar{
			x:      C.double(bar.Left),
			y:      C.double(bar.Top),
			width:  C.double(bar.Width),
			height: C.double(bar.Height),
			front:  C.uint32_t(bar.Front),
			count:  C.int(bar.Count),
			active: C.int(bar.Active),
		}
	}

	C.MimiStackbarsSync(&cBars[0], C.int(len(cBars)), &cStyle)
}

// StackbarCards is how many cards are drawn above and below the window in
// front, for a stack of this many windows with that one active, in a frame of
// this size.
func StackbarCards(windows, active int, width, height float64, style StackbarStyle) (int, int) {
	var above, below C.int

	C.MimiStackbarCards(
		C.int(windows), C.int(active), C.double(width), C.double(height),
		C.double(style.Step), C.double(style.Taper), &above, &below,
	)

	return int(above), int(below)
}

// ClearStackbars takes every stack off screen.
func ClearStackbars() {
	C.MimiStackbarsClear()
}
