//go:build integration

// Recorder for the window baseline in window_baseline.json.
//
// It drives the real actions against real windows and records the frames macOS
// actually produces, so that the pure geometry these actions are being factored
// into can be checked against observed behavior rather than against a reading
// of the old code.
//
// Two rules shape the whole file. It only ever touches windows it opened
// itself — every step that would otherwise reach somebody else's window skips
// instead. And it degrades to a skip, never a failure, whenever the machine
// cannot support it: no Accessibility permission, a display the recording was
// not captured on, or a screen too small for the fixed arrangement.
//
// Re-record with:
//
//	MIMI_RECORD_BASELINE=1 go test -tags=integration -run TestWindowBaseline_ResizeAndFocus ./internal/baseline
package baseline_test

import (
	"math"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/baseline"
	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/window"
)

// recordEnv re-records the baseline instead of asserting against it.
const recordEnv = "MIMI_RECORD_BASELINE"

// Every geometry below is expressed relative to the visible frame, so the same
// arrangement can be re-recorded on a different display.
const (
	// The frame every resize case starts from.
	startOffsetX = 300
	startOffsetY = 150
	startWidth   = 700
	startHeight  = 500

	// The size the anchor and explicit-flag cases ask for.
	fixedWidth  = 800
	fixedHeight = 600

	// The position the explicit --x/--y cases ask for.
	explicitOffsetX = 100
	explicitOffsetY = 200

	// The 3x3 grid the focus cases navigate, given as the middle window's
	// center offset from the visible frame's top-left corner, the step between
	// neighboring centers, and the window size. The grid is rebuilt for each
	// direction, rotated so that the tight step runs along the direction of
	// travel.
	//
	// The tight step is a few points, which is what lets the test run at all on
	// a machine somebody is using: focus_window measures distance between
	// window centers, so the recorder's own windows have to be nearer to each
	// other than any window it did not create.
	//
	// The wide step exceeds the window size, so across the direction of travel
	// the diagonal neighbors do not overlap the current window while the
	// aligned one does. That is what makes the overlap bonus, rather than a
	// tie, decide each direction. None of this costs coverage: the scoring
	// compares distances and has no thresholds, so a grid four points across
	// exercises it exactly as one four hundred points across would.
	gridOriginX = 500
	gridOriginY = 420
	gridTight   = 4
	gridWide    = 300
	gridWidth   = 240
	gridHeight  = 160
	gridSide    = 3

	// The smallest visible frame every arrangement fits inside.
	minVisibleWidth  = 1000
	minVisibleHeight = 850
)

// The four spatial directions focus_window supports.
const (
	dirUp    = "up"
	dirDown  = "down"
	dirLeft  = "left"
	dirRight = "right"
)

// The resize_window flags and preset name the cases are built from.
const (
	flagWidth         = "--width"
	flagHeight        = "--height"
	flagWidthPercent  = "--width-percent"
	flagHeightPercent = "--height-percent"
	flagAnchor        = "--anchor"
	flagX             = "--x"
	flagY             = "--y"
	flagMargin        = "--margin"
	flagNoMargin      = "--no-margin"

	presetLeftHalf = "left-half"
)

// resizeSpec is one resize_window invocation the baseline covers.
type resizeSpec struct {
	name string
	args []string
}

