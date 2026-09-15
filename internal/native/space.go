package native

/*
#include <stdlib.h>
#include "mimi.h"
*/
import "C"

import (
	"unsafe"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// FocusSpace focuses the Mission Control space at the given 1-based index.
//
// The index is expected to name a space that exists — the range check is the
// caller's, made once above this package rather than once per entry point.
func FocusSpace(index int) error {
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

	// The gesture reports that it was posted, not that the Dock took it.
	// Reading the space in front back is what shows a dropped swipe.
	inFront := func() uint64 { return uint64(C.MimiDisplayActiveSpaceID(C.uint32_t(did))) }
	if !awaitSpace(inFront, sid) {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"space %d did not come in front within %s, the display is still on space %d",
			index,
			spaceSettleTimeout,
			SpaceIndexes()[inFront()],
		)
	}

	return nil
}

// CursorDisplayID is the display the pointer is on, 0 when none.
func CursorDisplayID() uint32 {
	return uint32(C.MimiCursorDisplayID())
}

// SpaceCount returns the total number of Mission Control spaces.
func SpaceCount() int {
	return int(C.MimiCountMissionControlSpaces())
}

// ActiveSpaceIndex returns the 1-based index of the currently active space.
func ActiveSpaceIndex() (int, error) {
	count := SpaceCount()
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

// ActiveSpaceIndexes reports the 1-based Mission Control index of the space
// in front on each of the given displays, leaving out a display whose space
// cannot be resolved.
func ActiveSpaceIndexes(displayIDs []uint32) map[uint32]int {
	indexes := SpaceIndexes()
	active := make(map[uint32]int, len(displayIDs))

	for _, did := range displayIDs {
		sid := uint64(C.MimiDisplayActiveSpaceID(C.uint32_t(did)))
		if index, ok := indexes[sid]; ok {
			active[did] = index
		}
	}

	return active
}

// ActiveSpaceIDs reports the window server's own identifier for the space in
// front on each of the given displays, leaving out a display whose space
// cannot be resolved.
//
// The window server assigns the identifier when the space is created and
// never changes it. ActiveSpaceIndexes reports where that space sits in
// Mission Control instead, which is the number a user counts. A caller that
// remembers something about a space keys it by the identifier, because adding
// or removing a space changes the index of every space after it.
func ActiveSpaceIDs(displayIDs []uint32) map[uint32]uint64 {
	active := make(map[uint32]uint64, len(displayIDs))

	for _, did := range displayIDs {
		sid := uint64(C.MimiDisplayActiveSpaceID(C.uint32_t(did)))
		if sid != 0 {
			active[did] = sid
		}
	}

	return active
}

// FullScreenDisplays reports which of the given displays show a full-screen
// application space in front. macOS lays that window out itself, so nothing
// else should move it.
func FullScreenDisplays(displayIDs []uint32) map[uint32]bool {
	fullScreen := make(map[uint32]bool, len(displayIDs))

	for _, did := range displayIDs {
		if C.MimiDisplaySpaceIsFullScreen(C.uint32_t(did)) != 0 {
			fullScreen[did] = true
		}
	}

	return fullScreen
}

// MoveWindowToSpace moves the frontmost window to the space at the given
// 1-based index and returns the pid and window number of the window it moved,
// which is what a caller that follows the window needs to raise it again once
// the destination space is in front. The number is 0 when the window server
// reports none.
//
// As with FocusSpace, the index is expected to name a space that exists.
func MoveWindowToSpace(index int) (int, uint32, error) {
	sid := uint64(C.MimiMissionControlSpaceID(C.int(index)))
	if sid == 0 {
		return 0, 0, derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve Mission Control space at index %d",
			index,
		)
	}

	frontmost := FrontmostWindow()
	if frontmost == nil {
		return 0, 0, derrors.New(
			derrors.CodeActionFailed,
			"no active window found to move",
		)
	}

	defer frontmost.Release()

	// Read the identity before the move: once the window leaves the active
	// space its application stops listing it through Accessibility.
	pid, err := frontmost.PID()
	if err != nil {
		return 0, 0, err
	}

	number := frontmost.Number()

	if C.MimiMoveWindowToSpace(frontmost.ref, C.uint64_t(sid)) == 0 { //nolint:nlreturn
		return 0, 0, derrors.New(derrors.CodeActionFailed, "failed to move window to space")
	}

	targetDid := uint32(C.MimiSpaceDisplayID(C.uint64_t(sid)))
	if targetDid != 0 && targetDid != uint32(C.MimiCursorDisplayID()) {
		C.MimiActivateDisplay(C.uint32_t(targetDid))
	}

	return pid, number, nil
}

// MoveWindowNumberToSpace moves a window to the space at the given 1-based
// index by its window server number, which reaches a window on any space,
// where MoveWindowToSpace reaches the frontmost through Accessibility.
func MoveWindowNumberToSpace(number uint32, index int) error {
	sid := uint64(C.MimiMissionControlSpaceID(C.int(index)))
	if sid == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve Mission Control space at index %d",
			index,
		)
	}

	result := C.MimiMoveWindowNumberToSpace(C.uint32_t(number), C.uint64_t(sid))
	if result == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to move window %d to space %d",
			number,
			index,
		)
	}

	return nil
}

// Space is one Mission Control space as the window server lists it: its
// identifier, the display it belongs to, whether it is a full-screen
// application space, and whether it is the one in front on its display.
type Space struct {
	ID         uint64
	DisplayID  uint32
	FullScreen bool
	Current    bool
}

// Spaces lists every Mission Control space in Mission Control order, display
// by display, or nothing when they cannot be enumerated.
func Spaces() []Space {
	var count C.int

	rows := C.MimiCopySpaces(&count)
	if rows == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(rows)) //nolint:nlreturn

	values := unsafe.Slice(rows, int(count))
	spaces := make([]Space, int(count))

	for index, row := range values {
		spaces[index] = Space{
			ID:         uint64(row.id),
			DisplayID:  uint32(row.display),
			FullScreen: row.fullScreen != 0,
			Current:    row.current != 0,
		}
	}

	return spaces
}

// WindowSpaceID is the window server's identifier for the one space a window
// is on, or 0 when it is on every space or on none.
func WindowSpaceID(number uint32) uint64 {
	return uint64(C.MimiSpaceForWindowNumber(C.uint32_t(number)))
}

// WindowsOnSpace lists the real, unminimized windows on one space by number,
// in theorder, front to back.
func WindowsOnSpace(id uint64) []uint32 {
	var count C.int

	rows := C.MimiCopyRealWindowNumbersOnSpace(C.uint64_t(id), &count)
	if rows == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(rows)) //nolint:nlreturn

	values := unsafe.Slice(rows, int(count))
	numbers := make([]uint32, int(count))

	for index, row := range values {
		numbers[index] = uint32(row)
	}

	return numbers
}
