package native

/*
#include "animate.h"
#include <stdlib.h>
*/
import "C"

import (
	"time"
	"unsafe"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Easing is the curve a frame animation follows.
type Easing int

// The easing curves, in the order animate.h numbers them.
const (
	EasingLinear Easing = iota
	EasingEaseIn
	EasingEaseOut
	EasingEaseInOut
)

// FrameTarget is one window a frame animation moves, by number, and the
// frame it is being given, in window coordinates.
type FrameTarget struct {
	Number uint32
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// BeginFrameAnimation puts a still of the screen over the given windows and
// prepares to move pictures of them to their new frames, so the caller can
// write the real frames without anything moving on screen, then call
// StartFrameAnimation. It reports how many windows will animate. Without
// Screen Recording permission it prepares nothing and fails. It never asks
// for the permission.
func BeginFrameAnimation(
	targets []FrameTarget,
	duration time.Duration,
	easing Easing,
) (int, error) {
	if len(targets) == 0 {
		return 0, nil
	}

	cTargets := (*C.MimiAnimationTarget)(
		C.calloc(C.size_t(len(targets)), C.size_t(unsafe.Sizeof(C.MimiAnimationTarget{}))),
	)
	defer C.free(unsafe.Pointer(cTargets)) //nolint:nlreturn

	slice := unsafe.Slice(cTargets, len(targets))
	for index, target := range targets {
		slice[index] = C.MimiAnimationTarget{
			number: C.uint32_t(target.Number),
			x:      C.double(target.X),
			y:      C.double(target.Y),
			w:      C.double(target.Width),
			h:      C.double(target.Height),
		}
	}

	count := int(
		C.MimiAnimationBegin(
			cTargets,
			C.int(len(targets)),
			C.double(duration.Seconds()),
			C.int(easing),
		),
	)
	if count < 0 {
		return 0, derrors.New(
			derrors.CodeActionFailed,
			"screen recording permission is required to animate frames",
		)
	}

	return count, nil
}

// StartFrameAnimation runs the animation BeginFrameAnimation prepared,
// leaving out the windows whose frames did not land.
func StartFrameAnimation(dropped []uint32) {
	if len(dropped) == 0 {
		C.MimiAnimationStart(nil, 0)

		return
	}

	C.MimiAnimationStart((*C.uint32_t)(unsafe.Pointer(&dropped[0])), C.int(len(dropped)))
}

// ScreenCaptureGranted reports whether macOS lets mimi capture the screen,
// which animating frames needs. It never prompts.
func ScreenCaptureGranted() bool {
	return C.MimiScreenCaptureGranted() != 0
}
