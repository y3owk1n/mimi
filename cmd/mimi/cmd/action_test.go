//nolint:testpackage
package cmd

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/y3owk1n/mimi/internal/action"
)

const (
	// actionCommandName is the parent every action subcommand hangs off.
	actionCommandName = "action"

	// The two action subcommands whose flags these tests malform, spelled the
	// way the CLI takes them.
	focusWindowCommandName  = string(action.NameFocusWindow)
	resizeWindowCommandName = string(action.NameResizeWindow)

	// unknownPreset is a name that is not one of the ten presets, and never
	// becomes one.
	unknownPreset = "left-third"

	// whitespaceOnlyArg is a positional argument made of nothing but
	// whitespace, which names neither a preset nor a space.
	whitespaceOnlyArg = "   "
)

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

// TestResizeWindowCommand_RejectsAnUnknownPreset covers mimi#125 at the CLI:
// an unknown preset is rejected before anything reaches the desktop, and with
// action.ParseResizePreset's own wording rather than a second copy of the valid
// name list that could drift from it. This fails during argument validation,
// not execution, so it is safe to run through the real command tree.
func TestResizeWindowCommand_RejectsAnUnknownPreset(t *testing.T) {
	t.Parallel()

	_, err := runCommand(t, actionCommandName, resizeWindowCommandName, unknownPreset)
	if err == nil {
		t.Fatalf("%s %s: expected an error", resizeWindowCommandName, unknownPreset)
	}

	_, wantErr := action.ParseResizePreset(unknownPreset)
	if err.Error() != wantErr.Error() {
		t.Errorf("rejected with its own wording:\n got: %s\nwant: %s", err, wantErr)
	}
}

// TestResizeWindowCommand_ReportsThePositionalArgumentAndTheFlagsInOneOrder is
// the evidence mimi#133 asks for: what a command line with more than one thing
// wrong is rejected for, now that resize_window's preset is validated in the
// Args layer rather than in the command body.
//
// The order does not depend on that layer, which is why moving the check was
// free — every case below reports the same thing as it did when the preset was
// checked in the body. Cobra parses flags before it validates arguments, so a
// flag it cannot parse at all is reported ahead of the preset either way — the
// same order "space 0 --nope" has always had, and the case below runs both
// styles to show it. A flag whose value parses but breaks a rule is reported
// after the preset: the Args layer now rejects the preset before RunE reads a
// flag at all, and action.ResizeRequestFromArgs checks the preset first for the
// daemon path, so both agree. An extra positional argument is reported as
// arity, ahead of both.
func TestResizeWindowCommand_ReportsThePositionalArgumentAndTheFlagsInOneOrder(t *testing.T) {
	t.Parallel()

	_, presetErr := action.ParseResizePreset(unknownPreset)
	if presetErr == nil {
		t.Fatalf("ParseResizePreset(%q): expected an error", unknownPreset)
	}

	testCases := []struct {
		name    string
		argv    []string
		wantErr string
	}{
		{
			name:    "an unparsable flag value beats the preset",
			argv:    []string{resizeWindowCommandName, unknownPreset, "--height", "abc"},
			wantErr: `invalid argument "abc" for "--height" flag: strconv.ParseInt: parsing "abc": invalid syntax`,
		},
		{
			name:    "an unknown flag beats the preset",
			argv:    []string{resizeWindowCommandName, unknownPreset, "--nope"},
			wantErr: "unknown flag: --nope",
		},
		{
			name:    "an unknown flag beats a space argument too",
			argv:    []string{string(action.NameSpace), "0", "--nope"},
			wantErr: "unknown flag: --nope",
		},
		{
			name:    "the preset beats an unknown anchor",
			argv:    []string{resizeWindowCommandName, unknownPreset, "--anchor", "xx"},
			wantErr: presetErr.Error(),
		},
		{
			name:    "the preset beats a width-percent above 100",
			argv:    []string{resizeWindowCommandName, unknownPreset, "--width-percent", "150"},
			wantErr: presetErr.Error(),
		},
		{
			name:    "arity beats the preset",
			argv:    []string{resizeWindowCommandName, unknownPreset, "extra"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out, err := runCommand(t, append([]string{actionCommandName}, testCase.argv...)...)
			if err == nil {
				t.Fatalf("%v: expected an error", testCase.argv)
			}

			if err.Error() != testCase.wantErr {
				t.Errorf(
					"%v rejected for the wrong thing:\n got: %s\nwant: %s",
					testCase.argv,
					err,
					testCase.wantErr,
				)
			}

			if !strings.Contains(out, "Usage:") {
				t.Errorf("%v printed no usage, got: %s", testCase.argv, out)
			}
		})
	}
}

