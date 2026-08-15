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

	// installAX and removeAX default to the native cgo bridge and are
	// reassigned directly by in-package tests so they can exercise
	// Install/Remove without calling into cgo.
	installAX func(pid int) bool
	removeAX  func(pid int)
}

// NewAXTracker creates a tracker with the enabled flag.
func NewAXTracker(enabled bool) *AXTracker {
	return &AXTracker{
		tracked:   make(map[int]struct{}),
		enabled:   enabled,
		installAX: native.InstallAXObserver,
		removeAX:  native.RemoveAXObserver,
	}
}

// Update updates the enabled state. When disabled, removes all active AX observers.
func (t *AXTracker) Update(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.enabled = enabled
	if !enabled {
		for pid := range t.tracked {
			t.removeAX(pid)
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

	if ok := t.installAX(pid); ok {
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

	t.removeAX(pid)
	delete(t.tracked, pid)
}
