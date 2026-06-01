package observers

import (
	"sync"

	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
)

// AccessibilityManager tracks which PIDs have AX observers installed.
type AccessibilityManager struct {
	mu      sync.Mutex
	tracked map[int]struct{}
	enabled bool
}

// NewAccessibilityManager creates a manager with the enabled flag.
func NewAccessibilityManager(axEnabled bool) *AccessibilityManager {
	return &AccessibilityManager{
		tracked: make(map[int]struct{}),
		enabled: axEnabled,
	}
}

// Install installs an AX observer for the given PID. Returns false if AX is disabled.
func (m *AccessibilityManager) Install(pid int) bool {
	if !m.enabled {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tracked[pid]; ok {
		return true
	}

	if ok := cgo_bridge.InstallAXObserver(pid); ok {
		m.tracked[pid] = struct{}{}

		return true
	}

	return false
}

// Remove removes the AX observer for the given PID.
func (m *AccessibilityManager) Remove(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tracked[pid]; !ok {
		return
	}

	cgo_bridge.RemoveAXObserver(pid)
	delete(m.tracked, pid)
}