func TestWindowBaseline_ResizeAndFocus(t *testing.T) {
	if !permissions.Check().Accessibility {
		t.Skip(
			"Accessibility permission is not granted; window baselines can neither be recorded nor replayed",
		)
	}

	recorded, err := baseline.Load()
	if err != nil {
		t.Fatalf("failed to load the recorded window baseline: %v", err)
	}

	live := liveDisplay(t)
	recording := recorded
	recording.Display = live
	record := os.Getenv(recordEnv) == "1"

	if !record && recorded.Display != live {
		t.Skipf(
			"the baseline was recorded on a different display (recorded %+v, live %+v); re-record with %s=1",
			recorded.Display,
			live,
			recordEnv,
		)
	}

	if live.Visible.W < minVisibleWidth || live.Visible.H < minVisibleHeight {
		t.Skipf(
			"the visible frame %s is smaller than the %dx%d the baseline arrangement needs",
			live.Visible,
			minVisibleWidth,
			minVisibleHeight,
		)
	}

	// The helper has to start before the first window enumeration; see
	// launchHelper. Everything above this line only reads screens and files.
	fixture := newHarness(t, live, launchHelper(t))

	var resizeCount, focusCount int

	t.Run("resize_window", func(t *testing.T) {
		observed := fixture.runResizeCases(t, recorded, record)
		recording.Resize = observed
		resizeCount = len(observed)
	})

	t.Run("focus_window", func(t *testing.T) {
		observed := fixture.runFocusCases(t, recorded, record)
		recording.Focus = observed
		focusCount = len(observed)
	})

	if !record {
		return
	}

	// A partial recording is worse than none: its display would describe frames
	// that were never captured against it. Write only a complete one.
	if resizeCount != len(fixture.resizeSpecs()) || focusCount != len(focusDirections) {
		t.Errorf(
			"recording captured %d of %d resize and %d of %d focus cases; %s was left unchanged",
			resizeCount,
			len(fixture.resizeSpecs()),
			focusCount,
			len(focusDirections),
			baseline.FileName,
		)

		return
	}

	writeRecording(t, recording)
}

// liveDisplay reads everything the action layer asks macOS about the screen.
func liveDisplay(t *testing.T) baseline.Display {
	t.Helper()

	visX, visY, visW, visH, err := window.ScreenVisibleFrame(0, 0)
	if err != nil {
		t.Skipf("cannot read the screen's visible frame: %v", err)
	}

	primaryH, err := window.PrimaryScreenHeight()
	if err != nil {
		t.Skipf("cannot read the primary screen height: %v", err)
	}

	return baseline.Display{
		Visible:        baseline.Rect{X: visX, Y: visY, W: visW, H: visH},
		PrimaryHeight:  primaryH,
		MarginsEnabled: window.TiledWindowMarginsEnabled(),
		MarginSize:     window.TiledWindowMarginSize(),
	}
}

// runResizeCases drives every resize_window case and either records the frames
// macOS produced or asserts them against the recording.
func (h *harness) runResizeCases(
	t *testing.T,
	recorded baseline.Recording,
	record bool,
) []baseline.ResizeCase {
	t.Helper()

	specs := h.resizeSpecs()
	observed := make([]baseline.ResizeCase, 0, len(specs))

	if !record && len(recorded.Resize) != len(specs) {
		t.Errorf(
			"the recording holds %d resize cases but the recorder covers %d; re-record with %s=1",
			len(recorded.Resize),
			len(specs),
			recordEnv,
		)
	}

	for _, spec := range specs {
		start, got := h.runResize(t, spec.args)
		observed = append(observed, baseline.ResizeCase{
			Name:  spec.name,
			Args:  spec.args,
			Start: start,
			Want:  got,
		})

		if record {
			continue
		}

		t.Run(spec.name, func(t *testing.T) {
			want, ok := recorded.ResizeCase(spec.name)
			if !ok {
				t.Fatalf("no recorded baseline for this case; re-record with %s=1", recordEnv)
			}

			if !slices.Equal(want.Args, spec.args) {
				t.Fatalf(
					"the recording covers args %v but the recorder ran %v; re-record with %s=1",
					want.Args,
					spec.args,
					recordEnv,
				)
			}

			if start != want.Start {
				t.Fatalf(
					"the window started at %s but the baseline was recorded from %s; re-record with %s=1",
					start,
					want.Start,
					recordEnv,
				)
			}

			if got != want.Want {
				t.Errorf(
					"resize_window %v produced %s, baseline says %s",
					spec.args,
					got,
					want.Want,
				)
			}
		})
	}

	return observed
}

