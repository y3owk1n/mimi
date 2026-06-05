//
//  space.m
//  Mimi
//
//  Implementation of the space/window CGo bridge. Cherry-picked
//  from neru/internal/core/infra/platform/darwin/ and renamed
//  from Neru* to Mimi* identifiers. macOS-only by design; mimi
//  is a macOS-only project.
//

#include "space.h"

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreFoundation/CoreFoundation.h>
#import <Foundation/Foundation.h>
#import <dispatch/dispatch.h>
#import <mach-o/dyld.h>
#import <mach-o/loader.h>
#import <mach-o/nlist.h>
#import <objc/message.h>
#import <objc/objc.h>
#import <objc/runtime.h>

#pragma mark - Private SkyLight / WindowServer Declarations

extern int SLSMainConnectionID(void);
extern CFArrayRef SLSCopyManagedDisplaySpaces(int cid);
extern CFStringRef SLSCopyManagedDisplayForSpace(int cid, uint64_t sid);
extern uint64_t SLSManagedDisplayGetCurrentSpace(int cid, CFStringRef uuid);
extern CGError SLSSetActiveMenuBarDisplayIdentifier(int cid, CFStringRef uuid, CFStringRef repeat_uuid);
extern CGError SLSGetCurrentCursorLocation(int cid, CGPoint *point);
extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *out);
extern CGError SLSMoveWindowsToManagedSpace(int cid, CFArrayRef window_list, uint64_t sid);

#pragma mark - Cocoa Bootstrap

static dispatch_once_t kMimiCocoaInitOnce = 0;
static bool kMimiCocoaInitialized = false;

