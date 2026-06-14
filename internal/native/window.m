//
//  window.m
//  mimi
//

#import "mimi.h"

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>

static NSSet<NSNumber *> *mimiVisibleRegularAppPIDs(void) {
	CFArrayRef windowList = CGWindowListCopyWindowInfo(
	    kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!windowList)
		return [NSSet set];

	NSMutableSet<NSNumber *> *pids = [NSMutableSet set];
	CFIndex count = CFArrayGetCount(windowList);
	for (CFIndex i = 0; i < count; i++) {
		CFDictionaryRef info = CFArrayGetValueAtIndex(windowList, i);
		if (!info)
			continue;

		CFNumberRef layerRef = CFDictionaryGetValue(info, kCGWindowLayer);
		if (!layerRef)
			continue;

		int layer = 0;
		if (!CFNumberGetValue(layerRef, kCFNumberIntType, &layer) || layer != 0)
			continue;

		CFNumberRef pidRef = CFDictionaryGetValue(info, kCGWindowOwnerPID);
		if (!pidRef)
			continue;

		int pid = 0;
		if (!CFNumberGetValue(pidRef, kCFNumberIntType, &pid) || pid <= 0)
			continue;

		[pids addObject:@(pid)];
	}

	CFRelease(windowList);
	return [pids copy];
}

