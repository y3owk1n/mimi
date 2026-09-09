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
	// Number is the window server's number for the window: stable for the
	// window's lifetime and the same however the window was reached, which is
	// how a window found one way is recognized when found another. It is 0
	// when the desktop cannot read it.
	Number uint32
}

// AppWindow is one window of an application as the window server lists it:
// its number, and the 1-based index of the space it is on, or 0 when it is on
// every space or none. It carries no handle: a window on another space cannot
// be driven until that space is in front, which is what RaiseWindow needs.
type AppWindow struct {
	Number     uint32
	SpaceIndex int
}

// AppInfo describes the application behind a pid as the queries report it:
// its localized name and its bundle identifier, either "" when macOS does not
// report it.
type AppInfo struct {
	Name     string
	BundleID string
}

// Display is one connected display as the actions see it: an identifier to
// hand back to the desktop, and its frames in screen coordinates.
type Display struct {
	// ID names the display in later calls to the desktop it came from.
	ID uint32
	// Frame is the whole display, in screen (y-up) coordinates.
	Frame geometry.Rect
	// Visible is the frame less the menu bar and the Dock, in the same
	// coordinates: the area a window is placed within.
	Visible geometry.Rect
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

	// KnownWindows is what the last FocusableWindows call reported, in the
	// same order, when it was recent enough to still trust, else nil. An
	// action that names its windows by number can find them here without
	// asking every application again: an enumeration is a round trip into
	// each one, and an application just activated answers late.
	KnownWindows() []Window

	// WindowFrame reads one window's frame, in window coordinates. It is its
	// own call rather than part of the enumeration because reading a frame
	// costs a round trip to the owning application, and the cycling path never
	// needs one.
	WindowFrame(id WindowID) (geometry.Rect, error)

	// WindowTitle reads one window's title, "" when it has none. Like
	// WindowFrame it is its own call: it costs a round trip to the owning
	// application, and only the window listing needs it.
	WindowTitle(id WindowID) (string, error)

	// ApplicationInfo describes the running application with the given pid,
	// reporting an error when no application has it.
	ApplicationInfo(pid int) (AppInfo, error)

	// ActivateWindow raises a window's application and gives the window
	// keyboard focus.
	ActivateWindow(id WindowID) error

	// FrontmostWindow is the window in front, which is the one the window
	// actions that take no window act on. It reports an error when there is
	// none.
	FrontmostWindow() (Window, error)

	// SetWindowFrame moves and resizes one window.
	SetWindowFrame(id WindowID, frame geometry.Rect) error

	// ScreenAt describes the screen the given window frame sits on, including
	// the system's tiled-window margin settings, which are part of what the
	// geometry resizes against. It stands on several reads, and names the one
	// that failed in its error, so callers pass that error on unwrapped.
	ScreenAt(window geometry.Rect) (geometry.Screen, error)

	// Displays lists every connected display, in no particular order; the
	// actions order them themselves.
	Displays() ([]Display, error)

	// ActivateDisplay makes a display the active one for the menu bar and
	// event routing, which is what a window landing on it expects.
	ActivateDisplay(id uint32)

	// FindApplication resolves a bundle identifier or an application name to
	// the pid of the running application it names, reporting an error when
	// nothing running matches.
	FindApplication(query string) (int, error)

	// ApplicationWindows lists an application's real windows on every space,
	// front to back, which is most recently used first. Minimized and
	// auxiliary windows are left out.
	ApplicationWindows(pid int) ([]AppWindow, error)

	// RaiseWindow brings one of an application's windows to the front by
	// number. It works for a window on the active space and reports an
	// error for one that is not: the caller switches first.
	RaiseWindow(pid int, number uint32) error

	// ReopenApplication reopens an application as a Dock click would. An
	// application with no window opens one and comes to the front.
	ReopenApplication(pid int) error

	// MissionControlActive reports whether Mission Control is open, which is
	// the state the space actions refuse to run in.
	MissionControlActive() bool

	// SpaceCount is how many Mission Control spaces exist, or 0 when they
	// cannot be enumerated.
	SpaceCount() int

	// ActiveSpaceIndex is the 1-based index of the space in front.
	ActiveSpaceIndex() (int, error)

	// ActiveSpaces is the 1-based index of the space in front on every
	// connected display, keyed by display id. With displays sharing one
	// space every entry is the same index.
	ActiveSpaces() (map[uint32]int, error)

	// FullScreenDisplays is the set of connected displays, by id, whose
	// space in front is a full-screen application space. macOS lays that
	// space out itself, for one window or a split-view pair.
	FullScreenDisplays() (map[uint32]bool, error)

	// FocusSpace switches to the Mission Control space at the given 1-based
	// index, which the caller has already checked against SpaceCount.
	FocusSpace(index int) error

	// MoveWindowToSpace moves the frontmost window to the Mission Control
	// space at the given 1-based index, which the caller has already checked
	// against SpaceCount, and reports the window it moved. The window's ID is
	// not meaningful afterwards: the window has left the active space, so
	// only its PID and Number identify it, and those are what RaiseWindow
	// takes once that space is in front.
	MoveWindowToSpace(index int) (Window, error)

	// RefreshWorkspaceTitle brings any on-screen UI for the active space (the
	// systray's title, on the desktop macOS itself runs) up to date after a
	// space change. It is a no-op wherever there is nothing on screen to
	// update, so every caller of FocusSpace or MoveWindowToSpace may call it
	// unconditionally on success.
	RefreshWorkspaceTitle()
}
