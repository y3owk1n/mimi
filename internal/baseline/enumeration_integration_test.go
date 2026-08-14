//go:build integration

// Regression test for the enumeration staleness in #69.
//
// The applications the active-space enumeration walks used to come from
// -[NSWorkspace runningApplications], which only refreshes while the main
// thread's run loop runs. A test binary never pumps it, so the list froze at
// the first enumeration and nothing launched afterwards was ever seen again.
// The enumeration now derives its applications from the window list instead,
// which has no such dependency.
//
// The test opens one throwaway TextEdit document, never touches a window it did
// not open, and skips rather than fails whenever the machine cannot support it.

package baseline_test

import (
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/permissions"
	"github.com/y3owk1n/mimi/internal/window"
)

func TestEnumeration_SeesAnApplicationLaunchedAfterTheFirstEnumeration(t *testing.T) {
	if !permissions.Check().Accessibility {
		t.Skip("Accessibility permission is not granted; windows cannot be enumerated")
	}

	// Read the window list once, so that anything cached behind it is cached
	// before the helper exists. Without this the test would only be meaningful
	// when it happens to run first in the binary.
	windows, err := window.AllFocusableOnActiveSpace()
	if err != nil {
		t.Fatalf("could not enumerate the windows on the active space: %v", err)
	}

	window.ReleaseAll(windows)

	pids := launchHelper(t, 1)

	// Whether the helper really has a usable window is established through the
	// focused-application path, which reads Accessibility directly and so
	// cannot be answered out of the same cache the enumeration used to depend
	// on. If that never agrees, this machine is not in a state the test can
	// judge — somebody took focus back, or the document did not open.
	if !waitForHelperToBeFrontmost(t, pids) {
		t.Skipf("the helper never came to the front within %s", focusTimeout)
	}

	// The helper owns a real, focused window that the enumeration did not know
	// about when it last ran. Anything short of finding it is the defect.
	if !waitForHelperInEnumeration(t, pids) {
		t.Fatalf(
			"the helper is frontmost but is still missing from the enumeration after %s; "+
				"the application list is stale (see #69)",
			settleTimeout,
		)
	}
}

// waitForHelperToBeFrontmost reports whether the frontmost window belongs to one
// of the given helper processes before focusTimeout.
func waitForHelperToBeFrontmost(t *testing.T, pids map[int]bool) bool {
	t.Helper()

	return waitFor(focusTimeout, func() bool {
		front := window.Frontmost()
		if front == nil {
			return false
		}
		defer front.Release()

		pid, err := front.PID()

		return err == nil && pids[pid]
	})
}

// waitForHelperInEnumeration reports whether the active space's enumeration
// includes a window owned by one of the given helper processes before
// settleTimeout.
func waitForHelperInEnumeration(t *testing.T, pids map[int]bool) bool {
	t.Helper()

	return waitFor(settleTimeout, func() bool {
		windows, err := window.AllFocusableOnActiveSpace()
		if err != nil {
			t.Fatalf("could not enumerate the windows on the active space: %v", err)
		}
		defer window.ReleaseAll(windows)

		for _, element := range windows {
			pid, pidErr := element.PID()
			if pidErr == nil && pids[pid] {
				return true
			}
		}

		return false
	})
}

// waitFor polls condition until it holds or the timeout expires.
func waitFor(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)

	for {
		if condition() {
			return true
		}

		if time.Now().After(deadline) {
			return false
		}

		time.Sleep(pollInterval)
	}
}
