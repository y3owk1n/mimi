package ipc

import (
	"strconv"

	"github.com/y3owk1n/mimi/internal/action"
)

// Keywords the wire's single positional space argument accepts instead of a
// number — the same two ParseSpaceArg and marshalSpaceArg agree on.
const (
	spaceKeywordNext = "next"
	spaceKeywordPrev = "prev"
)

// marshalCommand flattens a typed action.Command into the (action name,
// string args) pair the wire protocol carries. This is where the CLI's
// typed values are turned into strings — the direct-execution path never
// pays for it, since it runs cmd through action.ExecuteCommand instead.
func marshalCommand(cmd action.Command) (string, []string) {
	switch cmd.Name {
	case action.NameFocusWindow:
		return string(cmd.Name), marshalFocusWindow(cmd.FocusWindow)
	case action.NameSpace:
		return string(cmd.Name), marshalSpaceArg(cmd.Space)
	case action.NameMoveWindowToSpace:
		return string(cmd.Name), marshalSpaceArg(cmd.MoveWindowToSpace)
	case action.NameResizeWindow:
		return string(cmd.Name), marshalResizeWindow(cmd.ResizeWindow)
	default:
		return string(cmd.Name), nil
	}
}

func marshalFocusWindow(focusArgs action.FocusWindowArgs) []string {
	args := []string{}

	if focusArgs.Backward {
		args = append(args, "--backward")
	}

	// Direction is always "", "up", "down", "left", or "right" — the flag
	// spelling is the direction with "--" in front of it.
	if focusArgs.Direction != "" {
		args = append(args, "--"+focusArgs.Direction)
	}

	return args
}

func marshalSpaceArg(spaceArg action.SpaceArg) []string {
	switch spaceArg.Direction {
	case 1:
		return []string{spaceKeywordNext}
	case -1:
		return []string{spaceKeywordPrev}
	default:
		return []string{strconv.Itoa(spaceArg.Index)}
	}
}

func marshalResizeWindow(resizeArgs action.ResizeWindowArgs) []string {
	args := []string{}

	if resizeArgs.Preset != "" {
		args = append(args, resizeArgs.Preset)
	}

	if resizeArgs.WidthSet {
		args = append(args, "--width", strconv.Itoa(resizeArgs.Width))
	}

	if resizeArgs.HeightSet {
		args = append(args, "--height", strconv.Itoa(resizeArgs.Height))
	}

	if resizeArgs.WidthPercentSet {
		args = append(
			args,
			"--width-percent",
			strconv.FormatFloat(resizeArgs.WidthPercent, 'f', -1, 64),
		)
	}

	if resizeArgs.HeightPercentSet {
		args = append(
			args,
			"--height-percent",
			strconv.FormatFloat(resizeArgs.HeightPercent, 'f', -1, 64),
		)
	}

	if resizeArgs.XSet {
		args = append(args, "--x", strconv.Itoa(resizeArgs.X))
	}

	if resizeArgs.YSet {
		args = append(args, "--y", strconv.Itoa(resizeArgs.Y))
	}

	if resizeArgs.AnchorSet {
		args = append(args, "--anchor", resizeArgs.Anchor)
	}

	if resizeArgs.UseMargin {
		args = append(args, "--margin")
	}

	if resizeArgs.NoMargin {
		args = append(args, "--no-margin")
	}

	return args
}