void MimiInitCocoaApp(void) {
	dispatch_once(&kMimiCocoaInitOnce, ^{
		@autoreleasepool {
			[NSApplication sharedApplication];
			[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		}
		kMimiCocoaInitialized = true;
	});
}

#pragma mark - Element Reference Lifecycle

void MimiReleaseElement(void *element) {
	if (element) {
		CFRelease((AXUIElementRef)element);
	}
}

#pragma mark - Window Enumeration Helpers

/// Sort-friendly key: store a CGPoint by value (NSPoint structs
/// are interchangeable with CGPoint for our purposes).
static NSValue *mimiPositionKey(CGPoint point) {
	NSValue *value = [[NSValue alloc] initWithBytes:&point objCType:@encode(CGPoint)];

	return value;
}

static CGPoint mimiGetWindowPosition(AXUIElementRef window) {
	CFTypeRef positionValue = NULL;
	if (AXUIElementCopyAttributeValue(window, kAXPositionAttribute, &positionValue) == kAXErrorSuccess &&
	    positionValue) {
		CGPoint point = CGPointZero;
		if (CFGetTypeID(positionValue) == AXValueGetTypeID()) {
			AXValueGetValue((AXValueRef)positionValue, kAXValueCGPointType, &point);
		}
		CFRelease(positionValue);

		return point;
	}

	return CGPointZero;
}

#pragma mark - Window Enumeration (focus_window)

void **MimiGetAllFocusableWindowsOnActiveSpace(int *count) {
	if (!count) {
		return NULL;
	}

	@autoreleasepool {
		*count = 0;

		NSArray *runningApps = [[NSWorkspace sharedWorkspace].runningApplications
		    sortedArrayUsingComparator:^NSComparisonResult(NSRunningApplication *obj1, NSRunningApplication *obj2) {
			    if (obj1.processIdentifier < obj2.processIdentifier) {
				    return NSOrderedAscending;
			    } else if (obj1.processIdentifier > obj2.processIdentifier) {
				    return NSOrderedDescending;
			    }

			    return NSOrderedSame;
		    }];
		CFMutableArrayRef windowsCollector = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
		if (!windowsCollector) {
			return NULL;
		}

		for (NSRunningApplication *app in runningApps) {
			if (app.activationPolicy != NSApplicationActivationPolicyRegular) {
				continue;
			}
			if (app.hidden) {
				continue;
			}

			pid_t pid = app.processIdentifier;
			AXUIElementRef appElement = AXUIElementCreateApplication(pid);
			if (!appElement) {
				continue;
			}

			CFTypeRef windowsValue = NULL;
			AXError error = AXUIElementCopyAttributeValue(appElement, kAXWindowsAttribute, &windowsValue);
			if (error != kAXErrorSuccess || !windowsValue) {
				CFRelease(appElement);

				continue;
			}

			if (CFGetTypeID(windowsValue) != CFArrayGetTypeID()) {
				CFRelease(windowsValue);
				CFRelease(appElement);

				continue;
			}

			CFArrayRef windows = (CFArrayRef)windowsValue;
			CFIndex windowCount = CFArrayGetCount(windows);

			for (CFIndex i = 0; i < windowCount; i++) {
				AXUIElementRef window = (AXUIElementRef)CFArrayGetValueAtIndex(windows, i);
				if (!window) {
					continue;
				}

				CFStringRef attrs[] = {
				    kAXRoleAttribute,
				    kAXMinimizedAttribute,
				    CFSTR("AXWindowIsOnActiveSpace"),
				};
				CFArrayRef attrArray = CFArrayCreate(NULL, (const void **)attrs, 3, &kCFTypeArrayCallBacks);
				if (!attrArray) {
					continue;
				}

				CFArrayRef values = NULL;
				AXUIElementCopyMultipleAttributeValues(window, attrArray, 0, &values);
				CFRelease(attrArray);

				if (!values) {
					continue;
				}

				bool shouldInclude = false;

				if (CFArrayGetCount(values) > 0) {
					CFTypeRef roleVal = (CFTypeRef)CFArrayGetValueAtIndex(values, 0);
					if (roleVal && CFGetTypeID(roleVal) == CFStringGetTypeID() &&
					    CFStringCompare((CFStringRef)roleVal, CFSTR("AXWindow"), 0) == kCFCompareEqualTo) {
						shouldInclude = true;
					}
				}

				if (shouldInclude && CFArrayGetCount(values) > 1) {
					CFTypeRef minVal = (CFTypeRef)CFArrayGetValueAtIndex(values, 1);
					if (minVal && CFGetTypeID(minVal) == CFBooleanGetTypeID() &&
					    CFBooleanGetValue((CFBooleanRef)minVal)) {
						shouldInclude = false;
					}
				}

				// AXWindowIsOnActiveSpace: exclude if explicitly false
				// (gracefully include when the attribute is unsupported).
				if (shouldInclude && CFArrayGetCount(values) > 2) {
					CFTypeRef spaceVal = (CFTypeRef)CFArrayGetValueAtIndex(values, 2);
					if (spaceVal && CFGetTypeID(spaceVal) == CFBooleanGetTypeID() &&
					    !CFBooleanGetValue((CFBooleanRef)spaceVal)) {
						shouldInclude = false;
					}
				}

				CFRelease(values);

				if (shouldInclude) {
					CFArrayAppendValue(windowsCollector, window);
				}
			}

			CFRelease(windowsValue);
			CFRelease(appElement);
		}

		CFIndex total = CFArrayGetCount(windowsCollector);
		if (total == 0) {
			CFRelease(windowsCollector);

			return NULL;
		}

		// Pre-compute positions and PIDs once so the sort comparator
		// doesn't trigger fresh AX round-trips per comparison.
		NSMutableDictionary<NSValue *, NSValue *> *positions = [NSMutableDictionary dictionaryWithCapacity:total];
		NSMutableDictionary<NSValue *, NSNumber *> *pids = [NSMutableDictionary dictionaryWithCapacity:total];
		for (CFIndex i = 0; i < total; i++) {
			AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windowsCollector, i);
			CGPoint pos = mimiGetWindowPosition(w);
			positions[[NSValue valueWithPointer:w]] = mimiPositionKey(pos);

			pid_t pid = 0;
			AXUIElementGetPid(w, &pid);
			pids[[NSValue valueWithPointer:w]] = @(pid);
		}

		NSArray *sortedWindows =
		    [(__bridge NSArray *)windowsCollector sortedArrayUsingComparator:^NSComparisonResult(id obj1, id obj2) {
			    AXUIElementRef w1 = (__bridge AXUIElementRef)obj1;
			    AXUIElementRef w2 = (__bridge AXUIElementRef)obj2;

			    NSValue *key1 = [NSValue valueWithPointer:w1];
			    NSValue *key2 = [NSValue valueWithPointer:w2];

			    CGPoint p1 = CGPointZero;
			    CGPoint p2 = CGPointZero;
			    [positions[key1] getValue:&p1];
			    [positions[key2] getValue:&p2];

			    if (p1.y < p2.y) {
				    return NSOrderedAscending;
			    }
			    if (p1.y > p2.y) {
				    return NSOrderedDescending;
			    }
			    if (p1.x < p2.x) {
				    return NSOrderedAscending;
			    }
			    if (p1.x > p2.x) {
				    return NSOrderedDescending;
			    }

			    int pid1 = [pids[key1] intValue];
			    int pid2 = [pids[key2] intValue];
			    if (pid1 < pid2) {
				    return NSOrderedAscending;
			    }
			    if (pid1 > pid2) {
				    return NSOrderedDescending;
			    }

			    return NSOrderedSame;
		    }];

		void **result = (void **)malloc(total * sizeof(void *));
		if (!result) {
			CFRelease(windowsCollector);

			return NULL;
		}

		for (CFIndex i = 0; i < total; i++) {
			result[i] = (void *)(__bridge AXUIElementRef)sortedWindows[i];
			CFRetain(result[i]);
		}

		CFRelease(windowsCollector);
		*count = (int)total;

		return result;
	}
}

