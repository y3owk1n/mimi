package tiling

import "github.com/y3owk1n/mimi/internal/action"

// LiveDesktop is the Desktop macOS itself, through action's queries and
// apply_frames, so the engine reads and writes exactly what the CLI does.
type LiveDesktop struct{}

// Windows lists the focusable windows on the active space.
func (LiveDesktop) Windows() (action.WindowsInfo, error) { return action.QueryWindows() }

// Displays lists the connected displays.
func (LiveDesktop) Displays() ([]action.DisplayEntry, error) { return action.QueryDisplays() }

// ActiveSpaces is the space in front on every display.
func (LiveDesktop) ActiveSpaces() (map[uint32]int, error) { return action.QueryActiveSpaces() }

// FullScreenDisplays is the set of displays showing a full-screen space.
func (LiveDesktop) FullScreenDisplays() (map[uint32]bool, error) {
	return action.QueryFullScreenDisplays()
}

// Margins is the system tiled-window margins setting.
func (LiveDesktop) Margins() (action.MarginsInfo, error) { return action.QueryMargins() }

// Focus gives keyboard focus to a window by number.
func (LiveDesktop) Focus(number uint32) error { return action.FocusWindowNumber(number) }

// Apply writes the frames the way apply_frames does, animated when
// animation is set.
func (LiveDesktop) Apply(frames []action.WindowFrame, animation *action.Animation) error {
	cmd, err := action.NewApplyFramesCommand(frames)
	if err != nil {
		return err
	}

	cmd.ApplyFrames.Animation = animation

	return action.ExecuteCommand(cmd)
}
