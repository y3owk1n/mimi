package action

import (
	"github.com/y3owk1n/mimi/internal/geometry"
)

// WindowID identifies one window on the desktop for as long as the desktop
// that handed it out keeps it. It is opaque on purpose: an action may pass one
// back to the desktop it came from and compare two of them, and nothing else.
type WindowID uint64

// Window is a window as the actions see it — plain data, with no native
// reference and so no lifetime of its own. The desktop that enumerated it owns
// whatever handle stands behind the id.
type Window struct {
	// ID names the window in later calls to the desktop it came from.
	ID WindowID
	// PID is the process ID of the application the window belongs to, which is
	// how one application's windows are told from another's.
	PID int
}

// Desktop is everything the actions need from macOS: the permission to drive
// it, the windows on it, the screens under them, and the Mission Control
// spaces beside it.
//
// Windows cross this boundary as values rather than handles, which keeps the
// native reference — and its release — on the desktop's side of it.
type Desktop interface {
	// EnsureAccessible reports whether the actions may drive this desktop at
	// all, returning an error describing what is missing when they may not.
	// It is checked per action rather than once at startup, because the
	// permission behind it can be revoked while mimi runs.
	EnsureAccessible() error

	// FocusableWindows lists the focusable windows on the active space, in the
	// order focus cycles through them, along with the 0-based index of the
	// focused window — or -1 when no window in the list holds focus.
	FocusableWindows() ([]Window, int, error)

	// WindowFrame reads one window's frame, in window coordinates. It is its
	// own call rather than part of the enumeration because reading a frame
	// costs a round trip to the owning application, and the cycling path never
	// needs one.
	WindowFrame(id WindowID) (geometry.Rect, error)

	// ActivateWindow raises a window's application and gives the window
	// keyboard focus.
	ActivateWindow(id WindowID) error

	// FrontmostWindow is the window in front, which is the one the window
	// actions that take no window act on. It reports an error when there is
	// none.
	FrontmostWindow() (WindowID, error)

	// SetWindowFrame moves and resizes one window.
	SetWindowFrame(id WindowID, frame geometry.Rect) error

	// ScreenAt describes the screen the given window frame sits on, including
	// the system's tiled-window margin settings, which are part of what the
	// geometry resizes against. It stands on several reads, and names the one
	// that failed in its error, so callers pass that error on unwrapped.
	ScreenAt(window geometry.Rect) (geometry.Screen, error)

	// MissionControlActive reports whether Mission Control is open, which is
	// the state the space actions refuse to run in.
	MissionControlActive() bool

	// SpaceCount is how many Mission Control spaces exist, or 0 when they
	// cannot be enumerated.
	SpaceCount() int

	// ActiveSpaceIndex is the 1-based index of the space in front.
	ActiveSpaceIndex() (int, error)

	// FocusSpace switches to the Mission Control space at the given 1-based
	// index, which the caller has already checked against SpaceCount.
	FocusSpace(index int) error

	// MoveWindowToSpace moves the frontmost window to the Mission Control
	// space at the given 1-based index, which the caller has already checked
	// against SpaceCount.
	MoveWindowToSpace(index int) error
}