void *MimiGetFrontmostWindow(void) {
	@autoreleasepool {
		// Resolve the focused application.
		AXUIElementRef sysWide = AXUIElementCreateSystemWide();
		AXUIElementRef focusedApp = NULL;
		if (sysWide) {
			AXUIElementCopyAttributeValue(sysWide, kAXFocusedApplicationAttribute, (CFTypeRef *)&focusedApp);
			CFRelease(sysWide);
		}

		AXUIElementRef appRef = focusedApp;
		bool shouldReleaseAppRef = false;

		if (!appRef) {
			NSRunningApplication *front = [NSWorkspace sharedWorkspace].frontmostApplication;
			if (!front) {
				return NULL;
			}

			pid_t pid = front.processIdentifier;
			appRef = AXUIElementCreateApplication(pid);
			if (!appRef) {
				return NULL;
			}

			shouldReleaseAppRef = true;
		}

		CFArrayRef windowAttrs = CFArrayCreate(
		    NULL,
		    (CFTypeRef[]){
		        kAXFocusedWindowAttribute,
		        kAXWindowsAttribute,
		    },
		    2, &kCFTypeArrayCallBacks);
		if (!windowAttrs) {
			if (shouldReleaseAppRef && appRef) {
				CFRelease(appRef);
			} else if (focusedApp) {
				CFRelease(focusedApp);
			}

			return NULL;
		}

		CFArrayRef windowValues = NULL;
		AXError batchError = AXUIElementCopyMultipleAttributeValues(appRef, windowAttrs, 0, &windowValues);
		CFRelease(windowAttrs);

		AXUIElementRef window = NULL;
		CFArrayRef windows = NULL;

		if (batchError == kAXErrorSuccess && windowValues && CFArrayGetCount(windowValues) >= 2) {
			CFTypeRef focusedVal = (CFTypeRef)CFArrayGetValueAtIndex(windowValues, 0);
			if (focusedVal && CFGetTypeID(focusedVal) != CFNullGetTypeID()) {
				window = (AXUIElementRef)focusedVal;
				CFRetain(window);
			}

			CFTypeRef windowsVal = (CFTypeRef)CFArrayGetValueAtIndex(windowValues, 1);
			if (windowsVal && CFGetTypeID(windowsVal) == CFArrayGetTypeID()) {
				windows = (CFArrayRef)windowsVal;
				CFRetain(windows);
			}
		}

		if (windowValues) {
			CFRelease(windowValues);
		}

		if (shouldReleaseAppRef && appRef) {
			CFRelease(appRef);
		}

		if (window) {
			if (focusedApp) {
				CFRelease(focusedApp);
			}
			if (windows) {
				CFRelease(windows);
			}

			return (void *)window;
		}

		if (windows && CFArrayGetCount(windows) > 0) {
			AXUIElementRef firstWindow = (AXUIElementRef)CFArrayGetValueAtIndex(windows, 0);
			CFRetain(firstWindow);
			CFRelease(windows);
			if (focusedApp) {
				CFRelease(focusedApp);
			}

			return (void *)firstWindow;
		}

		if (windows) {
			CFRelease(windows);
		}

		if (focusedApp) {
			CFRelease(focusedApp);
		}

		return NULL;
	}
}

