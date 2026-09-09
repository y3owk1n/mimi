package action

import (
	"cmp"
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
// It owns every window reference behind the ids it hands out, by window
// number, for as long as the window server has the window. An application
// is asked for its windows only when the server lists a number the desktop
// has not seen: a round trip into an application waits on its main thread,
// and the one just activated is busy, so a pass that asks every application
// waits on the slowest one. No action above this seam ever holds, or has to
// release, a native reference.
type nativeDesktop struct {
	// mu is held for writing while the window set changes, and for reading
	// around every use of a window in it, so frames can be written to
	// several windows at once.
	mu      sync.RWMutex
	lastID  WindowID
	windows map[WindowID]*native.Element
	// entries is what the desktop knows of each window by number.
	entries map[uint32]*windowEntry
	// missing is when an application last failed to list a number the
	// window server has for it, so it is not asked again at once.
	missing map[uint32]time.Time
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
	listed map[uint32]native.ListedWindow
}

// windowEntry is one window the desktop holds a reference to.
type windowEntry struct {
	id      WindowID
	element *native.Element
	pid     int
	// isWindow is whether Accessibility calls it a window, rather than a
	// sheet, a popover or the like, which never changes.
	isWindow bool
}

// knownWindowsFor is how long an enumeration is trusted by number. Within
// it a window may close, and its write fails, but it does not leave the
// space unnoticed.
const knownWindowsFor = time.Second

// missingFor is how long an application is left alone after it did not
// list a window the window server has for it.
const missingFor = 2 * time.Second

// newNativeDesktop returns the Desktop backed by macOS.
func newNativeDesktop() *nativeDesktop {
	return &nativeDesktop{
		windows: map[WindowID]*native.Element{},
		entries: map[uint32]*windowEntry{},
		missing: map[uint32]time.Time{},
		frames:  map[WindowID]geometry.Rect{},
	}
}

// EnsureAccessible reports whether macOS still lets mimi drive the desktop.
func (d *nativeDesktop) EnsureAccessible() error {
	return permissions.FriendlyError(permissions.Check())
}

// FocusableWindows enumerates the focusable windows on the active space:
// the window server's on-screen windows at the ordinary layer owned by a
// regular, visible application, that Accessibility calls windows. They are
// ordered top to bottom, then left to right, then by application, and the
// focused one is the frontmost of them: the window server orders its list
// front to back, and the window with keyboard focus is in front of every
// other application's. Asking the Accessibility server instead was measured
// to wait on the application just activated.
func (d *nativeDesktop) FocusableWindows() ([]Window, int, error) {
	onScreen := native.WindowList(true)
	all := native.WindowList(false)

	d.mu.Lock()
	defer d.mu.Unlock()

	d.evictLocked(all)

	// The numbers the desktop has not seen, by application, each asked
	// once for every window it has, at the same time as the others.
	unknown := map[int]bool{}

	for _, window := range onScreen {
		if window.Layer != 0 || !window.Regular {
			continue
		}

		if _, ok := d.entries[window.Number]; ok {
			continue
		}

		if asked, ok := d.missing[window.Number]; ok && time.Since(asked) < missingFor {
			continue
		}

		unknown[window.PID] = true
	}

	d.learnLocked(unknown)

	windows := make([]Window, 0, len(onScreen))
	focused := uint32(0)

	for _, window := range onScreen {
		if window.Layer != 0 || !window.Regular {
			continue
		}

		entry, ok := d.entries[window.Number]
		if !ok {
			d.missing[window.Number] = time.Now()

			continue
		}

		if !entry.isWindow {
			continue
		}

		if focused == 0 {
			focused = window.Number
		}

		windows = append(windows, Window{ID: entry.id, PID: entry.pid, Number: window.Number})
	}

	frames := map[uint32]native.Frame{}
	for _, window := range onScreen {
		frames[window.Number] = window.Frame
	}

	slices.SortStableFunc(windows, func(left, right Window) int {
		leftFrame, rightFrame := frames[left.Number], frames[right.Number]
		if leftFrame.Y != rightFrame.Y {
			return cmp.Compare(leftFrame.Y, rightFrame.Y)
		}

		if leftFrame.X != rightFrame.X {
			return cmp.Compare(leftFrame.X, rightFrame.X)
		}

		return cmp.Compare(left.PID, right.PID)
	})

	focusedIndex := -1
	for index, window := range windows {
		if window.Number == focused {
			focusedIndex = index
		}
	}

	d.known = windows
	d.knownAt = time.Now()
	d.listed = listing(onScreen)

	return windows, focusedIndex, nil
}

// listing indexes a window list by number.
func listing(windows []native.ListedWindow) map[uint32]native.ListedWindow {
	listed := make(map[uint32]native.ListedWindow, len(windows))
	for _, window := range windows {
		listed[window.Number] = window
	}

	return listed
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

	number := element.Number()

	d.mu.Lock()
	defer d.mu.Unlock()

	// The window in front is kept like any other, by number; one without a
	// number is held until the next enumeration evicts it.
	if entry, ok := d.entries[number]; ok && number != 0 {
		element.Release()

		return Window{ID: entry.id, PID: pid, Number: number}, nil
	}

	d.lastID++
	d.windows[d.lastID] = element
	d.entries[number] = &windowEntry{id: d.lastID, element: element, pid: pid, isWindow: true}

	return Window{ID: d.lastID, PID: pid, Number: number}, nil
}

// WindowFrame reads one window's frame.
func (d *nativeDesktop) WindowFrame(windowID WindowID) (geometry.Rect, error) {
	var frame geometry.Rect

	err := d.withWindow(windowID, func(element *native.Element) error {
		// The window server's answer is where the window is on screen,
		// and costs no round trip into the application.
		if listed, ok := d.listedWindow(element); ok {
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
		if listed, ok := d.listedWindow(element); ok && listed.Named {
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

func (d *nativeDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	displays, err := native.Displays()
	if err != nil {
		return nil, err
	}

	ids := make([]uint32, len(displays))
	for index, display := range displays {
		ids[index] = display.ID
	}

	return native.FullScreenDisplays(ids), nil
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
	listed := listing(native.WindowList(true))

	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	d.listed = listed
}

// listing is the window server's entry for a window, when it has one.
func (d *nativeDesktop) listedWindow(element *native.Element) (native.ListedWindow, bool) {
	number := element.Number()
	if number == 0 {
		return native.ListedWindow{}, false
	}

	d.framesMu.Lock()
	defer d.framesMu.Unlock()

	window, ok := d.listed[number]

	return window, ok
}

// learnLocked asks each given application for its windows, at once, and
// keeps a reference to every one that has a number. The caller holds mu.
func (d *nativeDesktop) learnLocked(pids map[int]bool) {
	if len(pids) == 0 {
		return
	}

	results := make(chan []native.ApplicationWindow, len(pids))

	for pid := range pids {
		go func(pid int) {
			results <- native.ApplicationWindowElements(pid)
		}(pid)
	}

	for range pids {
		for _, window := range <-results {
			if _, ok := d.entries[window.Number]; ok {
				window.Element.Release()

				continue
			}

			pid, err := window.Element.PID()
			if err != nil {
				pid = 0
			}

			d.lastID++
			d.windows[d.lastID] = window.Element
			d.entries[window.Number] = &windowEntry{
				id:       d.lastID,
				element:  window.Element,
				pid:      pid,
				isWindow: window.IsWindow,
			}
			delete(d.missing, window.Number)
		}
	}
}

// evictLocked releases the windows the window server no longer has. The
// caller holds mu.
func (d *nativeDesktop) evictLocked(all []native.ListedWindow) {
	alive := make(map[uint32]bool, len(all))
	for _, window := range all {
		alive[window.Number] = true
	}

	for number, entry := range d.entries {
		if alive[number] {
			continue
		}

		entry.element.Release()
		delete(d.windows, entry.id)
		delete(d.entries, number)

		d.framesMu.Lock()
		delete(d.frames, entry.id)
		d.framesMu.Unlock()
	}

	for number := range d.missing {
		if !alive[number] {
			delete(d.missing, number)
		}
	}
}
