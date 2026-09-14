package native

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// MinimizedWindow is one window in the Dock: its owner, its number and its
// title.
type MinimizedWindow struct {
	PID    int
	Number uint32
	Title  string
}

// MinimizedWindows lists every minimized window of every regular
// application, through Accessibility, which lists an application's
// minimized windows whatever space they came from.
func MinimizedWindows() []MinimizedWindow {
	var minimized []MinimizedWindow

	for _, pid := range RegularApplicationPIDs() {
		for _, win := range ApplicationWindowElements(pid) {
			if win.IsWindow {
				isMinimized, err := win.Element.Minimized()
				if err == nil && isMinimized {
					minimized = append(minimized, MinimizedWindow{
						PID:    pid,
						Number: win.Number,
						Title:  win.Element.Title(),
					})
				}
			}

			win.Element.Release()
		}
	}

	return minimized
}

// UnminimizeWindow restores a minimized window from the Dock and brings it
// to the front.
func UnminimizeWindow(pid int, number uint32) error {
	var restored bool

	for _, win := range ApplicationWindowElements(pid) {
		if !restored && win.Number == number {
			err := win.Element.SetMinimized(false)
			if err != nil {
				win.Element.Release()

				return err
			}

			restored = true
		}

		win.Element.Release()
	}

	if !restored {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"window %d is not in application %d's window list",
			number,
			pid,
		)
	}

	return RaiseWindowNumber(pid, number)
}
