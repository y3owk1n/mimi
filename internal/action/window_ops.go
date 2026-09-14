package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// CloseWindow presses the close button of the window args names. The
// application decides what happens next, as it does after a Command-W.
func (e *Executor) CloseWindow(args WindowArgs) error {
	win, err := e.windowFor(args)
	if err != nil {
		return err
	}

	return e.desktop.CloseWindow(win.ID)
}

// MinimizeWindow minimizes the window args names to the Dock.
func (e *Executor) MinimizeWindow(args WindowArgs) error {
	win, err := e.windowFor(args)
	if err != nil {
		return err
	}

	return e.desktop.SetWindowMinimized(win.ID, true)
}

// UnminimizeWindow restores the window args names from the Dock and brings
// it to the front. The number is required. A minimized window is not on
// any space's window list, so there is no frontmost one to default to.
func (e *Executor) UnminimizeWindow(args WindowArgs) error {
	if args.Number == 0 {
		return derrors.New(derrors.CodeInvalidInput, "unminimize_window needs --number")
	}

	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	minimized, err := e.desktop.MinimizedWindows()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to list minimized windows")
	}

	for _, win := range minimized {
		if win.Number == args.Number {
			return e.desktop.UnminimizeWindow(win.PID, win.Number)
		}
	}

	return derrors.Newf(derrors.CodeActionFailed, "window %d is not minimized", args.Number)
}

// ToggleFullscreenWindow puts the window args names into native full screen,
// or takes it out when it is there already.
func (e *Executor) ToggleFullscreenWindow(args WindowArgs) error {
	win, err := e.windowFor(args)
	if err != nil {
		return err
	}

	fullScreen, err := e.desktop.WindowFullScreen(win.ID)
	if err != nil {
		return err
	}

	return e.desktop.SetWindowFullScreen(win.ID, !fullScreen)
}

// windowFor is the window args names. A zero number names the frontmost
// window, any other the window with that number on the active space.
func (e *Executor) windowFor(args WindowArgs) (Window, error) {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return Window{}, err
	}

	if args.Number == 0 {
		return e.desktop.FrontmostWindow()
	}

	windows, err := e.windowsNamed([]uint32{args.Number})
	if err != nil {
		return Window{}, err
	}

	for _, win := range windows {
		if win.Number == args.Number {
			return win, nil
		}
	}

	return Window{}, derrors.Newf(
		derrors.CodeActionFailed,
		"window %d is not on the active space",
		args.Number,
	)
}
