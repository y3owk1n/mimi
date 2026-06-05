package actions

import (
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/space"
)

// FocusWindow cycles focus through focusable windows on the
// active space. When backward is true it walks the list in the
// opposite direction. Both directions wrap at the boundaries.
//
// Returns nil on success. Errors are domain errors with
// CodeInvalidInput for caller mistakes and CodeActionFailed for
// platform or accessibility failures.
func FocusWindow(backward bool) error {
	windows, err := space.AllFocusableWindowsOnActiveSpace()
	if err != nil {
		return derrors.Wrap(err, derrors.CodeActionFailed, "enumerate focusable windows")
	}

	defer space.ReleaseAll(windows)

	if len(windows) == 0 {
		return derrors.New(
			derrors.CodeActionFailed,
			"no focusable windows found on the active space",
		)
	}

	// Determine the index of the currently focused window. We use
	// the system-wide frontmost window (kAXFocusedWindowAttribute
	// on the app element) rather than per-window IsFocused(),
	// because kAXFocusedAttribute on window elements is unreliable
	// across applications.
	frontmost := space.FrontmostWindow()
	defer space.ReleaseWindow(frontmost)

	currentIndex := -1
	if frontmost != nil {
		for i, w := range windows {
			if space.ElementsEqual(w, frontmost) {
				currentIndex = i

				break
			}
		}
	}

	targetIndex := nextIndex(currentIndex, len(windows), backward)

	err = space.ActivateWindow(windows[targetIndex])
	if err != nil {
		return derrors.Wrap(err, derrors.CodeActionFailed, "activate target window")
	}

	return nil
}

// Space focuses the Mission Control space at the given 1-based
// index. Refuses to run while Mission Control is visible, since
// the synthetic swipe would fight the user's own gesture.
func Space(index int) error {
	if index < 1 {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number must be a positive integer, got %d",
			index,
		)
	}

	if space.IsMissionControlActive() {
		return derrors.New(
			derrors.CodeActionFailed,
			"cannot switch spaces while Mission Control is active",
		)
	}

	var err error

	count := space.CountMissionControlSpaces()
	if count == 0 {
		return derrors.New(
			derrors.CodeActionFailed,
			"failed to enumerate Mission Control spaces",
		)
	}

	if index > count {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number %d is out of range; valid range is 1..%d",
			index,
			count,
		)
	}

	sid := space.MissionControlSpaceID(index)
	if sid == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve Mission Control space at index %d",
			index,
		)
	}

	did := space.DisplayIDForSpace(sid)
	if did == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve display for Mission Control space at index %d",
			index,
		)
	}

	err = space.FocusSpaceUsingGesture(did, sid)
	if err != nil {
		return derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"focus space at index %d",
			index,
		)
	}

	return nil
}

// MoveWindowToSpace moves the current focused window to the
// Mission Control space at the given 1-based index. Refuses to
// run while Mission Control is visible.
func MoveWindowToSpace(index int) error {
	if index < 1 {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number must be a positive integer, got %d",
			index,
		)
	}

	if space.IsMissionControlActive() {
		return derrors.New(
			derrors.CodeActionFailed,
			"cannot move window while Mission Control is active",
		)
	}

	var err error

	count := space.CountMissionControlSpaces()
	if count == 0 {
		return derrors.New(
			derrors.CodeActionFailed,
			"failed to enumerate Mission Control spaces",
		)
	}

	if index > count {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"space number %d is out of range; valid range is 1..%d",
			index,
			count,
		)
	}

	sid := space.MissionControlSpaceID(index)
	if sid == 0 {
		return derrors.Newf(
			derrors.CodeActionFailed,
			"failed to resolve Mission Control space at index %d",
			index,
		)
	}

	frontmost := space.FrontmostWindow()
	if frontmost == nil {
		return derrors.New(
			derrors.CodeActionFailed,
			"no active window found to move",
		)
	}
	defer space.ReleaseWindow(frontmost)

	err = space.MoveWindowToSpace(frontmost, sid)
	if err != nil {
		return derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"move window to space at index %d",
			index,
		)
	}

	return nil
}

// ParseIndexArg extracts and validates a single 1-based index
// argument. actionName is interpolated into the error message
// ("<actionName> requires exactly one positional argument...").
// The returned *derrors.Error is suitable for direct return from
// a cobra RunE.
func ParseIndexArg(args []string, actionName string) (int, error) {
	if len(args) != 1 {
		return 0, derrors.Newf(
			derrors.CodeInvalidInput,
			"%s requires exactly one positional argument: the 1-based space number",
			actionName,
		)
	}

	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return 0, derrors.New(
			derrors.CodeInvalidInput,
			"space number cannot be empty",
		)
	}

	index, parseErr := strconv.Atoi(raw)
	if parseErr != nil || index < 1 {
		return 0, derrors.Newf(
			derrors.CodeInvalidInput,
			"space number must be a positive integer, got %q",
			raw,
		)
	}

	return index, nil
}

// nextIndex returns the next focusable-window index relative to
// currentIndex. currentIndex may be -1 (no current window),
// which causes the function to return 0 (or len-1 when
// backward). The result always wraps into the [0, total) range.
func nextIndex(currentIndex, total int, backward bool) int {
	if total <= 0 {
		return 0
	}

	if backward {
		if currentIndex < 0 {
			return total - 1
		}

		return (currentIndex - 1 + total) % total
	}

	if currentIndex < 0 {
		return 0
	}

	return (currentIndex + 1) % total
}
