package action

import (
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// Command is one fully-specified, validated instance of an action. Only the
// field matching Name is read; the others sit at their zero value.
//
// Build one through the constructor its action carries —
// NewFocusWindowCommand, NewSpaceCommand, NewMoveWindowToSpaceCommand,
// NewMoveWindowToDisplayCommand or NewResizeWindowCommand. Each validates as it builds, so a command's
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

	FocusWindow         FocusWindowArgs       `json:"focusWindow,omitzero"`
	Space               SpaceArg              `json:"space,omitzero"`
	MoveWindowToSpace   MoveWindowToSpaceArgs `json:"moveWindowToSpace,omitzero"`
	MoveWindowToDisplay DisplayArg            `json:"moveWindowToDisplay,omitzero"`
	ResizeWindow        ResizeWindowArgs      `json:"resizeWindow,omitzero"`
	FocusApp            FocusAppArgs          `json:"focusApp,omitzero"`
	ApplyFrames         ApplyFramesArgs       `json:"applyFrames,omitzero"`
}

// FocusAppArgs is focus_app's typed payload: the application, as a bundle
// identifier or a name.
type FocusAppArgs struct {
	App string `json:"app"`
}

// NewFocusAppCommand builds focus_app's command from the one positional
// argument the action takes. The rule is ParseFocusAppArg's, called rather
// than restated.
func NewFocusAppCommand(args []string) (Command, error) {
	app, err := ParseFocusAppArg(args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameFocusApp, FocusApp: FocusAppArgs{App: app}}, nil
}

// ParseFocusAppArg is the only place focus_app's argument is read: exactly one,
// naming an application by bundle identifier or name, with surrounding
// whitespace not part of the name. It runs validateFocusAppArgs on what it
// read, the check ExecuteCommand applies to a payload off the socket, so the
// two cannot disagree about what names an application.
func ParseFocusAppArg(args []string) (string, error) {
	if len(args) != 1 {
		return "", derrors.New(
			derrors.CodeInvalidInput,
			"focus_app requires exactly one argument: an application name or bundle identifier",
		)
	}

	app := strings.TrimSpace(args[0])

	err := validateFocusAppArgs(FocusAppArgs{App: app})
	if err != nil {
		return "", err
	}

	return app, nil
}

// validateFocusAppArgs holds focus_app's one payload rule: the application
// has to be named.
func validateFocusAppArgs(args FocusAppArgs) error {
	if strings.TrimSpace(args.App) == "" {
		return derrors.New(
			derrors.CodeInvalidInput,
			"focus_app argument cannot be empty: give an application name or bundle identifier",
		)
	}

	return nil
}

// MoveWindowToSpaceArgs is move_window_to_space's typed payload: the space the
// window goes to, and whether focus goes with it.
type MoveWindowToSpaceArgs struct {
	Space SpaceArg `json:"space"`
	// Follow switches to the destination space once the window is there, so
	// one hotkey does what "move_window_to_space N" then "space N" would.
	Follow bool `json:"follow"`
}

// FocusWindowArgs is focus_window's typed payload.
type FocusWindowArgs struct {
	Backward bool `json:"backward"`
	// Direction is "", "up", "down", "left", or "right".
	Direction string `json:"direction"`
	// SameApp confines the cycle, or the directional move, to the windows of
	// the application that owns the focused window.
	SameApp bool `json:"sameApp"`
}

