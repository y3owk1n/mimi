//nolint:testpackage // needs direct access to AXTracker's unexported installAX/removeAX/tracked fields
package observe

import "testing"

// fakeAXCalls records calls made through the installAX/removeAX seams so
// tests can assert on them without touching cgo.
type fakeAXCalls struct {
	installed []int
	removed   []int
	installOK bool
}

func (f *fakeAXCalls) install(pid int) bool {
	f.installed = append(f.installed, pid)

	return f.installOK
}

func (f *fakeAXCalls) remove(pid int) {
	f.removed = append(f.removed, pid)
}

func newFakeTracker(enabled bool, installOK bool) (*AXTracker, *fakeAXCalls) {
	calls := &fakeAXCalls{installOK: installOK}
	tracker := NewAXTracker(enabled)
	tracker.installAX = calls.install
	tracker.removeAX = calls.remove

	return tracker, calls
}

func TestAXTracker_Install_DisabledNeverCallsNative(t *testing.T) {
	tracker, calls := newFakeTracker(false, true)

	if ok := tracker.Install(42); ok {
		t.Error("Install() on a disabled tracker = true, want false")
	}

	if len(calls.installed) != 0 {
		t.Errorf("installAX called %d times on a disabled tracker, want 0", len(calls.installed))
	}
}

func TestAXTracker_Install_EnabledTracksOnSuccess(t *testing.T) {
	tracker, calls := newFakeTracker(true, true)

	if ok := tracker.Install(42); !ok {
		t.Fatal("Install() = false, want true")
	}

	if len(calls.installed) != 1 || calls.installed[0] != 42 {
		t.Errorf("installAX calls = %v, want [42]", calls.installed)
	}

	// A second Install for the same PID must be a no-op: no repeat call to
	// installAX, since the PID is already tracked.
	if ok := tracker.Install(42); !ok {
		t.Error("Install() on an already-tracked PID = false, want true")
	}

	if len(calls.installed) != 1 {
		t.Errorf(
			"installAX called %d times for a repeated Install, want 1 (deduped)",
			len(calls.installed),
		)
	}
}

func TestAXTracker_Install_FailureNotTracked(t *testing.T) {
	tracker, calls := newFakeTracker(true, false)

	if ok := tracker.Install(42); ok {
		t.Error("Install() = true when installAX reports failure, want false")
	}

	if len(calls.installed) != 1 {
		t.Errorf("installAX calls = %d, want 1", len(calls.installed))
	}

	tracker.mu.Lock()
	_, tracked := tracker.tracked[42]
	tracker.mu.Unlock()

	if tracked {
		t.Error("PID marked tracked despite installAX reporting failure")
	}
}

// TestAXTracker_Remove_HasNoEnabledGuard pins the asymmetry the ticket flags:
// unlike Install, Remove does not check t.enabled. That is safe today only
// because a disabled tracker can never populate tracked (Install's own
// !t.enabled guard prevents it) — so this test forces the otherwise
// unreachable state (disabled tracker, non-empty tracked) directly, to prove
// Remove's behavior rests on "nothing is tracked", not on "we're enabled".
func TestAXTracker_Remove_HasNoEnabledGuard(t *testing.T) {
	tracker, calls := newFakeTracker(false, true)

	// Seed tracked directly: Install can't do this while disabled, and that
	// is exactly the point.
	tracker.mu.Lock()
	tracker.tracked[42] = struct{}{}
	tracker.mu.Unlock()

	tracker.Remove(42)

	if len(calls.removed) != 1 || calls.removed[0] != 42 {
		t.Errorf(
			"removeAX calls = %v, want [42]: Remove must act regardless of the enabled flag",
			calls.removed,
		)
	}

	tracker.mu.Lock()
	_, tracked := tracker.tracked[42]
	tracker.mu.Unlock()

	if tracked {
		t.Error("PID still tracked after Remove")
	}
}

func TestAXTracker_Remove_UntrackedPIDNoOp(t *testing.T) {
	tracker, calls := newFakeTracker(true, true)

	tracker.Remove(99)

	if len(calls.removed) != 0 {
		t.Errorf("removeAX called %d times for an untracked PID, want 0", len(calls.removed))
	}
}

func TestAXTracker_Update_DisableRemovesAllTracked(t *testing.T) {
	tracker, calls := newFakeTracker(true, true)

	tracker.Install(1)
	tracker.Install(2)
	tracker.Install(3)

	tracker.Update(false)

	if len(calls.removed) != 3 {
		t.Errorf("removeAX called %d times after disabling, want 3", len(calls.removed))
	}

	tracker.mu.Lock()
	remaining := len(tracker.tracked)
	tracker.mu.Unlock()

	if remaining != 0 {
		t.Errorf("tracked has %d entries after disabling, want 0", remaining)
	}

	// With the tracker now disabled, Install must refuse again.
	if ok := tracker.Install(4); ok {
		t.Error("Install() after Update(false) = true, want false")
	}
}

func TestAXTracker_Update_EnableAgainAllowsInstall(t *testing.T) {
	tracker, calls := newFakeTracker(false, true)

	if ok := tracker.Install(7); ok {
		t.Fatal("Install() before Update(true) = true, want false")
	}

	tracker.Update(true)

	if ok := tracker.Install(7); !ok {
		t.Error("Install() after Update(true) = false, want true")
	}

	if len(calls.installed) != 1 {
		t.Errorf("installAX calls = %d, want 1", len(calls.installed))
	}
}
