package geometry_test

import (
	"slices"
	"testing"

	"github.com/y3owk1n/mimi/internal/geometry"
)

func cycling(name string) geometry.Request {
	return geometry.Request{Preset: presetFor(name), Cycle: true}
}

// TestResize_CycleStepsAHalfThroughItsThirds pins the sequence a repeated
// press walks: half, two thirds, a third, and back to the half, on either
// side. A window at none of those frames starts the sequence at the half.
func TestResize_CycleStepsAHalfThroughItsThirds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		half  string
		steps []geometry.Rect
	}{
		{
			name: presetLeftHalf,
			half: presetLeftHalf,
			steps: []geometry.Rect{
				{X: 0, Y: 30, W: 960, H: 1050},
				{X: 0, Y: 30, W: 1280, H: 1050},
				{X: 0, Y: 30, W: 640, H: 1050},
				{X: 0, Y: 30, W: 960, H: 1050},
			},
		},
		{
			name: presetRightHalf,
			half: presetRightHalf,
			steps: []geometry.Rect{
				{X: 960, Y: 30, W: 960, H: 1050},
				{X: 640, Y: 30, W: 1280, H: 1050},
				{X: 1280, Y: 30, W: 640, H: 1050},
				{X: 960, Y: 30, W: 960, H: 1050},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			frame := startFrame
			for index, want := range testCase.steps {
				frame = geometry.Resize(frame, singleDisplay, cycling(testCase.half))
				if frame != want {
					t.Fatalf(
						"press %d: Resize(%s --cycle) = %v, want %v",
						index+1,
						testCase.half,
						frame,
						want,
					)
				}
			}
		})
	}
}

// TestResize_CycleRecognisesAFrameMacOSRounded: the frame macOS stores is in
// whole points, so a third of 1470 (490) matches exactly while a third of
// 2560 (853.33) comes back as 853. Either has to read as "already at the
// third" for the cycle to step on rather than start over.
func TestResize_CycleRecognisesAFrameMacOSRounded(t *testing.T) {
	t.Parallel()

	wide := geometry.Screen{
		Visible:       geometry.Rect{X: 0, Y: 0, W: 2560, H: 1415},
		PrimaryHeight: 1440,
	}
	atTheThirdAsStored := geometry.Rect{X: 0, Y: 25, W: 853, H: 1415}

	got := geometry.Resize(atTheThirdAsStored, wide, cycling(presetLeftHalf))

	want := geometry.Rect{X: 0, Y: 25, W: 1280, H: 1415}
	if got != want {
		t.Fatalf(
			"Resize(left-half --cycle) from the stored third = %v, want the half %v",
			got,
			want,
		)
	}
}

// TestResize_CycleHonorsMargins: each step is measured with the same margins
// it is placed with, so a cycle with margins on still steps rather than
// restarting because the inset frame never matched an uninset one.
func TestResize_CycleHonorsMargins(t *testing.T) {
	t.Parallel()

	withMargins := singleDisplay
	withMargins.MarginsEnabled = true

	first := geometry.Resize(startFrame, withMargins, cycling(presetLeftHalf))
	second := geometry.Resize(first, withMargins, cycling(presetLeftHalf))

	// The half is flush left and top and bottom: full margins there, half a
	// margin on the shared right edge.
	wantFirst := geometry.Rect{X: 8, Y: 38, W: 948, H: 1034}
	if first != wantFirst {
		t.Fatalf("first press = %v, want %v", first, wantFirst)
	}

	if second.W <= first.W {
		t.Fatalf("second press = %v, want wider than the half %v", second, first)
	}
}

func TestResize_CycleIsIgnoredByAPresetThatDoesNotCycle(t *testing.T) {
	t.Parallel()

	got := geometry.Resize(startFrame, singleDisplay, cycling(presetFill))
	want := geometry.Resize(startFrame, singleDisplay, geometry.Request{Preset: fill})

	if got != want {
		t.Fatalf("Resize(fill --cycle) = %v, want fill's own frame %v", got, want)
	}
}

func TestCyclingPresetNames_AreTheTwoHalves(t *testing.T) {
	t.Parallel()

	want := []string{presetLeftHalf, presetRightHalf}
	if got := geometry.CyclingPresetNames(); !slices.Equal(got, want) {
		t.Fatalf("CyclingPresetNames() = %v, want %v", got, want)
	}

	if !presetFor(presetLeftHalf).Cycles() || presetFor(presetFill).Cycles() {
		t.Fatal("Cycles() disagrees with CyclingPresetNames()")
	}
}
