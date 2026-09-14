package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// FocusDisplay makes a display the active one and focuses the window in
// front on it, when it has one. "next" and "prev" step from the display the
// frontmost window is on, which is where the user is working.
func (e *Executor) FocusDisplay(arg DisplayArg) error {
	err := validateDisplayArg(NameFocusDisplay, arg)
	if err != nil {
		return err
	}

	err = e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	displays, err := e.QueryDisplays()
	if err != nil {
		return err
	}

	fromIndex := 0

	if arg.Direction != 0 {
		fromIndex, err = e.frontmostDisplayIndex(displays)
		if err != nil {
			return err
		}
	}

	toIndex, err := resolveDisplayArg(arg, fromIndex, len(displays))
	if err != nil {
		return err
	}

	target := displays[toIndex]
	e.desktop.ActivateDisplay(target.ID)

	windows, err := e.QueryWindows()
	if err != nil {
		return err
	}

	front, found := frontWindowOn(target, windows.Windows)
	if !found {
		return nil
	}

	return e.FocusWindowNumber(front.Number)
}

// frontmostDisplayIndex is the 0-based position, among displays, of the one
// holding the frontmost window.
func (e *Executor) frontmostDisplayIndex(displays []DisplayEntry) (int, error) {
	win, err := e.desktop.FrontmostWindow()
	if err != nil {
		return 0, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"no frontmost window to tell which display is current",
		)
	}

	current, err := e.desktop.WindowFrame(win.ID)
	if err != nil {
		return 0, derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get window frame")
	}

	display, found := DisplayOf(frameOf(current), displays)
	if !found {
		return 0, derrors.New(
			derrors.CodeActionFailed,
			"could not tell which display the frontmost window is on",
		)
	}

	return display.Index - 1, nil
}

// frontWindowOn is the window in front on display, by stacking order, among
// windows.
func frontWindowOn(display DisplayEntry, windows []WindowEntry) (WindowEntry, bool) {
	var (
		front WindowEntry
		found bool
	)

	for _, win := range windows {
		if win.Display != display.Index {
			continue
		}

		if !found || win.Order < front.Order {
			front, found = win, true
		}
	}

	return front, found
}
