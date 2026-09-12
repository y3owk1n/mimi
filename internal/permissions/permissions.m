#import "permissions.h"

#import "../native/mimi_log.h"

#import <ApplicationServices/ApplicationServices.h>
#import <Cocoa/Cocoa.h>

static int MimiRunOnMainThreadSync(int (^block)(void)) {
	if ([NSThread isMainThread]) {
		return block();
	}
	__block int result = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		result = block();
	});
	return result;
}

// MimiResetPermissionDecision clears the TCC decision macOS holds for mimi
// under service, so that the next request prompts again. An unsigned build
// is a new program to TCC on every rebuild, while System Settings keeps
// showing the old entry as allowed. Without the reset the request never
// prompts and mimi never gets the permission.
static int MimiResetPermissionDecision(NSString *service) {
	NSString *bundleID = [[NSBundle mainBundle] bundleIdentifier];
	if (bundleID == nil || [bundleID length] == 0) {
		bundleID = @"com.y3owk1n.mimi";
	}

	NSTask *task = [[NSTask alloc] init];
	task.launchPath = @"/usr/bin/tccutil";
	task.arguments = @[ @"reset", service, bundleID ];

	@try {
		[task launch];
		[task waitUntilExit];

		int status = [task terminationStatus];
		if (status != 0) {
			MIMI_LOG(
			    "tccutil reset %@ %@ exited with status %d; system permission dialog may not appear", service, bundleID,
			    status);
			return 0;
		}
	} @catch (NSException *exception) {
		MIMI_LOG("failed to reset %@ permission decision: %@", service, exception);
		return 0;
	}

	return 1;
}

// MimiPresentAlert runs alert as a floating modal, restores the accessory
// activation policy, and returns the button pressed.
static NSModalResponse MimiPresentAlert(NSAlert *alert) {
	[[alert window] setLevel:NSFloatingWindowLevel];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	[[alert window] center];
	[[alert window] makeKeyAndOrderFront:nil];
	[NSApp activateIgnoringOtherApps:YES];

	NSModalResponse response = [alert runModal];
	[[alert window] orderOut:nil];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	return response;
}

// MimiShowRestartRequiredAlert tells the user the grant for permission takes
// effect on the next start, and that mimi is quitting.
static void MimiShowRestartRequiredAlert(NSString *permission) {
	NSAlert *alert = [[NSAlert alloc] init];
	alert.messageText = @"Mimi Restart Required";
	alert.informativeText =
	    [NSString stringWithFormat:@"macOS requires restarting Mimi for %@ permissions to take effect.\n\n"
	                                "Mimi will now quit. Please relaunch the application.",
	                               permission];
	alert.alertStyle = NSAlertStyleInformational;
	[alert addButtonWithTitle:@"Quit Mimi"];
	MimiPresentAlert(alert);
}

int MimiCheckAccessibilityPermissions(void) {
	Boolean trusted = AXIsProcessTrusted();
	return trusted ? 1 : 0;
}

int MimiRequestAccessibilityPermissions(void) {
	@autoreleasepool {
		if (!MimiResetPermissionDecision(@"Accessibility")) {
			MIMI_LOG("continuing with Accessibility permission request after reset failure");
		}

		NSDictionary *options = @{(__bridge id)kAXTrustedCheckOptionPrompt : @YES};
		Boolean trusted = AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
		return trusted ? 1 : 0;
	}
}

int MimiShowAccessibilityPermissionStartupAlert(void) {
	return MimiRunOnMainThreadSync(^int {
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

				NSModalResponse response = MimiPresentAlert(alert);

				if (response == NSAlertFirstButtonReturn) {
					MimiRequestAccessibilityPermissions();
				} else if (response == NSAlertSecondButtonReturn) {
					if (MimiCheckAccessibilityPermissions() == 1) {
						return 1;
					}

					MimiShowRestartRequiredAlert(@"Accessibility");
					return 3;
				} else if (response == NSAlertThirdButtonReturn) {
					return 2;
				}
			}

			return 1;
		}
	});
}

int MimiShowConfigOnboardingAlert(const char *configPath) {
	return MimiRunOnMainThreadSync(^int {
		@autoreleasepool {
			[NSApplication sharedApplication];

			NSString *path = configPath ? [NSString stringWithUTF8String:configPath] : @"~/.config/mimi/config.toml";

			NSAlert *alert = [[NSAlert alloc] init];
			alert.messageText = @"Welcome to Mimi";
			alert.informativeText =
			    [NSString stringWithFormat:@"No configuration file found.\n\nCreate a starter config at:\n%@", path];
			alert.alertStyle = NSAlertStyleInformational;

			[alert addButtonWithTitle:@"Create Config"];
			[alert addButtonWithTitle:@"Quit"];

			NSModalResponse response = MimiPresentAlert(alert);

			if (response == NSAlertFirstButtonReturn) {
				return 1;
			} else if (response == NSAlertSecondButtonReturn) {
				return 2;
			}

			return 2;
		}
	});
}
