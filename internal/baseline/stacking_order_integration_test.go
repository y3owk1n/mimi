//go:build integration

// The stacking order the windows query reports, checked against the live
// window server.
//
// The query lists windows by position, top to bottom and then left to right,
// because that is the order focus cycles through them. The window server lists
// them front to back instead. Both orders are useful and they are rarely the
// same, so the query carries the second one as each window's order while
// listing in the first. This checks that what comes back is a real stacking
// order rather than the positional one under another name.
//
// It only reads. It opens no window, moves nothing, and skips rather than
// fails on a machine that cannot answer.

package baseline_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/permissions"
)

func TestStackingOrder_NamesOneWindowInFrontAndRanksTheRest(t *testing.T) {
	if !permissions.Check().Accessibility {
		t.Skip("Accessibility permission is not granted; windows cannot be enumerated")
	}

	info, err := action.QueryWindows()
	if err != nil {
		t.Skipf("the windows could not be listed: %v", err)
	}

	if len(info.Windows) < 2 {
		t.Skip("fewer than two windows are open, so there is no stacking order to check")
	}

	// Every window has a place in the stack, and no two share one.
	seen := make(map[int]uint32, len(info.Windows))

	for _, win := range info.Windows {
		if win.Order < 0 || win.Order >= len(info.Windows) {
			t.Fatalf(
				"window %d reported stacking order %d, outside the %d windows listed",
				win.Number, win.Order, len(info.Windows),
			)
		}

		if other, taken := seen[win.Order]; taken {
			t.Fatalf(
				"windows %d and %d both report stacking order %d",
				other, win.Number, win.Order,
			)
		}

		seen[win.Order] = win.Number
	}

	// The window the query calls focused is the one the window server put in
	// front, so it is the one at the head of the stack.
	if info.Focused < 0 {
		t.Skip("no listed window holds focus, so there is no front window to check")
	}

	if front := info.Windows[info.Focused]; front.Order != 0 {
		t.Fatalf(
			"the focused window %d reports stacking order %d, want 0 for the window in front",
			front.Number, front.Order,
		)
	}
}