// stubbedRun runs argv against a fresh command tree whose leaf at path has had
// its body replaced with a stub, and reports what the tree printed, whether the
// stub ran, and how the run ended. Stubbing the body is what lets a test drive
// the real tree without the action reaching the desktop.
func stubbedRun(t *testing.T, path, argv []string) (string, bool, error) {
	t.Helper()

	var out bytes.Buffer

	ran := false
	root := newRootCmd()

	target, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("finding %v: %v", path, err)
	}

	target.RunE = func(_ *cobra.Command, _ []string) error {
		ran = true

		return nil
	}

	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(argv)

	err = root.Execute()

	return out.String(), ran, err
}

// assertRejectedBeforeRunE checks the tree rejected argv in the Args layer:
// with wantErr's own wording, printing usage, and with the command body never
// running. That layer is what produces the usage output and the exit code, so
// both actions that validate a positional argument there are pinned through
// here.
func assertRejectedBeforeRunE(t *testing.T, path, argv []string, wantErr error) {
	t.Helper()

	out, ran, err := stubbedRun(t, path, argv)
	if err == nil {
		t.Fatalf("%v: expected an error", argv)
	}

	if err.Error() != wantErr.Error() {
		t.Errorf(
			"%v failed with something other than the rule:\n got: %s\nwant: %s",
			argv,
			err,
			wantErr,
		)
	}

	if ran {
		t.Errorf("%v reached RunE", argv)
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("%v printed no usage, got: %s", argv, out)
	}
}

// TestResizeWindowArgValidation_RejectsBeforeRunE is resize_window's half of
// mimi#133: its positional argument is now checked in the same layer the space
// actions check theirs in, so an unknown preset never reaches the command body.
// The tree fails with action.ParseResizePreset's own wording and prints its
// usage, exactly as it did when the check sat in the body.
func TestResizeWindowArgValidation_RejectsBeforeRunE(t *testing.T) {
	t.Parallel()

	for _, preset := range []string{unknownPreset, " " + unknownPreset + " ", whitespaceOnlyArg} {
		t.Run(preset, func(t *testing.T) {
			t.Parallel()

			_, wantErr := action.ParseResizePreset(preset)
			if wantErr == nil {
				t.Fatalf("ParseResizePreset(%q): expected an error", preset)
			}

			assertRejectedBeforeRunE(
				t,
				[]string{actionCommandName, resizeWindowCommandName},
				[]string{actionCommandName, resizeWindowCommandName, preset},
				wantErr,
			)
		})
	}
}

// TestResizeWindowArgValidation_AcceptsWhatItAlwaysAccepted is the other half
// of mimi#133's promise: checking the preset in the Args layer rejects nothing
// the command body used to let through.
//
// The empty argument is the case that needs saying. resize_window's positional
// argument is optional, so "" is the argument nobody gave rather than a preset
// named "" — a distinction only action.ParseResizePresetArg's empty-string
// branch keeps, and one the Args layer would otherwise turn into a rejection
// for an input that has always been accepted. The command body is stubbed out,
// so nothing here reaches the desktop.
func TestResizeWindowArgValidation_AcceptsWhatItAlwaysAccepted(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "no argument", args: nil},
		{name: "the empty argument", args: []string{""}},
		{name: "a preset", args: []string{"left-half"}},
		{name: "a padded preset", args: []string{" left-half "}},
		{name: "a preset beside its flags", args: []string{"center", "--x", "10"}},
		{name: "flags with no preset", args: []string{"--margin"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := []string{actionCommandName, resizeWindowCommandName}

			_, ran, err := stubbedRun(t, path, append(path, testCase.args...))
			if err != nil {
				t.Fatalf("%v rejected: %v", testCase.args, err)
			}

			if !ran {
				t.Errorf("%v never reached RunE", testCase.args)
			}
		})
	}
}

