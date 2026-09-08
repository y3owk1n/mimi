package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/tiling"
)

// tilingInputFlag names the flag that prints the layout's input instead of
// its output.
const tilingInputFlag = "input"

func newTilingCmd(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tiling",
		Short: "Work with the layout program named in [tiling]",
		Long: `Work with the layout program named in the [tiling] section of the config.

mimi ships no layout. The daemon runs the program you name on window events,
giving it the windows and displays as JSON on stdin and applying the frames it
prints. examples/tiling in the mimi repository holds programs to copy.

Available subcommands:
  preview   run the layout once against the desktop and print what it would apply

Examples:
  mimi tiling preview
  mimi tiling preview --input`,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			cobraCmd.SilenceUsage = false

			return derrors.New(
				derrors.CodeInvalidInput,
				"tiling subcommand required (e.g., mimi tiling preview)",
			)
		},
	}

	cmd.AddCommand(buildTilingPreviewCommand(state))

	return cmd
}

func buildTilingPreviewCommand(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Run the layout once and print the frames it would apply",
		Long: `Run the layout named by tiling.layout once, against the desktop as it is now,
and print what it returned as one line of JSON without applying any of it:

  {"frames":[{"number":4242,"frame":{...}}],"state":...}

The layout is given a "preview" event and a null state, so this is what the
daemon's first pass on a space would do. It runs whether or not tiling.enabled
is set, which is how a layout is tried before it is switched on. With --input
the JSON handed to the layout is printed instead, and the layout is not run.
Accessibility permission is required.`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(state.configPath)
			if err != nil {
				return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
			}

			inputOnly, _ := cobraCmd.Flags().GetBool(tilingInputFlag)

			return answerQuery(cobraCmd, func() (any, error) {
				return previewLayout(cobraCmd, cfg, inputOnly)
			})
		},
	}

	cmd.Flags().Bool(tilingInputFlag, false, "Print the layout's input instead of running it")

	return cmd
}

// previewLayout runs the engine's preview on the live desktop and returns
// whichever side of it was asked for. With inputOnly the layout is not run at
// all, so a config with no layout still shows the input one would get.
func previewLayout(cobraCmd *cobra.Command, cfg *config.Config, inputOnly bool) (any, error) {
	engine := tiling.New(tiling.LiveDesktop{}, nil, nil)

	if inputOnly {
		layout := cfg.Tiling
		layout.Layout = "cat >/dev/null"
		engine.Update(layout, cfg.Settings.HookShell)
	} else {
		if cfg.Tiling.Layout == "" {
			return nil, derrors.New(derrors.CodeInvalidConfig, "tiling.layout is not set")
		}

		engine.Update(cfg.Tiling, cfg.Settings.HookShell)
	}

	input, out, err := engine.Preview(cobraCmd.Context(), tiling.Event{Kind: tiling.EventPreview})
	if err != nil {
		return nil, err
	}

	if inputOnly {
		return input, nil
	}

	frames := out.Frames
	if frames == nil {
		frames = []action.WindowFrame{}
	}

	state := out.State
	if state == nil {
		state = json.RawMessage("null")
	}

	return previewOutput{Frames: frames, State: state}, nil
}

// previewOutput is a layout's Output printed the way the layout printed it,
// with an absent state shown as null rather than left out.
type previewOutput struct {
	Frames []action.WindowFrame `json:"frames"`
	State  json.RawMessage      `json:"state"`
}
