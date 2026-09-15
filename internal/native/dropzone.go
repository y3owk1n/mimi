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
	cStyle := cDropzoneStyle(style)
	C.MimiDropzoneShow(&cStyle, C.double(left), C.double(top), C.double(width), C.double(height))
}

func cDropzoneStyle(style DropzoneStyle) C.MimiDropzoneStyle {
	return C.MimiDropzoneStyle{
		fill:    cColor(style.Fill),
		outline: cColor(style.Outline),
		width:   C.double(style.Width),
		radius:  C.double(style.Radius),
	}
}

// HideDropzone takes the drop zone off screen, the target mark with it.
func HideDropzone() {
	C.MimiDropzoneHide()
}

// ShowDropzoneTarget marks the window a drop would act on, over a frame in
// window coordinates, in the zone shown by ShowDropzone.
func ShowDropzoneTarget(style DropzoneStyle, left, top, width, height float64) {
	cStyle := cDropzoneStyle(style)
	C.MimiDropzoneShowTarget(
		&cStyle,
		C.double(left),
		C.double(top),
		C.double(width),
		C.double(height),
	)
}

// HideDropzoneTarget takes the target mark down and leaves the zone up.
func HideDropzoneTarget() {
	C.MimiDropzoneHideTarget()
}

// LeftMouseButtonDown reports whether the left mouse button is down now.
func LeftMouseButtonDown() bool {
	return C.MimiLeftMouseButtonDown() != 0
}

// The modifier keys as CGEventFlags names them.
const (
	flagShift   = 1 << 17
	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
)

// ModifierKeys is the modifier keys held right now, by name, in a fixed
// order: shift, control, option, command. Empty when none is held.
func ModifierKeys() []string {
	return modifierNames(uint64(C.MimiModifierFlags()))
}

// modifierNames is the keys a CGEventFlags value names.
func modifierNames(flags uint64) []string {
	var keys []string

	for _, key := range []struct {
		flag uint64
		name string
	}{
		{flagShift, "shift"}, {flagControl, "control"}, {flagOption, "option"}, {flagCommand, "command"},
	} {
		if flags&key.flag != 0 {
			keys = append(keys, key.name)
		}
	}

	return keys
}
