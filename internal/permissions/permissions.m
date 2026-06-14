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

static int MimiResetAccessibilityPermissionDecision(void) {
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
			MIMI_LOG(
			    "tccutil reset Accessibility %@ exited with status %d; system permission dialog may not appear",
			    bundleID, status);
			return 0;
		}
	} @catch (NSException *exception) {
		MIMI_LOG("failed to reset Accessibility permission decision: %@", exception);
		return 0;
	}

	return 1;
}

int MimiCheckAccessibilityPermissions(void) {
	Boolean trusted = AXIsProcessTrusted();
	return trusted ? 1 : 0;
}

int MimiRequestAccessibilityPermissions(void) {
	@autoreleasepool {
		if (!MimiResetAccessibilityPermissionDecision()) {
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

				[[alert window] setLevel:NSFloatingWindowLevel];
				[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
				[[alert window] center];
				[[alert window] makeKeyAndOrderFront:nil];
				[NSApp activateIgnoringOtherApps:YES];

				NSModalResponse response = [alert runModal];
				[[alert window] orderOut:nil];
				[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

				if (response == NSAlertFirstButtonReturn) {
					MimiRequestAccessibilityPermissions();
				} else if (response == NSAlertSecondButtonReturn) {
					if (MimiCheckAccessibilityPermissions() == 1) {
						return 1;
					}

					NSAlert *restartAlert = [[NSAlert alloc] init];
					restartAlert.messageText = @"Mimi Restart Required";
					restartAlert.informativeText =
					    @"macOS requires restarting Mimi for Accessibility permissions to take effect.\n\n"
					     "Mimi will now quit. Please relaunch the application.";
					restartAlert.alertStyle = NSAlertStyleInformational;
					[restartAlert addButtonWithTitle:@"Quit Mimi"];

					[[restartAlert window] setLevel:NSFloatingWindowLevel];
					[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
					[[restartAlert window] center];
					[[restartAlert window] makeKeyAndOrderFront:nil];
					[NSApp activateIgnoringOtherApps:YES];

					[restartAlert runModal];
					[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

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

			[[alert window] setLevel:NSFloatingWindowLevel];
			[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
			[[alert window] center];
			[[alert window] makeKeyAndOrderFront:nil];
			[NSApp activateIgnoringOtherApps:YES];

			NSModalResponse response = [alert runModal];
			[[alert window] orderOut:nil];
			[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

			if (response == NSAlertFirstButtonReturn) {
				return 1;
			} else if (response == NSAlertSecondButtonReturn) {
				return 2;
			}

			return 2;
		}
	});
}