int MimiActivateWindow(void *window) {
	if (!window) {
		return 0;
	}

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	@autoreleasepool {
		AXUIElementRef axWindow = (AXUIElementRef)window;

		pid_t pid;
		if (AXUIElementGetPid(axWindow, &pid) != kAXErrorSuccess) {
			return 0;
		}

		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		if (!app) {
			return 0;
		}

		// NSApplicationActivateIgnoringOtherApps is deprecated in macOS 14+
		// but remains the only public way to programmatically activate an
		// app from a non-bundled CLI process. Suppress the warning locally.
		[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];

		AXUIElementSetAttributeValue(axWindow, kAXMainAttribute, kCFBooleanTrue);
		AXUIElementSetAttributeValue(axWindow, kAXFocusedAttribute, kCFBooleanTrue);

		AXError raiseError = AXUIElementPerformAction(axWindow, kAXRaiseAction);

		return (raiseError == kAXErrorSuccess) ? 1 : 0;
	}
#pragma clang diagnostic pop
}

int MimiAreElementsEqual(void *element1, void *element2) {
	if (!element1 || !element2) {
		return element1 == element2;
	}

	return CFEqual((AXUIElementRef)element1, (AXUIElementRef)element2) ? 1 : 0;
}

#pragma mark - Mission Control Detection (space / move_window_to_space)