// TestResizeWindowArgValidation_HelpStillWinsOverABadPreset pins the one thing
// asking for help must keep doing: cobra answers --help before it validates
// arguments, so a command line that also carries an unknown preset prints the
// help text and succeeds rather than being rejected for the preset.
func TestResizeWindowArgValidation_HelpStillWinsOverABadPreset(t *testing.T) {
	t.Parallel()

	path := []string{actionCommandName, resizeWindowCommandName}

	out, ran, err := stubbedRun(t, path, append(path, unknownPreset, "--help"))
	if err != nil {
		t.Fatalf("--help beside %q failed: %v", unknownPreset, err)
	}

	if ran {
		t.Error("--help reached RunE")
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("--help printed no usage, got: %s", out)
	}
}

// TestFocusWindowCommand_RejectsBackwardWithDirection checks the CLI surfaces
// action.NewFocusWindowCommand's validation before ever reaching the desktop —
// this fails while the command is being built, not while it runs, so it is
// safe to run through the real command tree.
func TestFocusWindowCommand_RejectsBackwardWithDirection(t *testing.T) {
	t.Parallel()

	_, err := runCommand(t, actionCommandName, focusWindowCommandName, "--backward", "--up")
	if err == nil {
		t.Fatal("expected an error combining --backward with a direction flag")
	}
}

// malformedAction is one command line that names an action and gives it
// arguments the action must reject.
type malformedAction struct {
	name string
	argv []string
}

// malformedActionArgv lists one command line per way an action's arguments can
// be malformed — every case mimi#126 names, plus the two space arguments,
// which the subcommand's own argument check rejects on the same rule the
// constructor calls. None of them can reach the desktop.
func malformedActionArgv() []malformedAction {
	return []malformedAction{
		{name: "negative width", argv: []string{resizeWindowCommandName, "--width", "-5"}},
		{name: "negative height", argv: []string{resizeWindowCommandName, "--height", "-5"}},
		{
			name: "width-percent above 100",
			argv: []string{resizeWindowCommandName, "--width-percent", "150"},
		},
		{
			name: "negative height-percent",
			argv: []string{resizeWindowCommandName, "--height-percent", "-1"},
		},
		{name: "unknown anchor", argv: []string{resizeWindowCommandName, "--anchor", "xx"}},
		{name: "unknown preset", argv: []string{resizeWindowCommandName, unknownPreset}},
		{
			// mimi#132: a padded name is trimmed by action.ParseResizePreset, so
			// this is rejected for the name it carries and not for its padding.
			name: "padded unknown preset",
			argv: []string{resizeWindowCommandName, " " + unknownPreset + " "},
		},
		{
			// mimi#132: whitespace is not part of a preset name, so an argument
			// made of nothing else names no preset — and says so here rather
			// than quietly resizing nothing.
			name: "whitespace-only preset",
			argv: []string{resizeWindowCommandName, whitespaceOnlyArg},
		},
		{
			name: "a direction combined with --backward",
			argv: []string{focusWindowCommandName, "--backward", "--up"},
		},
		{name: "space zero", argv: []string{string(action.NameSpace), "0"}},
		{
			name: "move_window_to_space nonsense",
			argv: []string{string(action.NameMoveWindowToSpace), "nxt"},
		},
	}
}

// listeningDaemon starts a Unix listener that answers every request the way a
// daemon would, and returns the number of connections it has accepted. A
// command that validates its arguments before routing them never makes that
// count move.
func listeningDaemon(t *testing.T, socketPath string) *atomic.Int64 {
	t.Helper()

	lc := net.ListenConfig{}

	listener, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		t.Fatalf("listening on fake daemon socket: %v", err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	accepted := &atomic.Int64{}

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			accepted.Add(1)

			_, _ = bufio.NewReader(conn).ReadBytes('\n')
			_, _ = conn.Write([]byte("{\"ok\":true}\n"))
			_ = conn.Close()
		}
	}()

	return accepted
}

