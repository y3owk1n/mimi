package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// fakeWindow is one window on the fake desktop: an identity, a frame, and the
// failures macOS is allowed to report for it.
type fakeWindow struct {
	id          action.WindowID
	pid         int
	frame       geometry.Rect
	frameErr    error
	activateErr error
	setFrameErr error
}

// fakeDesktop is a desktop made of plain values. Every action's effect lands
// back in these fields, so a test asserts what the desktop looks like
// afterwards rather than which methods ran.
type fakeDesktop struct {
	accessibilityErr error

	windows      []fakeWindow
	focused      int // index into windows; -1 when nothing is focused
	enumerateErr error

	frontmost    action.WindowID
	frontmostErr error

	screen    geometry.Screen
	screenErr error

	missionControlActive bool

	spaceCount     int
	activeSpace    int // 1-based
	activeSpaceErr error
	focusSpaceErr  error
	moveErr        error
	// windowSpace is the space the frontmost window sits on, which is what
	// move_window_to_space is observed through.
	windowSpace int
}

func (d *fakeDesktop) EnsureAccessible() error {
	return d.accessibilityErr
}

func (d *fakeDesktop) FocusableWindows() ([]action.Window, int, error) {
	if d.enumerateErr != nil {
		return nil, -1, d.enumerateErr
	}

	windows := make([]action.Window, len(d.windows))
	for index, win := range d.windows {
		windows[index] = action.Window{ID: win.id, PID: win.pid}
	}

	return windows, d.focused, nil
}

func (d *fakeDesktop) WindowFrame(windowID action.WindowID) (geometry.Rect, error) {
	index, err := d.indexOf(windowID)
	if err != nil {
		return geometry.Rect{}, err
	}

	if d.windows[index].frameErr != nil {
		return geometry.Rect{}, d.windows[index].frameErr
	}

	return d.windows[index].frame, nil
}

func (d *fakeDesktop) ActivateWindow(windowID action.WindowID) error {
	index, err := d.indexOf(windowID)
	if err != nil {
		return err
	}

	if d.windows[index].activateErr != nil {
		return d.windows[index].activateErr
	}

	d.focused = index
	d.frontmost = windowID

	return nil
}

func (d *fakeDesktop) FrontmostWindow() (action.WindowID, error) {
	if d.frontmostErr != nil {
		return 0, d.frontmostErr
	}

	return d.frontmost, nil
}

func (d *fakeDesktop) SetWindowFrame(windowID action.WindowID, frame geometry.Rect) error {
	index, err := d.indexOf(windowID)
	if err != nil {
		return err
	}

	if d.windows[index].setFrameErr != nil {
		return d.windows[index].setFrameErr
	}

	d.windows[index].frame = frame

	return nil
}

func (d *fakeDesktop) ScreenAt(_ geometry.Rect) (geometry.Screen, error) {
	if d.screenErr != nil {
		return geometry.Screen{}, d.screenErr
	}

	return d.screen, nil
}

func (d *fakeDesktop) MissionControlActive() bool {
	return d.missionControlActive
}

func (d *fakeDesktop) SpaceCount() int {
	return d.spaceCount
}

func (d *fakeDesktop) ActiveSpaceIndex() (int, error) {
	if d.activeSpaceErr != nil {
		return 0, d.activeSpaceErr
	}

	return d.activeSpace, nil
}

func (d *fakeDesktop) FocusSpace(index int) error {
	if d.focusSpaceErr != nil {
		return d.focusSpaceErr
	}

	d.activeSpace = index

	return nil
}

func (d *fakeDesktop) MoveWindowToSpace(index int) error {
	if d.moveErr != nil {
		return d.moveErr
	}

	d.windowSpace = index

	return nil
}

// indexOf resolves a window id the way the real adapter's handle table does:
// an id it never handed out is not a window.
func (d *fakeDesktop) indexOf(windowID action.WindowID) (int, error) {
	for index, win := range d.windows {
		if win.id == windowID {
			return index, nil
		}
	}

	return 0, derrors.Newf(derrors.CodeAccessibilityFailed, "unknown window %d", uint64(windowID))
}

// focusedID is the identity of whichever window the desktop ended up focused
// on, or 0 when none is.
func (d *fakeDesktop) focusedID() action.WindowID {
	if d.focused < 0 || d.focused >= len(d.windows) {
		return 0
	}

	return d.windows[d.focused].id
}

// desktopWithWindows builds a desktop holding count windows, ids 1..count,
// focused on the 0-based index given (-1 for none).
func desktopWithWindows(count, focused int) *fakeDesktop {
	windows := make([]fakeWindow, count)
	for index := range windows {
		windows[index] = fakeWindow{id: action.WindowID(index + 1), pid: 100 + index}
	}

	desktop := &fakeDesktop{windows: windows, focused: focused}
	if focused >= 0 && focused < count {
		desktop.frontmost = windows[focused].id
	}

	return desktop
}

// wantFocused fails unless the desktop ended up focused on want.
func wantFocused(t *testing.T, desktop *fakeDesktop, want action.WindowID) {
	t.Helper()

	if got := desktop.focusedID(); got != want {
		t.Fatalf("focused window = %d, want %d", got, want)
	}
}