void *MimiGetFrontmostWindow(void) {
	@autoreleasepool {
		AXUIElementRef focusedApp = (AXUIElementRef)MimiGetFocusedApplication();
		AXUIElementRef appRef = focusedApp;
		bool shouldReleaseAppRef = false;

		if (!appRef) {
			NSRunningApplication *front = [NSWorkspace sharedWorkspace].frontmostApplication;
			if (!front)
				return NULL;

			pid_t pid = front.processIdentifier;
			appRef = AXUIElementCreateApplication(pid);
			if (!appRef)
				return NULL;

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
			if (shouldReleaseAppRef && appRef)
				CFRelease(appRef);
			else if (focusedApp)
				CFRelease(focusedApp);
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

		if (windowValues)
			CFRelease(windowValues);

		if (shouldReleaseAppRef && appRef) {
			CFRelease(appRef);
		}

		if (window) {
			if (focusedApp)
				CFRelease(focusedApp);
			if (windows)
				CFRelease(windows);
			return (void *)window;
		}

		if (windows && CFArrayGetCount(windows) > 0) {
			AXUIElementRef firstWindow = (AXUIElementRef)CFArrayGetValueAtIndex(windows, 0);
			CFRetain(firstWindow);
			CFRelease(windows);
			if (focusedApp)
				CFRelease(focusedApp);
			return (void *)firstWindow;
		}

		if (windows)
			CFRelease(windows);

		if (focusedApp)
			CFRelease(focusedApp);

		return NULL;
	}
}

static CGPoint getWindowPosition(AXUIElementRef window) {
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

// Internal helper: returns CFArrayRef of focusable windows on the active
// space. On success, sets *outCount to the number of windows and (if
// requested) *outFocusedIndex to the 0-based index of the focused window, or
// -1 if no window is focused / none matches.
static CFArrayRef mimiCollectFocusableWindowsOnActiveSpace(int *outCount, int *outFocusedIndex) {
	if (!outCount)
		return NULL;

	@autoreleasepool {
		*outCount = 0;
		if (outFocusedIndex)
			*outFocusedIndex = -1;

		NSSet<NSNumber *> *visiblePIDs = mimiVisibleRegularAppPIDs();
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
		if (!windowsCollector)
			return NULL;

		// Track the focused window in app-element form so we can match it
		// against the collected list in a single pass after enumeration.
		// Uses CFEqual (via MimiAreElementsEqual) for matching since
		// AXUIElementRef equality is based on the underlying CFType, not
		// pointer identity.
		AXUIElementRef focusedWindow = NULL;
		AXUIElementRef focusedApp = (AXUIElementRef)MimiGetFocusedApplication();
		if (focusedApp) {
			CFTypeRef focusedVal = NULL;
			if (AXUIElementCopyAttributeValue(focusedApp, kAXFocusedWindowAttribute, &focusedVal) == kAXErrorSuccess &&
			    focusedVal) {
				if (CFGetTypeID(focusedVal) == AXUIElementGetTypeID()) {
					focusedWindow = (AXUIElementRef)CFRetain(focusedVal);
				}
				CFRelease(focusedVal);
			}
			CFRelease(focusedApp);
		}

		for (NSRunningApplication *app in runningApps) {
			if (app.activationPolicy != NSApplicationActivationPolicyRegular)
				continue;
			if (app.hidden)
				continue;

			pid_t pid = app.processIdentifier;
			if (![visiblePIDs containsObject:@(pid)])
				continue;

			AXUIElementRef appElement = AXUIElementCreateApplication(pid);
			if (!appElement)
				continue;

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
				if (!window)
					continue;

				CFStringRef attrs[] = {
				    kAXRoleAttribute,
				    kAXMinimizedAttribute,
				    CFSTR("AXWindowIsOnActiveSpace"),
				};
				CFArrayRef attrArray = CFArrayCreate(NULL, (const void **)attrs, 3, &kCFTypeArrayCallBacks);
				if (!attrArray)
					continue;

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

		// After enumeration, find the focused window's position in the
		// collected list via CFEqual (matches the Go-side equality check
		// used by window.Element.Equal).
		if (outFocusedIndex && focusedWindow) {
			CFIndex total = CFArrayGetCount(windowsCollector);
			for (CFIndex i = 0; i < total; i++) {
				AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windowsCollector, i);
				if (CFEqual(w, focusedWindow)) {
					*outFocusedIndex = (int)i;
					break;
				}
			}
		}
		if (focusedWindow) {
			CFRelease(focusedWindow);
		}

		*outCount = (int)CFArrayGetCount(windowsCollector);
		if (*outCount == 0) {
			CFRelease(windowsCollector);
			return NULL;
		}

		return windowsCollector;
	}
}

void **MimiGetAllFocusableWindowsOnActiveSpace(int *count) {
	return MimiGetAllFocusableWindowsOnActiveSpaceWithFocused(count, NULL);
}

void **MimiGetAllFocusableWindowsOnActiveSpaceWithFocused(int *count, int *focusedIndex) {
	if (!count)
		return NULL;
	if (focusedIndex)
		*focusedIndex = -1;

	@autoreleasepool {
		CFArrayRef windowsCollector = mimiCollectFocusableWindowsOnActiveSpace(count, focusedIndex);
		if (!windowsCollector)
			return NULL;

		CFIndex total = *count;
		NSMutableDictionary<NSValue *, NSValue *> *positions = [NSMutableDictionary dictionaryWithCapacity:total];
		NSMutableDictionary<NSValue *, NSNumber *> *pids = [NSMutableDictionary dictionaryWithCapacity:total];
		for (CFIndex i = 0; i < total; i++) {
			AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windowsCollector, i);
			CGPoint pos = getWindowPosition(w);
			positions[[NSValue valueWithPointer:w]] = [NSValue valueWithBytes:&pos objCType:@encode(CGPoint)];

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

			    if (p1.y < p2.y)
				    return NSOrderedAscending;
			    if (p1.y > p2.y)
				    return NSOrderedDescending;
			    if (p1.x < p2.x)
				    return NSOrderedAscending;
			    if (p1.x > p2.x)
				    return NSOrderedDescending;

			    int pid1 = [pids[key1] intValue];
			    int pid2 = [pids[key2] intValue];
			    if (pid1 < pid2)
				    return NSOrderedAscending;
			    if (pid1 > pid2)
				    return NSOrderedDescending;

			    return NSOrderedSame;
		    }];

		void **result = (void **)malloc(total * sizeof(void *));
		if (!result) {
			CFRelease(windowsCollector);
			return NULL;
		}

		// Sort focusedIndex to match the new sorted order, since the
		// focused window's index in the returned array is what callers
		// will use to identify "the current window."
		int sortedFocusedIndex = -1;
		if (focusedIndex && *focusedIndex >= 0) {
			AXUIElementRef focusedWin =
			    (AXUIElementRef)CFArrayGetValueAtIndex(windowsCollector, (CFIndex)*focusedIndex);
			for (CFIndex i = 0; i < total; i++) {
				AXUIElementRef w = (__bridge AXUIElementRef)sortedWindows[i];
				if (CFEqual(w, focusedWin)) {
					sortedFocusedIndex = (int)i;
					break;
				}
			}
		}

		for (CFIndex i = 0; i < total; i++) {
			result[i] = (void *)(__bridge AXUIElementRef)sortedWindows[i];
			CFRetain(result[i]);
		}

		CFRelease(windowsCollector);

		if (focusedIndex)
			*focusedIndex = sortedFocusedIndex;

		return result;
	}
}

