package action

import (
	"math"
	"slices"
	"sync"
	"time"

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
	// mu is held for writing while the window set is replaced, and for
	// reading around every use of a window in it, so frames can be written to
	// several windows at once.
	mu      sync.RWMutex
	lastID  WindowID
	windows map[WindowID]*native.Element
	// known is the last enumeration, and knownAt when it was taken.
	known   []Window
	knownAt time.Time
	// frames is the frame each window was last read at or written to,
	// as the application reported it, for the windows of the last
	// enumeration. framesMu covers it: frames are written to several
	// windows at once under mu's read lock.
	framesMu sync.Mutex
	frames   map[WindowID]geometry.Rect
	// listed is the window server's list by number, taken with the last
	// enumeration and again after a write, since a write moves the
	// windows. It answers frames, and titles when the server names them,
	// without a round trip into the application.
	listed map[uint32]native.OnScreenWindow
}

// knownWindowsFor is how long an enumeration is trusted by number. Within
// it a window may close, and its write fails, but it does not leave the
// space unnoticed.
const knownWindowsFor = time.Second

// newNativeDesktop returns the Desktop backed by macOS.
func newNativeDesktop() *nativeDesktop {
	return &nativeDesktop{
		windows: map[WindowID]*native.Element{},
		frames:  map[WindowID]geometry.Rect{},
	}
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

		windows = append(windows, Window{
			ID:     d.registerLocked(element),
			PID:    pid,
			Number: element.Number(),
		})
	}

	d.known = windows
	d.knownAt = time.Now()
	d.relist()

	return windows, focused, nil
}

// KnownWindows is the last enumeration, when it is recent.
func (d *nativeDesktop) KnownWindows() []Window {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if time.Since(d.knownAt) > knownWindowsFor {
		return nil
	}

	return d.known
}

// FindApplication resolves a bundle identifier or a name to a running pid.
func (d *nativeDesktop) FindApplication(query string) (int, error) {
	return native.FindApplication(query)
}

// ApplicationWindows lists an application's real windows on every space,
// front to back, with each space id resolved to its Mission Control index.
func (d *nativeDesktop) ApplicationWindows(pid int) ([]AppWindow, error) {
	found, err := native.ApplicationWindows(pid)
	if err != nil {
		return nil, err
	}

	indexes := native.SpaceIndexes()
	windows := make([]AppWindow, len(found))

	for index, window := range found {
		windows[index] = AppWindow{Number: window.Number, SpaceIndex: indexes[window.SpaceID]}
	}

	return windows, nil
}

// RaiseWindow brings one of an application's windows to the front by number.
func (d *nativeDesktop) RaiseWindow(pid int, number uint32) error {
	return native.RaiseWindowNumber(pid, number)
}

// ReopenApplication reopens an application as a Dock click does.
func (d *nativeDesktop) ReopenApplication(pid int) error {
	return native.ReopenApplication(pid)
}