/// Hybrid Mission Control detector used by both space and
/// move_window_to_space.
///
/// macOS does not expose a public API to query the Mission Control
/// state, so we inspect the window list. The "Mission Control" app
/// surfaces its window on macOS 13 and earlier; Sonoma/Sequoia/Tahoe
/// instead expose a Dock overlay at higher CGWindowLayer values.
static bool mimiDetectMissionControlActive(void) {
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
			if (!windowInfo) {
				continue;
			}

			CFStringRef ownerName = (CFStringRef)CFDictionaryGetValue(windowInfo, kCGWindowOwnerName);
			if (!ownerName) {
				continue;
			}

			if (CFStringCompare(ownerName, CFSTR("Mission Control"), 0) == kCFCompareEqualTo) {
				CFRelease(windowList);

				return YES;
			}

			if (CFStringCompare(ownerName, CFSTR("Dock"), 0) != kCFCompareEqualTo) {
				continue;
			}

			CFNumberRef windowLayer = (CFNumberRef)CFDictionaryGetValue(windowInfo, kCGWindowLayer);
			if (!windowLayer) {
				continue;
			}

			int layer = 0;
			CFNumberGetValue(windowLayer, CFNumberGetType(windowLayer), &layer);

			// Layers 18-20: Dock MC overlays on macOS 14 Sonoma
			if (layer >= 18 && layer <= 20) {
				dockHighLayerWindows++;
				if (dockHighLayerWindows >= 2) {
					CFRelease(windowList);

					return YES;
				}
			}

			// Layers 14-25: broader range covering macOS 15 Sequoia/Tahoe
			// where the window manager may use different layers.
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

bool MimiIsMissionControlActive(void) { return mimiDetectMissionControlActive(); }

#pragma mark - Display / Space Helpers (space)

/// Translate a display ID to its UUID string.
/// @return Retained CFStringRef (caller must CFRelease), or NULL on failure
static CFStringRef mimiDisplayUUID(uint32_t did) {
	CFUUIDRef uuidRef = CGDisplayCreateUUIDFromDisplayID(did);
	if (!uuidRef) {
		return NULL;
	}

	CFStringRef uuidStr = CFUUIDCreateString(NULL, uuidRef);
	CFRelease(uuidRef);

	return uuidStr;
}

/// Get the current Mission Control space for a display.
/// @return Space ID, or 0 on failure
static uint64_t mimiDisplaySpaceID(uint32_t did) {
	CFStringRef uuid = mimiDisplayUUID(did);
	if (!uuid) {
		return 0;
	}

	uint64_t sid = SLSManagedDisplayGetCurrentSpace(SLSMainConnectionID(), uuid);
	CFRelease(uuid);

	return sid;
}

/// Return the center point of a display's bounds (in CG coordinates).
static CGPoint mimiDisplayCenter(uint32_t did) {
	CGRect bounds = CGDisplayBounds(did);

	return (CGPoint){bounds.origin.x + bounds.size.width / 2.0, bounds.origin.y + bounds.size.height / 2.0};
}

/// Return the display ID that currently contains the cursor.
static uint32_t mimiCursorDisplayID(void) {
	CGPoint cursor;
	SLSGetCurrentCursorLocation(SLSMainConnectionID(), &cursor);

	uint32_t matchingDisplays[16];
	uint32_t matchingCount = 0;

	CGError err = CGGetDisplaysWithPoint(cursor, 16, matchingDisplays, &matchingCount);
	if (err == kCGErrorSuccess && matchingCount > 0) {
		return matchingDisplays[0];
	}

	// Fall back to the main display if the cursor is somehow outside every display.
	return CGMainDisplayID();
}

/// Set the active menu bar display, which updates which display is the
/// "focused" one for Space purposes.
static void mimiSetActiveMenuBarDisplay(uint32_t did) {
	CFStringRef uuid = mimiDisplayUUID(did);
	if (!uuid) {
		return;
	}

	SLSSetActiveMenuBarDisplayIdentifier(SLSMainConnectionID(), uuid, uuid);
	CFRelease(uuid);
}

#pragma mark - Mission Control Index Resolution (space)

/// Find the 1-based Mission Control indices of two space IDs in a single
/// pass over SLSCopyManagedDisplaySpaces. Returns false if either sid is
/// not found in the current Mission Control ordering.
static bool mimiResolveMCIndices(uint64_t curSid, uint64_t newSid, int *outCurIndex, int *outNewIndex) {
	*outCurIndex = 0;
	*outNewIndex = 0;

	@autoreleasepool {
		CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
		if (!displaySpaces) {
			return false;
		}

		int counter = 1;

		CFIndex displayCount = CFArrayGetCount(displaySpaces);
		for (CFIndex i = 0; i < displayCount; i++) {
			CFDictionaryRef displayRef = (CFDictionaryRef)CFArrayGetValueAtIndex(displaySpaces, i);
			CFArrayRef spacesRef = (CFArrayRef)CFDictionaryGetValue(displayRef, CFSTR("Spaces"));
			if (!spacesRef) {
				continue;
			}

			CFIndex spacesCount = CFArrayGetCount(spacesRef);
			for (CFIndex j = 0; j < spacesCount; j++) {
				CFDictionaryRef spaceRef = (CFDictionaryRef)CFArrayGetValueAtIndex(spacesRef, j);
				CFNumberRef sidRef = (CFNumberRef)CFDictionaryGetValue(spaceRef, CFSTR("id64"));
				if (!sidRef) {
					continue;
				}

				uint64_t sid = 0;
				CFNumberGetValue(sidRef, CFNumberGetType(sidRef), &sid);

				if (sid == curSid) {
					*outCurIndex = counter;
				}

				if (sid == newSid) {
					*outNewIndex = counter;
				}

				counter++;
			}
		}

		CFRelease(displaySpaces);

		return (*outCurIndex > 0) && (*outNewIndex > 0);
	}
}

#pragma mark - Space Counting and Lookup (space)

int MimiCountMissionControlSpaces(void) {
	@autoreleasepool {
		CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
		if (!displaySpaces) {
			return 0;
		}

		int total = 0;
		CFIndex displayCount = CFArrayGetCount(displaySpaces);
		for (CFIndex i = 0; i < displayCount; i++) {
			CFDictionaryRef displayRef = (CFDictionaryRef)CFArrayGetValueAtIndex(displaySpaces, i);
			CFArrayRef spacesRef = (CFArrayRef)CFDictionaryGetValue(displayRef, CFSTR("Spaces"));
			if (!spacesRef) {
				continue;
			}

			total += (int)CFArrayGetCount(spacesRef);
		}

		CFRelease(displaySpaces);

		return total;
	}
}

uint64_t MimiMissionControlSpaceID(int index) {
	if (index < 1) {
		return 0;
	}

	@autoreleasepool {
		CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
		if (!displaySpaces) {
			return 0;
		}

		uint64_t result = 0;
		int counter = 1;

		CFIndex displayCount = CFArrayGetCount(displaySpaces);
		for (CFIndex i = 0; i < displayCount; i++) {
			CFDictionaryRef displayRef = (CFDictionaryRef)CFArrayGetValueAtIndex(displaySpaces, i);
			CFArrayRef spacesRef = (CFArrayRef)CFDictionaryGetValue(displayRef, CFSTR("Spaces"));
			if (!spacesRef) {
				continue;
			}

			CFIndex spacesCount = CFArrayGetCount(spacesRef);
			for (CFIndex j = 0; j < spacesCount; j++) {
				if (counter == index) {
					CFDictionaryRef spaceRef = (CFDictionaryRef)CFArrayGetValueAtIndex(spacesRef, j);
					CFNumberRef sidRef = (CFNumberRef)CFDictionaryGetValue(spaceRef, CFSTR("id64"));
					if (sidRef) {
						CFNumberGetValue(sidRef, CFNumberGetType(sidRef), &result);
					}

					CFRelease(displaySpaces);

					return result;
				}

				counter++;
			}
		}

		CFRelease(displaySpaces);

		return 0;
	}
}

uint32_t MimiSpaceDisplayID(uint64_t sid) {
	CFStringRef uuid = SLSCopyManagedDisplayForSpace(SLSMainConnectionID(), sid);
	if (!uuid) {
		return 0;
	}

	// Convert UUID string back to numeric display ID. The CFStringRef
	// is also a valid input to CFUUIDCreateFromString, so we can
	// resolve the ID without an extra round trip.
	CFUUIDRef uuidRef = CFUUIDCreateFromString(NULL, uuid);
	uint32_t did = 0;
	if (uuidRef) {
		did = CGDisplayGetDisplayIDFromUUID(uuidRef);
		CFRelease(uuidRef);
	}
	CFRelease(uuid);

	return did;
}

#pragma mark - Gesture-Based Space Focus (space)

// Private Core Graphics event field IDs used to synthesize a
// high-velocity horizontal dock swipe that the Dock treats as a
// real multi-finger swipe gesture. These constants are not part
// of the public SDK and require suppressing
// -Wdeprecated-declarations around the implementation.
static const int kMimiCGSEventTypeField = 55;              // kCGSEventTypeField
static const int kMimiCGSEventDockControl = 30;            // kCGSEventDockControl
static const int kMimiCGEventGestureHIDType = 110;         // kCGEventGestureHIDType
static const int kMimiIOHIDEventTypeDockSwipe = 23;        // kIOHIDEventTypeDockSwipe
static const int kMimiCGEventGestureSwipeProgress = 124;   // kCGEventGestureSwipeProgress
static const int kMimiCGEventGestureSwipeMotion = 123;     // kCGEventGestureSwipeMotion
static const int kMimiCGGestureMotionHorizontal = 1;       // kCGGestureMotionHorizontal
static const int kMimiCGEventGestureSwipeVelocityX = 129;  // kCGEventGestureSwipeVelocityX
static const int kMimiCGEventGesturePhase = 132;           // kCGEventGesturePhase
static const int kMimiCGSGesturePhaseBegan = 1;            // kCGSGesturePhaseBegan
static const int kMimiCGSGesturePhaseEnded = 4;            // kCGSGesturePhaseEnded

int MimiFocusSpaceUsingGesture(uint32_t new_did, uint64_t new_sid) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

	uint32_t curDid = mimiCursorDisplayID();
	uint64_t curSid = mimiDisplaySpaceID(curDid);
	CGPoint point = mimiDisplayCenter(new_did);
	bool focusDisplay = curDid != new_did;

	if (focusDisplay) {
		CGWarpMouseCursorPosition(point);
	}

	int curIndex = 0;
	int newIndex = 0;
	if (!mimiResolveMCIndices(curSid, new_sid, &curIndex, &newIndex)) {
		// Could not resolve Mission Control indices (e.g. transient
		// state). Best-effort fallback: ensure the right display is
		// active so the OS picks the closest matching space on that
		// display.
		mimiSetActiveMenuBarDisplay(new_did);

		return 1;
	}

	int count = abs(newIndex - curIndex);
	if (count == 0) {
		// Already on the same Mission Control index. Make sure the
		// right display is active in case the destination space sits
		// on a different display at the same index.
		if (focusDisplay) {
			mimiSetActiveMenuBarDisplay(new_did);
			if (mimiDisplaySpaceID(new_did) != new_sid) {
				CGPostMouseEvent(point, false, 1, true);
				CGPostMouseEvent(point, false, 1, false);
			}
		}

		return 1;
	}

	CGEventRef event = CGEventCreate(NULL);
	if (!event) {
		return 0;
	}

	double sign = (newIndex - curIndex) > 0 ? 1.0 : -1.0;

	CGEventSetIntegerValueField(event, kMimiCGSEventTypeField, kMimiCGSEventDockControl);
	CGEventSetIntegerValueField(event, kMimiCGEventGestureHIDType, kMimiIOHIDEventTypeDockSwipe);
	CGEventSetIntegerValueField(event, kMimiCGEventGestureSwipeMotion, kMimiCGGestureMotionHorizontal);
	CGEventSetDoubleValueField(event, kMimiCGEventGestureSwipeProgress, sign);
	CGEventSetDoubleValueField(event, kMimiCGEventGestureSwipeVelocityX, sign * 9999.0);

	for (int i = 0; i < count; i++) {
		CGEventSetIntegerValueField(event, kMimiCGEventGesturePhase, kMimiCGSGesturePhaseBegan);
		CGEventPost(kCGSessionEventTap, event);
		CGEventSetIntegerValueField(event, kMimiCGEventGesturePhase, kMimiCGSGesturePhaseEnded);
		CGEventPost(kCGSessionEventTap, event);
	}

	CFRelease(event);

	// Short run loop iteration so the WindowServer connection
	// flushes the posted gesture events to the Dock. The Mach
	// IPC is queued by CGEventPost and needs servicing.
	[NSRunLoop.currentRunLoop runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];

	if (focusDisplay) {
		mimiSetActiveMenuBarDisplay(new_did);
		if (mimiDisplaySpaceID(new_did) != new_sid) {
			CGPostMouseEvent(point, false, 1, true);
			CGPostMouseEvent(point, false, 1, false);
		}
	}

	return 1;

#pragma clang diagnostic pop
}

