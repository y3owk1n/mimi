package geometry_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/geometry"
)

func TestNearest_BreaksTiesOnTopThenLeftThenIndex(t *testing.T) {
	current := &geometry.Rect{X: 0, Y: 0, W: 100, H: 100}

	tests := []struct {
		name   string
		frames []*geometry.Rect
		dir    geometry.Direction
		want   int
	}{
		{
			// Both score 200; the higher one wins even though it comes last.
			name: "equal scores resolve to the topmost window",
			frames: []*geometry.Rect{
				current,
				{X: 200, Y: 50, W: 100, H: 100},
				{X: 200, Y: -50, W: 100, H: 100},
			},
			dir:  geometry.Right,
			want: 2,
		},
		{
			// Both score 300 and share a top edge; the leftmost one wins.
			name: "equal scores at the same top resolve to the leftmost window",
			frames: []*geometry.Rect{
				current,
				{X: 100, Y: 200, W: 100, H: 100},
				{X: -100, Y: 200, W: 100, H: 100},
			},
			dir:  geometry.Down,
			want: 2,
		},
		{
			// Identical windows: the lowest index wins.
			name: "identical windows resolve to the lowest index",
			frames: []*geometry.Rect{
				current,
				{X: 200, Y: 0, W: 100, H: 100},
				{X: 200, Y: 0, W: 100, H: 100},
			},
			dir:  geometry.Right,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := geometry.Nearest(tt.frames, 0, tt.dir)
			if !ok || got != tt.want {
				t.Errorf("Nearest() = %d, %v; want %d, true", got, ok, tt.want)
			}
		})
	}
}

func TestNearest_PrefersAnOverlappingWindowOverACloserOffsetOne(t *testing.T) {
	// In every row the offset window is the closer one along the direction of
	// travel, but it does not overlap the current window on the perpendicular
	// axis, so it pays its 300 points of sideways distance and loses to the
	// aligned window 200 points away.
	tests := []struct {
		name    string
		dir     geometry.Direction
		offset  *geometry.Rect
		aligned *geometry.Rect
	}{
		{
			name:    "up",
			dir:     geometry.Up,
			offset:  &geometry.Rect{X: 300, Y: -10, W: 100, H: 100},
			aligned: &geometry.Rect{X: 0, Y: -200, W: 100, H: 100},
		},
		{
			name:    "down",
			dir:     geometry.Down,
			offset:  &geometry.Rect{X: 300, Y: 110, W: 100, H: 100},
			aligned: &geometry.Rect{X: 0, Y: 200, W: 100, H: 100},
		},
		{
			name:    "left",
			dir:     geometry.Left,
			offset:  &geometry.Rect{X: -10, Y: 300, W: 100, H: 100},
			aligned: &geometry.Rect{X: -200, Y: 0, W: 100, H: 100},
		},
		{
			name:    "right",
			dir:     geometry.Right,
			offset:  &geometry.Rect{X: 110, Y: 300, W: 100, H: 100},
			aligned: &geometry.Rect{X: 200, Y: 0, W: 100, H: 100},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			current := &geometry.Rect{X: 0, Y: 0, W: 100, H: 100}
			frames := []*geometry.Rect{testCase.offset, testCase.aligned, current}

			got, ok := geometry.Nearest(frames, 2, testCase.dir)
			if !ok || got != 1 {
				t.Errorf("Nearest(%s) = %d, %v; want 1, true", testCase.name, got, ok)
			}
		})
	}
}

// grid is a 3x3 arrangement of 100x100 windows 200 points apart, in the (y, x)
// order the native layer enumerates windows in. Index 4 is the middle cell.
func grid() []*geometry.Rect {
	frames := make([]*geometry.Rect, 0, 9)

	for row := range 3 {
		for col := range 3 {
			frames = append(frames, &geometry.Rect{
				X: float64(col) * 200,
				Y: float64(row) * 200,
				W: 100,
				H: 100,
			})
		}
	}

	return frames
}

func TestNearest_PicksTheNeighborInTheRequestedDirection(t *testing.T) {
	tests := []struct {
		name string
		dir  geometry.Direction
		want int
	}{
		{"up", geometry.Up, 1},
		{"down", geometry.Down, 7},
		{"left", geometry.Left, 3},
		{"right", geometry.Right, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := geometry.Nearest(grid(), 4, tt.dir)
			if !ok || got != tt.want {
				t.Errorf("Nearest(%s) = %d, %v; want %d, true", tt.name, got, ok, tt.want)
			}
		})
	}
}

func TestNearest_ReportsNotFoundWhenNothingLiesInTheDirection(t *testing.T) {
	tests := []struct {
		name    string
		current int
		dir     geometry.Direction
	}{
		{"nothing above the top row", 1, geometry.Up},
		{"nothing below the bottom row", 7, geometry.Down},
		{"nothing left of the first column", 3, geometry.Left},
		{"nothing right of the last column", 5, geometry.Right},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := geometry.Nearest(grid(), tt.current, tt.dir)
			if ok {
				t.Errorf("Nearest() = %d, true; want not found", got)
			}
		})
	}
}

func TestNearest_SkipsUnreadableFrames(t *testing.T) {
	frames := grid()
	// The window directly above the middle cell cannot be read, so focus
	// falls to the next best one in that direction rather than shifting
	// every index below it.
	frames[1] = nil

	got, ok := geometry.Nearest(frames, 4, geometry.Up)
	if !ok || got != 0 {
		t.Errorf("Nearest(up) = %d, %v; want 0, true", got, ok)
	}
}

func TestNearest_ReportsNotFoundWhenTheCurrentFrameIsUnusable(t *testing.T) {
	frames := grid()
	frames[4] = nil

	tests := []struct {
		name    string
		frames  []*geometry.Rect
		current int
	}{
		{"the current frame could not be read", frames, 4},
		{"there is no current window", grid(), -1},
		{"the current index is past the end", grid(), 9},
		{"there are no windows at all", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := geometry.Nearest(tt.frames, tt.current, geometry.Right)
			if ok {
				t.Errorf("Nearest() = %d, true; want not found", got)
			}
		})
	}
}