// runResize parks the recorder's own window at the shared start frame, runs one
// resize_window invocation against it and reports the frames either side.
func (h *harness) runResize(t *testing.T, args []string) (baseline.Rect, baseline.Rect) {
	t.Helper()

	target := h.windows[0]

	start := placeWindow(t, target, baseline.Rect{
		X: h.visible.X + startOffsetX,
		Y: h.top + startOffsetY,
		W: startWidth,
		H: startHeight,
	})

	// resize_window resolves the frontmost window itself, so the recorder's own
	// window has to be frontmost before it runs. That check narrows the window
	// in which the action could reach somebody else's window, but the action
	// re-reads the frontmost window and nothing can close the gap entirely, so
	// the check is repeated afterwards to turn a focus steal into a failure
	// rather than a silently wrong recording.
	bringToFront(t, target)

	err := action.Execute(string(action.NameResizeWindow), args)
	if err != nil {
		t.Fatalf("resize_window %v failed: %v", args, err)
	}

	if !isFrontmost(target) {
		t.Fatalf(
			"focus moved off the recorder's own window while resize_window %v ran",
			args,
		)
	}

	return start, settledFrame(t, target)
}

// resizeSpecs enumerates every resize_window invocation the baseline covers:
// all ten presets in both margin states, all nine anchors in both margin
// states, and the explicit-flag combinations.
func (h *harness) resizeSpecs() []resizeSpec {
	presets := []string{
		presetLeftHalf, "right-half", "top-half", "bottom-half",
		"top-left", "top-right", "bottom-left", "bottom-right",
		"center", "fill",
	}
	anchors := []string{"tl", "tc", "tr", "cl", "cc", "cr", "bl", "bc", "br"}

	// The system-default preset case plus the ten explicit-flag cases.
	const extraCases = 11

	specs := make([]resizeSpec, 0, len(presets)*2+len(anchors)*2+extraCases)

	for _, preset := range presets {
		specs = append(specs,
			resizeSpec{name: "preset/" + preset + "/margin", args: []string{preset, flagMargin}},
			resizeSpec{
				name: "preset/" + preset + "/no-margin",
				args: []string{preset, flagNoMargin},
			},
		)
	}

	// The one case that exercises the system margin setting rather than a flag.
	specs = append(
		specs,
		resizeSpec{name: "preset/left-half/system-default", args: []string{presetLeftHalf}},
	)

	width := strconv.Itoa(fixedWidth)
	height := strconv.Itoa(fixedHeight)

	for _, anchor := range anchors {
		sized := []string{flagWidth, width, flagHeight, height, flagAnchor, anchor}
		specs = append(specs,
			resizeSpec{
				name: "anchor/" + anchor + "/margin",
				args: append(slices.Clone(sized), flagMargin),
			},
			resizeSpec{
				name: "anchor/" + anchor + "/no-margin",
				args: append(slices.Clone(sized), flagNoMargin),
			},
		)
	}

	posX := strconv.Itoa(int(h.visible.X) + explicitOffsetX)
	posY := strconv.Itoa(int(h.top) + explicitOffsetY)

	return append(specs,
		resizeSpec{
			name: "explicit/width-height",
			args: []string{flagWidth, width, flagHeight, height, flagNoMargin},
		},
		resizeSpec{
			name: "explicit/width-only",
			args: []string{flagWidth, width, flagNoMargin},
		},
		resizeSpec{
			name: "explicit/height-only",
			args: []string{flagHeight, height, flagNoMargin},
		},
		resizeSpec{
			name: "explicit/percent",
			args: []string{flagWidthPercent, "45", flagHeightPercent, "55", flagNoMargin},
		},
		resizeSpec{
			name: "explicit/percent-margin",
			args: []string{flagWidthPercent, "45", flagHeightPercent, "55", flagMargin},
		},
		resizeSpec{
			name: "explicit/position",
			args: []string{flagX, posX, flagY, posY, flagNoMargin},
		},
		resizeSpec{
			name: "explicit/position-size",
			args: []string{
				flagX, posX, flagY, posY,
				flagWidth, width, flagHeight, height, flagNoMargin,
			},
		},
		resizeSpec{
			name: "explicit/position-size-margin",
			args: []string{
				flagX, posX, flagY, posY,
				flagWidth, width, flagHeight, height, flagMargin,
			},
		},
		resizeSpec{
			name: "explicit/preset-with-anchor",
			args: []string{presetLeftHalf, flagAnchor, "br", flagNoMargin},
		},
		resizeSpec{
			name: "explicit/preset-with-width",
			args: []string{presetLeftHalf, flagWidth, width, flagNoMargin},
		},
	)
}

