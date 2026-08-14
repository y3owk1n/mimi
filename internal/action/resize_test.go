package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// TestParseResizeRequest_MapsFlagsOntoTheGeometryRequest covers the flag
// spellings whose meaning is decided here rather than in the geometry: which
// of the three dimension kinds a size flag produces, and which of the optional
// fields a flag makes explicit.
func TestParseResizeRequest_MapsFlagsOntoTheGeometryRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want geometry.Request
	}{
		{
			name: "no arguments ask for nothing",
			args: nil,
			want: geometry.Request{},
		},
		{
			name: "a preset is taken from the first positional argument",
			args: []string{presetLeftHalf},
			want: geometry.Request{Preset: presetLeftHalf},
		},
		{
			name: "absolute sizes",
			args: []string{flagWidth, "800", flagHeight, "600"},
			want: geometry.Request{
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
			},
		},
		{
			name: "percentage sizes",
			args: []string{flagWidthPercent, "45", flagHeightPercent, "55"},
			want: geometry.Request{
				Width:  geometry.Percent(45),
				Height: geometry.Percent(55),
			},
		},
		{
			name: "an absolute size wins over a percentage one",
			args: []string{flagWidth, "800", flagWidthPercent, "45"},
			want: geometry.Request{Width: geometry.Absolute(800)},
		},
		{
			name: "an explicit position",
			args: []string{"--x", "100", "--y", "230"},
			want: geometry.Request{X: new(100.0), Y: new(230.0)},
		},
		{
			name: "a negative position is accepted, for displays left of the primary",
			args: []string{"--x", "-1920"},
			want: geometry.Request{X: new(-1920.0)},
		},
		{
			name: "an anchor",
			args: []string{flagAnchor, "br"},
			want: geometry.Request{Anchor: new(geometry.BottomRight)},
		},
		{
			name: "margins forced on",
			args: []string{"--margin"},
			want: geometry.Request{UseMargins: new(true)},
		},
		{
			name: "margins forced off",
			args: []string{"--no-margin"},
			want: geometry.Request{UseMargins: new(false)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ParseResizeRequest(testCase.args)
			if err != nil {
				t.Fatalf("ParseResizeRequest(%v) error = %v", testCase.args, err)
			}

			assertRequest(t, got, testCase.want)
		})
	}
}

// TestParseResizeRequest_ZeroSizeKeepsTheCurrentOne pins the CLI convention
// that a zero size flag means "not given": the window keeps the size it has,
// or takes the one a preset or a percentage supplies.
func TestParseResizeRequest_ZeroSizeKeepsTheCurrentOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want geometry.Request
	}{
		{
			name: "a zero width keeps the current width",
			args: []string{flagWidth, "0", flagHeight, "0"},
			want: geometry.Request{Width: geometry.Keep(), Height: geometry.Keep()},
		},
		{
			name: "a zero percentage keeps the current size too",
			args: []string{flagWidthPercent, "0"},
			want: geometry.Request{Width: geometry.Keep()},
		},
		{
			name: "a zero width still leaves a percentage to apply",
			args: []string{flagWidth, "0", flagWidthPercent, "45"},
			want: geometry.Request{Width: geometry.Percent(45)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := action.ParseResizeRequest(testCase.args)
			if err != nil {
				t.Fatalf("ParseResizeRequest(%v) error = %v", testCase.args, err)
			}

			assertRequest(t, got, testCase.want)
		})
	}
}

// TestParseResizeRequest_ZeroWidthLeavesTheWindowAsWide pins the same
// convention end to end: a zero width reaches the geometry as a kept
// dimension, so the frame it produces is as wide as the window already was.
func TestParseResizeRequest_ZeroWidthLeavesTheWindowAsWide(t *testing.T) {
	t.Parallel()

	req, err := action.ParseResizeRequest([]string{flagWidth, "0"})
	if err != nil {
		t.Fatalf("ParseResizeRequest(--width 0) error = %v", err)
	}

	screen := geometry.Screen{
		Visible:       geometry.Rect{X: 0, Y: 0, W: 1920, H: 1050},
		PrimaryHeight: 1080,
	}
	current := geometry.Rect{X: 300, Y: 180, W: 700, H: 500}

	if got := geometry.Resize(current, screen, req); got.W != current.W {
		t.Errorf("Resize(--width 0) = %v, want the current width %v", got, current.W)
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
