package native_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/native"
)

func TestWindowAmong(t *testing.T) {
	t.Parallel()

	const self = 99

	full := native.Frame{W: 1000, H: 1000}
	windows := []native.ListedWindow{
		{Number: 50, PID: self, Alpha: 1, Frame: full},
		{Number: 51, PID: 20, Regular: true, Alpha: 0, Frame: full},
		{
			Number:  52,
			PID:     30,
			Layer:   101,
			Regular: true,
			Alpha:   1,
			Frame:   native.Frame{W: 200, H: 300},
		},
		{
			Number: 53,
			PID:    40,
			Layer:  8,
			Alpha:  1,
			Frame:  native.Frame{X: 500, Y: 500, W: 100, H: 100},
		},
		{Number: 1, PID: 10, Regular: true, Alpha: 1, Frame: full},
		{Number: 54, PID: 50, Layer: -2147483623, Alpha: 1, Frame: native.Frame{W: 2000, H: 2000}},
	}

	tests := []struct {
		name  string
		point native.Point
		want  uint32
		found bool
	}{
		{"menu covers the window", native.Point{X: 100, Y: 100}, 0, false},
		{"floating panel covers the window", native.Point{X: 550, Y: 550}, 0, false},
		{"own and transparent windows are not in the way", native.Point{X: 800, Y: 800}, 1, true},
		{"desktop alone is nothing", native.Point{X: 1500, Y: 1500}, 0, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, got, found := native.WindowAmong(windows, test.point, self)
			if got != test.want || found != test.found {
				t.Fatalf("got %d, %t; want %d, %t", got, found, test.want, test.found)
			}
		})
	}
}
