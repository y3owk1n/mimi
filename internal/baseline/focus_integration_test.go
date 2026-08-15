//go:build integration

package baseline_test

import (
	"slices"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/baseline"
	"github.com/y3owk1n/mimi/internal/native"
)

// focusDirections is every direction focus_window navigates in.
var focusDirections = []string{dirUp, dirDown, dirLeft, dirRight}

// spaceSnapshot is one reading of the active space, split into the windows the
// recorder created and the ones it did not.
type spaceSnapshot struct {
	// order holds indices into harness.windows in the order the action layer
	// enumerates them.
	order []int
	// frames holds the frame of each window in order.
	frames []baseline.Rect
	// foreign holds the readable frames of every other window on the space.
	foreign []baseline.Rect
	// focused indexes order, or -1 when the focused window is not one of the
	// recorder's own.
	focused int
}

// runFocusCases arranges the recorder's own windows in a grid and drives
// focus_window in each direction across it.
func (h *harness) runFocusCases(
	t *testing.T,
	recorded baseline.Recording,
	record bool,
) []baseline.FocusCase {
	t.Helper()

	observed := make([]baseline.FocusCase, 0, len(focusDirections))

	for _, dir := range focusDirections {
		t.Run(dir, func(t *testing.T) {
			got := h.runFocus(t, h.arrangeGrid(t, dir), dir)

			if record {
				observed = append(observed, got)

				return
			}

			want, ok := recorded.FocusCase(got.Direction)
			if !ok {
				t.Fatalf("no recorded baseline for this direction; re-record with %s=1", recordEnv)
			}

			if !slices.Equal(want.Arrangement, got.Arrangement) {
				t.Fatalf(
					"the windows settled at %v but the baseline was recorded from %v; re-record with %s=1",
					got.Arrangement,
					want.Arrangement,
					recordEnv,
				)
			}

			if got.Current != want.Current {
				t.Fatalf(
					"navigation started from window %d but the baseline started from %d",
					got.Current,
					want.Current,
				)
			}

			if got.Want != want.Want {
				t.Errorf(
					"focus_window --%s moved focus to window %d (%s), baseline says %d (%s)",
					dir,
					got.Want,
					got.Arrangement[got.Want],
					want.Want,
					want.Arrangement[want.Want],
				)
			}
		})
	}

	return observed
}

// runFocus drives one focus_window direction, skipping rather than running when
// a window the recorder did not create could be the one that gets focused.
func (h *harness) runFocus(t *testing.T, center *native.Element, dir string) baseline.FocusCase {
	t.Helper()

	bringToFront(t, center)

	before := h.snapshotSpace(t)
	if len(before.order) != windowCount {
		t.Skipf(
			"only %d of the recorder's %d windows are readable on the active space",
			len(before.order),
			windowCount,
		)
	}

	if before.focused < 0 {
		t.Skip(
			"the focused window is not one the recorder created; refusing to navigate away from somebody else's window",
		)
	}

	own, others := focusReach(dir, before.frames[before.focused], before.frames, before.foreign)
	if own >= others {
		t.Skipf(
			"one of the %d windows the recorder did not create could be focused %s from here (its reach %.0f is inside the recorder's own %.0f); refusing to touch it",
			len(before.foreign),
			dir,
			others,
			own,
		)
	}

	focusCmd, err := action.NewFocusWindowCommand(
		false,
		dir == dirUp,
		dir == dirDown,
		dir == dirLeft,
		dir == dirRight,
	)
	if err != nil {
		t.Fatalf("focus_window --%s is not a command: %v", dir, err)
	}

	err = action.ExecuteCommand(focusCmd)
	if err != nil {
		t.Fatalf("focus_window --%s failed: %v", dir, err)
	}

	after := h.waitForFocusChange(t, before, dir)
	if !slices.Equal(after.order, before.order) {
		t.Fatalf("the window arrangement changed while focus_window --%s ran", dir)
	}

	if after.focused < 0 {
		t.Fatalf("focus_window --%s left focus on a window the recorder did not create", dir)
	}

	return baseline.FocusCase{
		Direction:   dir,
		Arrangement: before.frames,
		Current:     before.focused,
		Want:        after.focused,
	}
}

// arrangeGrid parks the recorder's own windows in a 3x3 grid oriented for the
// given direction and returns the window in the middle, which the direction
// navigates away from.
func (h *harness) arrangeGrid(t *testing.T, dir string) *native.Element {
	t.Helper()

	stepX, stepY := gridSteps(dir)
	positions := make([]baseline.Rect, 0, windowCount)

	for row := range gridSide {
		for col := range gridSide {
			centerX := h.visible.X + gridOriginX + float64(col-1)*stepX
			centerY := h.top + gridOriginY + float64(row-1)*stepY

			positions = append(positions, baseline.Rect{
				X: centerX - gridWidth/2,
				Y: centerY - gridHeight/2,
				W: gridWidth,
				H: gridHeight,
			})
		}
	}

	// The middle cell of the grid.
	const middleIndex = gridSide * gridSide / 2

	if len(positions) != len(h.windows) {
		t.Fatalf(
			"the grid needs %d windows, the recorder opened %d",
			len(positions),
			len(h.windows),
		)
	}

	for index, target := range h.windows {
		placeWindow(t, target, positions[index])
	}

	return h.windows[middleIndex]
}

// gridSteps returns the step between neighboring window centers for a
// direction: the tight step along the direction of travel, the wide step
// across it.
func gridSteps(dir string) (float64, float64) {
	if dir == dirUp || dir == dirDown {
		return gridWide, gridTight
	}

	return gridTight, gridWide
}

// waitForFocusChange reads the active space until focus has moved off the
// window navigation started from. focus_window reports an error when it finds
// no window in the requested direction, so a successful call always moves it.
func (h *harness) waitForFocusChange(
	t *testing.T,
	before spaceSnapshot,
	dir string,
) spaceSnapshot {
	t.Helper()

	deadline := time.Now().Add(focusTimeout)

	for {
		after := h.snapshotSpace(t)
		if after.focused != before.focused {
			return after
		}

		if time.Now().After(deadline) {
			t.Fatalf("focus_window --%s reported success but focus did not move", dir)
		}

		time.Sleep(settleInterval)
	}
}

// snapshotSpace reads every focusable window on the active space, splitting the
// recorder's own windows from the rest. Windows whose frame cannot be read are
// dropped, exactly as the action layer drops them.
func (h *harness) snapshotSpace(t *testing.T) spaceSnapshot {
	t.Helper()

	all, focusedIndex := enumerateFocused(t)
	defer native.ReleaseAll(all)

	snap := spaceSnapshot{focused: -1}

	for index, element := range all {
		posX, posY, width, height, err := element.GetFrame()
		if err != nil {
			continue
		}

		rect := baseline.Rect{X: posX, Y: posY, W: width, H: height}

		owned := indexOfElement(h.windows, element)
		if owned < 0 {
			snap.foreign = append(snap.foreign, rect)

			continue
		}

		if index == focusedIndex {
			snap.focused = len(snap.order)
		}

		snap.order = append(snap.order, owned)
		snap.frames = append(snap.frames, rect)
	}

	return snap
}
