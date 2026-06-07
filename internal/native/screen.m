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
