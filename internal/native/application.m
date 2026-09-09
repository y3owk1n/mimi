//
//  application.m
//  mimi
//

#import "mimi.h"
#import "mimi_log.h"

#import <Cocoa/Cocoa.h>
#import <dlfcn.h>

#pragma mark - SkyLight External Declarations

extern int SLSMainConnectionID(void);
extern CFArrayRef SLSCopyManagedDisplaySpaces(int cid);
extern CFArrayRef SLSCopySpacesForWindows(int cid, int mask, CFArrayRef window_ids);
extern CFArrayRef SLSCopyWindowsWithOptionsAndTags(
    int cid, uint32_t owner, CFArrayRef spaces, uint32_t options, uint64_t *set_tags, uint64_t *clear_tags);
extern CFTypeRef SLSWindowQueryWindows(int cid, CFArrayRef windows, int count);
extern CFTypeRef SLSWindowQueryResultCopyWindows(CFTypeRef query);
extern bool SLSWindowIteratorAdvance(CFTypeRef iterator);
extern uint32_t SLSWindowIteratorGetParentID(CFTypeRef iterator);
extern uint32_t SLSWindowIteratorGetWindowID(CFTypeRef iterator);
extern uint64_t SLSWindowIteratorGetTags(CFTypeRef iterator);
extern uint64_t SLSWindowIteratorGetAttributes(CFTypeRef iterator);
extern int SLSWindowIteratorGetLevel(CFTypeRef iterator);

// The corner radii of the window under an iterator, in points, as macOS 26
// added it. Looked up at first use, since earlier releases have no such
// function.
typedef CFArrayRef (*MimiCornerRadiiFn)(CFTypeRef iterator);

static MimiCornerRadiiFn mimiCornerRadii(void) {
	static MimiCornerRadiiFn fn;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		fn = (MimiCornerRadiiFn)dlsym(RTLD_DEFAULT, "SLSWindowIteratorGetCornerRadii");
	});
	return fn;
}

// The corner radius of the window under iterator, or -1 when unknown.
static double mimiIteratorRadius(CFTypeRef iterator) {
	MimiCornerRadiiFn fn = mimiCornerRadii();
	if (!fn)
		return -1;
	CFArrayRef radii = fn(iterator);
	if (!radii)
		return -1;
	double radius = -1;
	if (CFArrayGetCount(radii) > 0) {
		CFNumberRef value = CFArrayGetValueAtIndex(radii, 0);
		if (value)
			CFNumberGetValue(value, kCFNumberDoubleType, &radius);
	}
	CFRelease(radii);
	return radius;
}

extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *out);

/// Every space a window can be on: current, other, and full-screen ones.
static const int kMimiAllSpacesMask = 0x7;

/// SLSCopyWindowsWithOptionsAndTags option: windows that are not minimized.
static const uint32_t kMimiWindowsNotMinimized = 0x2;

// The window server's own notion of a real window, as yabai reads it: a
// top-level window (no parent) at a normal level, ordered in, and either a
// document-style window or one an application asked to be treated as such.
// The tags and attributes are undocumented bit fields; the tests below are
// the ones yabai has kept working across macOS releases.
static const uint64_t kMimiAttributeReal = 0x2;
static const uint64_t kMimiTagReal = 0x400000000000000;
static const uint64_t kMimiTagOrderedIn = 0x1;
static const uint64_t kMimiTagStandard = 0x2;
static const uint64_t kMimiTagVisible = 0x80000000;

static bool mimiWindowLevelIsNormal(int level) { return level == 0 || level == 3 || level == 8; }

static bool mimiWindowIsReal(uint64_t tags, uint64_t attributes, uint32_t parent, int level) {
	if (parent != 0 || !mimiWindowLevelIsNormal(level))
		return false;
	bool documentLike = (attributes & kMimiAttributeReal) || (tags & kMimiTagReal);
	bool shown = (tags & kMimiTagOrderedIn) || ((tags & kMimiTagStandard) && (tags & kMimiTagVisible));
	return documentLike && shown;
}

/// Every Mission Control space id on every display, full-screen ones included.
static NSArray<NSNumber *> *mimiAllSpaceIDs(void) {
	NSMutableArray<NSNumber *> *ids = [NSMutableArray array];
	CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
	if (!displaySpaces)
		return ids;

	for (NSDictionary *display in (__bridge NSArray *)displaySpaces) {
		for (NSDictionary *space in display[@"Spaces"]) {
			NSNumber *sid = space[@"id64"];
			if (sid)
				[ids addObject:sid];
		}
	}

	CFRelease(displaySpaces);
	return ids;
}

