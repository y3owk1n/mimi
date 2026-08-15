package baseline_test

import (
	"math"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/baseline"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// quantum is how far a computed frame may sit from the recorded one.
//
// macOS stores window frames in whole points: every frame in the recording was
// read back after the Accessibility API had truncated the one it was handed.
// Forty-seven of the forty-nine recorded cases are whole points already and
// match exactly; the two percentage ones are fractional, and the truncation
// moves them by less than a point. Nothing the geometry itself could get wrong
// is that small — the margin rules move an edge by four points, the anchors
// and dimensions by hundreds.
const quantum = 1.0

// TestResize_ReproducesTheRecordedFrames replays every recorded resize_window
// invocation through the argument parsing and the pure geometry, and checks it
// arrives at the frame macOS actually produced. This is the equivalence oracle
// for the extraction: it lives here, next to the recording, so that
// internal/geometry stays free of every import outside the standard library.
func TestResize_ReproducesTheRecordedFrames(t *testing.T) {
	recording, err := baseline.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(recording.Resize) == 0 {
		t.Fatal("the recording holds no resize cases")
	}

	screen := recording.Display.Screen()

	for _, resize := range recording.Resize {
		t.Run(resize.Name, func(t *testing.T) {
			req, err := action.ResizeRequestFromArgs(action.ResizeWindowArgs(resize.Args))
			if err != nil {
				t.Fatalf("ResizeRequestFromArgs(%+v) error = %v", resize.Args, err)
			}

			got := baseline.Rect(geometry.Resize(resize.Start.Geometry(), screen, req))
			if !within(got, resize.Want) {
				t.Errorf(
					"resize_window %v computed %s, macOS produced %s",
					resize.Args,
					got,
					resize.Want,
				)
			}
		})
	}
}

// within reports whether two frames agree to within the point macOS quantizes
// a frame to.
func within(got, want baseline.Rect) bool {
	return math.Abs(got.X-want.X) < quantum &&
		math.Abs(got.Y-want.Y) < quantum &&
		math.Abs(got.W-want.W) < quantum &&
		math.Abs(got.H-want.H) < quantum
}

// TestNearest_ReproducesTheRecordedFocusTargets replays every recorded
// focus_window arrangement through the pure geometry and checks it picks the
// window macOS actually focused. This is the equivalence oracle for the
// extraction: it lives here, next to the recording, so internal/geometry stays
// free of every import outside the standard library.
func TestNearest_ReproducesTheRecordedFocusTargets(t *testing.T) {
	recording, err := baseline.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(recording.Focus) == 0 {
		t.Fatal("the recording holds no focus cases")
	}

	for _, focus := range recording.Focus {
		t.Run(focus.Direction, func(t *testing.T) {
			dir, ok := geometry.ParseDirection(focus.Direction)
			if !ok {
				t.Fatalf("the recording holds an unknown direction %q", focus.Direction)
			}

			frames := make([]*geometry.Rect, 0, len(focus.Arrangement))

			for _, rect := range focus.Arrangement {
				frame := rect.Geometry()
				frames = append(frames, &frame)
			}

			got, found := geometry.Nearest(frames, focus.Current, dir)
			if !found {
				t.Fatalf(
					"Nearest(%s) found no window, macOS focused %d (%s)",
					focus.Direction,
					focus.Want,
					focus.Arrangement[focus.Want],
				)
			}

			if got != focus.Want {
				t.Errorf(
					"Nearest(%s) = %d (%s), macOS focused %d (%s)",
					focus.Direction,
					got,
					focus.Arrangement[got],
					focus.Want,
					focus.Arrangement[focus.Want],
				)
			}
		})
	}
}
