package action_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

const (
	nextKeyword     = "next"
	prevKeyword     = "prev"
	previousKeyword = "previous"

	presetLeftHalf    = "left-half"
	presetRightHalf   = "right-half"
	presetTopHalf     = "top-half"
	presetBottomHalf  = "bottom-half"
	presetTopLeft     = "top-left"
	presetTopRight    = "top-right"
	presetBottomLeft  = "bottom-left"
	presetBottomRight = "bottom-right"
	presetCenter      = "center"
	presetFill        = "fill"

	// unknownPreset is a name that is not one of the ten, and never becomes
	// one: the input every rejection case below is written against.
	unknownPreset = "left-third"

	// nonNumericArg names no space and no preset — the argument every
	// "that is not a number or a keyword" case is written against.
	nonNumericArg = "foo"

	// whitespaceOnlyArg is made of nothing but whitespace, which is part of
	// no argument's spelling: it names no space, and names no preset either.
	whitespaceOnlyArg = "   "
)

// TestParseSpaceArg_AcceptedForms covers the accepted half of the one space
// argument rule: a 1-based number, "next", "prev", or "previous", surrounding
// whitespace included.
func TestParseSpaceArg_AcceptedForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arg  string
		want action.SpaceArg
	}{
		{name: "first space", arg: "1", want: action.SpaceArg{Index: 1}},
		{name: "padded number", arg: " 2 ", want: action.SpaceArg{Index: 2}},
		{name: nextKeyword, arg: nextKeyword, want: action.SpaceArg{Direction: 1}},
		{name: prevKeyword, arg: prevKeyword, want: action.SpaceArg{Direction: -1}},
		{name: previousKeyword, arg: previousKeyword, want: action.SpaceArg{Direction: -1}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, name := range []action.Name{action.NameSpace, action.NameMoveWindowToSpace} {
				got, err := action.ParseSpaceArg(name, []string{testCase.arg})
				if err != nil {
					t.Fatalf("ParseSpaceArg(%s, %q) error = %v, want nil", name, testCase.arg, err)
				}

				if got != testCase.want {
					t.Errorf(
						"ParseSpaceArg(%s, %q) = %+v, want %+v",
						name,
						testCase.arg,
						got,
						testCase.want,
					)
				}
			}
		})
	}
}

// TestParseSpaceArg_MalformedNamesTheAction checks a rejected argument is
// reported as invalid input and names the action it was given to — the only
// part of the wording that differs between the two space actions.
func TestParseSpaceArg_MalformedNamesTheAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{""}},
		{name: "whitespace only", args: []string{whitespaceOnlyArg}},
		{name: "non-numeric", args: []string{nonNumericArg}},
		{name: "zero", args: []string{"0"}},
		{name: "negative", args: []string{"-1"}},
		{name: "no argument", args: nil},
		{name: "two arguments", args: []string{"1", "2"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, name := range []action.Name{action.NameSpace, action.NameMoveWindowToSpace} {
				_, err := action.ParseSpaceArg(name, testCase.args)
				if err == nil {
					t.Fatalf(
						"ParseSpaceArg(%s, %v) error = nil, want an error",
						name,
						testCase.args,
					)
				}

				if !derrors.IsCode(err, derrors.CodeInvalidInput) {
					t.Errorf(
						"ParseSpaceArg(%s, %v) got %v, want CodeInvalidInput",
						name,
						testCase.args,
						err,
					)
				}

				if !strings.Contains(err.Error(), string(name)) {
					t.Errorf(
						"ParseSpaceArg(%s, %v) did not name the action: %v",
						name,
						testCase.args,
						err,
					)
				}
			}
		})
	}
}

// everyPreset lists the ten preset names resize_window takes. The cases below
// are written against this list rather than against the geometry's own, so a
// preset silently leaving the table is a failure rather than a case that stops
// running.
func everyPreset() []string {
	return []string{
		presetLeftHalf, presetRightHalf, presetTopHalf, presetBottomHalf,
		presetTopLeft, presetTopRight, presetBottomLeft, presetBottomRight,
		presetCenter, presetFill,
	}
}

// presetFor is the preset a name names, for the cases that expect one to reach
// the geometry.
func presetFor(t *testing.T, name string) geometry.Preset {
	t.Helper()

	preset, ok := geometry.ParsePreset(name)
	if !ok {
		t.Fatalf("ParsePreset(%q) is not a preset", name)
	}

	return preset
}