// NewFocusWindowCommand builds focus_window's command directly from the CLI's
// already-typed flags. At most one direction flag may be set, which is a rule
// only the flags can break; the payload it builds then goes through
// validateFocusWindowArgs, the same check ExecuteCommand applies to a payload
// that arrived off the socket.
func NewFocusWindowCommand(
	backward, focusUp, focusDown, focusLeft, focusRight, sameApp bool,
) (Command, error) {
	direction, err := focusDirectionOf(focusUp, focusDown, focusLeft, focusRight)
	if err != nil {
		return Command{}, err
	}

	args := FocusWindowArgs{Backward: backward, Direction: direction, SameApp: sameApp}

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
// one positional argument the action takes and its --follow flag. It is
// NewSpaceCommand's counterpart: the same rule, reported against this action's
// name, landing on the field this action reads.
func NewMoveWindowToSpaceCommand(args []string, follow bool) (Command, error) {
	spaceArg, err := ParseSpaceArg(NameMoveWindowToSpace, args)
	if err != nil {
		return Command{}, err
	}

	return Command{
		Name:              NameMoveWindowToSpace,
		MoveWindowToSpace: MoveWindowToSpaceArgs{Space: spaceArg, Follow: follow},
	}, nil
}

// NewMoveWindowToDisplayCommand builds move_window_to_display's command from
// the one positional argument the action takes. The rule is ParseDisplayArg's,
// called rather than restated.
func NewMoveWindowToDisplayCommand(args []string) (Command, error) {
	displayArg, err := ParseDisplayArg(args)
	if err != nil {
		return Command{}, err
	}

	return Command{Name: NameMoveWindowToDisplay, MoveWindowToDisplay: displayArg}, nil
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

	// Cycle steps the preset through its cycle: left-half to left-two-thirds
	// to left-third and back, and the same on the right.
	Cycle bool `json:"cycle"`
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

	if args.Cycle {
		err = validateCycle(args, preset)
		if err != nil {
			return geometry.Request{}, err
		}
	}

	req := geometry.Request{Preset: preset, Cycle: args.Cycle}

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

	useMargins, err := marginPreferenceOf(args.UseMargin, args.NoMargin)
	if err != nil {
		return geometry.Request{}, err
	}

	req.UseMargins = useMargins

	return req, nil
}

// validateCycle holds --cycle's two rules: it needs a preset that has a cycle,
// and the cycle decides the size and the place, so no flag may set either.
// Margins are the one thing left to ask for, since every step honors them.
func validateCycle(args ResizeWindowArgs, preset geometry.Preset) error {
	if !preset.Cycles() {
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"--cycle needs a preset that cycles (%s)",
			strings.Join(geometry.CyclingPresetNames(), ", "),
		)
	}

	if args.WidthSet || args.HeightSet || args.WidthPercentSet || args.HeightPercentSet ||
		args.XSet || args.YSet || args.AnchorSet {
		return derrors.New(
			derrors.CodeInvalidInput,
			"--cycle cannot be combined with a size, position or anchor flag",
		)
	}

	return nil
}

// marginPreferenceOf turns resize_window's two margin flags into the margin
// preference they express: forced on, forced off, or — for the pair nobody
// gave — none at all, the nil that defers to the system tiled-window-margins
// setting.
//
// Asking for both at once expresses no preference either, but it is a
// contradiction rather than a silence, so it is refused (mimi#138). The
// conversion used to apply one flag and then the other, which made the
// assignment written second in the code the answer regardless of what was
// asked for.
func marginPreferenceOf(useMargin, noMargin bool) (*bool, error) {
	if useMargin && noMargin {
		return nil, derrors.New(
			derrors.CodeInvalidInput,
			"--margin cannot be combined with --no-margin",
		)
	}

	if !useMargin && !noMargin {
		return nil, nil //nolint:nilnil // intentional: signals "no preference"
	}

	useMargins := useMargin

	return &useMargins, nil
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

		return e.FocusWindow(cmd.FocusWindow)
	case NameSpace:
		index, err := e.resolveSpaceArg(NameSpace, cmd.Space)
		if err != nil {
			return err
		}

		return e.FocusSpace(index)
	case NameMoveWindowToSpace:
		index, err := e.resolveSpaceArg(NameMoveWindowToSpace, cmd.MoveWindowToSpace.Space)
		if err != nil {
			return err
		}

		return e.MoveWindowToSpace(index, cmd.MoveWindowToSpace.Follow)
	case NameMoveWindowToDisplay:
		return e.MoveWindowToDisplay(cmd.MoveWindowToDisplay)
	case NameFocusApp:
		err := validateFocusAppArgs(cmd.FocusApp)
		if err != nil {
			return err
		}

		return e.FocusApp(cmd.FocusApp.App)
	case NameResizeWindow:
		req, err := ResizeRequestFromArgs(cmd.ResizeWindow)
		if err != nil {
			return err
		}

		return e.ResizeWindow(req)
	case NameApplyFrames:
		err := validateApplyFramesArgs(cmd.ApplyFrames)
		if err != nil {
			return err
		}

		return e.ApplyFrames(cmd.ApplyFrames)
	default:
		return derrors.Newf(
			derrors.CodeInvalidInput,
			"unknown action %q (supported: focus_window, focus_app, space, move_window_to_space, move_window_to_display, resize_window, apply_frames)",
			cmd.Name,
		)
	}
}
