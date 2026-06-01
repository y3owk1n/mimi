package permissions

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#include <stdlib.h>
#include "permissions.h"
*/
import "C"

import (
	"unsafe"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// ConfigOnboardingChoice represents the user's choice in the config onboarding alert.
type ConfigOnboardingChoice int

// AccessibilityStartupChoice represents the user's choice in the startup permission alert.
type AccessibilityStartupChoice int

const (
	// ConfigOnboardingCreate indicates the user chose to create a config file.
	ConfigOnboardingCreate ConfigOnboardingChoice = 1
	// ConfigOnboardingQuit indicates the user chose to quit.
	ConfigOnboardingQuit ConfigOnboardingChoice = 2

	// AccessibilityStartupGranted indicates accessibility permission is granted.
	AccessibilityStartupGranted AccessibilityStartupChoice = 1
	// AccessibilityStartupQuit indicates the user chose to quit.
	AccessibilityStartupQuit AccessibilityStartupChoice = 2
)

// CheckResult holds the results of a permissions check.
type CheckResult struct {
	Accessibility    bool
	AccessibilityMsg string
}

// Check verifies macOS accessibility permissions.
func Check() CheckResult {
	trusted := C.MimiCheckAccessibilityPermissions() != 0
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

// RequestAccessibility asks macOS to start the accessibility permission flow.
func RequestAccessibility() bool {
	return C.MimiRequestAccessibilityPermissions() != 0
}

// ShowConfigOnboardingAlert displays startup guidance for creating the first config file.
func ShowConfigOnboardingAlert(configPath string) ConfigOnboardingChoice {
	cPath := C.CString(configPath)
	defer C.free(unsafe.Pointer(cPath)) //nolint:nlreturn

	return ConfigOnboardingChoice(C.MimiShowConfigOnboardingAlert(cPath))
}

// ShowAccessibilityStartupAlert displays startup guidance for granting accessibility permission.
func ShowAccessibilityStartupAlert() AccessibilityStartupChoice {
	return AccessibilityStartupChoice(C.MimiShowAccessibilityPermissionStartupAlert())
}

// FriendlyError returns an error if accessibility permission is denied.
func FriendlyError(r CheckResult) error {
	if r.Accessibility {
		return nil
	}

	return derrors.New(derrors.CodeAccessibilityDenied, r.AccessibilityMsg)
}