#pragma mark - Mach-O / Symbol Resolution Helpers (move_window_to_space)

static struct mach_header_64 *mimiMachOFindImageHeader(const char *target_name, uint64_t *slide) {
	uint32_t image_count = _dyld_image_count();
	for (uint32_t i = 0; i < image_count; ++i) {
		const char *image_name = _dyld_get_image_name(i);
		if (!image_name) {
			continue;
		}
		if (strcmp(image_name, target_name) == 0) {
			*slide = _dyld_get_image_vmaddr_slide(i);

			return (struct mach_header_64 *)_dyld_get_image_header(i);
		}
	}

	return NULL;
}

static struct segment_command_64 *mimiMachOFindLinkeditSegment(struct mach_header_64 *header) {
	uint64_t offset = sizeof(struct mach_header_64);
	for (uint32_t i = 0; i < header->ncmds; ++i) {
		struct load_command *cmd = (struct load_command *)(((uint8_t *)header) + offset);
		if (cmd->cmd == LC_SEGMENT_64) {
			struct segment_command_64 *segment = (struct segment_command_64 *)cmd;
			if (strcmp(segment->segname, SEG_LINKEDIT) == 0) {
				return segment;
			}
		}
		offset += cmd->cmdsize;
	}

	return NULL;
}

static struct symtab_command *mimiMachOFindSymtabCommand(struct mach_header_64 *header) {
	uint64_t offset = sizeof(struct mach_header_64);
	for (uint32_t i = 0; i < header->ncmds; ++i) {
		struct load_command *cmd = (struct load_command *)(((uint8_t *)header) + offset);
		if (cmd->cmd == LC_SYMTAB) {
			return (struct symtab_command *)cmd;
		}
		offset += cmd->cmdsize;
	}

