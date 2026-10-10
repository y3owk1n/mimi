package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// SpaceInfo is what a space query reports: the space in front and how many
// there are, both counted in Mission Control ordering across every display.
type SpaceInfo struct {
	Index int `json:"index"`
	Count int `json:"count"`
}

// Frame is a window frame as a query prints it, in window coordinates: the
// origin is the top-left corner of the primary display and y grows downward,
// which is the frame resize_window's --x and --y take.
type Frame struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// QuerySpace reports the active Mission Control space on the desktop mimi is
// running on.
func QuerySpace() (SpaceInfo, error) {
	return defaultExecutor.QuerySpace()
}

// QueryWindow reports the frontmost window on the desktop mimi is running on.
func QueryWindow() (WindowEntry, error) {
	return defaultExecutor.QueryWindow()
}

// QuerySpace reports which space is in front and how many there are.
//
// It reads Mission Control the way the space actions do, through SkyLight
// rather than Accessibility, so unlike every action it does not check the
// permission first: the daemon's workspace hooks answer the same question
// without it, and a query that refused would be refusing what the daemon
// already does.
func (e *Executor) QuerySpace() (SpaceInfo, error) {
	count := e.desktop.SpaceCount()
	if count == 0 {
		return SpaceInfo{}, derrors.New(
			derrors.CodeActionFailed,
			"failed to enumerate Mission Control spaces",
		)
	}

	index, err := e.desktop.ActiveSpaceIndex()
	if err != nil {
		return SpaceInfo{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to resolve the active space",
		)
	}

	return SpaceInfo{Index: index, Count: count}, nil
}

// QueryWindow reports the frontmost window as the windows query reports each
// of its windows. It is in front, so its order is 0.
//
// The frame is read through Accessibility, so this checks the permission the
// way resize_window does, and reports the same window resize_window would act
// on.
func (e *Executor) QueryWindow() (WindowEntry, error) {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return WindowEntry{}, err
	}

	win, err := e.desktop.FrontmostWindow()
	if err != nil {
		return WindowEntry{}, err
	}

	frame, err := e.desktop.WindowFrame(win.ID)
	if err != nil {
		return WindowEntry{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get window frame",
		)
	}

	title, _ := e.desktop.WindowTitle(win.ID)
	app, _ := e.desktop.ApplicationInfo(win.PID)

	entry := []WindowEntry{{
		Number:   win.Number,
		PID:      win.PID,
		App:      app.Name,
		BundleID: app.BundleID,
		Title:    title,
		Frame:    frameOf(frame),
	}}
	e.locateWindows(entry)

	return entry[0], nil
}
