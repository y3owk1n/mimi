package geometry_test

import (
	"fmt"
	"math"
	"slices"
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

// The presets the cases below place windows with, in the form a request holds
// them.
var (
	leftHalf   = presetFor(presetLeftHalf)
	rightHalf  = presetFor(presetRightHalf)
	bottomHalf = presetFor(presetBottomHalf)
	center     = presetFor(presetCenter)
	fill       = presetFor(presetFill)
)

// presetFor is ParsePreset for the names this file spells out as constants,
// which TestParsePreset pins as ten of the ten. A name that somehow stopped
// being one yields the zero preset, and the frame the case expects then fails
// to match.
func presetFor(name string) geometry.Preset {
	preset, _ := geometry.ParsePreset(name)

	return preset
}

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

			req := geometry.Request{Preset: presetFor(testCase.preset)}

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
		Preset: leftHalf,
		Width:  geometry.Absolute(800),
	})

	want := geometry.Rect{X: 0, Y: 30, W: 800, H: 1050}
	if got != want {
		t.Errorf("Resize(left-half --width 800) = %v, want %v", got, want)
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
			req:  geometry.Request{Preset: leftHalf},
			want: geometry.Rect{X: 8, Y: 38, W: 948, H: 1034},
		},
		{
			name: "a filling preset takes a full margin on all four edges",
			req:  geometry.Request{Preset: fill},
			want: geometry.Rect{X: 8, Y: 38, W: 1904, H: 1034},
		},
		{
			name: "a preset that abuts nothing takes half a margin on all four edges",
			req:  geometry.Request{Preset: center},
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
			req:  geometry.Request{Preset: leftHalf},
			want: inset,
		},
		{
			name: "no preference follows a system setting that is off",
			scr:  singleDisplay,
			req:  geometry.Request{Preset: leftHalf},
			want: full,
		},
		{
			name: "margins off overrides a system setting that is on",
			scr:  withMargins,
			req:  geometry.Request{Preset: leftHalf, UseMargins: &disabled},
			want: full,
		},
		{
			name: "margins on overrides a system setting that is off",
			scr:  singleDisplay,
			req:  geometry.Request{Preset: leftHalf, UseMargins: &enabled},
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
			req:  geometry.Request{Preset: fill},
			want: geometry.Rect{X: -1920, Y: 0, W: 1920, H: 1080},
		},
		{
			name: "the right half of the display to the left",
			scr:  toTheLeft,
			req:  geometry.Request{Preset: rightHalf},
			want: geometry.Rect{X: -960, Y: 0, W: 960, H: 1080},
		},
		{
			name: "margins on the display to the left",
			scr:  toTheLeft,
			req:  geometry.Request{Preset: fill, UseMargins: new(true)},
			want: geometry.Rect{X: -1912, Y: 8, W: 1904, H: 1064},
		},
		{
			name: "filling the display above sits a whole primary height up",
			scr:  above,
			req:  geometry.Request{Preset: fill},
			want: geometry.Rect{X: 0, Y: -1080, W: 1920, H: 1080},
		},
		{
			name: "the bottom half of the display above still ends at the primary's top",
			scr:  above,
			req:  geometry.Request{Preset: bottomHalf},
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

// TestResize_PresetAnchorIsADefault covers issue #64: a preset supplies its
// anchor on the same terms as its width and height — only where the request
// left one unset — so an explicit --anchor places the window the preset sized.
// The expected frame here is the one macOS produced for
// `resize_window left-half --anchor br --no-margin` in the re-recorded
// baseline.
func TestResize_PresetAnchorIsADefault(t *testing.T) {
	t.Parallel()

	got := geometry.Resize(startFrame, singleDisplay, geometry.Request{
		Preset: leftHalf,
		Anchor: new(geometry.BottomRight),
	})

	// The bottom-right that was asked for, at left-half's own 50% x 100%.
	want := geometry.Rect{X: 960, Y: 30, W: 960, H: 1050}
	if got != want {
		t.Errorf("Resize(left-half --anchor br) = %v, want %v", got, want)
	}
}

// TestResize_SkipsMarginsThatWouldLeaveNoWindow covers issue #65: a window
// narrower or shorter than the margins its edges take is placed unmargined,
// at the size that was asked for, rather than being handed a zero or negative
// dimension. Margins are cosmetic, and clamping the dimension instead would
// leave a window that is valid but useless.
//
// The threshold is the last point at which something is left: a dimension
// takes its margins while what remains of it stays above zero, and gives them
// all up as soon as it does not. Each axis is checked either side of that
// line, and each drops the margins on both axes.
func TestResize_SkipsMarginsThatWouldLeaveNoWindow(t *testing.T) {
	t.Parallel()

	// A large configured margin. None of these windows abuts the visible
	// frame, so every edge takes half of it — 16 points an edge, 32 across
	// either axis.
	wide := singleDisplay
	wide.MarginsEnabled = true
	wide.MarginSize = 32

	tests := []struct {
		name string
		req  geometry.Request
		want geometry.Rect
	}{
		{
			name: "both axes too small to take their margins",
			req: geometry.Request{
				Width:  geometry.Absolute(20),
				Height: geometry.Absolute(20),
			},
			// Centered, at the 20x20 asked for rather than at -12x-12.
			want: geometry.Rect{X: 950, Y: 545, W: 20, H: 20},
		},
		{
			name: "a width exactly equal to its margins keeps all of it",
			req: geometry.Request{
				Width:  geometry.Absolute(32),
				Height: geometry.Absolute(600),
			},
			// Nothing would be left of the width, so the height that could
			// have taken its own margins keeps them too.
			want: geometry.Rect{X: 944, Y: 255, W: 32, H: 600},
		},
		{
			name: "a width one point wider takes them",
			req: geometry.Request{
				Width:  geometry.Absolute(33),
				Height: geometry.Absolute(600),
			},
			want: geometry.Rect{X: 959.5, Y: 271, W: 1, H: 568},
		},
		{
			name: "a height exactly equal to its margins keeps all of it",
			req: geometry.Request{
				Width:  geometry.Absolute(600),
				Height: geometry.Absolute(32),
			},
			want: geometry.Rect{X: 660, Y: 539, W: 600, H: 32},
		},
		{
			name: "a height one point taller takes them",
			req: geometry.Request{
				Width:  geometry.Absolute(600),
				Height: geometry.Absolute(33),
			},
			want: geometry.Rect{X: 676, Y: 554.5, W: 568, H: 1},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, wide, testCase.req)
			if got != testCase.want {
				t.Errorf("Resize(%s --margin) = %v, want %v", testCase.name, got, testCase.want)
			}
		})
	}
}

