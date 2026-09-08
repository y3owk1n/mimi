package action

import (
	"fmt"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// WindowFrame is one entry of apply_frames' payload: a window, by the number
// "mimi query windows" reported for it, and the frame to give it, in window
// coordinates.
type WindowFrame struct {
	Number uint32 `json:"number"`
	Frame  Frame  `json:"frame"`
}

// ApplyFramesArgs is apply_frames' typed payload: the frames to apply, in the
// order they are applied.
type ApplyFramesArgs struct {
	Frames []WindowFrame `json:"frames"`
}

// NewApplyFramesCommand builds apply_frames' command from the decoded
// payload, rejecting one the action would refuse.
func NewApplyFramesCommand(frames []WindowFrame) (Command, error) {
	args := ApplyFramesArgs{Frames: frames}

	err := validateApplyFramesArgs(args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameApplyFrames, ApplyFrames: args}, nil
}

// validateApplyFramesArgs is the one rule an apply_frames payload is held to,
// on both paths: at least one frame, every window named once, and every
// frame with a positive size. A window number of 0 is what a window without
// one reports, and names nothing.
func validateApplyFramesArgs(args ApplyFramesArgs) error {
	if len(args.Frames) == 0 {
		return derrors.New(derrors.CodeInvalidInput, "apply_frames needs at least one frame")
	}

	seen := make(map[uint32]struct{}, len(args.Frames))

	for index, entry := range args.Frames {
		if entry.Number == 0 {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"frames[%d]: window number is required",
				index,
			)
		}

		if _, dup := seen[entry.Number]; dup {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"frames[%d]: window %d is listed more than once",
				index,
				entry.Number,
			)
		}

		seen[entry.Number] = struct{}{}

		if entry.Frame.Width <= 0 || entry.Frame.Height <= 0 {
			return derrors.Newf(
				derrors.CodeInvalidInput,
				"frames[%d]: window %d needs a positive width and height",
				index,
				entry.Number,
			)
		}
	}

	return nil
}

// FocusWindowNumber gives keyboard focus to the window with the given
// number on the active space, the way a layout asks for it after moving
// focus along its own structure.
func (e *Executor) FocusWindowNumber(number uint32) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	windows, _, err := e.desktop.FocusableWindows()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get focusable windows")
	}

	for _, win := range windows {
		if win.Number == number {
			return e.desktop.ActivateWindow(win.ID)
		}
	}

	return derrors.Newf(derrors.CodeActionFailed, "window %d is not on the active space", number)
}

// FocusWindowNumber focuses a window by number on the desktop mimi is
// running on.
func FocusWindowNumber(number uint32) error {
	return defaultExecutor.FocusWindowNumber(number)
}

// ApplyFrames moves and resizes several windows on the active space in one
// action. Every frame is attempted, in order, whatever happened to the ones
// before it: a layout is more useful mostly applied than abandoned at its
// first failure. The action fails when any frame did not land, naming each
// window it could not place and why.
func (e *Executor) ApplyFrames(args ApplyFramesArgs) error {
	err := e.desktop.EnsureAccessible()
	if err != nil {
		return err
	}

	windows, _, err := e.desktop.FocusableWindows()
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeActionFailed, "failed to get focusable windows")
	}

	byNumber := make(map[uint32]WindowID, len(windows))
	for _, win := range windows {
		byNumber[win.Number] = win.ID
	}

	var failures []string

	for _, entry := range args.Frames {
		windowID, ok := byNumber[entry.Number]
		if !ok {
			failures = append(
				failures,
				fmt.Sprintf("window %d is not on the active space", entry.Number),
			)

			continue
		}

		setErr := e.desktop.SetWindowFrame(windowID, rectOfFrame(entry.Frame))
		if setErr != nil {
			failures = append(
				failures,
				fmt.Sprintf("window %d: %s", entry.Number, derrors.Message(setErr)),
			)
		}
	}

	if len(failures) > 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"%d of %d frames not applied: %s",
			len(failures),
			len(args.Frames),
			strings.Join(failures, "; "),
		)
	}

	return nil
}