// FrontmostWindow returns the window currently in front.
func (d *nativeDesktop) FrontmostWindow() (Window, error) {
	element := native.FrontmostWindow()
	if element == nil {
		return Window{}, derrors.New(derrors.CodeActionFailed, "no active window found")
	}

	// As in FocusableWindows, a window whose owning process cannot be read is
	// still the window in front; the pid is reported as 0 rather than the
	// window refused.
	pid, pidErr := element.PID()
	if pidErr != nil {
		pid = 0
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.releaseLocked()

	return Window{ID: d.registerLocked(element), PID: pid, Number: element.Number()}, nil
}

// WindowFrame reads one window's frame.
func (d *nativeDesktop) WindowFrame(windowID WindowID) (geometry.Rect, error) {
	var frame geometry.Rect

	err := d.withWindow(windowID, func(element *native.Element) error {
		// The window server's answer is where the window is on screen,
		// and costs no round trip into the application.
		if listed, ok := d.listing(element); ok {
			frame = geometry.Rect{
				X: listed.Frame.X,
				Y: listed.Frame.Y,
				W: listed.Frame.W,
				H: listed.Frame.H,
			}
			d.rememberFrame(windowID, frame)

			return nil
		}

		posX, posY, width, height, err := element.GetFrame()
		if err != nil {
			return err
		}

		frame = geometry.Rect{X: posX, Y: posY, W: width, H: height}
		d.rememberFrame(windowID, frame)

		return nil
	})

	return frame, err
}

// WindowTitle reads one window's title.
func (d *nativeDesktop) WindowTitle(id WindowID) (string, error) {
	var title string

	err := d.withWindow(id, func(element *native.Element) error {
		// The window server names windows only with Screen Recording
		// granted; otherwise the application is asked.
		if listed, ok := d.listing(element); ok && listed.Named {
			title = listed.Title

			return nil
		}

		title = element.Title()

		return nil
	})

	return title, err
}

// ApplicationInfo describes the running application with the given pid.
func (d *nativeDesktop) ApplicationInfo(pid int) (AppInfo, error) {
	info, err := native.LookupApplication(pid)
	if err != nil {
		return AppInfo{}, err
	}

	return AppInfo{Name: info.Name, BundleID: info.BundleID}, nil
}

// SetWindowFrame moves and resizes one window. A window the application
// last reported at the size asked for is only moved: each write is a round
// trip into the application, some ten milliseconds for a heavy one, and a
// move is the common case, as a layout that scrolls or swaps two windows
// of a size makes.
func (d *nativeDesktop) SetWindowFrame(windowID WindowID, frame geometry.Rect) error {
	return d.withWindow(windowID, func(element *native.Element) error {
		last, known := d.rememberedSize(windowID)
		if known && sameLength(last.W, frame.W) && sameLength(last.H, frame.H) {
			err := element.SetPosition(frame.X, frame.Y)
			if err != nil {
				return err
			}
		} else {
			err := element.SetFrame(frame.X, frame.Y, frame.W, frame.H)
			if err != nil {
				return err
			}
		}

		d.rememberFrame(windowID, frame)
		d.relist()

		return nil
	})
}

// samePoint is how far two lengths may differ and still be the same as
// macOS stores them, in whole points.
const samePoint = 0.5

// sameLength is whether two lengths agree in whole points.
func sameLength(a, b float64) bool {
	return math.Abs(a-b) < samePoint
}

// BeginFrameAnimation prepares to fly the given windows to their frames.
func (d *nativeDesktop) BeginFrameAnimation(
	targets []WindowFrame,
	animation Animation,
) (int, error) {
	frames := make([]native.FrameTarget, 0, len(targets))
	for _, target := range targets {
		frames = append(frames, native.FrameTarget{
			Number: target.Number,
			X:      target.Frame.X,
			Y:      target.Frame.Y,
			Width:  target.Frame.Width,
			Height: target.Frame.Height,
		})
	}

	return native.BeginFrameAnimation(
		frames,
		time.Duration(animation.DurationMS)*time.Millisecond,
		native.Easing(slices.Index(Easings, animation.Easing)),
	)
}

// StartFrameAnimation runs the animation BeginFrameAnimation prepared.
func (d *nativeDesktop) StartFrameAnimation(dropped []uint32) {
	native.StartFrameAnimation(dropped)
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

// Displays lists the connected displays in the order macOS reports them.
func (d *nativeDesktop) Displays() ([]Display, error) {
	found, err := native.Displays()
	if err != nil {
		return nil, err
	}

	displays := make([]Display, len(found))
	for index, display := range found {
		displays[index] = Display{
			ID:      display.ID,
			Frame:   rectOf(display.Frame),
			Visible: rectOf(display.Visible),
		}
	}

	return displays, nil
}

// ActivateDisplay makes the display the active one for the menu bar and
// event routing.
func (d *nativeDesktop) ActivateDisplay(id uint32) {
	native.ActivateDisplay(id)
}

// rectOf is a native frame as the geometry holds it.
func rectOf(frame native.Frame) geometry.Rect {
	return geometry.Rect{X: frame.X, Y: frame.Y, W: frame.W, H: frame.H}
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

// ActiveSpaces is the space in front on every display.
func (d *nativeDesktop) ActiveSpaces() (map[uint32]int, error) {
	displays, err := native.Displays()
	if err != nil {
		return nil, err
	}

	ids := make([]uint32, len(displays))
	for index, display := range displays {
		ids[index] = display.ID
	}

	return native.ActiveSpaceIndexes(ids), nil
}

// FocusSpace switches to the space at the given 1-based index.
func (d *nativeDesktop) FocusSpace(index int) error {
	return native.FocusSpace(index)
}

// MoveWindowToSpace moves the frontmost window to the space at the given
// 1-based index and reports which window it moved.
func (d *nativeDesktop) MoveWindowToSpace(index int) (Window, error) {
	pid, number, err := native.MoveWindowToSpace(index)
	if err != nil {
		return Window{}, err
	}

	return Window{PID: pid, Number: number}, nil
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
	d.mu.RLock()
	defer d.mu.RUnlock()

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

	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	clear(d.frames)
}

func (d *nativeDesktop) rememberFrame(id WindowID, frame geometry.Rect) {
	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	d.frames[id] = frame
}

// rememberedSize is the size the window was last read at or written to,
// when it was.
func (d *nativeDesktop) rememberedSize(id WindowID) (geometry.Rect, bool) {
	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	frame, ok := d.frames[id]

	return frame, ok
}

// relist takes the window server's list afresh.
func (d *nativeDesktop) relist() {
	listed := map[uint32]native.OnScreenWindow{}
	for _, window := range native.OnScreenWindows() {
		listed[window.Number] = window
	}

	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	d.listed = listed
}

// listing is the window server's entry for a window, when it has one.
func (d *nativeDesktop) listing(element *native.Element) (native.OnScreenWindow, bool) {
	number := element.Number()
	if number == 0 {
		return native.OnScreenWindow{}, false
	}

	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	window, ok := d.listed[number]

	return window, ok
}