// TestResize_EdgeAdjacencyFollowsTheAnchorNotTheArithmetic covers issue #66: an
// edge that is flush against the visible frame by construction takes a full
// margin whichever anchor put it there, rather than only when the arithmetic
// happens to land on the boundary exactly.
//
// The display here is a 1512x982 primary with a second display directly above
// it, which is what makes the arithmetic inexact: the flip puts the upper
// display's top at -1080, and 10% of 982 is 98.2, so a bottom edge computed as
// y+h misses the visible frame's lower boundary by a few ulps. Under the old
// rule the top-anchored window took its full 8pt margin and the bottom-anchored
// one, flush against the same kind of boundary, took half of one.
//
// That scenario is deliberate. #66's own first repro — `fill` against a
// centered 100%x100% request — does not actually diverge on a display whose
// metrics are whole points: a 100% dimension comes back exactly, and centering
// a window as wide as the frame lands exactly on its edge. Only a fractional
// dimension separates the two rules, which is why this test reaches for one.
func TestResize_EdgeAdjacencyFollowsTheAnchorNotTheArithmetic(t *testing.T) {
	t.Parallel()

	// Both windows are one tenth of the visible frame tall and flush against
	// one of its horizontal boundaries, so both take a full 8pt margin there
	// and half of one on the opposite, internal edge.
	const requested = 98.2

	tests := []struct {
		name   string
		anchor geometry.Anchor
	}{
		{
			name:   "an edge assigned from the visible frame is flush",
			anchor: geometry.TopLeft,
		},
		{
			name:   "an edge arrived at by arithmetic is flush too",
			anchor: geometry.BottomLeft,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, displayAbove, geometry.Request{
				Height: geometry.Percent(10),
				Anchor: &testCase.anchor,
			})

			assertMargin(
				t,
				fmt.Sprintf("--height-percent 10 --anchor %s --margin", testCase.anchor),
				"height",
				requested-got.H,
				pinnedMargin,
			)
		})
	}
}

// displayAbove is a 1512x982 primary with a second display directly above it,
// with the system tiled-window margins on. Every dimension a percentage takes
// of its visible frame is fractional, and the flip puts the frame's top at a
// whole -1080, so the positions the anchors compute are inexact.
var displayAbove = geometry.Screen{
	Visible:        geometry.Rect{X: 0, Y: 1080, W: 1512, H: 982},
	PrimaryHeight:  982,
	MarginsEnabled: true,
	MarginSize:     8,
}

// What an 8pt margin takes across one axis of a window: a full margin on the
// end that lies against the visible frame's boundary plus half of one on the
// internal end, half of one on each of two internal ends, or a full margin on
// each end of a window that reaches the boundary on both.
const (
	pinnedMargin   = 8 + 4
	internalMargin = 4 + 4
	spanningMargin = 8 + 8
)

