package action

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// Command is one fully-specified, validated instance of an action. Only the
// field matching Name is read; the others sit at their zero value.
//
// Build one through the constructor its action carries —
// NewFocusWindowCommand, NewSpaceCommand, NewMoveWindowToSpaceCommand or
// NewResizeWindowCommand. Each validates as it builds, so a command's
// arguments are checked once, in one implementation, at the moment the command
// comes into existence: the direct path and the daemon path then reject the
// same argument in the same words, and neither reaches a socket to do it.
//
// This is also the value the daemon's socket carries, as JSON, which is why it
// carries JSON tags and why they are worth keeping stable: a daemon that has
// not restarted reads the names below, not the Go ones (see
// docs/adr/0001-typed-versioned-daemon-wire.md, and
// TestRequest_EncodesTheGoldenBytes in internal/ipc, which pins them). The
// action's name lives here and nowhere else on the wire.
//
// The fields stay exported, which makes an ill-formed command unconventional
// to build rather than impossible. That is deliberate: an unexported field
// encodes to {} and reports no error, which would silently empty a payload on
// the daemon path while the direct path worked. What stands in for the
// constructor on that path is ExecuteCommand, which re-checks the payload it
// decoded before driving the desktop with it.
type Command struct {
	Name Name `json:"name"`

	FocusWindow       FocusWindowArgs  `json:"focusWindow,omitzero"`
	Space             SpaceArg         `json:"space,omitzero"`
	MoveWindowToSpace SpaceArg         `json:"moveWindowToSpace,omitzero"`
	ResizeWindow      ResizeWindowArgs `json:"resizeWindow,omitzero"`
}

// FocusWindowArgs is focus_window's typed payload.
type FocusWindowArgs struct {
	Backward bool `json:"backward"`
	// Direction is "", "up", "down", "left", or "right".
	Direction string `json:"direction"`
}

// NewFocusWindowCommand builds focus_window's command directly from the CLI's
// already-typed flags. At most one direction flag may be set, which is a rule
// only the flags can break; the payload it builds then goes through
// validateFocusWindowArgs, the same check ExecuteCommand applies to a payload
// that arrived off the socket.
func NewFocusWindowCommand(
	backward, focusUp, focusDown, focusLeft, focusRight bool,
) (Command, error) {
	direction, err := focusDirectionOf(focusUp, focusDown, focusLeft, focusRight)
	if err != nil {
		return Command{}, err
	}

	args := FocusWindowArgs{Backward: backward, Direction: direction}

	err = validateFocusWindowArgs(args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameFocusWindow, FocusWindow: args}, nil
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

// validateFocusWindowArgs holds focus_window's two payload rules: a direction
// has to be one, and it can never be combined with --backward.
//
// Both the constructor and ExecuteCommand call it, so the rules are enforced
// once wherever a payload comes from — the CLI's flags, or a socket the daemon
// decoded a command off with nothing having checked it.
func validateFocusWindowArgs(args FocusWindowArgs) error {
	if args.Direction == "" {
		return nil
	}

	_, err := parseFocusDirection(args.Direction)
	if err != nil {
		return err
	}

	if args.Backward {
		return derrors.New(
			derrors.CodeInvalidInput,
			"--backward cannot be combined with a direction flag",
		)
	}

	return nil
}

// parseFocusDirection maps focus_window's direction argument onto the
// direction it names, and is the one place a name that is not one of them is
// rejected.
//
// It runs twice on the way to a directional focus — once here as the payload's
// check, once inside FocusWindow, which takes the name as a string because ""
// has to mean "cycle instead" and geometry.Direction has no room for that.
// Both calls are the same rule rather than two copies of it, which is what
// makes an unknown name read the same wherever it is caught.
func parseFocusDirection(name string) (geometry.Direction, error) {
	direction, ok := geometry.ParseDirection(name)
	if !ok {
		return 0, derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown direction %q (use up, down, left, or right)",
			name,
		)
	}

	return direction, nil
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
	Preset string `json:"preset"`

	Width    int  `json:"width"`
	WidthSet bool `json:"widthSet"`

	Height    int  `json:"height"`
	HeightSet bool `json:"heightSet"`

	WidthPercent    float64 `json:"widthPercent"`
	WidthPercentSet bool    `json:"widthPercentSet"`

	HeightPercent    float64 `json:"heightPercent"`
	HeightPercentSet bool    `json:"heightPercentSet"`

	X    int  `json:"x"`
	XSet bool `json:"xSet"`

	Y    int  `json:"y"`
	YSet bool `json:"ySet"`

	// Anchor is a two-letter anchor name (tl, cc, br, ...).
	Anchor    string `json:"anchor"`
	AnchorSet bool   `json:"anchorSet"`

	UseMargin bool `json:"useMargin"`
	NoMargin  bool `json:"noMargin"`
}

// ResizeRequestFromArgs turns resize_window's arguments into the geometry
// request they describe, and is the only conversion between the two.
//
// It is also resize_window's validation: the range checks below are the rules,
// so the constructor runs it for its rejection and ExecuteCommand runs it
// again after a decode, rather than either restating what it enforces. That is
// deliberate — a second implementation of the rules is the defect
// docs/adr/0001-typed-versioned-daemon-wire.md exists to remove.
//
// The optional fields of the request it builds are pointers because their
// absence is a real input to the geometry: an anchor nobody gave is what lets
// a preset supply one, and a margin preference nobody expressed is what defers
// to the system setting. A zero --width or --width-percent keeps the window's
// current size rather than collapsing it, which is the convention the CLI has
// always followed.
func ResizeRequestFromArgs(args ResizeWindowArgs) (geometry.Request, error) {
	// The preset is checked first, as the positional argument it is, so a
	// command carrying both a mistyped preset and a bad flag is rejected for
	// the preset on this path too. That is what makes the daemon path agree
	// with the CLI's, where the preset is rejected in the Args layer before a
	// flag is ever read.
	//
	// ParseResizePresetArg is that layer's rule as well as this one's, called
	// rather than restated, so the empty argument means the same thing here as
	// it does there.
	preset, err := ParseResizePresetArg(args.Preset)
	if err != nil {
		return geometry.Request{}, err
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

// ExecuteCommand runs a fully-typed command.
//
// Every branch checks the payload it is handed before driving the desktop with
// it. On the direct path that check has already passed at the constructor and
// costs nothing; on the daemon path the command was decoded off a socket, so
// this is the first thing that has looked at it. The checks are the rules
// themselves — validateFocusWindowArgs, validateSpaceArg and the conversion
// ResizeRequestFromArgs — rather than second copies of them, so both paths
// reject the same payload in the same words.
func (e *Executor) ExecuteCommand(cmd Command) error {
	switch cmd.Name {
	case NameFocusWindow:
		err := validateFocusWindowArgs(cmd.FocusWindow)
		if err != nil {
			return err
		}

		return e.FocusWindow(cmd.FocusWindow.Backward, cmd.FocusWindow.Direction)
	case NameSpace:
		index, err := e.resolveSpaceArg(NameSpace, cmd.Space)
		if err != nil {
			return err
		}

		return e.FocusSpace(index)
	case NameMoveWindowToSpace:
		index, err := e.resolveSpaceArg(NameMoveWindowToSpace, cmd.MoveWindowToSpace)
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
