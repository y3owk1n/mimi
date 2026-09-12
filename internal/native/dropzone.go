package native

/*
#include "dropzone.h"
*/
import "C"

// DropzoneStyle is how the drop zone is drawn: its fill, its outline and
// how wide, and its corner radius, in points.
type DropzoneStyle struct {
	Fill    Color
	Outline Color
	Width   float64
	Radius  float64
}

// ShowDropzone shows the drop zone over a frame, in window coordinates, or
// slides it there when it is shown already.
func ShowDropzone(style DropzoneStyle, left, top, width, height float64) {
	cStyle := C.MimiDropzoneStyle{
		fill:    cColor(style.Fill),
		outline: cColor(style.Outline),
		width:   C.double(style.Width),
		radius:  C.double(style.Radius),
	}
	C.MimiDropzoneShow(&cStyle, C.double(left), C.double(top), C.double(width), C.double(height))
}

// HideDropzone takes the drop zone off screen.
func HideDropzone() {
	C.MimiDropzoneHide()
}

// LeftMouseButtonDown reports whether the left mouse button is down now.
func LeftMouseButtonDown() bool {
	return C.MimiLeftMouseButtonDown() != 0
}