// assertListsEveryPreset checks a rejection names all ten presets, which is
// what makes it useful to whoever mistyped one.
func assertListsEveryPreset(t *testing.T, err error) {
	t.Helper()

	for _, name := range everyPreset() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("rejection does not list %q: %v", name, err)
		}
	}
}

// TestResizePreset covers mimi#125: one rule decides whether a name is a
// preset, and it hands back the preset itself rather than a boolean beside it,
// so a caller cannot carry an unrecognized name past the check.
//
// Each preset is run as it is written and with padding around it, because
// mimi#132 makes surrounding whitespace this rule's business rather than the
// CLI's: both spellings name the same preset, whoever asks.
func TestResizePreset(t *testing.T) {
	t.Parallel()

	for _, name := range everyPreset() {
		spellings := map[string]string{
			"as written": name,
			"padded":     "  " + name + "\t",
		}

		for label, given := range spellings {
			t.Run(name+"/"+label, func(t *testing.T) {
				t.Parallel()

				got, err := action.ParseResizePreset(given)
				if err != nil {
					t.Fatalf("ParseResizePreset(%q) error = %v", given, err)
				}

				if got != presetFor(t, name) {
					t.Fatalf("ParseResizePreset(%q) = %q, want %q", given, got, name)
				}
			})
		}
	}

	t.Run("whitespace only", func(t *testing.T) {
		t.Parallel()

		_, err := action.ParseResizePreset(whitespaceOnlyArg)
		if err == nil {
			t.Fatalf("ParseResizePreset(%q) expected error", whitespaceOnlyArg)
		}

		if !derrors.IsCode(err, derrors.CodeInvalidInput) {
			t.Fatalf("expected CodeInvalidInput, got %v", err)
		}

		// The rejection quotes the argument as it was given, so it never
		// reports an empty name for something the user did type.
		if !strings.Contains(err.Error(), strconv.Quote(whitespaceOnlyArg)) {
			t.Errorf("rejection does not quote %q as given: %v", whitespaceOnlyArg, err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()

		_, err := action.ParseResizePreset(unknownPreset)
		if err == nil {
			t.Fatalf("ParseResizePreset(%q) expected error", unknownPreset)
		}

		if !derrors.IsCode(err, derrors.CodeInvalidInput) {
			t.Fatalf("expected CodeInvalidInput, got %v", err)
		}

		assertListsEveryPreset(t, err)
	})
}

// TestParseResizePresetArg covers the one thing resize_window's positional
// argument adds to the preset rule: it is optional, so the empty string is the
// argument nobody gave and names no preset without that being an error.
// Everything else is ParseResizePreset's decision, which is why an unknown name
// still reads in its words — the CLI's Args layer and ResizeRequestFromArgs
// both reject through here (mimi#133).
func TestParseResizePresetArg(t *testing.T) {
	t.Parallel()

	t.Run("no argument names no preset", func(t *testing.T) {
		t.Parallel()

		got, err := action.ParseResizePresetArg("")
		if err != nil {
			t.Fatalf(`ParseResizePresetArg("") error = %v`, err)
		}

		if (got != geometry.Preset{}) {
			t.Fatalf(`ParseResizePresetArg("") = %v, want the zero preset`, got)
		}
	})

	t.Run("a name is the preset rule's decision", func(t *testing.T) {
		t.Parallel()

		for _, name := range append(everyPreset(), unknownPreset, whitespaceOnlyArg) {
			got, gotErr := action.ParseResizePresetArg(name)
			want, wantErr := action.ParseResizePreset(name)

			switch {
			case (gotErr == nil) != (wantErr == nil):
				t.Errorf("ParseResizePresetArg(%q) error = %v, want %v", name, gotErr, wantErr)
			case gotErr != nil && gotErr.Error() != wantErr.Error():
				t.Errorf(
					"ParseResizePresetArg(%q) rejected in other words:\n got: %s\nwant: %s",
					name,
					gotErr,
					wantErr,
				)
			case gotErr == nil && got != want:
				t.Errorf("ParseResizePresetArg(%q) = %v, want %v", name, got, want)
			}
		}
	})
}