CFArrayRef MimiCopyRealWindowsOnSpaces(CFArrayRef spaceIDs, CFArrayRef *radii) {
	NSMutableArray<NSNumber *> *real = [NSMutableArray array];
	NSMutableArray<NSNumber *> *corners = [NSMutableArray array];
	if (radii)
		*radii = CFBridgingRetain(corners);
	if (!spaceIDs || CFArrayGetCount(spaceIDs) == 0)
		return CFBridgingRetain(real);

	uint64_t setTags = 0;
	uint64_t clearTags = 0;
	CFArrayRef windows = SLSCopyWindowsWithOptionsAndTags(
	    SLSMainConnectionID(), 0, spaceIDs, kMimiWindowsNotMinimized, &setTags, &clearTags);
	if (!windows)
		return CFBridgingRetain(real);

	CFIndex count = CFArrayGetCount(windows);
	if (count > 0) {
		CFTypeRef query = SLSWindowQueryWindows(SLSMainConnectionID(), windows, (int)count);
		if (query) {
			CFTypeRef iterator = SLSWindowQueryResultCopyWindows(query);
			if (iterator) {
				while (SLSWindowIteratorAdvance(iterator)) {
					if (mimiWindowIsReal(
					        SLSWindowIteratorGetTags(iterator), SLSWindowIteratorGetAttributes(iterator),
					        SLSWindowIteratorGetParentID(iterator), SLSWindowIteratorGetLevel(iterator))) {
						[real addObject:@(SLSWindowIteratorGetWindowID(iterator))];
						[corners addObject:@(radii ? mimiIteratorRadius(iterator) : -1)];
					}
				}
				CFRelease(iterator);
			}
			CFRelease(query);
		}
	}

	CFRelease(windows);
	return CFBridgingRetain(real);
}

/// Window numbers of every real, unminimized window on any space, whoever
/// owns them.
static NSSet<NSNumber *> *mimiRealWindowNumbers(void) {
	NSArray<NSNumber *> *spaces = mimiAllSpaceIDs();
	NSArray<NSNumber *> *real = CFBridgingRelease(MimiCopyRealWindowsOnSpaces((__bridge CFArrayRef)spaces, NULL));
	return [NSSet setWithArray:real];
}

#pragma mark - Helpers

/// Window numbers of the layer-0 windows a process owns, on every space, in
/// the window server's order: front to back, which is most recently used
/// first.
static NSArray<NSNumber *> *mimiWindowNumbersForPID(pid_t pid) {
	CFArrayRef windowList =
	    CGWindowListCopyWindowInfo(kCGWindowListOptionAll | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!windowList)
		return @[];

	NSMutableArray<NSNumber *> *numbers = [NSMutableArray array];
	CFIndex count = CFArrayGetCount(windowList);
	for (CFIndex i = 0; i < count; i++) {
		NSDictionary *info = (__bridge NSDictionary *)CFArrayGetValueAtIndex(windowList, i);
		if ([info[(__bridge NSString *)kCGWindowOwnerPID] intValue] != pid)
			continue;
		if ([info[(__bridge NSString *)kCGWindowLayer] intValue] != 0)
			continue;

		NSNumber *number = info[(__bridge NSString *)kCGWindowNumber];
		if (number)
			[numbers addObject:number];
	}

	CFRelease(windowList);
	return numbers;
}

/// Process identifiers of every process owning a window on any space.
static NSArray<NSNumber *> *mimiAllWindowOwnerPIDs(void) {
	CFArrayRef windowList =
	    CGWindowListCopyWindowInfo(kCGWindowListOptionAll | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!windowList)
		return @[];

	NSMutableOrderedSet<NSNumber *> *pids = [NSMutableOrderedSet orderedSet];
	CFIndex count = CFArrayGetCount(windowList);
	for (CFIndex i = 0; i < count; i++) {
		NSDictionary *info = (__bridge NSDictionary *)CFArrayGetValueAtIndex(windowList, i);
		NSNumber *pid = info[(__bridge NSString *)kCGWindowOwnerPID];
		if (pid && pid.intValue > 0)
			[pids addObject:pid];
	}

	CFRelease(windowList);
	return pids.array;
}