// assertMargin fails when a window gave up something other than want points of
// one dimension to its margins.
func assertMargin(t *testing.T, request, axis string, taken, want float64) {
	t.Helper()

	if math.Abs(taken-want) > tolerance {
		t.Errorf(
			"Resize(%s) gave up %v points of %s to margins, want %v",
			request,
			taken,
			axis,
			want,
		)
	}
}

// TestResize_MarginsAreTheSameAtWholeAndFractionalSizes covers issue #66 across
// all nine anchors: which edges take a full margin follows from the anchor
// alone, so the same anchor gives up the same margin whether the size it was
// asked for lands on whole points or not.
//
// None of these windows spans either axis, so each anchor takes a full 8pt
// margin on the two boundaries it pins itself against and half of one on the
// two internal edges — 12 points across an axis it pins, 8 across one it
// centers on.
func TestResize_MarginsAreTheSameAtWholeAndFractionalSizes(t *testing.T) {
	t.Parallel()

	// The recorded display with the system margins switched on. 45% of 1920 and
	// 20% of 1050 are whole points; the same shares of displayAbove's 1512x982
	// frame are not, and neither is any position an anchor computes from them.
	whole := singleDisplay
	whole.MarginsEnabled = true

	const (
		widthShare  = 45.0
		heightShare = 20.0
	)

	anchors := []struct {
		anchor     geometry.Anchor
		horizontal float64
		vertical   float64
	}{
		{anchor: geometry.TopLeft, horizontal: pinnedMargin, vertical: pinnedMargin},
		{anchor: geometry.TopCenter, horizontal: internalMargin, vertical: pinnedMargin},
		{anchor: geometry.TopRight, horizontal: pinnedMargin, vertical: pinnedMargin},
		{anchor: geometry.CenterLeft, horizontal: pinnedMargin, vertical: internalMargin},
		{anchor: geometry.Center, horizontal: internalMargin, vertical: internalMargin},
		{anchor: geometry.CenterRight, horizontal: pinnedMargin, vertical: internalMargin},
		{anchor: geometry.BottomLeft, horizontal: pinnedMargin, vertical: pinnedMargin},
		{anchor: geometry.BottomCenter, horizontal: internalMargin, vertical: pinnedMargin},
		{anchor: geometry.BottomRight, horizontal: pinnedMargin, vertical: pinnedMargin},
	}

	displays := []struct {
		name string
		scr  geometry.Screen
	}{
		{name: "whole", scr: whole},
		{name: "fractional", scr: displayAbove},
	}

	for _, display := range displays {
		for _, testCase := range anchors {
			t.Run(display.name+"/"+testCase.anchor.String(), func(t *testing.T) {
				t.Parallel()

				anchor := testCase.anchor
				req := geometry.Request{
					Width:  geometry.Percent(widthShare),
					Height: geometry.Percent(heightShare),
					Anchor: &anchor,
				}

				requestedW := display.scr.Visible.W * widthShare / 100
				requestedH := display.scr.Visible.H * heightShare / 100

				got := geometry.Resize(startFrame, display.scr, req)
				request := fmt.Sprintf("--anchor %s --margin", testCase.anchor)

				assertMargin(t, request, "width", requestedW-got.W, testCase.horizontal)
				assertMargin(t, request, "height", requestedH-got.H, testCase.vertical)
			})
		}
	}
}

// TestResize_AWindowAsLargeAsTheFrameIsFlushOnBothEdges covers the other half
// of the anchored rule from issue #66: the edge an anchor does not pin is flush
// too once the window is as long as the visible frame, so a window filling the
// frame takes a full margin on all four edges rather than only on the two its
// anchor pins.
//
// The length is compared to the frame's within the same half point an explicit
// position is, so a window a fraction short of the frame — which the arithmetic
// can leave one — still counts as filling it, and a window a whole point short
// does not.
func TestResize_AWindowAsLargeAsTheFrameIsFlushOnBothEdges(t *testing.T) {
	t.Parallel()

	// 1512 wide, on a display whose window-coordinate origin the flip puts at a
	// negative y. Every case fills the frame's height, and varies its width.
	frame := displayAbove.Visible

	tests := []struct {
		name       string
		width      geometry.Dimension
		requested  float64
		horizontal float64
	}{
		{
			name:       "a window filling the frame",
			width:      geometry.Percent(100),
			requested:  frame.W,
			horizontal: spanningMargin,
		},
		{
			name:       "a window a quarter point short of it",
			width:      geometry.Absolute(frame.W - 0.25),
			requested:  frame.W - 0.25,
			horizontal: spanningMargin,
		},
		{
			name:       "a window a whole point short of it",
			width:      geometry.Absolute(frame.W - 1),
			requested:  frame.W - 1,
			horizontal: pinnedMargin,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			anchor := geometry.TopLeft
			got := geometry.Resize(startFrame, displayAbove, geometry.Request{
				Width:  testCase.width,
				Height: geometry.Percent(100),
				Anchor: &anchor,
			})

			const request = "--height-percent 100 --anchor tl --margin"

			assertMargin(t, request, "width", testCase.requested-got.W, testCase.horizontal)
			assertMargin(t, request, "height", frame.H-got.H, spanningMargin)
		})
	}
}

