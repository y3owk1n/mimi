package native

/*
#include "mouse.h"
*/
import "C"

// Point is where the pointer is, in window coordinates.
type Point struct {
	X, Y float64
}

// mouseMoves carries the pointer's position out of the tap. It holds one
// point, so a reader that is behind gets the latest and never a backlog.
//
//nolint:gochecknoglobals // one tap per process
var mouseMoves = make(chan Point, 1)

// MouseMoves is where the pointer's positions arrive while the monitor
// runs. A position nobody has read yet is replaced by the next.
func MouseMoves() <-chan Point { return mouseMoves }

// StartMouseMonitor starts reporting pointer moves on MouseMoves. It
// reports false when the window server refused the tap, which it does
// without Accessibility.
func StartMouseMonitor() bool {
	return C.MimiMouseMonitorStart() != 0
}

// StopMouseMonitor stops reporting pointer moves.
func StopMouseMonitor() {
	C.MimiMouseMonitorStop()
}

//export goMouseMoved
func goMouseMoved(x, y C.double) {
	point := Point{X: float64(x), Y: float64(y)}

	select {
	case mouseMoves <- point:
		return
	default:
	}

	// Drop the stale point and put the fresh one in its place.
	select {
	case <-mouseMoves:
	default:
	}

	select {
	case mouseMoves <- point:
	default:
	}
}

// WindowAtPoint is the window under the point: the frontmost real window
// of a regular application whose frame holds it, by number and owner. It
// reads the window server only, and never returns a menu, a popover, or a
// window mimi draws itself.
func WindowAtPoint(point Point) (int, uint32, bool) {
	for _, win := range WindowList(true) {
		if win.Layer != 0 || !win.Regular {
			continue
		}

		frame := win.Frame
		if point.X >= frame.X && point.X < frame.X+frame.W && point.Y >= frame.Y &&
			point.Y < frame.Y+frame.H {
			return win.PID, win.Number, true
		}
	}

	return 0, 0, false
}

// FrontmostWindowNumber is the number of the window in front, or 0 when
// there is none or it cannot be read.
func FrontmostWindowNumber() uint32 {
	element := FrontmostWindow()
	if element == nil {
		return 0
	}
	defer element.Release()

	return element.Number()
}
