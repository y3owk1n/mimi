//nolint:testpackage
package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
)

// actionCommandName is the parent every action subcommand hangs off.
const actionCommandName = "action"

// resizeFlags parses args against a fresh resize_window flag set and returns
// the typed payload resizeWindowArgsFromFlags builds from it, without
// running the command (which would reach for the real desktop).
func resizeFlags(t *testing.T, preset string, args ...string) action.ResizeWindowArgs {
	t.Helper()

	cmd := buildResizeWindowCommand(&cliState{})

	err := cmd.Flags().Parse(args)
	if err != nil {
		t.Fatalf("parsing flags %v: %v", args, err)
	}

	return resizeWindowArgsFromFlags(cmd, preset)
}

// TestResizeWindowArgsFromFlags_UsesChangedForEveryDimensionFlag pins the
// consistency fix mimi#95 asks for: --width, --height, --width-percent and
// --height-percent now all use Flags().Changed(...), the same idiom --x and
// --y already followed, so an explicitly-zero flag is forwarded rather than
// silently dropped for being "not > 0".
func TestResizeWindowArgsFromFlags_UsesChangedForEveryDimensionFlag(t *testing.T) {
	t.Parallel()

	t.Run("width 0 is forwarded when given", func(t *testing.T) {
		t.Parallel()

		got := resizeFlags(t, "", "--width", "0")

		if !got.WidthSet {
			t.Fatal("WidthSet = false, want true for an explicit --width 0")
		}

		if got.Width != 0 {
			t.Fatalf("Width = %d, want 0", got.Width)
		}
	})

	t.Run("height 0 is forwarded when given", func(t *testing.T) {
		t.Parallel()

		got := resizeFlags(t, "", "--height", "0")

		if !got.HeightSet {
			t.Fatal("HeightSet = false, want true for an explicit --height 0")
		}
	})

	t.Run("width-percent 0 is forwarded when given", func(t *testing.T) {
		t.Parallel()

		got := resizeFlags(t, "", "--width-percent", "0")

		if !got.WidthPercentSet {
			t.Fatal("WidthPercentSet = false, want true for an explicit --width-percent 0")
		}
	})

	t.Run("height-percent 0 is forwarded when given", func(t *testing.T) {
		t.Parallel()

		got := resizeFlags(t, "", "--height-percent", "0")

		if !got.HeightPercentSet {
			t.Fatal("HeightPercentSet = false, want true for an explicit --height-percent 0")
		}
	})

	t.Run("none of the four are set when never given", func(t *testing.T) {
		t.Parallel()

		got := resizeFlags(t, "")

		if got.WidthSet || got.HeightSet || got.WidthPercentSet || got.HeightPercentSet {
			t.Fatalf("expected no Set flags, got %+v", got)
		}
	})

	t.Run("a positive width is still forwarded, as before", func(t *testing.T) {
		t.Parallel()

		got := resizeFlags(t, "", "--width", "800")

		if !got.WidthSet || got.Width != 800 {
			t.Fatalf("got %+v, want WidthSet=true Width=800", got)
		}
	})
}

func TestResizeWindowArgsFromFlags_CarriesThePresetAndTheOtherFlags(t *testing.T) {
	t.Parallel()

	got := resizeFlags(t, "left-half", "--x", "10", "--y", "20", "--anchor", "cc", "--margin")

	want := action.ResizeWindowArgs{
		Preset:    "left-half",
		X:         10,
		XSet:      true,
		Y:         20,
		YSet:      true,
		Anchor:    "cc",
		AnchorSet: true,
		UseMargin: true,
	}

	if got != want {
		t.Fatalf("resizeWindowArgsFromFlags() = %+v, want %+v", got, want)
	}
}

// TestFocusWindowCommand_RejectsBackwardWithDirection checks the CLI surfaces
// action.NewFocusWindowArgs' validation before ever reaching the desktop —
// this fails during flag validation, not execution, so it is safe to run
// through the real command tree.
func TestFocusWindowCommand_RejectsBackwardWithDirection(t *testing.T) {
	t.Parallel()

	_, err := runCommand(t, actionCommandName, "focus_window", "--backward", "--up")
	if err == nil {
		t.Fatal("expected an error combining --backward with a direction flag")
	}
}

// TestSpaceCommand_RejectsZero checks the space subcommand's positional-arg
// validation still runs before state.runAction, the same guarantee it made
// before the CLI started building a typed action.Command.
func TestSpaceCommand_RejectsZero(t *testing.T) {
	t.Parallel()

	_, err := runCommand(t, actionCommandName, string(action.NameSpace), "0")
	if err == nil {
		t.Fatal("expected an error for space 0")
	}
}

// spaceArgValidators pairs each action that takes a space argument with the
// Args validator its subcommand carries, so a case can be run against both
// without executing either — the validators reject before RunE, which is what
// keeps these tests off the real desktop.
func spaceArgValidators() map[action.Name]cobra.PositionalArgs {
	return map[action.Name]cobra.PositionalArgs{
		action.NameSpace:             buildSpaceCommand(&cliState{}).Args,
		action.NameMoveWindowToSpace: buildMoveWindowToSpaceCommand(&cliState{}).Args,
	}
}

// malformedSpaceArgs lists every way a space argument can be malformed: it is
// neither a positive integer nor one of the accepted keywords.
func malformedSpaceArgs() []struct {
	name string
	args []string
} {
	return []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{""}},
		{name: "whitespace only", args: []string{"   "}},
		{name: "non-numeric", args: []string{"foo"}},
		{name: "zero", args: []string{"0"}},
		{name: "negative", args: []string{"-1"}},
		{name: "no argument", args: nil},
		{name: "two arguments", args: []string{"1", "2"}},
	}
}

