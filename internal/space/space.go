package space

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework CoreFoundation -framework Foundation -F/System/Library/PrivateFrameworks -framework SkyLight

#include "space.h"
*/
import "C"

import (
	"sync"
	"unsafe"
)

// WindowRef is an opaque reference to a macOS AXUIElementRef
// representing a window. Callers must hand it back to
// ReleaseWindow / ReleaseAll when done.
type WindowRef unsafe.Pointer

// initCocoaOnce ensures InitCocoa performs its NSApplication
// bootstrap exactly once per process.
var initCocoaOnce sync.Once

// AllFocusableWindowsOnActiveSpace returns every focusable
// window on the active space, sorted by screen position
// (y-coordinate first, then x-coordinate). Each returned
// WindowRef is retained; call ReleaseAll to release them.
func AllFocusableWindowsOnActiveSpace() ([]WindowRef, error) {
	var count C.int
	raw := C.MimiGetAllFocusableWindowsOnActiveSpace(&count)
	if raw == nil || count == 0 {
		empty := []WindowRef(nil)
		if raw != nil {
			C.free(unsafe.Pointer(raw))
		}

		return empty, nil
	}
	defer C.free(unsafe.Pointer(raw)) //nolint:nlreturn

	n := int(count)
	slice := (*[1 << 28]unsafe.Pointer)(unsafe.Pointer(raw))[:n:n]

	result := make([]WindowRef, n)
	for i := range slice {
		result[i] = WindowRef(slice[i])
	}

	return result, nil
}

// FrontmostWindow returns the current frontmost window or nil
// when the system has no focused window. The returned reference
// is retained; the caller is responsible for calling
// ReleaseWindow on it.
func FrontmostWindow() WindowRef {
	ref := C.MimiGetFrontmostWindow()
	if ref == nil {
		return nil
	}

	return WindowRef(ref)
}

// ReleaseWindow releases a window reference obtained from
// AllFocusableWindowsOnActiveSpace or FrontmostWindow. Safe to
// call on nil.
func ReleaseWindow(ref WindowRef) {
	if ref == nil {
		return
	}
	C.MimiReleaseElement(unsafe.Pointer(ref))
}

// ReleaseAll releases every window reference in the slice.
// Safe to call on nil or empty slices.
func ReleaseAll(refs []WindowRef) {
	for _, ref := range refs {
		ReleaseWindow(ref)
	}
}

// ElementsEqual reports whether two window references refer to
// the same underlying accessibility element.
func ElementsEqual(a, b WindowRef) bool {
	if a == nil || b == nil {
		return a == b
	}

	equal := C.MimiAreElementsEqual(unsafe.Pointer(a), unsafe.Pointer(b)) //nolint:nlreturn

	return equal == 1
}

// ActivateWindow brings the window to the foreground and gives
// it keyboard focus. Returns nil on success, an error otherwise.
func ActivateWindow(ref WindowRef) error {
	if ref == nil {
		return errNoWindow
	}
	ok := C.MimiActivateWindow(unsafe.Pointer(ref)) //nolint:nlreturn
	if ok == 1 {
		return nil
	}

	return errActivateFailed
}

// IsMissionControlActive returns true while Mission Control is
// currently on screen.
func IsMissionControlActive() bool {
	return bool(C.MimiIsMissionControlActive())
}

// CountMissionControlSpaces returns the total number of Mission
// Control spaces across all connected displays. Returns 0 when
// spaces cannot be enumerated.
func CountMissionControlSpaces() int {
	return int(C.MimiCountMissionControlSpaces())
}

// MissionControlSpaceID returns the space ID for the 1-based
// Mission Control index. Returns 0 when the index is out of
// range.
func MissionControlSpaceID(index int) uint64 {
	return uint64(C.MimiMissionControlSpaceID(C.int(index)))
}

// DisplayIDForSpace returns the display ID that owns a given
// space. Returns 0 when the space is invalid.
func DisplayIDForSpace(sid uint64) uint32 {
	return uint32(C.MimiSpaceDisplayID(C.uint64_t(sid)))
}

// FocusSpaceUsingGesture focuses the given space using a
// synthetic high-velocity horizontal dock swipe gesture.
//
// The caller must have already verified Mission Control is not
// active. Returns nil on success.
func FocusSpaceUsingGesture(did uint32, sid uint64) error {
	ok := C.MimiFocusSpaceUsingGesture(C.uint32_t(did), C.uint64_t(sid))
	if ok == 1 {
		return nil
	}

	return errFocusSpaceFailed
}

// MoveWindowToSpace moves the window to the given space. On
// the primary path the move is dispatched asynchronously; the
// return value reports whether the operation was queued, not
// whether the window has already moved.
func MoveWindowToSpace(ref WindowRef, sid uint64) error {
	if ref == nil {
		return errNoWindow
	}
	ok := C.MimiMoveWindowToSpace(unsafe.Pointer(ref), C.uint64_t(sid)) //nolint:nlreturn
	if ok == 1 {
		return nil
	}

	return errMoveWindowFailed
}

// InitCocoa initializes NSApplication on the calling thread.
// Must be called once, from the main OS thread, before any
// other call in this package. AppKit helpers such as
// NSRunningApplication and NSWorkspace require a live
// NSApplication to function.
//
// The function is idempotent and safe to call from multiple
// goroutines, but only the first call does work. Subsequent
// callers block on the internal once.
func InitCocoa() {
	initCocoaOnce.Do(func() {
		C.MimiInitCocoaApp()
	})
}