static bool mimiApplicationMatches(NSRunningApplication *app, NSString *query) {
	if (!app || app.activationPolicy != NSApplicationActivationPolicyRegular)
		return false;
	if (app.bundleIdentifier && [app.bundleIdentifier caseInsensitiveCompare:query] == NSOrderedSame)
		return true;
	if (app.localizedName && [app.localizedName caseInsensitiveCompare:query] == NSOrderedSame)
		return true;
	return false;
}

/// The space a window sits on: its one space, or 0 when it is on every space
/// (assigned to all desktops) or on none.
static uint64_t mimiSpaceForWindowNumber(CGWindowID number) {
	CFNumberRef numberRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &number);
	if (!numberRef)
		return 0;

	CFArrayRef windowIDs = CFArrayCreate(NULL, (const void **)&numberRef, 1, &kCFTypeArrayCallBacks);
	CFRelease(numberRef);
	if (!windowIDs)
		return 0;

	CFArrayRef spaces = SLSCopySpacesForWindows(SLSMainConnectionID(), kMimiAllSpacesMask, windowIDs);
	CFRelease(windowIDs);
	if (!spaces)
		return 0;

	uint64_t space = 0;
	if (CFArrayGetCount(spaces) == 1) {
		CFNumberRef spaceRef = (CFNumberRef)CFArrayGetValueAtIndex(spaces, 0);
		if (spaceRef)
			CFNumberGetValue(spaceRef, CFNumberGetType(spaceRef), &space);
	}

	CFRelease(spaces);
	return space;
}

#pragma mark - Public Application API

int MimiFindApplication(const char *query) {
	if (!query)
		return 0;

	@autoreleasepool {
		NSString *wanted = [NSString stringWithUTF8String:query];
		if (wanted.length == 0)
			return 0;

		// A bundle identifier is answered by Launch Services, which is live
		// on every thread.
		for (NSRunningApplication *app in [NSRunningApplication runningApplicationsWithBundleIdentifier:wanted]) {
			if (mimiApplicationMatches(app, wanted))
				return (int)app.processIdentifier;
		}

		// A name is matched against the owners of the windows on every space,
		// each looked up live by pid. -[NSWorkspace runningApplications] is
		// refreshed only from the main thread's run loop, which the daemon's
		// action thread never pumps, so it is the last resort below rather
		// than the first stop.
		for (NSNumber *pid in mimiAllWindowOwnerPIDs()) {
			NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid.intValue];
			if (mimiApplicationMatches(app, wanted))
				return (int)app.processIdentifier;
		}

		for (NSRunningApplication *app in [NSWorkspace sharedWorkspace].runningApplications) {
			if (mimiApplicationMatches(app, wanted))
				return (int)app.processIdentifier;
		}

		return 0;
	}
}

int *MimiCopyRegularApplicationPIDs(int *count) {
	if (!count)
		return NULL;
	*count = 0;

	@autoreleasepool {
		// Derived from the window list rather than -[NSWorkspace
		// runningApplications], for the reason window.m gives: that array
		// refreshes only while the main thread's run loop runs.
		NSArray<NSNumber *> *owners = mimiAllWindowOwnerPIDs();
		NSMutableArray<NSNumber *> *regular = [NSMutableArray array];
		for (NSNumber *owner in owners) {
			NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:owner.intValue];
			if (app && app.activationPolicy == NSApplicationActivationPolicyRegular)
				[regular addObject:owner];
		}

		if (regular.count == 0)
			return NULL;

		int *pids = malloc(sizeof(int) * regular.count);
		if (!pids)
			return NULL;

		for (NSUInteger i = 0; i < regular.count; i++)
			pids[i] = regular[i].intValue;
		*count = (int)regular.count;

		return pids;
	}
}

static char *mimiCopyUTF8(NSString *string) {
	const char *utf8 = string ? [string UTF8String] : NULL;
	return utf8 ? strdup(utf8) : NULL;
}

char *MimiCopyApplicationName(int pid) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
		return mimiCopyUTF8(app.localizedName);
	}
}

char *MimiCopyApplicationBundleID(int pid) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
		return mimiCopyUTF8(app.bundleIdentifier);
	}
}

