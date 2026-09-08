package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/ipc"
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
  relayout  run the layout once and apply it
  cmd       send a named command to the layout, for it to give meaning to

Examples:
  mimi tiling preview
  mimi tiling preview --input
  mimi tiling relayout
  mimi tiling cmd swap
  mimi tiling cmd ratio +0.05`,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			cobraCmd.SilenceUsage = false

			return derrors.New(
				derrors.CodeInvalidInput,
				"tiling subcommand required (e.g., mimi tiling preview)",
			)
		},
	}

	cmd.AddCommand(buildTilingPreviewCommand(state))
	cmd.AddCommand(buildTilingRelayoutCommand(state))
	cmd.AddCommand(buildTilingCmdCommand(state))

	return cmd
}

func buildTilingRelayoutCommand(state *cliState) *cobra.Command {
	return &cobra.Command{
		Use:   "relayout",
		Short: "Run the layout once and apply what it returns",
		Long: `Run the layout once, with a "relayout" event, and apply the frames it returns.

With the daemon running, the daemon's engine runs it, with the state it holds
for the active space, and tiling.enabled has to be set. Without a daemon the
layout runs in this process with a null state, whether or not tiling is
enabled. Accessibility permission is required.`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			tilingCmd, err := action.NewTilingCommand(action.TilingRelayout, "", nil)
			if err != nil {
				return err
			}

			return state.runTiling(cobraCmd, tilingCmd)
		},
	}
}

func buildTilingCmdCommand(state *cliState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cmd <name> [args...]",
		Short: "Send a named command to the layout",
		Long: `Send a named command to the layout, with any arguments after it. mimi gives
the name no meaning: the layout reads it from the event ("kind": "command",
"name": ..., "args": [...]) and decides what it does, which is how a layout
defines its own hotkeys, such as swapping the focused window with the master
or nudging a split ratio.

With the daemon running, the daemon's engine runs it, with the state it holds
for the active space, and tiling.enabled has to be set. Without a daemon the
layout runs in this process with a null state. Accessibility permission is
required.

Examples:
  mimi tiling cmd swap
  mimi tiling cmd ratio +0.05
  mimi tiling cmd focus left`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			tilingCmd, err := action.NewTilingCommand(action.TilingCommand, args[0], args[1:])
			if err != nil {
				return err
			}

			return state.runTiling(cobraCmd, tilingCmd)
		},
	}

	// Everything after the name belongs to the layout, including something
	// that looks like a flag: "ratio -0.05" is the commonest command there
	// is, and it would otherwise be read as flags of this command's own.
	cmd.Flags().SetInterspersed(false)

	return cmd
}

// runTiling sends a tiling command to the daemon, whose engine holds the
// layout's state, and falls back to an engine of this process's own when no
// daemon answers, on the same terms runAction falls back: silently when there
// is no daemon, with a warning when the daemon speaks another protocol.
func (s *cliState) runTiling(cobraCmd *cobra.Command, cmd action.Command) error {
	socketPath := ipc.ResolveSocketPath(s.configPath)

	err := ipc.TryExecute(socketPath, cmd)
	if err == nil {
		return nil
	}

	if derrors.IsCode(err, derrors.CodeProtocolMismatch) {
		_, _ = fmt.Fprintf(
			cobraCmd.ErrOrStderr(),
			"mimi: the daemon speaks a different request protocol than this CLI (%s) — restart the daemon so it runs this build. This command ran in the CLI's own engine, with no state.\n",
			derrors.Message(err),
		)
	} else if !derrors.IsCode(err, derrors.CodeDaemonUnavailable) {
		return err
	}

	cfg, err := config.Load(s.configPath)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeInvalidConfig, "loading config")
	}

	if cfg.Tiling.Layout == "" {
		return derrors.New(derrors.CodeInvalidConfig, "tiling.layout is not set")
	}

	// Without a daemon there is no engine to be disabled: the user asked for
	// this run by name, so it runs.
	local := cfg.Tiling
	local.Enabled = true

	engine := tiling.New(tiling.LiveDesktop{}, nil, nil)
	engine.Update(local, cfg.Settings.HookShell)

	return engine.Command(cobraCmd.Context(), tiling.EventFromArgs(cmd.Tiling))
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
