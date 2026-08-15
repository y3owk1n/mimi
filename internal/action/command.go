package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// Command is one fully-specified, validated instance of an action, built once
// by the caller instead of round-tripped through the string arguments the IPC
// wire format carries. Only the field matching Name is read; the others sit at
// their zero value.
//
// Build one through the constructor its action carries —
// NewFocusWindowCommand, NewSpaceCommand, NewMoveWindowToSpaceCommand or
// NewResizeWindowCommand. Each validates as it builds, so a command's
// arguments are checked once, in one implementation, at the moment the command
// comes into existence: the direct path and the daemon path then reject the
// same argument in the same words, and neither reaches a socket to do it.
//
// The fields stay exported, which makes an ill-formed command unconventional
// to build rather than impossible. That is deliberate: these payloads cross
// the daemon's socket as JSON, where an unexported field encodes to {} and
// reports no error (see docs/adr/0001-typed-versioned-daemon-wire.md).
//
// ExecuteCommand is the direct-execution counterpart to Execute: Execute
// exists because the IPC wire format is strings, and ExecuteCommand exists
// because the direct-execution path never needed to be.
type Command struct {
	Name Name

	FocusWindow       FocusWindowArgs
	Space             SpaceArg
	MoveWindowToSpace SpaceArg
	ResizeWindow      ResizeWindowArgs
}

// FocusWindowArgs is focus_window's typed payload.
type FocusWindowArgs struct {
	Backward bool
	// Direction is "", "up", "down", "left", or "right".
	Direction string
}

// NewFocusWindowCommand builds focus_window's command directly from the CLI's
// already-typed flags, without a string round trip. It applies the same rule
// parseFocusWindowArgs enforces on the string path: at most one direction, and
// never alongside --backward.
func NewFocusWindowCommand(
	backward, focusUp, focusDown, focusLeft, focusRight bool,
) (Command, error) {
	direction, err := focusDirectionOf(focusUp, focusDown, focusLeft, focusRight)
	if err != nil {
		return Command{}, err
	}

	err = validateFocusWindowCombo(direction, backward)
	if err != nil {
		return Command{}, err
	}

	return Command{
		Name:        NameFocusWindow,
		FocusWindow: FocusWindowArgs{Backward: backward, Direction: direction},
	}, nil
}

// NewSpaceCommand builds space's command from the one positional argument the
// action takes, rejecting an argument that names no space. The rule is
// ParseSpaceArg's, called rather than restated.
func NewSpaceCommand(args []string) (Command, error) {
	spaceArg, err := ParseSpaceArg(NameSpace, args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameSpace, Space: spaceArg}, nil
}

// NewMoveWindowToSpaceCommand builds move_window_to_space's command from the
// one positional argument the action takes. It is NewSpaceCommand's
// counterpart: the same rule, reported against this action's name, landing on
// the field this action reads.
func NewMoveWindowToSpaceCommand(args []string) (Command, error) {
	spaceArg, err := ParseSpaceArg(NameMoveWindowToSpace, args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameMoveWindowToSpace, MoveWindowToSpace: spaceArg}, nil
}

// NewResizeWindowCommand builds resize_window's command from the CLI's raw
// flags, rejecting arguments the geometry cannot be asked for.
//
// It validates by running ResizeRequestFromArgs and keeping only its
// rejection: the conversion from raw arguments to a geometry request is the
// rule, so a preset name, a size, a percentage or an anchor is checked here in
// exactly the words every other path checks it in. The command keeps the raw
// arguments rather than the request the conversion built, because those are
// what the socket carries and the request is not something that survives the
// trip.
func NewResizeWindowCommand(args ResizeWindowArgs) (Command, error) {
	_, err := ResizeRequestFromArgs(args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameResizeWindow, ResizeWindow: args}, nil
}

// focusDirectionOf turns the four direction flags into the one direction
// they name, rejecting more than one set at once.
func focusDirectionOf(focusUp, focusDown, focusLeft, focusRight bool) (string, error) {
	direction := ""

	for _, candidate := range []struct {
		set  bool
		name string
	}{{focusUp, "up"}, {focusDown, "down"}, {focusLeft, "left"}, {focusRight, "right"}} {
		if !candidate.set {
			continue
		}

		if direction != "" {
			return "", derrors.New(
				derrors.CodeInvalidInput,
				"only one direction flag allowed (--up, --down, --left, --right)",
			)
		}

		direction = candidate.name
	}

	return direction, nil
}

// validateFocusWindowCombo rejects a direction combined with --backward — the
// one rule both the typed and the string-parsed paths enforce.
func validateFocusWindowCombo(direction string, backward bool) error {
	if direction != "" && backward {
		return derrors.New(
			derrors.CodeInvalidInput,
			"--backward cannot be combined with a direction flag",
		)
	}

	return nil
}

