//
//  window.m
//  mimi
//

#import "mimi.h"
#import "mimi_log.h"

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>

extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *out);

/// Process identifiers of every process owning an on-screen, layer-0 window,
/// in ascending order. This is the source of the applications to enumerate, so
/// it is deliberately fetched fresh on every call.
static NSArray<NSNumber *> *mimiOnScreenWindowOwnerPIDs(void) {
	CFArrayRef windowList = CGWindowListCopyWindowInfo(
	    kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!windowList)
		return @[];

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
	return [pids.allObjects sortedArrayUsingSelector:@selector(compare:)];
}

void *MimiGetFrontmostWindow(void) {
	@autoreleasepool {
		AXUIElementRef focusedApp = (AXUIElementRef)MimiGetFocusedApplication();
		AXUIElementRef appRef = focusedApp;
		bool shouldReleaseAppRef = false;

		if (!appRef) {
			// Last resort only. NSWorkspace answers this out of state it
			// refreshes from the main thread's run loop, which neither the CLI
			// nor an action-serving daemon thread pumps, so it can name an
			// application that is no longer frontmost. The Accessibility path
			// above is the one that is always current.
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

// The bounds of every on-screen window by number, from the window server.
static NSDictionary<NSNumber *, NSValue *> *mimiOnScreenBounds(void) {
	NSMutableDictionary<NSNumber *, NSValue *> *bounds = [NSMutableDictionary dictionary];
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
	for (NSDictionary *info in (__bridge NSArray *)list) {
		CGRect rect;
		if (CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], &rect)) {
			bounds[info[(id)kCGWindowNumber]] = [NSValue valueWithBytes:&rect objCType:@encode(CGRect)];
		}
	}
	if (list) {
		CFRelease(list);
	}
	return bounds;
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

// The application's windows that are focusable and on the active space, or
// NULL for a process that is not a regular, visible application. The
// window list and each window's attributes are round trips into the
// application.
static CFMutableArrayRef mimiCollectFocusableWindowsOfApplication(pid_t pid) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		if (!app || app.activationPolicy != NSApplicationActivationPolicyRegular || app.hidden)
			return NULL;

		AXUIElementRef appElement = AXUIElementCreateApplication(pid);
		if (!appElement)
			return NULL;

		CFTypeRef windowsValue = NULL;
		AXError error = AXUIElementCopyAttributeValue(appElement, kAXWindowsAttribute, &windowsValue);
		CFRelease(appElement);
		if (error != kAXErrorSuccess || !windowsValue)
			return NULL;
		if (CFGetTypeID(windowsValue) != CFArrayGetTypeID()) {
			CFRelease(windowsValue);
			return NULL;
		}

		CFMutableArrayRef collected = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
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
				if (minVal && CFGetTypeID(minVal) == CFBooleanGetTypeID() && CFBooleanGetValue((CFBooleanRef)minVal)) {
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
				CFArrayAppendValue(collected, window);
			}
		}

		CFRelease(windowsValue);
		return collected;
	}
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

		// The applications to walk are derived from the window list, not from
		// -[NSWorkspace runningApplications]. That array is only refreshed
		// while the *main* thread's run loop runs, and nothing here can
		// guarantee that: the CLI never pumps it, and the daemon pumps it on
		// its own observer thread while actions are served from another. In a
		// process that does not pump it, the array stays frozen at its first
		// read and every application launched afterwards is invisible for the
		// life of the process. CGWindowListCopyWindowInfo carries no such
		// dependency and was already being fetched here anyway.
		//
		// The per-pid NSRunningApplication lookup below is answered live
		// rather than from that array, which is what lets the activation
		// policy and hidden filters stay exactly as they were. Dropping them
		// and taking the window list alone would widen enumeration to
		// processes that own a layer-0 window without being applications —
		// border drawers and overlays, which have no AX window list to walk.
		NSArray<NSNumber *> *ownerPIDs = mimiOnScreenWindowOwnerPIDs();
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

		// Each application is asked on a thread of its own, since a window
		// list is a round trip into the application, and one that is busy,
		// as the one just focused tends to be, answers late. The windows
		// are then collected in application order, as one thread would.
		NSUInteger appCount = ownerPIDs.count;
		CFMutableArrayRef *perApp = calloc(appCount, sizeof(CFMutableArrayRef));
		dispatch_apply(appCount, dispatch_get_global_queue(QOS_CLASS_USER_INTERACTIVE, 0), ^(size_t index) {
			perApp[index] = mimiCollectFocusableWindowsOfApplication((pid_t)ownerPIDs[index].intValue);
		});
		for (NSUInteger index = 0; index < appCount; index++) {
			if (!perApp[index]) {
				continue;
			}
			CFArrayAppendArray(windowsCollector, perApp[index], CFRangeMake(0, CFArrayGetCount(perApp[index])));
			CFRelease(perApp[index]);
		}
		free(perApp);

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
		// A position read through Accessibility is a round trip into the
		// window's application, and one just activated answers late. The
		// window server knows where every window is, in one call for all
		// of them; a window it does not list, one without a number, is
		// asked itself.
		NSDictionary<NSNumber *, NSValue *> *onScreen = mimiOnScreenBounds();
		for (CFIndex i = 0; i < total; i++) {
			AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windowsCollector, i);
			CGWindowID number = 0;
			NSValue *known = nil;
			if (_AXUIElementGetWindow(w, &number) == kAXErrorSuccess && number != 0) {
				known = onScreen[@(number)];
			}
			CGPoint pos;
			if (known) {
				CGRect bounds;
				[known getValue:&bounds];
				pos = bounds.origin;
			} else {
				pos = getWindowPosition(w);
			}
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

