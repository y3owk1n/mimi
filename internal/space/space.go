package space

/*
#cgo CFLAGS: -x objective-c
#include "../native/mimi.h"
*/
import "C"

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	_ "github.com/y3owk1n/mimi/internal/native"
)

// Focus focuses the Mission Control space at the given 1-based index.
func Focus(index int) error {
	count := int(C.MimiCountMissionControlSpaces())
	if count == 0 {
		return derrors.New(derrors.CodeActionFailed, "failed to enumerate Mission Control spaces")
	}

	if index < 1 || index > count {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number %d is out of range; valid range is 1..%d",
			index,
			count,
		)
	}

	sid := uint64(C.MimiMissionControlSpaceID(C.int(index)))
	if sid == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve Mission Control space at index %d",
			index,
		)
	}

	did := uint32(C.MimiSpaceDisplayID(C.uint64_t(sid)))
	if did == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve display for Mission Control space at index %d",
			index,
		)
	}

	if C.MimiFocusSpaceUsingGesture(C.uint32_t(did), C.uint64_t(sid)) == 0 {
		return derrors.New(derrors.CodeActionFailed, "failed to focus Mission Control space")
	}

	return nil
}

// Count returns the total number of Mission Control spaces.
func Count() int {
	return int(C.MimiCountMissionControlSpaces())
}

// ActiveIndex returns the 1-based index of the currently active space.
func ActiveIndex() (int, error) {
	count := Count()
	if count == 0 {
		return 0, derrors.New(
			derrors.CodeActionFailed,
			"failed to enumerate Mission Control spaces",
		)
	}

	activeID := uint64(C.MimiActiveSpaceID())
	if activeID == 0 {
		return 0, derrors.New(derrors.CodeActionFailed, "failed to resolve active space ID")
	}

	for i := 1; i <= count; i++ {
		sid := uint64(C.MimiMissionControlSpaceID(C.int(i)))
		if sid == activeID {
			return i, nil
		}
	}

	return 0, derrors.New(derrors.CodeActionFailed, "active space not found in space enumeration")
}

// MoveWindow moves the frontmost window to the space at the given 1-based index.
func MoveWindow(index int) error {
	count := int(C.MimiCountMissionControlSpaces())
	if count == 0 {
		return derrors.New(derrors.CodeActionFailed, "failed to enumerate Mission Control spaces")
	}

	if index < 1 || index > count {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number %d is out of range; valid range is 1..%d",
			index,
			count,
		)
	}

	sid := uint64(C.MimiMissionControlSpaceID(C.int(index)))
	if sid == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve Mission Control space at index %d",
			index,
		)
	}

	frontmost := C.MimiGetFrontmostWindow()
	if frontmost == nil {
		return derrors.New(
			derrors.CodeActionFailed,
			"no active window found to move",
		)
	}

	defer C.MimiReleaseElement(frontmost) //nolint:nlreturn

	if C.MimiMoveWindowToSpace(frontmost, C.uint64_t(sid)) == 0 { //nolint:nlreturn
		return derrors.New(derrors.CodeActionFailed, "failed to move window to space")
	}

	targetDid := uint32(C.MimiSpaceDisplayID(C.uint64_t(sid)))
	if targetDid != 0 && targetDid != uint32(C.MimiCursorDisplayID()) {
		C.MimiActivateDisplay(C.uint32_t(targetDid))
	}

	return nil
}
