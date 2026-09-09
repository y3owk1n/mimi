package action_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// fakeWindow is one window on the fake desktop: an identity, a frame, and the
// failures macOS is allowed to report for it.
type fakeWindow struct {
	id       action.WindowID
	pid      int
	number   uint32
	frame    geometry.Rect
	title    string
	frameErr error
	// frameErrOnce clears frameErr the next time the window is enumerated,
	// the way a relearned element answers where a dead one did not.
	frameErrOnce bool
	activateErr  error
	setFrameErr  error
	// setFrameWrites counts how many frames were written to the window.
	setFrameWrites int
	// clampsFirstWrite makes the first frame written land one point
	// narrower than asked, the way an application does when it applies a
	// size while it still counts the window as being on the display it just
	// left. Later writes land as asked.
	clampsFirstWrite bool
}

// fakeDesktop is a desktop made of plain values. Every action's effect lands
// back in these fields, so a test asserts what the desktop looks like
// afterwards rather than which methods ran.
type fakeDesktop struct {
	mu sync.Mutex
	// frameWrites is every window written by SetWindowFrame, in order.
	frameWrites []action.WindowID

	accessibilityErr error

	windows      []fakeWindow
	focused      int // index into windows; -1 when nothing is focused
	enumerateErr error

	frontmost    action.WindowID
	frontmostErr error

	screen    geometry.Screen
	screenErr error

	missionControlActive bool

	// apps maps a query focus_app may be given to the pid it names.
	apps map[string]int
	// appInfo describes each running application by pid, as the windows
	// query reports it.
	appInfo map[int]action.AppInfo
	// appWindows lists each application's windows front to back, as
	// ApplicationWindows reports them; every id is one of windows'.
	appWindows map[int][]action.AppWindow
	// reopenedApp is the last application reopened for having no window.
	reopenedApp int

	displays    []action.Display
	displaysErr error
	// activatedDisplay is the display last made active, or 0 for none.
	activatedDisplay uint32

	spaceCount  int
	activeSpace int // 1-based
	// activeSpaces is the space in front per display, when a test sets it;
	// otherwise every display shows activeSpace.
	activeSpaces map[uint32]int
	// fullScreenDisplays is what FullScreenDisplays reports.
	fullScreenDisplays map[uint32]bool
	// enumerations counts FocusableWindows calls.
	enumerations   int
	activeSpaceErr error
	focusSpaceErr  error
	moveErr        error
	// windowSpace is the space the frontmost window sits on, which is what
	// move_window_to_space is observed through.
	windowSpace int
	// movedWindow is the window move_window_to_space last moved, so a raise
	// after the follow can be checked against it.
	movedWindow action.WindowID
	// raiseErr fails every RaiseWindow when set.
	raiseErr error

	// refreshWorkspaceTitleCalls counts how many times the desktop's systray
	// title was asked to catch up with the active space.
	refreshWorkspaceTitleCalls int
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
		windows[index] = action.Window{ID: win.id, PID: win.pid, Number: win.number}

		if win.frameErrOnce && d.enumerations > 0 {
			d.windows[index].frameErr = nil
		}
	}

	d.enumerations++

	return windows, d.focused, nil
}

