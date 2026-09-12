//go:build integration

// What the window server says about the space in front, checked against the
// assumption the tiling engine's state is filed under.
//
// A space has two numbers. One is where it sits in Mission Control, which
// changes whenever a space is added or removed before it. The other is the
// window server's own identifier for it, which never changes. The engine
// reports the first to a layout and files the layout's state under the
// second, so the two have to name the same space. This test reads both off
// the live desktop and checks that they do.
//
// It only reads. It opens no window, moves nothing, and skips rather than
// fails on a machine that cannot answer.

package baseline_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/native"
)

func TestSpaceIdentity_TheIDAndTheIndexNameTheSameSpace(t *testing.T) {
	displays, err := native.Displays()
	if err != nil {
		t.Skipf("the displays could not be enumerated: %v", err)
	}

	if len(displays) == 0 {
		t.Skip("no display is connected")
	}

	ids := make([]uint32, len(displays))
	for index, display := range displays {
		ids[index] = display.ID
	}

	indexes := native.ActiveSpaceIndexes(ids)
	spaceIDs := native.ActiveSpaceIDs(ids)

	if len(indexes) == 0 {
		t.Skip("no display's space could be resolved, so Mission Control may be open")
	}

	// The window server identifies every space it also places in Mission
	// Control. The engine keys state on the identifier, so a display with a
	// place and no identifier would keep no layout state.
	for display, index := range indexes {
		sid, ok := spaceIDs[display]
		if !ok {
			t.Fatalf(
				"display %d shows Mission Control space %d but no space identifier, "+
					"so the engine would keep no state for it",
				display, index,
			)
		}

		if sid == 0 {
			t.Fatalf("display %d reported space identifier 0, which names nothing", display)
		}
	}

	// The two also agree about which space that is. The identifier's own
	// place in Mission Control is the index reported beside it.
	places := native.SpaceIndexes()

	for display, sid := range spaceIDs {
		index, ok := indexes[display]
		if !ok {
			continue
		}

		if place, known := places[sid]; known && place != index {
			t.Fatalf(
				"display %d: space %d sits at Mission Control index %d, but the "+
					"active-space index says %d",
				display, sid, place, index,
			)
		}
	}
}
