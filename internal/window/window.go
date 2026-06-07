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
	var count C.int
	windows := C.MimiGetAllFocusableWindowsOnActiveSpace(&count)
	if windows == nil || count == 0 {
		if windows != nil {
			C.free(unsafe.Pointer(windows))
		}

		return []*Element{}, nil
	}
	defer C.free(unsafe.Pointer(windows)) //nolint:nlreturn

	countInt := int(count)
	windowSlice := (*[1 << 30]unsafe.Pointer)(unsafe.Pointer(windows))[:countInt:countInt]
	result := make([]*Element, countInt)

	for index := range result {
		result[index] = &Element{ref: windowSlice[index]}
	}

	return result, nil
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

// MissionControlActive reports whether Mission Control is currently open.
func MissionControlActive() bool {
	return bool(C.MimiIsMissionControlActive())
}
