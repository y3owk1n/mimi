package action

import (
	"sync"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/systray"
)

// nativeDesktop is the Desktop macOS itself: the adapter between the actions'
// values and internal/native's handles.
//
// It owns every window reference behind the ids it hands out. A generation of
// references lives until the next lookup replaces it, at which point the whole
// previous generation is released — so no action above this seam ever holds,
// or has to release, a native reference.
type nativeDesktop struct {
	mu      sync.Mutex
	lastID  WindowID
	windows map[WindowID]*native.Element
}

// newNativeDesktop returns the Desktop backed by macOS.
func newNativeDesktop() *nativeDesktop {
	return &nativeDesktop{windows: map[WindowID]*native.Element{}}
}

// EnsureAccessible reports whether macOS still lets mimi drive the desktop.
func (d *nativeDesktop) EnsureAccessible() error {
	return permissions.FriendlyError(permissions.Check())
}

// FocusableWindows enumerates the focusable windows on the active space.
func (d *nativeDesktop) FocusableWindows() ([]Window, int, error) {
	elements, focused, err := native.AllFocusableOnActiveSpaceWithFocused()
	if err != nil {
		return nil, -1, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.releaseLocked()

	windows := make([]Window, 0, len(elements))

	for _, element := range elements {
		if element == nil {
			continue
		}

		// A window whose owning process cannot be read still cycles; the pid
		// is what tells applications apart, not what makes a window valid.
		pid, pidErr := element.PID()
		if pidErr != nil {
			pid = 0
		}

		windows = append(windows, Window{ID: d.registerLocked(element), PID: pid})
	}

	return windows, focused, nil
}

// FrontmostWindow returns the window currently in front.
func (d *nativeDesktop) FrontmostWindow() (WindowID, error) {
	element := native.FrontmostWindow()
	if element == nil {
		return 0, derrors.New(derrors.CodeActionFailed, "no active window found")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.releaseLocked()

	return d.registerLocked(element), nil
}

// WindowFrame reads one window's frame.
func (d *nativeDesktop) WindowFrame(id WindowID) (geometry.Rect, error) {
	var frame geometry.Rect

	err := d.withWindow(id, func(element *native.Element) error {
		posX, posY, width, height, err := element.GetFrame()
		if err != nil {
			return err
		}

		frame = geometry.Rect{X: posX, Y: posY, W: width, H: height}

		return nil
	})

	return frame, err
}

// SetWindowFrame moves and resizes one window.
func (d *nativeDesktop) SetWindowFrame(id WindowID, frame geometry.Rect) error {
	return d.withWindow(id, func(element *native.Element) error {
		return element.SetFrame(frame.X, frame.Y, frame.W, frame.H)
	})
}

// ActivateWindow raises a window's application and focuses the window.
func (d *nativeDesktop) ActivateWindow(id WindowID) error {
	return d.withWindow(id, func(element *native.Element) error {
		return element.Activate()
	})
}

// ScreenAt describes the screen the given window sits on.
func (d *nativeDesktop) ScreenAt(window geometry.Rect) (geometry.Screen, error) {
	// The visible frame of the screen containing the window, in NSScreen y-up
	// coordinates.
	visX, visY, visW, visH, err := native.ScreenVisibleFrame(window.X, window.Y)
	if err != nil {
		return geometry.Screen{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get screen frame",
		)
	}

	// The primary screen's height, which is the constant relating the y-up
	// screen coordinates to the y-down window ones. Resize applies the
	// conversion itself.
	primaryH, err := native.PrimaryScreenHeight()
	if err != nil {
		return geometry.Screen{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get primary screen height",
		)
	}

	return geometry.Screen{
		Visible:        geometry.Rect{X: visX, Y: visY, W: visW, H: visH},
		PrimaryHeight:  primaryH,
		MarginsEnabled: native.TiledWindowMarginsEnabled(),
		MarginSize:     native.TiledWindowMarginSize(),
	}, nil
}

// MissionControlActive reports whether Mission Control is open.
func (d *nativeDesktop) MissionControlActive() bool {
	return native.MissionControlActive()
}

// SpaceCount is how many Mission Control spaces exist.
func (d *nativeDesktop) SpaceCount() int {
	return native.SpaceCount()
}

// ActiveSpaceIndex is the 1-based index of the space in front.
func (d *nativeDesktop) ActiveSpaceIndex() (int, error) {
	return native.ActiveSpaceIndex()
}

// FocusSpace switches to the space at the given 1-based index.
func (d *nativeDesktop) FocusSpace(index int) error {
	return native.FocusSpace(index)
}

// MoveWindowToSpace moves the frontmost window to the space at the given
// 1-based index.
func (d *nativeDesktop) MoveWindowToSpace(index int) error {
	return native.MoveWindowToSpace(index)
}

// RefreshWorkspaceTitle brings the systray's title up to date with the active
// space. internal/systray already no-ops this when the tray is disabled or
// was never started, which is what keeps it harmless to call from a CLI
// invocation with no daemon running.
func (d *nativeDesktop) RefreshWorkspaceTitle() {
	systray.RefreshWorkspaceTitle()
}

// withWindow runs apply against the reference behind windowID, holding the lock
// for as long as apply does — which is what stops a later lookup releasing the
// reference out from under it.
func (d *nativeDesktop) withWindow(
	windowID WindowID,
	apply func(*native.Element) error,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	element, ok := d.windows[windowID]
	if !ok || element == nil {
		return derrors.Newf(
			derrors.CodeAccessibilityFailed,
			"window %d is no longer available",
			uint64(windowID),
		)
	}

	return apply(element)
}

// registerLocked takes ownership of one native reference and returns the id
// standing for it. The caller must hold the lock.
func (d *nativeDesktop) registerLocked(element *native.Element) WindowID {
	d.lastID++
	d.windows[d.lastID] = element

	return d.lastID
}

// releaseLocked releases every reference the desktop holds. The caller must
// hold the lock.
func (d *nativeDesktop) releaseLocked() {
	for id, element := range d.windows {
		element.Release()
		delete(d.windows, id)
	}
}
