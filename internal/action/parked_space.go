package action

import (
	"sync"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// parkedSpace is an empty space mimi switched to on a display that keyboard
// focus did not follow, because an empty space has no window to take focus.
// from is the space that kept focus, and here is whether the last switch went
// to the parked space or back to from. switches is the desktop's space switch
// count when mimi parked. While focus stays on from, recent toggles between
// the two spaces, and next and prev step from the parked space whenever here
// is true. Any change of the space you are on ends the park, even when focus
// returns to from before the next command.
type parkedSpace struct {
	mu       sync.Mutex
	id       uint64
	from     uint64
	here     bool
	switches uint64
}

func (p *parkedSpace) set(id, from uint64, here bool, switches uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.id, p.from, p.here, p.switches = id, from, here, switches
}

func (p *parkedSpace) get() (uint64, uint64, bool, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.id, p.from, p.here, p.switches
}

// leaveFor updates the park for a switch to target. Going back to the space
// that kept focus keeps the park, so recent can return to it. Any other
// switch drops it.
func (p *parkedSpace) leaveFor(target uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.id != 0 && target == p.from {
		p.here = false

		return
	}

	p.id, p.from, p.here, p.switches = 0, 0, false, 0
}

// parkedIndexes is the 1-based index of the parked space and of the space that
// kept focus, and whether the last switch went to the parked space. ok is
// false once the space you are on has changed since mimi parked, and
// parkedIndexes then drops the park.
func (e *Executor) parkedIndexes() (int, int, bool, bool) {
	parkedID, from, here, switches := e.parked.get()
	if parkedID == 0 {
		return 0, 0, false, false
	}

	focused, spaces, err := e.focusedSpace()
	parked, kept := spaceIndex(spaces, parkedID), spaceIndex(spaces, from)
	moved := e.desktop.SpaceSwitchCount() != switches

	if err != nil || moved || focused != from || parked == 0 || kept == 0 {
		e.parked.set(0, 0, false, 0)

		return 0, 0, false, false
	}

	return parked, kept, here, true
}

// parkOn records target as parked when keyboard focus is on another space.
func (e *Executor) parkOn(target uint64) {
	focused, _, err := e.focusedSpace()
	if err == nil && focused != target {
		e.parked.set(target, focused, true, e.desktop.SpaceSwitchCount())
	}
}

// currentSpaceIndex is the 1-based index next and prev step from: the parked
// space while you are on it, otherwise the space with keyboard focus.
func (e *Executor) currentSpaceIndex() (int, error) {
	if parked, _, here, ok := e.parkedIndexes(); ok && here {
		return parked, nil
	}

	return e.desktop.ActiveSpaceIndex()
}

// focusedSpace is the identifier of the space with keyboard focus, with every
// space listed.
func (e *Executor) focusedSpace() (uint64, []Space, error) {
	index, err := e.desktop.ActiveSpaceIndex()
	if err != nil {
		return 0, nil, err
	}

	spaces, err := e.desktop.Spaces()
	if err != nil {
		return 0, nil, err
	}

	if index < 1 || index > len(spaces) {
		return 0, spaces, derrors.Newf(
			derrors.CodeActionFailed,
			"active space %d is not listed",
			index,
		)
	}

	return spaces[index-1].ID, spaces, nil
}

// spaceIndex is the 1-based index of the space with this identifier, or 0.
func spaceIndex(spaces []Space, id uint64) int {
	for index, space := range spaces {
		if space.ID == id {
			return index + 1
		}
	}

	return 0
}
