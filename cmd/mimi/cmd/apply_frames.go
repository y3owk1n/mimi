package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// applyFramesFileFlag names the flag that reads the payload from a file
// instead of stdin.
const applyFramesFileFlag = "file"

func buildApplyFramesCommand(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply_frames",
		Short: "Move and resize several windows on the active space in one action",
		Long: `Move and resize several windows at once. The frames come in as a JSON array
on stdin, or from a file with --file, each naming a window by the number
"mimi query windows" reported and the frame to give it:

  [{"number":4242,"frame":{"x":0,"y":25,"width":720,"height":875}},
   {"number":4243,"frame":{"x":720,"y":25,"width":720,"height":875}}]

Frames are in window coordinates, the same as the queries report. Every frame
is attempted in order, whatever happened to the ones before it, and the
action fails naming each window it could not place. A window that is not on
the active space cannot be placed.

This is the primitive a layout script is built on: list the windows, decide
where each goes, apply. See examples/tiling in the mimi repository.

Examples:
  mimi action apply_frames < frames.json
  mimi action apply_frames --file frames.json
  my-layout | mimi action apply_frames`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			frames, err := readFramesPayload(cobraCmd)
			if err != nil {
				return err
			}

			applyCmd, err := action.NewApplyFramesCommand(frames)
			if err != nil {
				return err
			}

			return state.runAction(cobraCmd, applyCmd)
		},
	}

	cmd.Flags().String(applyFramesFileFlag, "", "Read the frames from this file instead of stdin")

	return cmd
}

// readFramesPayload decodes the frames from --file when it is set and from
// the command's stdin otherwise. Anything after the array is a second
// document, and rejected: a script that emits two layouts has a bug the
// action should not hide by applying the first.
func readFramesPayload(cobraCmd *cobra.Command) ([]action.WindowFrame, error) {
	reader := cobraCmd.InOrStdin()

	path, _ := cobraCmd.Flags().GetString(applyFramesFileFlag)
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, derrors.Wrapf(err, derrors.CodeInvalidInput, "opening --file %q", path)
		}
		defer file.Close() //nolint:errcheck // read only

		reader = file
	}

	decoder := json.NewDecoder(reader)

	var frames []action.WindowFrame

	err := decoder.Decode(&frames)
	if err != nil {
		return nil, derrors.Wrapf(
			err,
			derrors.CodeInvalidInput,
			"decoding frames: expected a JSON array of {number, frame}",
		)
	}

	if decoder.More() {
		return nil, derrors.New(
			derrors.CodeInvalidInput,
			"decoding frames: more than one JSON document",
		)
	}

	return frames, nil
}
