package action //nolint:testpackage // tests the native frame lookup seam

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/geometry"
	"github.com/y3owk1n/mimi/internal/native"
)

func TestReadWindowFrameUsesCurrentServerFrame(t *testing.T) {
	t.Parallel()

	current := native.Frame{X: 3840, Y: 30, W: 976, H: 876}
	desktop := newNativeDesktop()
	desktop.listed = map[uint32]native.ListedWindow{42: {Number: 42, Frame: current}}
	server := func(number uint32) (native.Frame, bool) {
		if number != 42 {
			t.Fatalf("window number = %d, want 42", number)
		}

		return current, true
	}
	axFrame := func() (float64, float64, float64, float64, error) {
		t.Fatal("AX fallback used for a listed window")

		return 0, 0, 0, 0, nil
	}

	_, err := desktop.readWindowFrame(1, 42, server, axFrame)
	if err != nil {
		t.Fatal(err)
	}

	current = native.Frame{X: 2700, Y: 30, W: 1000, H: 900}

	got, err := desktop.readWindowFrame(1, 42, server, axFrame)
	if err != nil {
		t.Fatal(err)
	}

	if want := (geometry.Rect{X: 2700, Y: 30, W: 1000, H: 900}); got != want {
		t.Fatalf("frame = %+v, want current server frame %+v", got, want)
	}
}

func TestReadWindowFrameFallsBackToAX(t *testing.T) {
	t.Parallel()

	server := func(uint32) (native.Frame, bool) { return native.Frame{}, false }
	ax := func() (float64, float64, float64, float64, error) { return 51, 30, 1254, 1410, nil }

	got, err := newNativeDesktop().readWindowFrame(1, 42, server, ax)
	if err != nil {
		t.Fatal(err)
	}

	if want := (geometry.Rect{X: 51, Y: 30, W: 1254, H: 1410}); got != want {
		t.Fatalf("frame = %+v, want AX frame %+v", got, want)
	}
}