MimiAppWindow *MimiCopyApplicationWindows(int pid, int *count) {
	if (!count)
		return NULL;
	*count = 0;

	@autoreleasepool {
		// The application's windows in the window server's order, front to
		// back, narrowed to the ones the window server itself counts as real
		// and not minimized. Accessibility is not consulted here on purpose:
		// its window list leaves out every window on another space, which is
		// exactly the window this action exists to reach.
		NSArray<NSNumber *> *order = mimiWindowNumbersForPID((pid_t)pid);
		if (order.count == 0)
			return NULL;

		NSSet<NSNumber *> *real = mimiRealWindowNumbers();
		NSMutableArray<NSNumber *> *kept = [NSMutableArray array];
		for (NSNumber *number in order) {
			if ([real containsObject:number])
				[kept addObject:number];
		}
		if (kept.count == 0)
			return NULL;

		MimiAppWindow *result = (MimiAppWindow *)calloc(kept.count, sizeof(MimiAppWindow));
		if (!result)
			return NULL;

		for (NSUInteger i = 0; i < kept.count; i++) {
			CGWindowID number = kept[i].unsignedIntValue;
			result[i].number = number;
			result[i].space = mimiSpaceForWindowNumber(number);
		}

		*count = (int)kept.count;
		return result;
	}
}

/// Accessibility lists an application's windows on the active space only, and
/// updates that list a moment after a space switch lands, so the lookup is
/// tried a few times before giving up.
static const int kMimiRaiseAttempts = 5;
static const useconds_t kMimiRaiseRetryDelay = 30000;

static AXUIElementRef mimiCopyWindowElement(int pid, uint32_t number) {
	AXUIElementRef appElement = AXUIElementCreateApplication((pid_t)pid);
	if (!appElement)
		return NULL;

	CFTypeRef windowsValue = NULL;
	AXError error = AXUIElementCopyAttributeValue(appElement, kAXWindowsAttribute, &windowsValue);
	CFRelease(appElement);
	if (error != kAXErrorSuccess || !windowsValue)
		return NULL;

	AXUIElementRef found = NULL;
	if (CFGetTypeID(windowsValue) == CFArrayGetTypeID()) {
		CFArrayRef windows = (CFArrayRef)windowsValue;
		CFIndex windowCount = CFArrayGetCount(windows);
		for (CFIndex i = 0; i < windowCount && !found; i++) {
			AXUIElementRef window = (AXUIElementRef)CFArrayGetValueAtIndex(windows, i);
			CGWindowID candidate = 0;
			if (window && _AXUIElementGetWindow(window, &candidate) == kAXErrorSuccess && candidate == number)
				found = (AXUIElementRef)CFRetain(window);
		}
	}

	CFRelease(windowsValue);
	return found;
}

int MimiRaiseWindowNumber(int pid, uint32_t number) {
	@autoreleasepool {
		for (int attempt = 0; attempt < kMimiRaiseAttempts; attempt++) {
			AXUIElementRef window = mimiCopyWindowElement(pid, number);
			if (window) {
				int raised = MimiActivateWindow((void *)window);
				CFRelease(window);
				return raised;
			}
			usleep(kMimiRaiseRetryDelay);
		}

		MIMI_LOG("window %u of pid %d is not in its application's Accessibility window list", (unsigned)number, pid);
		return 0;
	}
}

/// How long a reopen may take before plain activation stands in for it.
static const int64_t kMimiReopenTimeout = 2 * NSEC_PER_SEC;

int MimiReopenApplication(int pid) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
		if (!app)
			return 0;

		// Launch Services sends a running application the same reopen event
		// a Dock click does, and the application answers by opening a window
		// when it has none. Plain activation does not, which is how Finder
		// used to end up in front with nothing to show.
		NSURL *bundleURL = app.bundleURL;
		if (!bundleURL)
			return [app activateWithOptions:0] ? 1 : 0;

		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		__block int reopened = 0;
		[[NSWorkspace sharedWorkspace] openApplicationAtURL:bundleURL
		                                      configuration:[NSWorkspaceOpenConfiguration configuration]
		                                  completionHandler:^(NSRunningApplication *running, NSError *error) {
			                                  reopened = running != nil && error == nil;
			                                  dispatch_semaphore_signal(sem);
		                                  }];
		if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, kMimiReopenTimeout)) == 0 && reopened)
			return 1;

		MIMI_LOG("reopen of pid %d did not complete, activating instead", pid);
		return [app activateWithOptions:0] ? 1 : 0;
	}
}
