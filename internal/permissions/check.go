package permissions

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#import <Cocoa/Cocoa.h>
#include <ApplicationServices/ApplicationServices.h>

static int MimiCheckAccessibilityPermissions(void) {
	Boolean trusted = AXIsProcessTrusted();
	return trusted ? 1 : 0;
}

static BOOL MimiResetAccessibilityPermissionDecision(void) {
	NSString *bundleID = [[NSBundle mainBundle] bundleIdentifier];
	if (bundleID == nil || [bundleID length] == 0) {
		bundleID = @"com.y3owk1n.mimi";
	}

	NSTask *task = [[NSTask alloc] init];
	task.launchPath = @"/usr/bin/tccutil";
	task.arguments = @[ @"reset", @"Accessibility", bundleID ];

	@try {
		[task launch];
		[task waitUntilExit];

		int status = [task terminationStatus];
		if (status != 0) {
			NSLog(
				@"Mimi: tccutil reset Accessibility %@ exited with status %d; system permission dialog may not appear",
				bundleID, status);
			return NO;
		}
	} @catch (NSException *exception) {
		NSLog(@"Mimi: failed to reset Accessibility permission decision: %@", exception);
		return NO;
	}

	return YES;
}

static int MimiRequestAccessibilityPermissions(void) {
	@autoreleasepool {
		if (!MimiResetAccessibilityPermissionDecision()) {
			NSLog(@"Mimi: continuing with Accessibility permission request after reset failure");
		}

		NSDictionary *options = @{(__bridge id)kAXTrustedCheckOptionPrompt : @YES};
		Boolean trusted = AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
		return trusted ? 1 : 0;
	}
}

static int MimiShowAccessibilityPermissionStartupAlert(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];

		while (MimiCheckAccessibilityPermissions() != 1) {
			NSAlert *alert = [[NSAlert alloc] init];
			alert.messageText = @"Accessibility Permission Needed";
			alert.informativeText =
				@"Mimi needs Accessibility permission to receive window focus and title change events. "
				@"Click Request Permission to open the macOS permission flow, grant access in System Settings, "
				@"then return here and click Granted, Start Mimi.";
			alert.alertStyle = NSAlertStyleWarning;
			alert.icon = [NSImage imageNamed:NSImageNameCaution];

			[alert addButtonWithTitle:@"Request Permission"];
			[alert addButtonWithTitle:@"Granted, Start Mimi"];
			[alert addButtonWithTitle:@"Quit"];

			[[alert window] setLevel:NSFloatingWindowLevel];
			[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
			[[alert window] center];
			[[alert window] makeKeyAndOrderFront:nil];
			[NSApp activateIgnoringOtherApps:YES];

			NSModalResponse response = [alert runModal];
			[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

			if (response == NSAlertFirstButtonReturn) {
				MimiRequestAccessibilityPermissions();
			} else if (response == NSAlertSecondButtonReturn) {
				return 1;
			} else if (response == NSAlertThirdButtonReturn) {
				return 2;
			}
		}

		return 1;
	}
}
*/
import "C"

import (
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// AccessibilityStartupChoice represents the user's choice in the startup permission alert.
type AccessibilityStartupChoice int

const (
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
