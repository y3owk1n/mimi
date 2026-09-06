package geometry_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/geometry"
)

const displayPrimaryHeight = 1080.0

var (
	// leftVisible is a 1920x1080 primary display less a 25-point menu bar.
	leftVisible = geometry.Rect{X: 0, Y: 0, W: 1920, H: 1055}
	// rightVisible is a 2560x1440 display to its right, bottoms aligned, less
	// the same menu bar.
	rightVisible = geometry.Rect{X: 1920, Y: 0, W: 2560, H: 1415}
)

func TestMoveToScreen_KeepsTheShareOfTheVisibleFrame(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cur  geometry.Rect
		want geometry.Rect
	}{
		{
			name: "left half stays a left half",
			cur:  geometry.Rect{X: 0, Y: 25, W: 960, H: 1055},
			want: geometry.Rect{X: 1920, Y: -335, W: 1280, H: 1415},
		},
		{
			// The shares here do not divide evenly, so this is also the case
			// that pins the frame coming back in whole points.
			name: "a centered window stays centered, in whole points",
			cur:  geometry.Rect{X: 480, Y: 289, W: 960, H: 527},
			want: geometry.Rect{X: 2560, Y: 19, W: 1280, H: 707},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := geometry.MoveToScreen(
				testCase.cur,
				displayPrimaryHeight,
				leftVisible,
				rightVisible,
			)
			if got != testCase.want {
				t.Fatalf("MoveToScreen() = %+v, want %+v", got, testCase.want)
			}

			// The trip back lands where it started.
			back := geometry.MoveToScreen(got, displayPrimaryHeight, rightVisible, leftVisible)
			if back != testCase.cur {
				t.Fatalf("MoveToScreen() back = %+v, want %+v", back, testCase.cur)
			}
		})
	}
}

func TestScreenContaining_DecidesByTheWindowsCenter(t *testing.T) {
	t.Parallel()

	frames := []geometry.Rect{
		{X: 0, Y: 0, W: 1920, H: 1080},
		{X: 1920, Y: 0, W: 2560, H: 1440},
	}

	cases := []struct {
		name      string
		cur       geometry.Rect
		wantIndex int
		wantFound bool
	}{
		{
			name:      "on the first",
			cur:       geometry.Rect{X: 100, Y: 100, W: 500, H: 500},
			wantIndex: 0,
			wantFound: true,
		},
		{
			name:      "on the second",
			cur:       geometry.Rect{X: 2000, Y: -300, W: 500, H: 500},
			wantIndex: 1,
			wantFound: true,
		},
		{
			name:      "straddling, mostly on the second",
			cur:       geometry.Rect{X: 1700, Y: 100, W: 600, H: 500},
			wantIndex: 1,
			wantFound: true,
		},
		{
			name:      "off every display",
			cur:       geometry.Rect{X: -900, Y: 100, W: 500, H: 500},
			wantFound: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotIndex, gotFound := geometry.ScreenContaining(
				testCase.cur,
				displayPrimaryHeight,
				frames,
			)
			if gotFound != testCase.wantFound || (gotFound && gotIndex != testCase.wantIndex) {
				t.Fatalf(
					"ScreenContaining() = %d, %v, want %d, %v",
					gotIndex, gotFound, testCase.wantIndex, testCase.wantFound,
				)
			}
		})
	}
}

func TestSameFrame_ToleratesHalfAPoint(t *testing.T) {
	t.Parallel()

	base := geometry.Rect{X: 10, Y: 20, W: 300, H: 400}

	if !geometry.SameFrame(base, geometry.Rect{X: 10.4, Y: 19.6, W: 300.4, H: 400}) {
		t.Fatal("SameFrame() = false for frames under half a point apart, want true")
	}

	if geometry.SameFrame(base, geometry.Rect{X: 10, Y: 20, W: 299, H: 400}) {
		t.Fatal("SameFrame() = true for frames a point apart, want false")
	}
}
