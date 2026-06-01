package permissions

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>
*/
import "C"

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// CheckResult holds the results of a permissions check.
type CheckResult struct {
	Accessibility    bool
	AccessibilityMsg string
}

// Check verifies macOS accessibility permissions.
func Check() CheckResult {
	trusted := C.AXIsProcessTrusted() != 0
	res := CheckResult{Accessibility: trusted}
	if !trusted {
		res.AccessibilityMsg = `Accessibility permission is required for window focus events.

  Grant it in:
    System Settings -> Privacy & Security -> Accessibility -> enable "mimi"

  After granting, restart mimi.
  Window events (on_window_focus, on_window_title_change, etc.) will be
  unavailable until the permission is granted.`
	}

	return res
}

// FriendlyError returns an error if accessibility permission is denied.
func FriendlyError(r CheckResult) error {
	if r.Accessibility {
		return nil
	}

	return derrors.New(derrors.CodeAccessibilityDenied, r.AccessibilityMsg)
}
