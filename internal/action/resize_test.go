package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// TestResizeRequestFromArgs_MapsArgumentsOntoTheGeometryRequest covers the
// arguments whose meaning is decided here rather than in the geometry: which
// of the three dimension kinds a size argument produces, and which of the
// optional fields an argument makes explicit.
//
// This is the only conversion left. It used to have a string-parsing twin that
// had to be kept in agreement with it, which is the drift
// docs/adr/0001-typed-versioned-daemon-wire.md exists to remove.
func TestResizeRequestFromArgs_MapsArgumentsOntoTheGeometryRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args action.ResizeWindowArgs
		want geometry.Request
	}{
		{
			name: "nothing set asks for nothing",
			args: action.ResizeWindowArgs{},
			want: geometry.Request{},
		},
		{
			name: "a preset carries through",
			args: action.ResizeWindowArgs{Preset: presetLeftHalf},
			want: geometry.Request{Preset: presetFor(t, presetLeftHalf)},
		},
		{
			name: "absolute sizes",
			args: action.ResizeWindowArgs{Width: 800, WidthSet: true, Height: 600, HeightSet: true},
			want: geometry.Request{Width: geometry.Absolute(800), Height: geometry.Absolute(600)},
		},
		{
			name: "percentage sizes",
			args: action.ResizeWindowArgs{
				WidthPercent: 45, WidthPercentSet: true,
				HeightPercent: 55, HeightPercentSet: true,
			},
			want: geometry.Request{Width: geometry.Percent(45), Height: geometry.Percent(55)},
		},
		{
			name: "an absolute size wins over a percentage one",
			args: action.ResizeWindowArgs{
				Width: 800, WidthSet: true,
				WidthPercent: 45, WidthPercentSet: true,
			},
			want: geometry.Request{Width: geometry.Absolute(800)},
		},
		{
			name: "an explicit position",
			args: action.ResizeWindowArgs{X: 100, XSet: true, Y: 230, YSet: true},
			want: geometry.Request{X: new(100.0), Y: new(230.0)},
		},
		{
			name: "a negative position is accepted, for displays left of the primary",
			args: action.ResizeWindowArgs{X: -1920, XSet: true},
			want: geometry.Request{X: new(-1920.0)},
		},
		{
			name: "an anchor",
			args: action.ResizeWindowArgs{Anchor: "br", AnchorSet: true},
			want: geometry.Request{Anchor: new(geometry.BottomRight)},
		},
		{
			name: "margins forced on",
			args: action.ResizeWindowArgs{UseMargin: true},
			want: geometry.Request{UseMargins: new(true)},
		},
		{
			name: "margins forced off",
			args: action.ResizeWindowArgs{NoMargin: true},
			want: geometry.Request{UseMargins: new(false)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ResizeRequestFromArgs(testCase.args)
			if err != nil {
				t.Fatalf("ResizeRequestFromArgs(%+v) error = %v", testCase.args, err)
			}

			assertRequest(t, got, testCase.want)
		})
	}
}

// TestResizeRequestFromArgs_TrimsAPaddedPresetName covers mimi#132 where it
// matters most: this conversion is what the daemon runs on a command it
// decoded off the socket, with no CLI in front of it to trim the name first. A
// padded preset therefore has to name its preset here, or the same argument
// means one thing on the direct path and nothing on the daemon path.
func TestResizeRequestFromArgs_TrimsAPaddedPresetName(t *testing.T) {
	t.Parallel()

	args := action.ResizeWindowArgs{Preset: " \t" + presetLeftHalf + " "}

	got, err := action.ResizeRequestFromArgs(args)
	if err != nil {
		t.Fatalf("ResizeRequestFromArgs(%+v) error = %v", args, err)
	}

	assertRequest(t, got, geometry.Request{Preset: presetFor(t, presetLeftHalf)})
}

