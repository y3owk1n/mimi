package observe

import (
	"sync"

	"github.com/y3owk1n/mimi/internal/native"
)

// AXTracker tracks which PIDs have AX observers installed.
type AXTracker struct {
	mu      sync.Mutex
	tracked map[int]struct{}
	enabled bool
}

// NewAXTracker creates a tracker with the enabled flag.
func NewAXTracker(enabled bool) *AXTracker {
	return &AXTracker{
		tracked: make(map[int]struct{}),
		enabled: enabled,
	}
}

// Update updates the enabled state. When disabled, removes all active AX observers.
func (t *AXTracker) Update(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.enabled = enabled
	if !enabled {
		for pid := range t.tracked {
			native.RemoveAXObserver(pid)
			delete(t.tracked, pid)
		}
	}
}

// Install installs an AX observer for the given PID.
func (t *AXTracker) Install(pid int) bool {
	if !t.enabled {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.tracked[pid]; ok {
		return true
	}

	if ok := native.InstallAXObserver(pid); ok {
		t.tracked[pid] = struct{}{}

		return true
	}

	return false
}

// Remove removes the AX observer for the given PID.
func (t *AXTracker) Remove(pid int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.tracked[pid]; !ok {
		return
	}

	native.RemoveAXObserver(pid)
	delete(t.tracked, pid)
}
