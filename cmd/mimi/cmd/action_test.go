//nolint:testpackage
package cmd

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
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

// TestFocusWindowCommand_RejectsBackwardWithDirection checks the CLI surfaces
// action.NewFocusWindowArgs' validation before ever reaching the desktop —
// this fails during flag validation, not execution, so it is safe to run
// through the real command tree.
func TestFocusWindowCommand_RejectsBackwardWithDirection(t *testing.T) {
	t.Parallel()

	_, err := runCommand(t, "action", "focus_window", "--backward", "--up")
	if err == nil {
		t.Fatal("expected an error combining --backward with a direction flag")
	}
}

// TestSpaceCommand_RejectsZero checks the space subcommand's positional-arg
// validation still runs before state.runAction, the same guarantee it made
// before the CLI started building a typed action.Command.
func TestSpaceCommand_RejectsZero(t *testing.T) {
	t.Parallel()

	_, err := runCommand(t, "action", "space", "0")
	if err == nil {
		t.Fatal("expected an error for space 0")
	}
}