int MimiGetWindowPID(void *window) {
	if (!window)
		return 0;

	pid_t pid = 0;
	if (AXUIElementGetPid((AXUIElementRef)window, &pid) != kAXErrorSuccess)
		return 0;

	return (int)pid;
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

double *MimiCopyWindowList(int onScreenOnly, int *count, char ***names) {
	if (!count) {
		return NULL;
	}
	*count = 0;
	if (names) {
		*names = NULL;
	}

	@autoreleasepool {
		CGWindowListOption option = onScreenOnly ? kCGWindowListOptionOnScreenOnly : kCGWindowListOptionAll;
		CFArrayRef list = CGWindowListCopyWindowInfo(option, kCGNullWindowID);
		if (!list) {
			return NULL;
		}
		CFIndex total = CFArrayGetCount(list);
		double *rows = calloc((size_t)total * MIMI_WINDOW_DOUBLES, sizeof(double));
		char **titles = calloc((size_t)total + 1, sizeof(char *));
		// Whether each owner is a regular, visible application, looked up
		// once per owner: it is what makes a window's owner an application
		// rather than a border drawer or an overlay.
		NSMutableDictionary<NSNumber *, NSNumber *> *regular = [NSMutableDictionary dictionary];
		int kept = 0;
		for (NSDictionary *info in (__bridge NSArray *)list) {
			CGRect rect;
			if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], &rect)) {
				continue;
			}
			NSNumber *pid = info[(id)kCGWindowOwnerPID];
			NSNumber *isRegular = regular[pid];
			if (!isRegular) {
				NSRunningApplication *app =
				    [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid.intValue];
				isRegular = @(app && app.activationPolicy == NSApplicationActivationPolicyRegular && !app.hidden);
				regular[pid] = isRegular;
			}
			double *row = rows + (size_t)kept * MIMI_WINDOW_DOUBLES;
			row[0] = [info[(id)kCGWindowNumber] doubleValue];
			row[1] = rect.origin.x;
			row[2] = rect.origin.y;
			row[3] = rect.size.width;
			row[4] = rect.size.height;
			// The name is absent without Screen Recording; "" then.
			NSString *name = info[(id)kCGWindowName];
			row[5] = name != nil;
			row[6] = pid.doubleValue;
			row[7] = [info[(id)kCGWindowLayer] doubleValue];
			row[8] = isRegular.boolValue;
			titles[kept] = strdup(name ? name.UTF8String : "");
			kept++;
		}
		CFRelease(list);
		*count = kept;
		if (names) {
			*names = titles;
		} else {
			for (int i = 0; i < kept; i++) {
				free(titles[i]);
			}
			free(titles);
		}
		return rows;
	}
}

void **MimiCopyApplicationWindowElements(int pid, int *count, unsigned int **numbers, int **windows) {
	if (!count) {
		return NULL;
	}
	*count = 0;

	@autoreleasepool {
		AXUIElementRef appElement = AXUIElementCreateApplication((pid_t)pid);
		if (!appElement) {
			return NULL;
		}
		CFTypeRef value = NULL;
		AXError error = AXUIElementCopyAttributeValue(appElement, kAXWindowsAttribute, &value);
		CFRelease(appElement);
		if (error != kAXErrorSuccess || !value) {
			return NULL;
		}
		if (CFGetTypeID(value) != CFArrayGetTypeID()) {
			CFRelease(value);
			return NULL;
		}
		CFArrayRef list = (CFArrayRef)value;
		CFIndex total = CFArrayGetCount(list);
		void **elements = calloc((size_t)total + 1, sizeof(void *));
		unsigned int *ids = calloc((size_t)total + 1, sizeof(unsigned int));
		int *roles = calloc((size_t)total + 1, sizeof(int));
		int kept = 0;
		for (CFIndex i = 0; i < total; i++) {
			AXUIElementRef window = (AXUIElementRef)CFArrayGetValueAtIndex(list, i);
			if (!window) {
				continue;
			}
			CGWindowID number = 0;
			if (_AXUIElementGetWindow(window, &number) != kAXErrorSuccess || number == 0) {
				continue;
			}
			CFTypeRef role = NULL;
			int isWindow = 0;
			if (AXUIElementCopyAttributeValue(window, kAXRoleAttribute, &role) == kAXErrorSuccess && role) {
				isWindow = CFGetTypeID(role) == CFStringGetTypeID() &&
				           CFStringCompare((CFStringRef)role, CFSTR("AXWindow"), 0) == kCFCompareEqualTo;
				CFRelease(role);
			}
			elements[kept] = (void *)CFRetain(window);
			ids[kept] = number;
			roles[kept] = isWindow;
			kept++;
		}
		CFRelease(value);
		*count = kept;
		*numbers = ids;
		*windows = roles;
		return elements;
	}
}

