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

// WindowInfo is what a window query reports about the frontmost window.
type WindowInfo struct {
	PID   int   `json:"pid"`
	Frame Frame `json:"frame"`
}

// QuerySpace reports the active Mission Control space on the desktop mimi is
// running on.
func QuerySpace() (SpaceInfo, error) {
	return defaultExecutor.QuerySpace()
}

// QueryWindow reports the frontmost window on the desktop mimi is running on.
func QueryWindow() (WindowInfo, error) {
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

// QueryWindow reports the frontmost window and its frame.
//
// The frame is read through Accessibility, so this checks the permission the
// way resize_window does, and reports the same window resize_window would act
// on.
func (e *Executor) QueryWindow() (WindowInfo, error) {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return WindowInfo{}, err
	}

	win, err := e.desktop.FrontmostWindow()
	if err != nil {
		return WindowInfo{}, err
	}

	frame, err := e.desktop.WindowFrame(win.ID)
	if err != nil {
		return WindowInfo{}, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to get window frame",
		)
	}

	return WindowInfo{
		PID: win.PID,
		Frame: Frame{
			X:      frame.X,
			Y:      frame.Y,
			Width:  frame.W,
			Height: frame.H,
		},
	}, nil
}
