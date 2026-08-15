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
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/baseline"
	"github.com/y3owk1n/mimi/internal/native"
	"github.com/y3owk1n/mimi/internal/permissions"
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

// The preset name the cases are built from.
const presetLeftHalf = "left-half"

// The two percentages the percentage cases ask for.
const (
	widthPercent  = 45.0
	heightPercent = 55.0
)

// resizeSpec is one resize_window invocation the baseline covers.
type resizeSpec struct {
	name string
	args action.ResizeWindowArgs
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

	fixture := newHarness(t, live, launchHelper(t, windowCount))

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

	visX, visY, visW, visH, err := native.ScreenVisibleFrame(0, 0)
	if err != nil {
		t.Skipf("cannot read the screen's visible frame: %v", err)
	}

	primaryH, err := native.PrimaryScreenHeight()
	if err != nil {
		t.Skipf("cannot read the primary screen height: %v", err)
	}

	return baseline.Display{
		Visible:        baseline.Rect{X: visX, Y: visY, W: visW, H: visH},
		PrimaryHeight:  primaryH,
		MarginsEnabled: native.TiledWindowMarginsEnabled(),
		MarginSize:     native.TiledWindowMarginSize(),
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
			Args:  baseline.ResizeArgs(spec.args),
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

			if want.Args != baseline.ResizeArgs(spec.args) {
				t.Fatalf(
					"the recording covers args %+v but the recorder ran %+v; re-record with %s=1",
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
					"resize_window %+v produced %s, baseline says %s",
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
func (h *harness) runResize(
	t *testing.T,
	args action.ResizeWindowArgs,
) (baseline.Rect, baseline.Rect) {
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

	resizeCmd, err := action.NewResizeWindowCommand(args)
	if err != nil {
		t.Fatalf("resize_window %+v is not a command: %v", args, err)
	}

	err = action.ExecuteCommand(resizeCmd)
	if err != nil {
		t.Fatalf("resize_window %+v failed: %v", args, err)
	}

	if !isFrontmost(target) {
		t.Fatalf(
			"focus moved off the recorder's own window while resize_window %+v ran",
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
			resizeSpec{
				name: "preset/" + preset + "/margin",
				args: action.ResizeWindowArgs{Preset: preset, UseMargin: true},
			},
			resizeSpec{
				name: "preset/" + preset + "/no-margin",
				args: action.ResizeWindowArgs{Preset: preset, NoMargin: true},
			},
		)
	}

	// The one case that exercises the system margin setting rather than a flag.
	specs = append(
		specs,
		resizeSpec{
			name: "preset/left-half/system-default",
			args: action.ResizeWindowArgs{Preset: presetLeftHalf},
		},
	)

	sized := action.ResizeWindowArgs{
		Width: fixedWidth, WidthSet: true,
		Height: fixedHeight, HeightSet: true,
	}

	for _, anchor := range anchors {
		anchored := sized
		anchored.Anchor, anchored.AnchorSet = anchor, true

		withMargin, withoutMargin := anchored, anchored
		withMargin.UseMargin = true
		withoutMargin.NoMargin = true

		specs = append(specs,
			resizeSpec{name: "anchor/" + anchor + "/margin", args: withMargin},
			resizeSpec{name: "anchor/" + anchor + "/no-margin", args: withoutMargin},
		)
	}

	posX := int(h.visible.X) + explicitOffsetX
	posY := int(h.top) + explicitOffsetY

	return append(specs,
		resizeSpec{
			name: "explicit/width-height",
			args: action.ResizeWindowArgs{
				Width: fixedWidth, WidthSet: true,
				Height: fixedHeight, HeightSet: true,
				NoMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/width-only",
			args: action.ResizeWindowArgs{Width: fixedWidth, WidthSet: true, NoMargin: true},
		},
		resizeSpec{
			name: "explicit/height-only",
			args: action.ResizeWindowArgs{Height: fixedHeight, HeightSet: true, NoMargin: true},
		},
		resizeSpec{
			name: "explicit/percent",
			args: action.ResizeWindowArgs{
				WidthPercent: widthPercent, WidthPercentSet: true,
				HeightPercent: heightPercent, HeightPercentSet: true,
				NoMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/percent-margin",
			args: action.ResizeWindowArgs{
				WidthPercent: widthPercent, WidthPercentSet: true,
				HeightPercent: heightPercent, HeightPercentSet: true,
				UseMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/position",
			args: action.ResizeWindowArgs{
				X: posX, XSet: true,
				Y: posY, YSet: true,
				NoMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/position-size",
			args: action.ResizeWindowArgs{
				X: posX, XSet: true,
				Y: posY, YSet: true,
				Width: fixedWidth, WidthSet: true,
				Height: fixedHeight, HeightSet: true,
				NoMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/position-size-margin",
			args: action.ResizeWindowArgs{
				X: posX, XSet: true,
				Y: posY, YSet: true,
				Width: fixedWidth, WidthSet: true,
				Height: fixedHeight, HeightSet: true,
				UseMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/preset-with-anchor",
			args: action.ResizeWindowArgs{
				Preset: presetLeftHalf,
				Anchor: "br", AnchorSet: true,
				NoMargin: true,
			},
		},
		resizeSpec{
			name: "explicit/preset-with-width",
			args: action.ResizeWindowArgs{
				Preset: presetLeftHalf,
				Width:  fixedWidth, WidthSet: true,
				NoMargin: true,
			},
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