func (d *fakeDesktop) KnownWindows() []action.Window {
	windows, _, err := d.FocusableWindows()
	if err != nil {
		return nil
	}

	return windows
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

func (d *fakeDesktop) WindowTitle(windowID action.WindowID) (string, error) {
	index, err := d.indexOf(windowID)
	if err != nil {
		return "", err
	}

	return d.windows[index].title, nil
}

func (d *fakeDesktop) ApplicationInfo(pid int) (action.AppInfo, error) {
	info, ok := d.appInfo[pid]
	if !ok {
		return action.AppInfo{}, derrors.Newf(
			derrors.CodeActionFailed,
			"no running application with pid %d",
			pid,
		)
	}

	return info, nil
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

func (d *fakeDesktop) FrontmostWindow() (action.Window, error) {
	if d.frontmostErr != nil {
		return action.Window{}, d.frontmostErr
	}

	index, err := d.indexOf(d.frontmost)
	if err != nil {
		return action.Window{}, err
	}

	return action.Window{
		ID:     d.frontmost,
		PID:    d.windows[index].pid,
		Number: d.windows[index].number,
	}, nil
}

func (d *fakeDesktop) FindApplication(query string) (int, error) {
	pid, ok := d.apps[query]
	if !ok {
		return 0, derrors.Newf(derrors.CodeActionFailed, "no running application named %q", query)
	}

	return pid, nil
}

func (d *fakeDesktop) ApplicationWindows(pid int) ([]action.AppWindow, error) {
	return slices.Clone(d.appWindows[pid]), nil
}

// RaiseWindow models the one constraint the real desktop has: a window is
// reachable through Accessibility only while its space is in front.
func (d *fakeDesktop) RaiseWindow(pid int, number uint32) error {
	if d.raiseErr != nil {
		return d.raiseErr
	}

	// A window that was just moved is on windowSpace, which the fake's
	// appWindows listing does not know about.
	if d.movedWindow != 0 {
		index, err := d.indexOf(d.movedWindow)
		if err == nil && d.windows[index].pid == pid && d.windows[index].number == number {
			if d.windowSpace != d.activeSpace {
				return derrors.Newf(
					derrors.CodeAccessibilityFailed,
					"window %d is on space %d, not the active space %d",
					number,
					d.windowSpace,
					d.activeSpace,
				)
			}

			return d.ActivateWindow(d.movedWindow)
		}
	}

	for _, win := range d.appWindows[pid] {
		if win.Number != number {
			continue
		}

		if win.SpaceIndex != 0 && win.SpaceIndex != d.activeSpace {
			return derrors.Newf(
				derrors.CodeAccessibilityFailed,
				"window %d is on space %d, not the active space %d",
				number,
				win.SpaceIndex,
				d.activeSpace,
			)
		}

		for _, candidate := range d.windows {
			if candidate.pid == pid && candidate.number == number {
				return d.ActivateWindow(candidate.id)
			}
		}
	}

	return derrors.Newf(derrors.CodeAccessibilityFailed, "window %d is not listed", number)
}

func (d *fakeDesktop) ReopenApplication(pid int) error {
	d.reopenedApp = pid

	return nil
}

func (d *fakeDesktop) SetWindowFrame(windowID action.WindowID, frame geometry.Rect) error {
	// apply_frames writes to different applications at once.
	d.mu.Lock()
	defer d.mu.Unlock()

	d.frameWrites = append(d.frameWrites, windowID)

	index, err := d.indexOf(windowID)
	if err != nil {
		return err
	}

	if d.windows[index].setFrameErr != nil {
		return d.windows[index].setFrameErr
	}

	d.windows[index].setFrameWrites++
	if d.windows[index].clampsFirstWrite && d.windows[index].setFrameWrites == 1 {
		frame.W--
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

func (d *fakeDesktop) Displays() ([]action.Display, error) {
	if d.displaysErr != nil {
		return nil, d.displaysErr
	}

	return slices.Clone(d.displays), nil
}

func (d *fakeDesktop) ActivateDisplay(id uint32) {
	d.activatedDisplay = id
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

func (d *fakeDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return d.fullScreenDisplays, nil
}

func (d *fakeDesktop) ActiveSpaces() (map[uint32]int, error) {
	if d.activeSpaceErr != nil {
		return nil, d.activeSpaceErr
	}

	if d.activeSpaces != nil {
		return d.activeSpaces, nil
	}

	spaces := map[uint32]int{}
	for _, display := range d.displays {
		spaces[display.ID] = d.activeSpace
	}

	return spaces, nil
}

func (d *fakeDesktop) FocusSpace(index int) error {
	if d.focusSpaceErr != nil {
		return d.focusSpaceErr
	}

	d.activeSpace = index

	return nil
}

func (d *fakeDesktop) MoveWindowToSpace(index int) (action.Window, error) {
	if d.moveErr != nil {
		return action.Window{}, d.moveErr
	}

	moved, err := d.FrontmostWindow()
	if err != nil {
		return action.Window{}, err
	}

	d.windowSpace = index
	d.movedWindow = moved.ID

	// macOS focuses the next window on the space the moved one left; the
	// fake forgets the frontmost window to stand in for that.
	d.frontmost = 0
	d.focused = -1

	return action.Window{PID: moved.PID, Number: moved.Number}, nil
}

func (d *fakeDesktop) RefreshWorkspaceTitle() {
	d.refreshWorkspaceTitleCalls++
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

// wantRefreshCalls fails unless the desktop's systray title was refreshed
// exactly want times.
func wantRefreshCalls(t *testing.T, desktop *fakeDesktop, want int) {
	t.Helper()

	if got := desktop.refreshWorkspaceTitleCalls; got != want {
		t.Fatalf("refreshWorkspaceTitleCalls = %d, want %d", got, want)
	}
}

// wantFocused fails unless the desktop ended up focused on want.
func wantFocused(t *testing.T, desktop *fakeDesktop, want action.WindowID) {
	t.Helper()

	if got := desktop.focusedID(); got != want {
		t.Fatalf("focused window = %d, want %d", got, want)
	}
}