// TestSpaceArgValidation_SameWordingForBothActions pins mimi#124: one rule
// decides what a space argument is, so a malformed one reads the same for
// space and for move_window_to_space, differing only where the action's own
// name appears.
func TestSpaceArgValidation_SameWordingForBothActions(t *testing.T) {
	t.Parallel()

	validators := spaceArgValidators()

	for _, testCase := range malformedSpaceArgs() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spaceErr := validators[action.NameSpace](nil, testCase.args)
			if spaceErr == nil {
				t.Fatalf("%s %v: expected an error", action.NameSpace, testCase.args)
			}

			moveErr := validators[action.NameMoveWindowToSpace](nil, testCase.args)
			if moveErr == nil {
				t.Fatalf("%s %v: expected an error", action.NameMoveWindowToSpace, testCase.args)
			}

			normalized := strings.ReplaceAll(
				moveErr.Error(),
				string(action.NameMoveWindowToSpace),
				string(action.NameSpace),
			)
			if normalized != spaceErr.Error() {
				t.Fatalf(
					"wording differs beyond the action name:\n space: %s\n  move: %s",
					spaceErr,
					moveErr,
				)
			}
		})
	}
}

// TestSpaceArgValidation_DelegatesToTheOneRule checks the CLI rejects a
// malformed space argument with the rule itself — action.ParseSpaceArg —
// rather than a second copy of it. The CLI's copy used to describe the rule as
// "a positive integer" alone, so a caller who mistyped "nxt" was told the
// keywords it had just rejected were not accepted at all.
func TestSpaceArgValidation_DelegatesToTheOneRule(t *testing.T) {
	t.Parallel()

	validators := spaceArgValidators()

	for _, testCase := range malformedSpaceArgs() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for actionName, validate := range validators {
				gotErr := validate(nil, testCase.args)
				if gotErr == nil {
					t.Fatalf("%s %v: expected an error", actionName, testCase.args)
				}

				_, wantErr := action.ParseSpaceArg(actionName, testCase.args)
				if wantErr == nil {
					t.Fatalf("ParseSpaceArg(%s, %v): expected an error", actionName, testCase.args)
				}

				if gotErr.Error() != wantErr.Error() {
					t.Errorf(
						"%s %v rejected with its own wording:\n got: %s\nwant: %s",
						actionName,
						testCase.args,
						gotErr,
						wantErr,
					)
				}
			}
		})
	}
}

// spaceArgv builds the command line that reaches a space action's Args layer
// with args as its positional arguments. A "--" goes in front of them so a
// negative number is read as the space argument it is, rather than as the
// flags cobra would otherwise take "-1" for.
func spaceArgv(name action.Name, args []string) []string {
	argv := make([]string, 0, len(args)+3)
	argv = append(argv, actionCommandName, string(name))

	if len(args) > 0 {
		argv = append(argv, "--")
		argv = append(argv, args...)
	}

	return argv
}

// TestSpaceArgValidation_RejectsBeforeRunE checks the Args layer still stops a
// malformed space argument before the action runs: the tree fails with the one
// rule's own wording and prints its usage, and RunE is never reached. That is
// what produces the usage output and the exit code, and delegating the rule
// must not change it.
func TestSpaceArgValidation_RejectsBeforeRunE(t *testing.T) {
	t.Parallel()

	for _, name := range []action.Name{action.NameSpace, action.NameMoveWindowToSpace} {
		t.Run(string(name), func(t *testing.T) {
			t.Parallel()

			for _, testCase := range malformedSpaceArgs() {
				t.Run(testCase.name, func(t *testing.T) {
					t.Parallel()

					var out bytes.Buffer

					ran := false
					root := newRootCmd()
					path := []string{actionCommandName, string(name)}

					target, _, err := root.Find(path)
					if err != nil {
						t.Fatalf("finding command: %v", err)
					}

					target.RunE = func(_ *cobra.Command, _ []string) error {
						ran = true

						return nil
					}

					root.SetOut(&out)
					root.SetErr(&out)
					root.SetArgs(spaceArgv(name, testCase.args))

					err = root.Execute()
					if err == nil {
						t.Fatalf("%s %v: expected an error", name, testCase.args)
					}

					_, wantErr := action.ParseSpaceArg(name, testCase.args)
					if err.Error() != wantErr.Error() {
						t.Errorf(
							"%s %v failed with something other than the rule:\n got: %s\nwant: %s",
							name,
							testCase.args,
							err,
							wantErr,
						)
					}

					if ran {
						t.Errorf("%s %v reached RunE", name, testCase.args)
					}

					if !strings.Contains(out.String(), "Usage:") {
						t.Errorf(
							"%s %v printed no usage, got: %s",
							name,
							testCase.args,
							out.String(),
						)
					}
				})
			}
		})
	}
}

// TestSpaceArgValidation_AcceptsEveryFormOfASpaceArgument covers the other
// half of the rule: a 1-based number and each accepted keyword pass the Args
// layer for both actions, so nothing here reaches the desktop.
func TestSpaceArgValidation_AcceptsEveryFormOfASpaceArgument(t *testing.T) {
	t.Parallel()

	accepted := []string{"1", "42", "next", "prev", "previous", " 2 "}
	validators := spaceArgValidators()

	for _, arg := range accepted {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			for actionName, validate := range validators {
				err := validate(nil, []string{arg})
				if err != nil {
					t.Errorf("%s %q rejected: %v", actionName, arg, err)
				}
			}
		})
	}
}