// TestResize_ExplicitPositionMeasuresItsEdges covers the other half of issue
// #66: an explicit --x or --y is the one placement whose edges have to be
// measured, because the coordinate is the user's own and follows from no
// anchor. The comparison allows half a point, which is below what macOS can
// store a frame at, and nothing beyond it.
func TestResize_ExplicitPositionMeasuresItsEdges(t *testing.T) {
	t.Parallel()

	withMargins := singleDisplay
	withMargins.MarginsEnabled = true

	// The visible frame in window coordinates: 1080 - 0 - 1050 puts its top at
	// y = 30, and its left edge at x = 0.
	const (
		boundsX = 0.0
		boundsY = 30.0
	)

	// A bottom anchor, for the axes no explicit coordinate is given for: its
	// window is flush at the bottom and internal at the top, either way round.
	bottomLeft := geometry.BottomLeft

	tests := []struct {
		name       string
		posX       *float64
		posY       *float64
		horizontal float64
		vertical   float64
	}{
		{
			name:       "an origin on the boundary is flush",
			posX:       new(boundsX),
			posY:       new(boundsY),
			horizontal: pinnedMargin,
			vertical:   pinnedMargin,
		},
		{
			name:       "an origin within half a point of it still is",
			posX:       new(boundsX + 0.25),
			posY:       new(boundsY - 0.25),
			horizontal: pinnedMargin,
			vertical:   pinnedMargin,
		},
		{
			name:       "a whole point off the boundary is internal",
			posX:       new(boundsX + 1),
			posY:       new(boundsY + 1),
			horizontal: internalMargin,
			vertical:   internalMargin,
		},
		{
			name:       "an explicit x leaves the vertical edges to the anchor",
			posX:       new(boundsX + 1),
			horizontal: internalMargin,
			vertical:   pinnedMargin,
		},
		{
			name:       "an explicit y leaves the horizontal edges to the anchor",
			posY:       new(boundsY + 1),
			horizontal: pinnedMargin,
			vertical:   internalMargin,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.Resize(startFrame, withMargins, geometry.Request{
				X:      testCase.posX,
				Y:      testCase.posY,
				Width:  geometry.Absolute(800),
				Height: geometry.Absolute(600),
				Anchor: &bottomLeft,
			})

			const request = "--x/--y --anchor bl --margin"

			assertMargin(t, request, "width", 800-got.W, testCase.horizontal)
			assertMargin(t, request, "height", 600-got.H, testCase.vertical)
		})
	}
}

// tolerance is the slack the float comparisons in this file allow, well below
// anything a display can show.
const tolerance = 1e-9

// TestParsePreset covers mimi#125: a preset name is only a preset once
// ParsePreset has turned it into one, and an unknown name has no Preset to be
// turned into — which is what stops one reaching Resize to be ignored there.
func TestParsePreset(t *testing.T) {
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

			got, isPreset := geometry.ParsePreset(testCase.name)
			if isPreset != testCase.want {
				t.Fatalf(
					"ParsePreset(%q) ok = %v, want %v",
					testCase.name,
					isPreset,
					testCase.want,
				)
			}

			if isPreset && got.String() != testCase.name {
				t.Fatalf("ParsePreset(%q).String() = %q", testCase.name, got.String())
			}

			if !isPreset && got != (geometry.Preset{}) {
				t.Fatalf("ParsePreset(%q) = %v, want the zero preset", testCase.name, got)
			}
		})
	}
}

// TestPresetNames_ListsEveryPresetInTheDocumentedOrder pins both halves of the
// list an unknown name is rejected with: which names are valid, and the order
// the CLI's help and docs/CLI.md present them in, which the rejection message
// is built from and so is user-visible.
func TestPresetNames_ListsEveryPresetInTheDocumentedOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		presetLeftHalf, presetRightHalf, presetTopHalf, presetBottomHalf,
		presetTopLeft, presetTopRight, presetBottomLeft, presetBottomRight,
		presetCenter, presetFill,
	}

	if got := geometry.PresetNames(); !slices.Equal(got, want) {
		t.Fatalf("PresetNames() = %v, want %v", got, want)
	}
}
