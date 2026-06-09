//
//  screen.m
//  mimi
//

#import "mimi.h"

#import <Cocoa/Cocoa.h>

static bool detectMissionControlActive(void) {
	@autoreleasepool {
		CFArrayRef windowList = CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID);
		if (!windowList) {
			return false;
		}

		CFIndex count = CFArrayGetCount(windowList);
		int dockHighLayerWindows = 0;
		int dockOverlayWindows = 0;

		for (CFIndex i = 0; i < count; i++) {
			CFDictionaryRef windowInfo = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
			if (!windowInfo)
				continue;

			CFStringRef ownerName = (CFStringRef)CFDictionaryGetValue(windowInfo, kCGWindowOwnerName);
			if (!ownerName)
				continue;

			if (CFStringCompare(ownerName, CFSTR("Mission Control"), 0) == kCFCompareEqualTo) {
				CFRelease(windowList);
				return YES;
			}

			if (CFStringCompare(ownerName, CFSTR("Dock"), 0) != kCFCompareEqualTo)
				continue;

			CFNumberRef windowLayer = (CFNumberRef)CFDictionaryGetValue(windowInfo, kCGWindowLayer);
			if (!windowLayer)
				continue;

			int layer = 0;
			CFNumberGetValue(windowLayer, kCFNumberIntType, &layer);

			if (layer >= 18 && layer <= 20) {
				dockHighLayerWindows++;
				if (dockHighLayerWindows >= 2) {
					CFRelease(windowList);
					return YES;
				}
			}

			if (layer >= 14 && layer <= 25) {
				dockOverlayWindows++;
				if (dockOverlayWindows >= 3) {
					CFRelease(windowList);
					return YES;
				}
			}
		}

		CFRelease(windowList);
		return NO;
	}
}

bool MimiIsMissionControlActive(void) { return detectMissionControlActive(); }

double *MimiGetScreenFrameForPoint(double x, double y) {
	@autoreleasepool {
		NSScreen *primary = [NSScreen mainScreen];
		CGFloat primaryHeight = [primary frame].size.height;

		// Input (x, y) is in AX coordinates (y-down, origin at top-left of primary).
		// Convert to NSScreen coordinates (y-up, origin at bottom-left of primary).
		CGPoint pt = CGPointMake(x, primaryHeight - y);

		NSScreen *targetScreen = nil;
		for (NSScreen *screen in [NSScreen screens]) {
			if (NSPointInRect(pt, [screen frame])) {
				targetScreen = screen;
				break;
			}
		}

		if (!targetScreen) {
			targetScreen = primary;
		}

		NSRect frame = [targetScreen frame];
		double *result = (double *)malloc(4 * sizeof(double));
		if (!result)
			return NULL;

		result[0] = frame.origin.x;
		result[1] = frame.origin.y;
		result[2] = frame.size.width;
		result[3] = frame.size.height;

		return result;
	}
}

double *MimiGetScreenVisibleFrameForPoint(double x, double y) {
	@autoreleasepool {
		NSScreen *primary = [NSScreen mainScreen];
		CGFloat primaryHeight = [primary frame].size.height;

		// Input (x, y) is in AX coordinates (y-down, origin at top-left of primary).
		// Convert to NSScreen coordinates (y-up, origin at bottom-left of primary).
		CGPoint pt = CGPointMake(x, primaryHeight - y);

		NSScreen *targetScreen = nil;
		for (NSScreen *screen in [NSScreen screens]) {
			if (NSPointInRect(pt, [screen frame])) {
				targetScreen = screen;
				break;
			}
		}

		if (!targetScreen) {
			targetScreen = primary;
		}

		NSRect frame = [targetScreen visibleFrame];
		double *result = (double *)malloc(4 * sizeof(double));
		if (!result)
			return NULL;

		result[0] = frame.origin.x;
		result[1] = frame.origin.y;
		result[2] = frame.size.width;
		result[3] = frame.size.height;

		return result;
	}
}

bool MimiTiledWindowMarginsEnabled(void) {
	@autoreleasepool {
		// CFPreferences is the most reliable way to read other app's preferences
		Boolean keyExists = false;
		Boolean enabled = CFPreferencesGetAppBooleanValue(
		    CFSTR("EnableTiledWindowMargins"), CFSTR("com.apple.WindowManager"), &keyExists);

		if (keyExists) {
			return (bool)enabled;
		}

		// Key does not exist — default to enabled (macOS 14+ behavior)
		return true;
	}
}

double MimiTiledWindowMarginSize(void) {
	@autoreleasepool {
		// macOS Sequoia (15+) added a configurable TiledWindowSpacing key.
		CFPropertyListRef val =
		    CFPreferencesCopyAppValue(CFSTR("TiledWindowSpacing"), CFSTR("com.apple.WindowManager"));
		if (val) {
			if (CFGetTypeID(val) == CFNumberGetTypeID()) {
				double spacing = 0;
				if (CFNumberGetValue((CFNumberRef)val, kCFNumberDoubleType, &spacing) && spacing > 0) {
					CFRelease(val);
					return spacing;
				}
			}
			CFRelease(val);
		}

		// Default margin: 8px (macOS standard on both Sonoma and Sequoia)
		return 8.0;
	}
}
