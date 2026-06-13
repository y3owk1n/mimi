package window

/*
#cgo CFLAGS: -x objective-c
#include "../native/mimi.h"
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	_ "github.com/y3owk1n/mimi/internal/native"
)

// Element represents a UI element in the macOS accessibility hierarchy.
type Element struct {
	ref unsafe.Pointer
}

// AllFocusableOnActiveSpace returns focusable windows on the active space.
func AllFocusableOnActiveSpace() ([]*Element, error) {
	wins, _, err := AllFocusableOnActiveSpaceWithFocused()

	return wins, err
}

// AllFocusableOnActiveSpaceWithFocused returns focusable windows on the active
// space along with the 0-based index of the currently focused window, or -1
// if no window is focused (or the focused window is not focusable). This
// avoids N additional CGO trips to find the current index after enumeration.
func AllFocusableOnActiveSpaceWithFocused() ([]*Element, int, error) {
	var count C.int
	var focused C.int
	windows := C.MimiGetAllFocusableWindowsOnActiveSpaceWithFocused(&count, &focused)
	if windows == nil || count == 0 {
		if windows != nil {
			C.free(unsafe.Pointer(windows))
		}

		return []*Element{}, int(focused), nil
	}
	defer C.free(unsafe.Pointer(windows)) //nolint:nlreturn

	countInt := int(count)
	windowSlice := (*[1 << 30]unsafe.Pointer)(unsafe.Pointer(windows))[:countInt:countInt]
	result := make([]*Element, countInt)

	for index := range result {
		result[index] = &Element{ref: windowSlice[index]}
	}

	return result, int(focused), nil
}

// Frontmost returns the frontmost window.
func Frontmost() *Element {
	ref := C.MimiGetFrontmostWindow()
	if ref == nil {
		return nil
	}

	return &Element{ref: ref}
}

// Activate brings the window's application to the foreground and sets focus.
func (e *Element) Activate() error {
	if e.ref == nil {
		return derrors.New(
			derrors.CodeAccessibilityFailed,
			"cannot activate window: element reference is nil",
		)
	}

	result := C.MimiActivateWindow(e.ref) //nolint:nlreturn
	if result == 0 {
		return derrors.New(derrors.CodeAccessibilityFailed, "failed to activate window")
	}

	return nil
}

// Release releases the element reference.
func (e *Element) Release() {
	if e.ref != nil {
		C.MimiReleaseElement(e.ref)
		e.ref = nil
	}
}

// ReleaseAll releases all elements in a slice.
func ReleaseAll(elements []*Element) {
	for _, element := range elements {
		if element != nil {
			element.Release()
		}
	}
}

// Equal checks if this element refers to the same underlying UI element as another.
func (e *Element) Equal(other *Element) bool {
	if e == nil && other == nil {
		return true
	}
	if e == nil || other == nil {
		return false
	}
	if e.ref == nil && other.ref == nil {
		return true
	}
	if e.ref == nil || other.ref == nil {
		return false
	}

	if e.ref == other.ref {
		return true
	}

	result := C.MimiAreElementsEqual(e.ref, other.ref) //nolint:nlreturn

	return result == 1
}

// GetFrame returns the window's position and size [x, y, w, h] in screen coordinates.
func (e *Element) GetFrame() (float64, float64, float64, float64, error) {
	if e.ref == nil {
		return 0, 0, 0, 0, derrors.New(
			derrors.CodeAccessibilityFailed,
			"cannot get window frame: element reference is nil",
		)
	}

	frame := C.MimiGetWindowFrame(e.ref) //nolint:nlreturn
	if frame == nil {
		return 0, 0, 0, 0, derrors.New(
			derrors.CodeAccessibilityFailed,
			"failed to get window frame",
		)
	}

	defer C.free(unsafe.Pointer(frame)) //nolint:nlreturn

	frameArr := (*[4]C.double)(unsafe.Pointer(frame))

	return float64(
			frameArr[0],
		), float64(
			frameArr[1],
		), float64(
			frameArr[2],
		), float64(
			frameArr[3],
		), nil
}

// SetFrame sets the window's position (x, y) and size (w, h) in screen coordinates.
func (e *Element) SetFrame(posX, posY, width, height float64) error {
	if e.ref == nil {
		return derrors.New(
			derrors.CodeAccessibilityFailed,
			"cannot set window frame: element reference is nil",
		)
	}

	result := C.MimiSetWindowFrame(
		e.ref,
		C.double(posX),
		C.double(posY),
		C.double(width),
		C.double(height), //nolint:nlreturn
	)
	if result == 0 {
		return derrors.New(
			derrors.CodeAccessibilityFailed,
			"failed to set window frame",
		)
	}

	return nil
}

// PrimaryScreenHeight returns the height of the primary display (the one with the menu bar).
// This is needed to convert between AX (y-down) and NSScreen (y-up) coordinate systems.
func PrimaryScreenHeight() (float64, error) {
	_, _, _, h, err := ScreenFrame(0, 0) //nolint:dogsled
	if err != nil {
		return 0, err
	}

	return h, nil
}

// ScreenFrame returns the full frame [x, y, w, h] of the screen containing (x, y).
func ScreenFrame(xCoord, yCoord float64) (float64, float64, float64, float64, error) {
	frame := C.MimiGetScreenFrameForPoint(C.double(xCoord), C.double(yCoord))
	if frame == nil {
		return 0, 0, 0, 0, derrors.New(
			derrors.CodeAccessibilityFailed,
			"failed to get screen frame",
		)
	}

	defer C.free(unsafe.Pointer(frame)) //nolint:nlreturn

	frameArr := (*[4]C.double)(unsafe.Pointer(frame))

	return float64(
			frameArr[0],
		), float64(
			frameArr[1],
		), float64(
			frameArr[2],
		), float64(
			frameArr[3],
		), nil
}

// ScreenVisibleFrame returns the visible frame [x, y, w, h] of the screen containing (x, y),
// excluding the dock and menu bar.
func ScreenVisibleFrame(xCoord, yCoord float64) (float64, float64, float64, float64, error) {
	frame := C.MimiGetScreenVisibleFrameForPoint(C.double(xCoord), C.double(yCoord))
	if frame == nil {
		return 0, 0, 0, 0, derrors.New(
			derrors.CodeAccessibilityFailed,
			"failed to get screen visible frame",
		)
	}

	defer C.free(unsafe.Pointer(frame)) //nolint:nlreturn

	frameArr := (*[4]C.double)(unsafe.Pointer(frame))

	return float64(
			frameArr[0],
		), float64(
			frameArr[1],
		), float64(
			frameArr[2],
		), float64(
			frameArr[3],
		), nil
}

// TiledWindowMarginsEnabled reports whether the system tiled window margins setting is enabled.
func TiledWindowMarginsEnabled() bool {
	return bool(C.MimiTiledWindowMarginsEnabled())
}

// TiledWindowMarginSize returns the tiled window margin size in points.
func TiledWindowMarginSize() float64 {
	return float64(C.MimiTiledWindowMarginSize())
}

// MissionControlActive reports whether Mission Control is currently open.
func MissionControlActive() bool {
	return bool(C.MimiIsMissionControlActive())
}
