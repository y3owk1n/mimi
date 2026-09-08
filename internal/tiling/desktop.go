package tiling

import "github.com/y3owk1n/mimi/internal/action"

// LiveDesktop is the Desktop macOS itself, through action's queries and
// apply_frames, so the engine reads and writes exactly what the CLI does.
type LiveDesktop struct{}

// Windows lists the focusable windows on the active space.
func (LiveDesktop) Windows() (action.WindowsInfo, error) { return action.QueryWindows() }

// Displays lists the connected displays.
func (LiveDesktop) Displays() ([]action.DisplayEntry, error) { return action.QueryDisplays() }

// ActiveSpace is the 1-based index of the space in front.
func (LiveDesktop) ActiveSpace() (int, error) {
	info, err := action.QuerySpace()
	if err != nil {
		return 0, err
	}

	return info.Index, nil
}

// Apply writes the frames the way apply_frames does.
func (LiveDesktop) Apply(frames []action.WindowFrame) error {
	cmd, err := action.NewApplyFramesCommand(frames)
	if err != nil {
		return err
	}

	return action.ExecuteCommand(cmd)
}