// ResizeWindowArgs is resize_window's typed payload: the CLI's raw flags,
// each paired with a Set bit so a flag the user never gave can be told apart
// from one given as its zero value — the same distinction
// cobraCmd.Flags().Changed(...) makes at the CLI layer.
type ResizeWindowArgs struct {
	// Preset is a named tiling shortcut, or "" for none. It stays a raw name
	// rather than a geometry.Preset because this is the payload the socket
	// carries, and a value whose fields are unexported does not survive the
	// trip; ResizeRequestFromArgs is what turns it into one, and rejects a
	// name that is not a preset.
	Preset string

	Width    int
	WidthSet bool

	Height    int
	HeightSet bool

	WidthPercent    float64
	WidthPercentSet bool

	HeightPercent    float64
	HeightPercentSet bool

	X    int
	XSet bool

	Y    int
	YSet bool

	// Anchor is a two-letter anchor name (tl, cc, br, ...).
	Anchor    string
	AnchorSet bool

	UseMargin bool
	NoMargin  bool
}

// ResizeRequestFromArgs turns resize_window's already-typed CLI flags into
// the geometry request they describe. It is ParseResizeRequest's
// direct-execution counterpart: the same range checks and the same
// zero-collapses-to-keep convention (see ParseResizeRequest's doc comment),
// without the string flags that convention was written against.
func ResizeRequestFromArgs(args ResizeWindowArgs) (geometry.Request, error) {
	// The preset is checked first, as the positional argument it is, so a
	// command carrying both a mistyped preset and a bad flag is rejected for
	// the preset on this path too.
	var preset geometry.Preset

	if args.Preset != "" {
		named, err := ParseResizePreset(args.Preset)
		if err != nil {
			return geometry.Request{}, err
		}

		preset = named
	}

	if args.WidthSet && args.Width < 0 {
		return geometry.Request{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"invalid width: %d",
			args.Width,
		)
	}

	if args.HeightSet && args.Height < 0 {
		return geometry.Request{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"invalid height: %d",
			args.Height,
		)
	}

	if args.WidthPercentSet && (args.WidthPercent < 0 || args.WidthPercent > percentageWhole) {
		return geometry.Request{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"invalid width-percent: %v (0-100)",
			args.WidthPercent,
		)
	}

	if args.HeightPercentSet && (args.HeightPercent < 0 || args.HeightPercent > percentageWhole) {
		return geometry.Request{}, derrors.Newf(
			derrors.CodeInvalidInput,
			"invalid height-percent: %v (0-100)",
			args.HeightPercent,
		)
	}

	req := geometry.Request{Preset: preset}

	width, widthPercent := 0.0, 0.0
	if args.WidthSet {
		width = float64(args.Width)
	}

	if args.WidthPercentSet {
		widthPercent = args.WidthPercent
	}

	req.Width = dimensionOf(width, widthPercent)

	height, heightPercent := 0.0, 0.0
	if args.HeightSet {
		height = float64(args.Height)
	}

	if args.HeightPercentSet {
		heightPercent = args.HeightPercent
	}

	req.Height = dimensionOf(height, heightPercent)

	if args.XSet {
		x := float64(args.X)
		req.X = &x
	}

	if args.YSet {
		y := float64(args.Y)
		req.Y = &y
	}

	if args.AnchorSet {
		anchor, ok := geometry.ParseAnchor(args.Anchor)
		if !ok {
			return geometry.Request{}, derrors.Newf(
				derrors.CodeInvalidInput,
				"invalid anchor: %q (use tl, tc, tr, cl, cc, cr, bl, bc, br)",
				args.Anchor,
			)
		}

		req.Anchor = &anchor
	}

	if args.UseMargin {
		useMargins := true
		req.UseMargins = &useMargins
	}

	if args.NoMargin {
		useMargins := false
		req.UseMargins = &useMargins
	}

	return req, nil
}

// ExecuteCommand runs cmd against the desktop mimi is running on.
func ExecuteCommand(cmd Command) error {
	return defaultExecutor.ExecuteCommand(cmd)
}

// ExecuteCommand runs a fully-typed command. It is Execute's
// direct-execution counterpart: no string carries the arguments here, so
// nothing has to re-parse one.
func (e *Executor) ExecuteCommand(cmd Command) error {
	switch cmd.Name {
	case NameFocusWindow:
		return e.FocusWindow(cmd.FocusWindow.Backward, cmd.FocusWindow.Direction)
	case NameSpace:
		index, err := e.resolveSpaceArgTyped(cmd.Space)
		if err != nil {
			return err
		}

		return e.FocusSpace(index)
	case NameMoveWindowToSpace:
		index, err := e.resolveSpaceArgTyped(cmd.MoveWindowToSpace)
		if err != nil {
			return err
		}

		return e.MoveWindowToSpace(index)
	case NameResizeWindow:
		req, err := ResizeRequestFromArgs(cmd.ResizeWindow)
		if err != nil {
			return err
		}

		return e.ResizeWindow(req)
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown action %q (supported: focus_window, space, move_window_to_space, resize_window)",
			cmd.Name,
		)
	}
}