// actionArgv builds the command line that runs argv as an action subcommand
// against the config at configPath.
func actionArgv(configPath string, argv []string) []string {
	return append([]string{"--config", configPath, actionCommandName}, argv...)
}

// TestActionCommands_RejectMalformedArgumentsWithoutOpeningASocket covers
// mimi#126: every action builds its command through a constructor that
// validates, so a malformed argument fails in the CLI's own process — with a
// daemon listening on the configured socket and never contacted.
//
// Each case then routes a command over the same socket, so an accepted count
// that stayed at zero is read as "nothing was sent" rather than as "nothing
// could have been": the empty command name below is one no action carries, so
// it never reaches the desktop whichever path takes it.
func TestActionCommands_RejectMalformedArgumentsWithoutOpeningASocket(t *testing.T) {
	t.Parallel()

	for _, testCase := range malformedActionArgv() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(shortSocketDir(t), "mimi.sock")
			accepted := listeningDaemon(t, socketPath)
			configPath := configWithSocket(t, socketPath)

			_, err := runCommand(t, actionArgv(configPath, testCase.argv)...)
			if err == nil {
				t.Fatalf("%v: expected an error", testCase.argv)
			}

			if accepted.Load() != 0 {
				t.Fatalf(
					"%v reached the daemon: %d connection(s) accepted",
					testCase.argv,
					accepted.Load(),
				)
			}

			err = (&cliState{configPath: configPath}).
				runAction(discardErrCommand(), action.Command{})
			if err != nil {
				t.Fatalf("routing a command over the same socket = %v, want nil", err)
			}

			if accepted.Load() != 1 {
				t.Fatalf(
					"the socket registers nothing at all: %d connection(s) accepted after routing one command",
					accepted.Load(),
				)
			}
		})
	}
}

// TestActionCommands_RejectMalformedArgumentsIdenticallyWithAndWithoutADaemon
// is the user-visible half of mimi#126: the same malformed argument used to
// read one way when a daemon was listening and another way when it was not,
// because only one of the two paths checked it. Validating as the command is
// built leaves one sentence for both.
func TestActionCommands_RejectMalformedArgumentsIdenticallyWithAndWithoutADaemon(t *testing.T) {
	t.Parallel()

	for _, testCase := range malformedActionArgv() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			liveSocket := filepath.Join(shortSocketDir(t), "mimi.sock")
			listeningDaemon(t, liveSocket)

			// Nothing ever listens here, so this run falls back to the direct
			// path — the other side of the behavior difference.
			deadSocket := filepath.Join(shortSocketDir(t), "mimi.sock")

			_, withDaemon := runCommand(
				t,
				actionArgv(configWithSocket(t, liveSocket), testCase.argv)...)
			if withDaemon == nil {
				t.Fatalf("%v with a daemon listening: expected an error", testCase.argv)
			}

			_, withoutDaemon := runCommand(
				t,
				actionArgv(configWithSocket(t, deadSocket), testCase.argv)...)
			if withoutDaemon == nil {
				t.Fatalf("%v with no daemon: expected an error", testCase.argv)
			}

			if withDaemon.Error() != withoutDaemon.Error() {
				t.Errorf(
					"%v reads differently depending on the daemon:\n with: %s\n without: %s",
					testCase.argv,
					withDaemon,
					withoutDaemon,
				)
			}
		})
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
		{name: "whitespace only", args: []string{whitespaceOnlyArg}},
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

					_, wantErr := action.ParseSpaceArg(name, testCase.args)
					if wantErr == nil {
						t.Fatalf("ParseSpaceArg(%s, %v): expected an error", name, testCase.args)
					}

					assertRejectedBeforeRunE(
						t,
						[]string{actionCommandName, string(name)},
						spaceArgv(name, testCase.args),
						wantErr,
					)
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