// TestResizeRequestFromArgs_RejectsAPresetNameThatIsOnlyWhitespace pins the
// other half of that rule: whitespace is not part of a preset name, so an
// argument made of nothing else names no preset and is rejected — on every
// path, rather than quietly meaning "no preset" on the one that trimmed.
func TestResizeRequestFromArgs_RejectsAPresetNameThatIsOnlyWhitespace(t *testing.T) {
	t.Parallel()

	_, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs{Preset: whitespaceOnlyArg})
	if err == nil {
		t.Fatal("ResizeRequestFromArgs(whitespace preset) error = nil, want an error")
	}

	if !derrors.IsCode(err, derrors.CodeInvalidInput) {
		t.Fatalf("expected CodeInvalidInput, got %v", err)
	}
}

// TestResizeRequestFromArgs_ZeroSizeKeepsTheCurrentOne pins the CLI convention
// that a zero size means "keep what the window has": the flag was given, so it
// crosses the wire, but the window keeps the size it has or takes the one a
// preset or a percentage supplies.
func TestResizeRequestFromArgs_ZeroSizeKeepsTheCurrentOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args action.ResizeWindowArgs
		want geometry.Request
	}{
		{
			name: "a zero width and height keep the current ones",
			args: action.ResizeWindowArgs{Width: 0, WidthSet: true, Height: 0, HeightSet: true},
			want: geometry.Request{Width: geometry.Keep(), Height: geometry.Keep()},
		},
		{
			name: "a zero percentage keeps the current size too",
			args: action.ResizeWindowArgs{WidthPercent: 0, WidthPercentSet: true},
			want: geometry.Request{Width: geometry.Keep()},
		},
		{
			name: "a zero width still leaves a percentage to apply",
			args: action.ResizeWindowArgs{
				Width: 0, WidthSet: true,
				WidthPercent: 45, WidthPercentSet: true,
			},
			want: geometry.Request{Width: geometry.Percent(45)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ResizeRequestFromArgs(testCase.args)
			if err != nil {
				t.Fatalf("ResizeRequestFromArgs(%+v) error = %v", testCase.args, err)
			}

			assertRequest(t, got, testCase.want)
		})
	}
}

// TestResizeRequestFromArgs_ZeroWidthLeavesTheWindowAsWide pins the same
// convention end to end: a zero width reaches the geometry as a kept
// dimension, so the frame it produces is as wide as the window already was.
func TestResizeRequestFromArgs_ZeroWidthLeavesTheWindowAsWide(t *testing.T) {
	t.Parallel()

	req, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs{Width: 0, WidthSet: true})
	if err != nil {
		t.Fatalf("ResizeRequestFromArgs(width 0) error = %v", err)
	}

	screen := geometry.Screen{
		Visible:       geometry.Rect{X: 0, Y: 0, W: 1920, H: 1050},
		PrimaryHeight: 1080,
	}
	current := geometry.Rect{X: 300, Y: 180, W: 700, H: 500}

	if got := geometry.Resize(current, screen, req); got.W != current.W {
		t.Errorf("Resize(width 0) = %v, want the current width %v", got, current.W)
	}
}

// assertRequest compares two requests field by field, following the pointers
// the optional fields are expressed with.
func assertRequest(t *testing.T, got, want geometry.Request) {
	t.Helper()

	if got.Preset != want.Preset {
		t.Errorf("Preset = %q, want %q", got.Preset, want.Preset)
	}

	if got.Width != want.Width {
		t.Errorf("Width = %+v, want %+v", got.Width, want.Width)
	}

	if got.Height != want.Height {
		t.Errorf("Height = %+v, want %+v", got.Height, want.Height)
	}

	assertPointer(t, "X", got.X, want.X)
	assertPointer(t, "Y", got.Y, want.Y)
	assertPointer(t, "Anchor", got.Anchor, want.Anchor)
	assertPointer(t, "UseMargins", got.UseMargins, want.UseMargins)
}

// assertPointer compares one optional field, whose nil is what "the user did
// not say" is expressed with.
func assertPointer[T comparable](t *testing.T, name string, got, want *T) {
	t.Helper()

	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s = nil, want %v", name, *want)
	case want == nil:
		t.Errorf("%s = %v, want nil", name, *got)
	case *got != *want:
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}
