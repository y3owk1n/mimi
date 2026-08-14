package geometry_test

import (
	"math"
	"testing"

	"github.com/y3owk1n/mimi/internal/geometry"
)

// The display the window baseline was recorded on: one 1920x1080 screen whose
// visible frame is inset by the 30pt menu bar. Wherever a case below matches a
// recorded one, its expected frame is the frame macOS actually produced;
// internal/baseline replays the whole recording against this same geometry.
var singleDisplay = geometry.Screen{
	Visible:        geometry.Rect{X: 0, Y: 0, W: 1920, H: 1050},
	PrimaryHeight:  1080,
	MarginsEnabled: false,
	MarginSize:     8,
}

// The preset names the tests below reach for by name.
const (
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
)

// The frame every recorded resize case started from.
var startFrame = geometry.Rect{X: 300, Y: 180, W: 700, H: 500}

func TestResize_AnchorsAnAbsolutelySizedWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		anchor geometry.Anchor
		want   geometry.Rect
	}{
		{anchor: geometry.TopLeft, want: geometry.Rect{X: 0, Y: 30, W: 800, H: 600}},
		{anchor: geometry.TopCenter, want: geometry.Rect{X: 560, Y: 30, W: 800, H: 600}},
		{anchor: geometry.TopRight, want: geometry.Rect{X: 1120, Y: 30, W: 800, H: 600}},
		{anchor: geometry.CenterLeft, want: geometry.Rect{X: 0, Y: 255, W: 800, H: 600}},
		{anchor: geometry.Center, want: geometry.Rect{X: 560, Y: 255, W: 800, H: 600}},
		{anchor: geometry.CenterRight, want: geometry.Rect{X: 1120, Y: 255, W: 800, H: 600}},
		{anchor: geometry.BottomLeft, want: geometry.Rect{X: 0, Y: 480, W: 800, H: 600}},
		{anchor: geometry.BottomCenter, want: geometry.Rect{X: 560, Y: 480, W: 800, H: 600}},
		{anchor: geometry.BottomRight, want: geometry.Rect{X: 1120, Y: 480, W: 800, H: 600}},
	}

	for _, testCase := range tests {
		t.Run(testCase.anchor.String(), func(t *testing.T) {
			t.Parallel()

			anchor := testCase.anchor
			got := geometry.Resize(startFrame, singleDisplay, geometry.Request{
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
				Anchor: &anchor,
			})

			if got != testCase.want {
				t.Errorf("Resize(anchor %s) = %v, want %v", testCase.anchor, got, testCase.want)
			}
		})
	}
}

