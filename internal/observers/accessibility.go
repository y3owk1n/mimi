package observers

import (
	"sync"

	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
)

type AccessibilityManager struct {
	mu      sync.Mutex
	tracked map[int]struct{}
	enabled bool
}

func NewAccessibilityManager(axEnabled bool) *AccessibilityManager {
	return &AccessibilityManager{
		tracked: make(map[int]struct{}),
		enabled: axEnabled,
	}
}

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

func (m *AccessibilityManager) Remove(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tracked[pid]; !ok {
		return
	}
	cgo_bridge.RemoveAXObserver(pid)
	delete(m.tracked, pid)
}
