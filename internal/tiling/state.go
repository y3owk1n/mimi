package tiling

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// SpaceState is what the engine remembers for one display and one space: the
// state the layout last returned there, and enough about the space to say
// which one it is.
//
// Display and SpaceID name the space the way the engine files it. Space is
// the same space's place in Mission Control at the time of the read, which is
// what the user counts and what the layout was told. Both are reported
// because only the first is an identity and only the second is recognizable.
type SpaceState struct {
	Display uint32 `json:"display"`
	SpaceID uint64 `json:"spaceId"`
	// Space is 0 when that space is not in front on any display now, which
	// is every space but the ones the user is looking at.
	Space int             `json:"space"`
	State json.RawMessage `json:"state"`
}

// State is everything the engine remembers. Spaces is one entry per display
// and space it has run a layout for, ordered by display and then by space so
// that two reads of an unchanged desktop print the same bytes.
//
// This is a read of the daemon's own memory rather than of the desktop, which
// is why it is not a query and why a CLI with no daemon to ask has nothing to
// report.
type State struct {
	Spaces []SpaceState `json:"spaces"`
	// Unmanaged is every window the layout said it is leaving alone, by
	// number. The engine keeps one set for the desktop rather than one per
	// space, because a window number names a window wherever it is.
	Unmanaged []uint32 `json:"unmanaged"`
}

// State is what the engine is holding right now.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()

	held := State{
		Spaces:    make([]SpaceState, 0, len(e.states)),
		Unmanaged: make([]uint32, 0, len(e.unmanaged)),
	}

	inFront := e.spacesInFront()

	for key, state := range e.states {
		display, sid, ok := splitStateKey(key)
		if !ok {
			continue
		}

		held.Spaces = append(held.Spaces, SpaceState{
			Display: display,
			SpaceID: sid,
			Space:   inFront[sid],
			State:   state,
		})
	}

	slices.SortFunc(held.Spaces, func(left, right SpaceState) int {
		if left.Display != right.Display {
			return cmp.Compare(left.Display, right.Display)
		}

		return cmp.Compare(left.SpaceID, right.SpaceID)
	})

	for number := range e.unmanaged {
		held.Unmanaged = append(held.Unmanaged, number)
	}

	slices.Sort(held.Unmanaged)

	return held
}

// Reset forgets what the layout returned for the space in front on every
// display, so the next pass there starts from null, and reports how many
// entries it dropped. With all set it forgets every space instead.
//
// This is the way out of a layout that has tied its own state in a knot.
// Without it the only way was restarting the daemon, which threw away every
// display's state rather than the one that had gone wrong.
func (e *Engine) Reset(all bool) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if all {
		dropped := len(e.states)

		clear(e.states)
		clear(e.writtenAt)

		return dropped, nil
	}

	ids, err := e.desktop.ActiveSpaceIDs()
	if err != nil {
		return 0, derrors.Wrapf(
			err,
			derrors.CodeActionFailed,
			"failed to resolve which space to reset",
		)
	}

	dropped := 0

	for display, sid := range ids {
		key := stateKeyOf(display, sid)
		if _, held := e.states[key]; !held {
			continue
		}

		delete(e.states, key)
		delete(e.writtenAt, key)

		dropped++
	}

	return dropped, nil
}

// spacesInFront is where each space that is in front on some display sits in
// Mission Control, by identifier. A space that is not in front anywhere is
// absent, and so is every space when the desktop will not say, which costs
// the read those numbers and nothing else. The caller holds the lock.
func (e *Engine) spacesInFront() map[uint64]int {
	ids, err := e.desktop.ActiveSpaceIDs()
	if err != nil {
		return nil
	}

	spaces, err := e.desktop.ActiveSpaces()
	if err != nil {
		return nil
	}

	inFront := make(map[uint64]int, len(ids))
	for display, sid := range ids {
		inFront[sid] = spaces[display]
	}

	return inFront
}

// splitStateKey reads back the display and the space a state key names.
func splitStateKey(key string) (uint32, uint64, bool) {
	left, right, ok := strings.Cut(key, "/")
	if !ok {
		return 0, 0, false
	}

	display, err := strconv.ParseUint(left, 10, 32)
	if err != nil {
		return 0, 0, false
	}

	sid, err := strconv.ParseUint(right, 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return uint32(display), sid, true
}
