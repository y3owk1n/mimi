package permissions

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>
*/
import "C"

import (
	"fmt"
)

type CheckResult struct {
	Accessibility    bool
	AccessibilityMsg string
}

func Check() CheckResult {
	trusted := C.AXIsProcessTrusted() != 0
	r := CheckResult{Accessibility: trusted}
	if !trusted {
		r.AccessibilityMsg = `Accessibility permission is required for window focus events.

  Grant it in:
    System Settings -> Privacy & Security -> Accessibility -> enable "mimi"

  After granting, restart mimi.
  Window events (on_window_focus, on_window_title_change, etc.) will be
  unavailable until the permission is granted.`
	}
	return r
}

func FriendlyError(r CheckResult) error {
	if r.Accessibility {
		return nil
	}
	return fmt.Errorf("%s", r.AccessibilityMsg)
}