	return NULL;
}

static void *mimiMachOFindSymbol(const char *target_image, const char *target_symbol) {
	uint64_t slide = 0;
	struct mach_header_64 *header = mimiMachOFindImageHeader(target_image, &slide);
	if (!header) {
		return NULL;
	}
	struct segment_command_64 *linkedit_segment = mimiMachOFindLinkeditSegment(header);
	if (!linkedit_segment) {
		return NULL;
	}
	struct symtab_command *symtab_command = mimiMachOFindSymtabCommand(header);
	if (!symtab_command) {
		return NULL;
	}
	uint32_t symbol_count = symtab_command->nsyms;
	void *symbol_str = (void *)(linkedit_segment->vmaddr - linkedit_segment->fileoff) + symtab_command->stroff + slide;
	void *symbol_sym = (void *)(linkedit_segment->vmaddr - linkedit_segment->fileoff) + symtab_command->symoff + slide;
	for (uint32_t i = 0; i < symbol_count; ++i) {
		struct nlist_64 *list = (void *)symbol_sym + (i * sizeof(struct nlist_64));
		char *symbol_name = (char *)symbol_str + list->n_un.n_strx;
		if (strcmp(symbol_name, target_symbol) == 0) {
			return (void *)(list->n_value + slide);
		}
	}

	return NULL;
}