// writeRecording replaces the recording file next to this test.
func writeRecording(t *testing.T, recording baseline.Recording) {
	t.Helper()

	data, err := baseline.Encode(recording)
	if err != nil {
		t.Fatalf("failed to encode the window baseline: %v", err)
	}

	err = os.WriteFile(baseline.FileName, data, 0o644) //nolint:gosec // a tracked repo file
	if err != nil {
		t.Fatalf("failed to write %s: %v", baseline.FileName, err)
	}

	t.Logf(
		"recorded %d resize and %d focus baselines into %s",
		len(recording.Resize),
		len(recording.Focus),
		baseline.FileName,
	)
}

// candidate is how the action layer sees one window when navigating in a
// direction: the distance along the requested axis, the distance across it,
// and whether the two windows overlap on that other axis.
type candidate struct {
	primary   float64
	secondary float64
	overlaps  bool
}

// measure describes a candidate window relative to the current one, and reports
// whether it lies in the requested direction at all.
//
// It mirrors the direction filter, the two distances, and the overlap test the
// action layer applies — never which candidate wins, which is the fact the
// baseline records.
func measure(dir string, cur, cand baseline.Rect) (candidate, bool) {
	deltaX := (cand.X + cand.W/2) - (cur.X + cur.W/2)
	deltaY := (cand.Y + cand.H/2) - (cur.Y + cur.H/2)
	acrossX := cur.X < cand.X+cand.W && cur.X+cur.W > cand.X
	acrossY := cur.Y < cand.Y+cand.H && cur.Y+cur.H > cand.Y

	switch dir {
	case dirLeft:
		if deltaX >= 0 {
			return candidate{}, false
		}

		return candidate{primary: -deltaX, secondary: math.Abs(deltaY), overlaps: acrossY}, true
	case dirRight:
		if deltaX <= 0 {
			return candidate{}, false
		}

		return candidate{primary: deltaX, secondary: math.Abs(deltaY), overlaps: acrossY}, true
	case dirUp:
		if deltaY >= 0 {
			return candidate{}, false
		}

		return candidate{primary: -deltaY, secondary: math.Abs(deltaX), overlaps: acrossX}, true
	case dirDown:
		if deltaY <= 0 {
			return candidate{}, false
		}

		return candidate{primary: deltaY, secondary: math.Abs(deltaX), overlaps: acrossX}, true
	}

	return candidate{}, false
}

// focusReach bounds the directional score on both sides: the worst score the
// recorder's own windows can produce, and the best score any other window on
// the space can produce. When the first is strictly smaller than the second,
// focus_window cannot reach a window the recorder did not create.
//
// The action layer scores a candidate at the primary distance, plus the
// secondary distance when the two windows do not overlap on the perpendicular
// axis. The recorder's side ignores the overlap and takes the sum, which is an
// upper bound however the overlap turns out; the other side applies the overlap
// test, because a window across the screen at the same height is otherwise
// indistinguishable from one directly below and the guard would never let the
// test run.
//
// Neither side ever picks a winner among the recorder's own windows — that is
// the fact the baseline records, and nothing here reproduces it.
//
// Either bound is math.MaxFloat64 when nothing lies in that direction.
func focusReach(dir string, cur baseline.Rect, mine, foreign []baseline.Rect) (float64, float64) {
	own := math.MaxFloat64

	for _, rect := range mine {
		measured, ok := measure(dir, cur, rect)
		if ok && measured.primary+measured.secondary < own {
			own = measured.primary + measured.secondary
		}
	}

	others := math.MaxFloat64

	for _, rect := range foreign {
		measured, ok := measure(dir, cur, rect)
		if !ok {
			continue
		}

		score := measured.primary
		if !measured.overlaps {
			score += measured.secondary
		}

		if score < others {
			others = score
		}
	}

	return own, others
}