int MimiSetWindowPosition(void *window, double x, double y) {
	if (!window)
		return 0;

	@autoreleasepool {
		CGPoint point = CGPointMake((CGFloat)x, (CGFloat)y);
		AXValueRef positionValue = AXValueCreate(kAXValueCGPointType, &point);
		if (!positionValue)
			return 0;

		AXError posError = AXUIElementSetAttributeValue((AXUIElementRef)window, kAXPositionAttribute, positionValue);
		CFRelease(positionValue);
		if (posError != kAXErrorSuccess) {
			MIMI_LOG("AXUIElementSetAttributeValue(kAXPositionAttribute) failed with error %d", (int)posError);
			return 0;
		}

		return 1;
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
		if (posError != kAXErrorSuccess) {
			MIMI_LOG("AXUIElementSetAttributeValue(kAXPositionAttribute) failed with error %d", (int)posError);
		}

		// Then set size
		CGSize size = CGSizeMake((CGFloat)w, (CGFloat)h);
		AXValueRef sizeValue = AXValueCreate(kAXValueCGSizeType, &size);
		if (!sizeValue) {
			CFRelease(positionValue);
			return 0;
		}

		AXError sizeError = AXUIElementSetAttributeValue(axWindow, kAXSizeAttribute, sizeValue);
		if (sizeError != kAXErrorSuccess) {
			MIMI_LOG("AXUIElementSetAttributeValue(kAXSizeAttribute) failed with error %d", (int)sizeError);
		}

		// Re-set position to correct any shifts caused by resize
		AXError posResetError = AXUIElementSetAttributeValue(axWindow, kAXPositionAttribute, positionValue);
		if (posResetError != kAXErrorSuccess) {
			MIMI_LOG(
			    "AXUIElementSetAttributeValue(kAXPositionAttribute) reset failed with error %d", (int)posResetError);
		}

		CFRelease(sizeValue);
		CFRelease(positionValue);

		return (posError == kAXErrorSuccess && sizeError == kAXErrorSuccess) ? 1 : 0;
	}
}

uint32_t MimiGetWindowNumber(void *window) {
	if (!window)
		return 0;

	CGWindowID number = 0;
	if (_AXUIElementGetWindow((AXUIElementRef)window, &number) != kAXErrorSuccess)
		return 0;

	return (uint32_t)number;
}

char *MimiCopyWindowTitle(void *window) {
	if (!window)
		return NULL;

	CFTypeRef value = NULL;
	if (AXUIElementCopyAttributeValue((AXUIElementRef)window, kAXTitleAttribute, &value) != kAXErrorSuccess || !value)
		return NULL;

	char *title = NULL;
	if (CFGetTypeID(value) == CFStringGetTypeID()) {
		const char *utf8 = [(__bridge NSString *)value UTF8String];
		if (utf8)
			title = strdup(utf8);
	}
	CFRelease(value);

	return title;
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

		AXError mainErr = AXUIElementSetAttributeValue(axWindow, kAXMainAttribute, kCFBooleanTrue);
		if (mainErr != kAXErrorSuccess) {
			MIMI_LOG("AXUIElementSetAttributeValue(kAXMainAttribute) failed with error %d (pid=%d)", (int)mainErr, pid);
		}
		AXError focusErr = AXUIElementSetAttributeValue(axWindow, kAXFocusedAttribute, kCFBooleanTrue);
		if (focusErr != kAXErrorSuccess) {
			MIMI_LOG(
			    "AXUIElementSetAttributeValue(kAXFocusedAttribute) failed with error %d (pid=%d)", (int)focusErr, pid);
		}

		AXError raiseError = AXUIElementPerformAction(axWindow, kAXRaiseAction);
		if (raiseError != kAXErrorSuccess) {
			MIMI_LOG("AXUIElementPerformAction(kAXRaiseAction) failed with error %d (pid=%d)", (int)raiseError, pid);
		}

		return (raiseError == kAXErrorSuccess) ? 1 : 0;
	}
}