#pragma mark - Window-to-Space Movement (move_window_to_space)

@protocol MimiSLSBridgedMoveWindowsToManagedSpaceOperationProtocol <NSObject>
- (instancetype)initWithWindows:(id)windows spaceID:(uint64_t)spaceID;
@end

int MimiMoveWindowToSpace(void *windowElement, uint64_t spaceID) {
	if (!windowElement) {
		return 0;
	}

	CGWindowID windowId = 0;
	AXError err = _AXUIElementGetWindow((AXUIElementRef)windowElement, &windowId);
	if (err != kAXErrorSuccess || windowId == 0) {
		return 0;
	}

	// Wrap the window ID in a single-element CFArray.
	CFNumberRef windowNumber = CFNumberCreate(NULL, kCFNumberSInt32Type, &windowId);
	if (!windowNumber) {
		return 0;
	}
	CFArrayRef windowList = CFArrayCreate(NULL, (const void **)&windowNumber, 1, &kCFTypeArrayCallBacks);
	CFRelease(windowNumber);
	if (!windowList) {
		return 0;
	}

	int success = 0;

	// Resolve SLSPerformAsynchronousBridgedWindowManagementOperation
	// dynamically so we don't have to link the unexported SkyLight
	// symbol at build time.
	static int64_t (*mimiSLSPerformAsyncBridged)(void *) = NULL;
	static dispatch_once_t onceToken;
	dispatch_once(&onceToken, ^{
		mimiSLSPerformAsyncBridged = (int64_t (*)(void *))mimiMachOFindSymbol(
		    "/System/Library/PrivateFrameworks/SkyLight.framework/Versions/A/SkyLight",
		    "__"
		    "ZL54SLSPerformAsynchronousBridgedWindowManagementOperationP47SLSAsynchronousBridgedWindowManagementOperati"
		    "on");
	});

	// Prefer the synchronous path for short-lived CLI processes.
	// The async bridged path is designed for daemon-style processes
	// that stay alive for the WindowServer to complete the operation.
	CGError cgErr = SLSMoveWindowsToManagedSpace(SLSMainConnectionID(), windowList, spaceID);
	if (cgErr == kCGErrorSuccess) {
		// Pump so the WindowServer processes the move.
		[NSRunLoop.currentRunLoop runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
		success = 1;
	} else if (mimiSLSPerformAsyncBridged) {
		Class cls = objc_getClass("SLSBridgedMoveWindowsToManagedSpaceOperation");
		if (cls) {
			id operation = [(id<MimiSLSBridgedMoveWindowsToManagedSpaceOperationProtocol>)[cls alloc]
			    initWithWindows:(__bridge id)windowList
			            spaceID:spaceID];
			if (operation) {
				mimiSLSPerformAsyncBridged((__bridge void *)operation);
				// Pump so the async operation dispatches before exit.
				[NSRunLoop.currentRunLoop runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
				success = 1;
			}
		}
	}

	CFRelease(windowList);

	return success;
}
