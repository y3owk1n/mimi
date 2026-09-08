package native

/*
#include "mimi.h"
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// AppWindow is one window of an application as the window server lists it:
// its number, and the id of the space it is on, which is 0 when it is on
// every space or none.
type AppWindow struct {
	Number  uint32
	SpaceID uint64
}

// FindApplication resolves a bundle identifier or a localized application
// name, case-insensitively, to the pid of the running application it names.
func FindApplication(query string) (int, error) {
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery)) //nolint:nlreturn

	pid := int(C.MimiFindApplication(cQuery))
	if pid == 0 {
		return 0, derrors.Newf(
			derrors.CodeActionFailed,
			"no running application named %q",
			query,
		)
	}

	return pid, nil
}

// ApplicationInfo describes a running application by pid: its localized name
// and its bundle identifier. Either is "" when macOS does not report it.
type ApplicationInfo struct {
	Name     string
	BundleID string
}

// LookupApplication describes the running application with the given pid,
// reporting an error when no application has it.
func LookupApplication(pid int) (ApplicationInfo, error) {
	cName := C.MimiCopyApplicationName(C.int(pid))
	if cName == nil {
		return ApplicationInfo{}, derrors.Newf(
			derrors.CodeActionFailed,
			"no running application with pid %d",
			pid,
		)
	}
	defer C.free(unsafe.Pointer(cName)) //nolint:nlreturn

	info := ApplicationInfo{Name: C.GoString(cName)}

	cBundle := C.MimiCopyApplicationBundleID(C.int(pid))
	if cBundle != nil {
		info.BundleID = C.GoString(cBundle)

		C.free(unsafe.Pointer(cBundle))
	}

	return info, nil
}

// ApplicationWindows lists an application's real, unminimized windows on
// every space, front to back, which is most recently used first.
func ApplicationWindows(pid int) ([]AppWindow, error) {
	var count C.int

	rows := C.MimiCopyApplicationWindows(C.int(pid), &count)
	if rows == nil || count == 0 {
		if rows != nil {
			C.free(unsafe.Pointer(rows))
		}

		return nil, nil
	}
	defer C.free(unsafe.Pointer(rows)) //nolint:nlreturn

	values := unsafe.Slice(rows, int(count))
	windows := make([]AppWindow, int(count))

	for index, row := range values {
		windows[index] = AppWindow{
			Number:  uint32(row.number),
			SpaceID: uint64(row.space),
		}
	}

	return windows, nil
}

// RaiseWindowNumber brings one of an application's windows to the front by
// its window server number. The window has to be on the active space: that
// is when its application lists it through Accessibility.
func RaiseWindowNumber(pid int, number uint32) error {
	if C.MimiRaiseWindowNumber(C.int(pid), C.uint32_t(number)) == 0 {
		return derrors.Newf(
			derrors.CodeAccessibilityFailed,
			"window %d of application %d could not be raised",
			number,
			pid,
		)
	}

	return nil
}

// ReopenApplication reopens an application as a Dock click would. An
// application with no window opens one and comes to the front.
func ReopenApplication(pid int) error {
	if C.MimiReopenApplication(C.int(pid)) == 0 {
		return derrors.Newf(derrors.CodeActionFailed, "failed to reopen application %d", pid)
	}

	return nil
}

// SpaceIndexes maps every Mission Control space id to its 1-based index, in
// one pass over the enumeration.
func SpaceIndexes() map[uint64]int {
	count := SpaceCount()
	indexes := make(map[uint64]int, count)

	for index := 1; index <= count; index++ {
		if sid := uint64(C.MimiMissionControlSpaceID(C.int(index))); sid != 0 {
			indexes[sid] = index
		}
	}

	return indexes
}
