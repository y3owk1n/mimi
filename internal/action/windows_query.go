package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// WindowEntry is one window as the window listing reports it: the window
// server's number, which is what apply_frames takes back, the application
// behind it, and its frame in window coordinates.
type WindowEntry struct {
	Number   uint32 `json:"number"`
	PID      int    `json:"pid"`
	App      string `json:"app"`
	BundleID string `json:"bundleId"`
	Title    string `json:"title"`
	Frame    Frame  `json:"frame"`
}

// WindowsInfo is what a windows query reports: every focusable window on the
// active space, in the order focus cycles through them, and the index of the
// focused one among them, or -1 when none holds focus.
type WindowsInfo struct {
	Focused int           `json:"focused"`
	Windows []WindowEntry `json:"windows"`
}

// DisplayEntry is one display as the displays query reports it: its 1-based
// index in the order move_window_to_display counts them, its identifier, and
// its frames in window coordinates.
type DisplayEntry struct {
	Index   int    `json:"index"`
	ID      uint32 `json:"id"`
	Frame   Frame  `json:"frame"`
	Visible Frame  `json:"visible"`
}

// QueryActiveSpaces reports the space in front on every display of the
// desktop mimi is running on.
func QueryActiveSpaces() (map[uint32]int, error) {
	return defaultExecutor.QueryActiveSpaces()
}

// QueryActiveSpaces reports the space in front on every display, keyed by
// display id, the way QuerySpace reports the cursor's: through SkyLight, with
// no Accessibility needed.
func (e *Executor) QueryActiveSpaces() (map[uint32]int, error) {
	spaces, err := e.desktop.ActiveSpaces()
	if err != nil {
		return nil, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to resolve the active spaces",
		)
	}

	if len(spaces) == 0 {
		return nil, derrors.New(
			derrors.CodeActionFailed,
			"failed to enumerate Mission Control spaces",
		)
	}

	return spaces, nil
}

// QueryWindows lists the focusable windows on the active space of the desktop
// mimi is running on.
func QueryWindows() (WindowsInfo, error) {
	return defaultExecutor.QueryWindows()
}

// QueryDisplays lists the connected displays of the desktop mimi is running
// on.
func QueryDisplays() ([]DisplayEntry, error) {
	return defaultExecutor.QueryDisplays()
}

// QueryWindows lists every focusable window on the active space with the
// frame apply_frames would overwrite.
//
// It reads through Accessibility, as focus_window does, so it checks the
// permission first and lists exactly the windows focus_window cycles. A
// window whose frame cannot be read is left out: without a frame there is
// nothing a layout can do with it, and the listing is more useful complete
// than refused. A missing title or application is reported as "" and the
// window kept.
func (e *Executor) QueryWindows() (WindowsInfo, error) {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return WindowsInfo{}, err
	}

	windows, focused, err := e.desktop.FocusableWindows()
	if err != nil {
		return WindowsInfo{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get focusable windows",
		)
	}

	info := WindowsInfo{Focused: -1, Windows: make([]WindowEntry, 0, len(windows))}

	for index, win := range windows {
		frame, frameErr := e.desktop.WindowFrame(win.ID)
		if frameErr != nil {
			continue
		}

		title, _ := e.desktop.WindowTitle(win.ID)
		app, _ := e.desktop.ApplicationInfo(win.PID)

		if index == focused {
			info.Focused = len(info.Windows)
		}

		info.Windows = append(info.Windows, WindowEntry{
			Number:   win.Number,
			PID:      win.PID,
			App:      app.Name,
			BundleID: app.BundleID,
			Title:    title,
			Frame:    frameOf(frame),
		})
	}

	return info, nil
}

// QueryDisplays lists the connected displays, numbered the way
// move_window_to_display counts them, with their frames in window
// coordinates so a layout can place windows on them directly. It needs no
// Accessibility permission: it reads the screens, not any window.
func (e *Executor) QueryDisplays() ([]DisplayEntry, error) {
	displays, err := e.desktop.Displays()
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeActionFailed, "failed to enumerate displays")
	}

	if len(displays) == 0 {
		return nil, derrors.New(derrors.CodeActionFailed, "no displays found")
	}

	// The origin of window coordinates is the top-left corner of the primary
	// display, so the screen at it is the primary one, which is where the
	// height relating the two coordinate systems comes from.
	screen, err := e.desktop.ScreenAt(geometry.Rect{X: 0, Y: 0, W: 1, H: 1})
	if err != nil {
		return nil, err
	}

	orderDisplays(displays)

	entries := make([]DisplayEntry, len(displays))
	for index, display := range displays {
		entries[index] = DisplayEntry{
			Index:   index + 1,
			ID:      display.ID,
			Frame:   frameOf(geometry.WindowBounds(display.Frame, screen.PrimaryHeight)),
			Visible: frameOf(geometry.WindowBounds(display.Visible, screen.PrimaryHeight)),
		}
	}

	return entries, nil
}

// frameOf is a rect as a query prints it.
func frameOf(rect geometry.Rect) Frame {
	return Frame{X: rect.X, Y: rect.Y, Width: rect.W, Height: rect.H}
}

// rectOfFrame is a printed frame as the geometry holds it.
func rectOfFrame(frame Frame) geometry.Rect {
	return geometry.Rect{X: frame.X, Y: frame.Y, W: frame.Width, H: frame.Height}
}