double *MimiGetWindowFrame(void *window) {
	if (!window)
		return NULL;

	@autoreleasepool {
		AXUIElementRef axWindow = (AXUIElementRef)window;

		double *result = (double *)malloc(4 * sizeof(double));
		if (!result)
			return NULL;

		result[0] = 0;
		result[1] = 0;
		result[2] = 0;
		result[3] = 0;

		CFTypeRef positionValue = NULL;
		AXError posError = AXUIElementCopyAttributeValue(axWindow, kAXPositionAttribute, &positionValue);
		if (posError == kAXErrorSuccess && positionValue) {
			CGPoint point;
			if (AXValueGetValue((AXValueRef)positionValue, kAXValueCGPointType, &point)) {
				result[0] = point.x;
				result[1] = point.y;
			}
			CFRelease(positionValue);
		}

		CFTypeRef sizeValue = NULL;
		AXError sizeError = AXUIElementCopyAttributeValue(axWindow, kAXSizeAttribute, &sizeValue);
		if (sizeError == kAXErrorSuccess && sizeValue) {
			CGSize size;
			if (AXValueGetValue((AXValueRef)sizeValue, kAXValueCGSizeType, &size)) {
				result[2] = size.width;
				result[3] = size.height;
			}
			CFRelease(sizeValue);
		}

		return result;
	}
}

int MimiSetWindowFrame(void *window, double x, double y, double w, double h) {
	if (!window)
		return 0;

	@autoreleasepool {
		AXUIElementRef axWindow = (AXUIElementRef)window;

		// Set position first to avoid size changes shifting the window
		CGPoint point = CGPointMake((CGFloat)x, (CGFloat)y);
		AXValueRef positionValue = AXValueCreate(kAXValueCGPointType, &point);
		if (!positionValue)
			return 0;

		AXError posError = AXUIElementSetAttributeValue(axWindow, kAXPositionAttribute, positionValue);

		// Then set size
		CGSize size = CGSizeMake((CGFloat)w, (CGFloat)h);
		AXValueRef sizeValue = AXValueCreate(kAXValueCGSizeType, &size);
		if (!sizeValue) {
			CFRelease(positionValue);
			return 0;
		}

		AXError sizeError = AXUIElementSetAttributeValue(axWindow, kAXSizeAttribute, sizeValue);

		// Re-set position to correct any shifts caused by resize
		AXUIElementSetAttributeValue(axWindow, kAXPositionAttribute, positionValue);

		CFRelease(sizeValue);
		CFRelease(positionValue);

		return (posError == kAXErrorSuccess && sizeError == kAXErrorSuccess) ? 1 : 0;
	}
}

int MimiActivateWindow(void *window) {
	if (!window)
		return 0;

	@autoreleasepool {
		AXUIElementRef axWindow = (AXUIElementRef)window;

		pid_t pid;
		if (AXUIElementGetPid(axWindow, &pid) != kAXErrorSuccess)
			return 0;

		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		if (!app)
			return 0;

		[app activateWithOptions:0];

		AXUIElementSetAttributeValue(axWindow, kAXMainAttribute, kCFBooleanTrue);
		AXUIElementSetAttributeValue(axWindow, kAXFocusedAttribute, kCFBooleanTrue);

		AXError raiseError = AXUIElementPerformAction(axWindow, kAXRaiseAction);

		return (raiseError == kAXErrorSuccess) ? 1 : 0;
	}
}
