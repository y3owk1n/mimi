package space

import derrors "github.com/y3owk1n/mimi/internal/errors"

var (
	errNoWindow = derrors.New(
		derrors.CodeActionFailed,
		"no window reference provided",
	)

	errActivateFailed = derrors.New(
		derrors.CodeActionFailed,
		"failed to activate window",
	)

	errFocusSpaceFailed = derrors.New(
		derrors.CodeActionFailed,
		"failed to focus Mission Control space",
	)

	errMoveWindowFailed = derrors.New(
		derrors.CodeActionFailed,
		"failed to move window to space",
	)
)
