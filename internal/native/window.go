package native

/*
#include "mimi.h"
#include <stdlib.h>
*/
import "C"

import (
	"time"
	"unsafe"

	derrors "github.com/y3owk1n/mimi/internal/errors"
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

// FrontmostWindow returns the frontmost window.
func FrontmostWindow() *Element {
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

// PID returns the process ID of the application that owns the window. It is how
// a caller tells one application's windows from another's — the integration
// baseline uses it to confirm it only ever drives windows it opened itself.
func (e *Element) PID() (int, error) {
	if e.ref == nil {
		return 0, derrors.New(
			derrors.CodeAccessibilityFailed,
			"cannot get window pid: element reference is nil",
		)
	}

	pid := int(C.MimiGetWindowPID(e.ref)) //nolint:nlreturn // cgo call expansion
	if pid <= 0 {
		return 0, derrors.New(derrors.CodeAccessibilityFailed, "failed to get window pid")
	}

	return pid, nil
}

// Number returns the window server's number for the window, which is stable
// for the window's lifetime and the same however the window was reached, or 0
// when it has none.
func (e *Element) Number() uint32 {
	if e.ref == nil {
		return 0
	}

	return uint32(C.MimiGetWindowNumber(e.ref)) //nolint:nlreturn // cgo call expansion
}

// Title returns the window's title, or "" when it has none or it cannot be
// read: a window without a title is still a window, so there is no error to
// report.
func (e *Element) Title() string {
	if e.ref == nil {
		return ""
	}

	cTitle := C.MimiCopyWindowTitle(e.ref) //nolint:nlreturn // cgo call expansion
	if cTitle == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cTitle)) //nolint:nlreturn

	return C.GoString(cTitle)
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

// SetPosition moves the window to (x, y) in screen coordinates, leaving its
// size alone: one round trip into the application where SetFrame makes
// three.
func (e *Element) SetPosition(posX, posY float64) error {
	if e.ref == nil {
		return derrors.New(
			derrors.CodeAccessibilityFailed,
			"cannot set window position: element reference is nil",
		)
	}

	result := C.MimiSetWindowPosition(
		e.ref,
		C.double(posX),
		C.double(posY), //nolint:nlreturn
	)
	if result == 0 {
		return derrors.New(
			derrors.CodeAccessibilityFailed,
			"failed to set window position",
		)
	}

	return nil
}

// ListedWindow is one window as the window server lists it: its number, its
// frame in screen coordinates, its owner, its layer, whether the owner is a
// regular, visible application, and its title when the window server gives
// it, which it does only with Screen Recording granted.
type ListedWindow struct {
	Number  uint32
	Frame   Frame
	PID     int
	Layer   int
	Regular bool
	Title   string
	Named   bool
}

// WindowList lists every window the window server has, front to back, in
// one call: on screen only, or on every space. No application is asked
// anything.
func WindowList(onScreenOnly bool) []ListedWindow {
	var (
		count C.int
		names **C.char
	)

	flag := C.int(0)
	if onScreenOnly {
		flag = 1
	}

	rows := C.MimiCopyWindowList(flag, &count, &names) //nolint:nlreturn // cgo call expansion
	if rows == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(rows)) //nolint:nlreturn

	total := int(count)
	perWindow := int(C.MIMI_WINDOW_DOUBLES)
	values := unsafe.Slice((*C.double)(unsafe.Pointer(rows)), total*perWindow)
	titles := unsafe.Slice(names, total)
	windows := make([]ListedWindow, total)

	for index := range windows {
		row := values[index*perWindow : (index+1)*perWindow]
		windows[index] = ListedWindow{
			Number: uint32(row[0]),
			Frame: Frame{
				X: float64(row[1]),
				Y: float64(row[2]),
				W: float64(row[3]),
				H: float64(row[4]),
			},
			Named:   row[5] != 0,
			PID:     int(row[6]),
			Layer:   int(row[7]),
			Regular: row[8] != 0,
			Title:   C.GoString(titles[index]),
		}

		C.free(unsafe.Pointer(titles[index]))
	}

	C.free(unsafe.Pointer(names))

	return windows
}

// ApplicationWindow is one of an application's windows as Accessibility
// lists it: the element, its window server number, and whether its role is
// a window rather than a sheet, a popover or the like.
type ApplicationWindow struct {
	Element  *Element
	Number   uint32
	IsWindow bool
}

// ApplicationWindowElements asks one application for its windows: a round
// trip into it, so a caller asks only when it must.
func ApplicationWindowElements(pid int) []ApplicationWindow {
	var (
		count   C.int
		numbers *C.uint
		roles   *C.int
	)

	elements := C.MimiCopyApplicationWindowElements(
		C.int(pid),
		&count,
		&numbers,
		&roles, //nolint:nlreturn
	)
	if elements == nil {
		return nil
	}

	total := int(count)
	refs := unsafe.Slice((*unsafe.Pointer)(unsafe.Pointer(elements)), total)
	ids := unsafe.Slice(numbers, total)
	isWindow := unsafe.Slice(roles, total)
	windows := make([]ApplicationWindow, total)

	for index := range windows {
		windows[index] = ApplicationWindow{
			Element:  &Element{ref: refs[index]},
			Number:   uint32(ids[index]),
			IsWindow: isWindow[index] != 0,
		}
	}

	C.free(unsafe.Pointer(elements))
	C.free(unsafe.Pointer(numbers))
	C.free(unsafe.Pointer(roles))

	return windows
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

// Frame is a rectangle in NSScreen coordinates: y-up, origin at the primary
// display's bottom-left.
type Frame struct {
	X, Y, W, H float64
}

// Display is one connected display: its identifier, its full frame and its
// visible frame (the full frame less the menu bar and the Dock).
type Display struct {
	ID      uint32
	Frame   Frame
	Visible Frame
}

// Displays lists every connected display, in the order macOS reports them.
func Displays() ([]Display, error) {
	var count C.int

	rows := C.MimiCopyScreenFrames(&count)
	if rows == nil || count == 0 {
		if rows != nil {
			C.free(unsafe.Pointer(rows))
		}

		return nil, derrors.New(derrors.CodeAccessibilityFailed, "failed to enumerate displays")
	}
	defer C.free(unsafe.Pointer(rows)) //nolint:nlreturn

	perDisplay := int(C.MIMI_SCREEN_DOUBLES)
	total := int(count) * perDisplay
	values := unsafe.Slice((*C.double)(unsafe.Pointer(rows)), total)
	displays := make([]Display, int(count))

	for index := range displays {
		row := values[index*perDisplay : (index+1)*perDisplay]
		displays[index] = Display{
			ID: uint32(row[0]),
			Frame: Frame{
				X: float64(row[1]),
				Y: float64(row[2]),
				W: float64(row[3]),
				H: float64(row[4]),
			},
			Visible: Frame{
				X: float64(row[5]),
				Y: float64(row[6]),
				W: float64(row[7]),
				H: float64(row[8]),
			},
		}
	}

	return displays, nil
}

// ActivateDisplay makes the given display the active one for the menu bar
// and event routing, as a window move across displays leaves it.
func ActivateDisplay(id uint32) {
	C.MimiActivateDisplay(C.uint32_t(id))
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

// LogAnimationSteps logs what a stepped animation cost: how many windows
// and frames, over how long, and the slowest single write.
func LogAnimationSteps(windows, frames int, elapsed, slowest time.Duration) {
	C.MimiLogAnimationSteps(
		C.int(windows),
		C.int(frames),
		C.double(float64(elapsed)/float64(time.Millisecond)),
		C.double(float64(slowest)/float64(time.Millisecond)),
	)
}