func TestResize_ResolvesEachKindOfDimension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  geometry.Request
		want geometry.Rect
	}{
		{
			name: "an empty request keeps both dimensions and centers the window",
			req:  geometry.Request{},
			want: geometry.Rect{X: 610, Y: 305, W: 700, H: 500},
		},
		{
			name: "an absolute width keeps the current height",
			req:  geometry.Request{Width: geometry.Absolute(800)},
			want: geometry.Rect{X: 560, Y: 305, W: 800, H: 500},
		},
		{
			name: "an absolute height keeps the current width",
			req:  geometry.Request{Height: geometry.Absolute(600)},
			want: geometry.Rect{X: 610, Y: 255, W: 700, H: 600},
		},
		{
			name: "a kept dimension is the zero dimension",
			req:  geometry.Request{Width: geometry.Keep(), Height: geometry.Keep()},
			want: geometry.Rect{X: 610, Y: 305, W: 700, H: 500},
		},
		{
			// 45% of 1920 and 55% of 1050. macOS quantizes the frame it is
			// handed, so the recording holds the whole-point form of this.
			name: "percentages are taken over the visible frame",
			req: geometry.Request{
				Width:  geometry.Percent(45),
				Height: geometry.Percent(55),
			},
			want: geometry.Rect{X: 528, Y: 266.25, W: 864, H: 577.5},
		},
		{
			name: "a full-height percentage fills the visible frame",
			req: geometry.Request{
				Width:  geometry.Percent(50),
				Height: geometry.Percent(100),
			},
			want: geometry.Rect{X: 480, Y: 30, W: 960, H: 1050},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, singleDisplay, testCase.req)
			if got != testCase.want {
				t.Errorf("Resize() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestResize_PlacesAnExplicitOriginVerbatim(t *testing.T) {
	t.Parallel()

	posX, posY := 100.0, 230.0

	tests := []struct {
		name string
		req  geometry.Request
		want geometry.Rect
	}{
		{
			name: "an explicit origin keeps the current size",
			req:  geometry.Request{X: &posX, Y: &posY},
			want: geometry.Rect{X: 100, Y: 230, W: 700, H: 500},
		},
		{
			name: "an explicit origin overrides the anchor on both axes",
			req: geometry.Request{
				X:      &posX,
				Y:      &posY,
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
			},
			want: geometry.Rect{X: 100, Y: 230, W: 800, H: 600},
		},
		{
			name: "an explicit x leaves y to the anchor",
			req:  geometry.Request{X: &posX, Height: geometry.Absolute(600)},
			want: geometry.Rect{X: 100, Y: 255, W: 700, H: 600},
		},
		{
			name: "an explicit y leaves x to the anchor",
			req:  geometry.Request{Y: &posY, Width: geometry.Absolute(800)},
			want: geometry.Rect{X: 560, Y: 230, W: 800, H: 500},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, singleDisplay, testCase.req)
			if got != testCase.want {
				t.Errorf("Resize() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestResize_ExpandsEveryPreset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset string
		want   geometry.Rect
	}{
		{preset: presetLeftHalf, want: geometry.Rect{X: 0, Y: 30, W: 960, H: 1050}},
		{preset: presetRightHalf, want: geometry.Rect{X: 960, Y: 30, W: 960, H: 1050}},
		{preset: presetTopHalf, want: geometry.Rect{X: 0, Y: 30, W: 1920, H: 525}},
		{preset: presetBottomHalf, want: geometry.Rect{X: 0, Y: 555, W: 1920, H: 525}},
		{preset: presetTopLeft, want: geometry.Rect{X: 0, Y: 30, W: 960, H: 525}},
		{preset: presetTopRight, want: geometry.Rect{X: 960, Y: 30, W: 960, H: 525}},
		{preset: presetBottomLeft, want: geometry.Rect{X: 0, Y: 555, W: 960, H: 525}},
		{preset: presetBottomRight, want: geometry.Rect{X: 960, Y: 555, W: 960, H: 525}},
		{preset: presetCenter, want: geometry.Rect{X: 384, Y: 135, W: 1152, H: 840}},
		{preset: presetFill, want: geometry.Rect{X: 0, Y: 30, W: 1920, H: 1050}},
	}

	for _, testCase := range tests {
		t.Run(testCase.preset, func(t *testing.T) {
			t.Parallel()

			req := geometry.Request{Preset: testCase.preset}

			got := geometry.Resize(startFrame, singleDisplay, req)
			if got != testCase.want {
				t.Errorf("Resize(preset %s) = %v, want %v", testCase.preset, got, testCase.want)
			}
		})
	}
}

func TestResize_PresetSizesAreDefaults(t *testing.T) {
	t.Parallel()

	// left-half is 50% x 100%; an explicit width replaces its width and leaves
	// its height and anchor in place.
	got := geometry.Resize(startFrame, singleDisplay, geometry.Request{
		Preset: presetLeftHalf,
		Width:  geometry.Absolute(800),
	})

	want := geometry.Rect{X: 0, Y: 30, W: 800, H: 1050}
	if got != want {
		t.Errorf("Resize(left-half --width 800) = %v, want %v", got, want)
	}
}

func TestResize_IgnoresAnUnknownPreset(t *testing.T) {
	t.Parallel()

	// Resize is total, so an unknown name cannot be an error here; the CLI
	// rejects one at parse time with IsPreset.
	got := geometry.Resize(startFrame, singleDisplay, geometry.Request{Preset: "left-third"})

	want := geometry.Resize(startFrame, singleDisplay, geometry.Request{})
	if got != want {
		t.Errorf("Resize(preset left-third) = %v, want the presetless %v", got, want)
	}
}

func TestResize_AppliesMargins(t *testing.T) {
	t.Parallel()

	// The same display with the system tiled-window margins switched on.
	withMargins := singleDisplay
	withMargins.MarginsEnabled = true

	posX, posY := 100.0, 230.0
	topCenter := geometry.TopCenter

	tests := []struct {
		name string
		req  geometry.Request
		want geometry.Rect
	}{
		{
			// Left, top and bottom abut the visible frame and take a full 8pt
			// margin; the right edge is internal and takes half of one.
			name: "a half-screen preset takes a full margin on the edges it abuts",
			req:  geometry.Request{Preset: presetLeftHalf},
			want: geometry.Rect{X: 8, Y: 38, W: 948, H: 1034},
		},
		{
			name: "a filling preset takes a full margin on all four edges",
			req:  geometry.Request{Preset: presetFill},
			want: geometry.Rect{X: 8, Y: 38, W: 1904, H: 1034},
		},
		{
			name: "a preset that abuts nothing takes half a margin on all four edges",
			req:  geometry.Request{Preset: presetCenter},
			want: geometry.Rect{X: 388, Y: 139, W: 1144, H: 832},
		},
		{
			name: "an anchored window takes a full margin only where it abuts",
			req: geometry.Request{
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
				Anchor: &topCenter,
			},
			want: geometry.Rect{X: 564, Y: 38, W: 792, H: 588},
		},
		{
			name: "an explicitly placed window is inset too",
			req: geometry.Request{
				X:      &posX,
				Y:      &posY,
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
			},
			want: geometry.Rect{X: 104, Y: 234, W: 792, H: 592},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, withMargins, testCase.req)
			if got != testCase.want {
				t.Errorf("Resize() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestResize_MarginPreferenceOverridesTheSystemSetting(t *testing.T) {
	t.Parallel()

	withMargins := singleDisplay
	withMargins.MarginsEnabled = true

	enabled, disabled := true, false
	inset := geometry.Rect{X: 8, Y: 38, W: 948, H: 1034}
	full := geometry.Rect{X: 0, Y: 30, W: 960, H: 1050}

	tests := []struct {
		name string
		scr  geometry.Screen
		req  geometry.Request
		want geometry.Rect
	}{
		{
			name: "no preference follows a system setting that is on",
			scr:  withMargins,
			req:  geometry.Request{Preset: presetLeftHalf},
			want: inset,
		},
		{
			name: "no preference follows a system setting that is off",
			scr:  singleDisplay,
			req:  geometry.Request{Preset: presetLeftHalf},
			want: full,
		},
		{
			name: "margins off overrides a system setting that is on",
			scr:  withMargins,
			req:  geometry.Request{Preset: presetLeftHalf, UseMargins: &disabled},
			want: full,
		},
		{
			name: "margins on overrides a system setting that is off",
			scr:  singleDisplay,
			req:  geometry.Request{Preset: presetLeftHalf, UseMargins: &enabled},
			want: inset,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, testCase.scr, testCase.req)
			if got != testCase.want {
				t.Errorf("Resize() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestResize_PlacesWindowsOnSecondaryDisplays(t *testing.T) {
	t.Parallel()

	// A second 1920x1080 display to the left of the primary. macOS gives it a
	// negative x in both coordinate systems; its bottom edge is level with the
	// primary's, so the flip leaves its top at 0.
	toTheLeft := geometry.Screen{
		Visible:       geometry.Rect{X: -1920, Y: 0, W: 1920, H: 1080},
		PrimaryHeight: 1080,
		MarginSize:    8,
	}

	// A second 1920x1080 display above the primary. In screen coordinates it
	// sits a whole primary height up; in window coordinates that is a whole
	// primary height *down* from the origin, so every y on it is negative.
	above := geometry.Screen{
		Visible:       geometry.Rect{X: 0, Y: 1080, W: 1920, H: 1080},
		PrimaryHeight: 1080,
		MarginSize:    8,
	}

	tests := []struct {
		name string
		scr  geometry.Screen
		req  geometry.Request
		want geometry.Rect
	}{
		{
			name: "filling the display to the left",
			scr:  toTheLeft,
			req:  geometry.Request{Preset: presetFill},
			want: geometry.Rect{X: -1920, Y: 0, W: 1920, H: 1080},
		},
		{
			name: "the right half of the display to the left",
			scr:  toTheLeft,
			req:  geometry.Request{Preset: presetRightHalf},
			want: geometry.Rect{X: -960, Y: 0, W: 960, H: 1080},
		},
		{
			name: "margins on the display to the left",
			scr:  toTheLeft,
			req:  geometry.Request{Preset: presetFill, UseMargins: new(true)},
			want: geometry.Rect{X: -1912, Y: 8, W: 1904, H: 1064},
		},
		{
			name: "filling the display above sits a whole primary height up",
			scr:  above,
			req:  geometry.Request{Preset: presetFill},
			want: geometry.Rect{X: 0, Y: -1080, W: 1920, H: 1080},
		},
		{
			name: "the bottom half of the display above still ends at the primary's top",
			scr:  above,
			req:  geometry.Request{Preset: presetBottomHalf},
			want: geometry.Rect{X: 0, Y: -540, W: 1920, H: 540},
		},
		{
			name: "an anchor on the display above is relative to its own frame",
			scr:  above,
			req: geometry.Request{
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
				Anchor: new(geometry.BottomRight),
			},
			want: geometry.Rect{X: 1120, Y: -600, W: 800, H: 600},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, testCase.scr, testCase.req)
			if got != testCase.want {
				t.Errorf("Resize() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestResize_PresetOverridesExplicitAnchor_KnownBug pins issue #64: a preset
// supplies its width and height only where the request left them unset, but
// takes the anchor unconditionally, so an explicit --anchor is silently
// dropped. The expected frame here is the one macOS produced for
// `resize_window left-half --anchor br --no-margin` in the recorded baseline.
//
// #64 inverts this test.
func TestResize_PresetOverridesExplicitAnchor_KnownBug(t *testing.T) {
	t.Parallel()

	got := geometry.Resize(startFrame, singleDisplay, geometry.Request{
		Preset: presetLeftHalf,
		Anchor: new(geometry.BottomRight),
	})

	// left-half's own top-left anchor, not the bottom-right that was asked for.
	want := geometry.Rect{X: 0, Y: 30, W: 960, H: 1050}
	if got != want {
		t.Errorf("Resize(left-half --anchor br) = %v, want %v", got, want)
	}
}

// TestResize_MarginsCanDriveADimensionNonPositive_KnownBug pins issue #65:
// margins are subtracted with no lower bound, so a window narrower than the
// margins it takes ends up with a zero or negative size, which is then handed
// to the Accessibility API as-is.
//
// #65 skips margins entirely in this case, and inverts this test.
func TestResize_MarginsCanDriveADimensionNonPositive_KnownBug(t *testing.T) {
	t.Parallel()

	// A large configured margin: 16pt on each internal edge.
	wide := singleDisplay
	wide.MarginsEnabled = true
	wide.MarginSize = 32

	got := geometry.Resize(startFrame, wide, geometry.Request{
		Width:  geometry.Absolute(20),
		Height: geometry.Absolute(20),
	})

	// 20 points of window less 32 points of margin, on both axes.
	want := geometry.Rect{X: 966, Y: 561, W: -12, H: -12}
	if got != want {
		t.Errorf("Resize(--width 20 --height 20 --margin) = %v, want %v", got, want)
	}
}

// TestResize_EdgeDetectionUsesExactFloatEquality_KnownBug pins issue #66:
// whether an edge takes a full margin or half of one is decided by comparing
// computed floats for exact equality, so an edge that is flush by construction
// can still be read as internal.
//
// The display here is a 1512x982 primary with a second display directly above
// it, which is what makes the arithmetic inexact: the flip puts the upper
// display's top at -1080, and 10% of 982 is 98.2, so the bottom edge computed
// as y+h misses the visible frame's lower boundary by a few ulps.
//
// That scenario is deliberate. #66's own repro — `fill` against a centered
// 100%x100% request — does not actually diverge on a display whose metrics are
// whole points: a 100% dimension comes back exactly, and centering a window as
// wide as the frame lands exactly on its edge. Only a fractional dimension
// separates the two rules, which is why this test reaches for one.
//
// #66 derives the edge flags from the anchor instead, and inverts this test.
func TestResize_EdgeDetectionUsesExactFloatEquality_KnownBug(t *testing.T) {
	t.Parallel()

	above := geometry.Screen{
		Visible:        geometry.Rect{X: 0, Y: 1080, W: 1512, H: 982},
		PrimaryHeight:  982,
		MarginsEnabled: true,
		MarginSize:     8,
	}

	// Both windows are one tenth of the visible frame tall and flush against
	// one of its horizontal boundaries, so both should take a full 8pt margin
	// there and half of one on the opposite, internal edge.
	const requested = 98.2

	tests := []struct {
		name   string
		anchor geometry.Anchor
		want   float64
	}{
		{
			// The top edge is assigned from the visible frame, so it compares
			// equal and takes its full margin.
			name:   "an edge assigned from the visible frame is recognized",
			anchor: geometry.TopLeft,
			want:   8 + 4,
		},
		{
			// The bottom edge is computed, so it does not.
			name:   "an edge arrived at by arithmetic is not",
			anchor: geometry.BottomLeft,
			want:   4 + 4,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, above, geometry.Request{
				Height: geometry.Percent(10),
				Anchor: &testCase.anchor,
			})

			if taken := requested - got.H; math.Abs(taken-testCase.want) > tolerance {
				t.Errorf(
					"Resize(--height-percent 10 --anchor %s --margin) gave up %v points of height to margins, want %v",
					testCase.anchor,
					taken,
					testCase.want,
				)
			}
		})
	}
}

// tolerance is the slack the float comparisons in this file allow, well below
// anything a display can show.
const tolerance = 1e-9

func TestIsPreset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: presetLeftHalf, want: true},
		{name: presetRightHalf, want: true},
		{name: presetTopHalf, want: true},
		{name: presetBottomHalf, want: true},
		{name: presetTopLeft, want: true},
		{name: presetTopRight, want: true},
		{name: presetBottomLeft, want: true},
		{name: presetBottomRight, want: true},
		{name: presetCenter, want: true},
		{name: presetFill, want: true},
		{name: "left-third", want: false},
		{name: "", want: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := geometry.IsPreset(testCase.name); got != testCase.want {
				t.Fatalf("IsPreset(%q) = %v, want %v", testCase.name, got, testCase.want)
			}
		})
	}
}
