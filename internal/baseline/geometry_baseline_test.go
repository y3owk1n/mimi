package baseline_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/baseline"
	"github.com/y3owk1n/mimi/internal/geometry"
)

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
			dir, ok := directionOf(focus.Direction)
			if !ok {
				t.Fatalf("the recording holds an unknown direction %q", focus.Direction)
			}

			frames := make([]*geometry.Rect, 0, len(focus.Arrangement))

			for _, rect := range focus.Arrangement {
				frames = append(frames, &geometry.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: rect.H})
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

// directionOf maps a recorded direction name onto the geometry direction.
func directionOf(name string) (geometry.Direction, bool) {
	switch name {
	case "up":
		return geometry.Up, true
	case "down":
		return geometry.Down, true
	case "left":
		return geometry.Left, true
	case "right":
		return geometry.Right, true
	default:
		return 0, false
	}
}
