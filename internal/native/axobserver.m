#import "axobserver.h"

#include "_cgo_export.h"
#import "eventkinds.h"
#import "workspace.h"

#import <ApplicationServices/ApplicationServices.h>
#import <Cocoa/Cocoa.h>

@interface AXEntry : NSObject
@property AXObserverRef observer;
@property AXUIElementRef appElement;
@property pid_t pid;
// Set of AXUIElementRef for elements that have passed the strict
// create filter (AXWindow role, parent == app element, has close
// button). At destroy time, the element's attributes are unreadable
// (kAXErrorAPIDisabled / kAXErrorInvalidUIElement — the element
// has been torn down), so we can't filter on role, parent, or
// close button at destroy time. Instead, the create handler
// populates this set with the AXUIElementRef of every element it
// has confirmed to be a real top-level window, and the destroy
// handler looks the destroyed element up in this set. A set hit
// means "we saw this element get created as a real window"; a miss
// means "we never saw this element, or it was a tab/transient
// overlay and was correctly rejected on the create path".
//
// CFHash/CFEqual on AXUIElementRef is pointer identity on the
// underlying accessibility object, which is stable across the
// create → destroy lifecycle (the OS hands us the same opaque
// pointer in both callbacks). Set membership survives even when
// the element's attributes no longer do.
@property CFMutableSetRef knownRealWindows;
@end
@implementation AXEntry

- (void)dealloc {
	if (_knownRealWindows) {
		CFRelease(_knownRealWindows);
	}
}

@end

static NSMutableDictionary<NSNumber *, AXEntry *> *gEntries;

static bool axElementHasWindowRole(AXUIElementRef element) {
	CFTypeRef roleRef = NULL;
	bool isWindow = false;
	if (AXUIElementCopyAttributeValue(element, kAXRoleAttribute, &roleRef) == kAXErrorSuccess && roleRef) {
		if (CFGetTypeID(roleRef) == CFStringGetTypeID() &&
		    CFStringCompare((CFStringRef)roleRef, CFSTR("AXWindow"), 0) == kCFCompareEqualTo) {
			isWindow = true;
		}
		CFRelease(roleRef);
	}

	return isWindow;
}

static void dispatchAXEvent(int kind, pid_t pid, AXUIElementRef element) {
	CFTypeRef titleRef = NULL;
	AXUIElementCopyAttributeValue(element, kAXTitleAttribute, &titleRef);
	const char *title = "";
	if (titleRef) {
		if (CFGetTypeID(titleRef) == CFStringGetTypeID()) {
			title = [(__bridge NSString *)titleRef UTF8String];
			if (!title)
				title = "";
		}
		CFRelease(titleRef);
	}

	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	const char *appName = app ? [app.localizedName UTF8String] : "";
	const char *bundleID = app ? [app.bundleIdentifier UTF8String] : "";

	goAXEvent(kind, (char *)appName, (char *)bundleID, (int)pid, (char *)title);
}

static void axCallback(AXObserverRef observer, AXUIElementRef element, CFStringRef notification, void *refcon) {
	@autoreleasepool {
		pid_t pid = (pid_t)(intptr_t)refcon;

		if (CFEqual(notification, kAXWindowCreatedNotification)) {
			// kAXWindowCreatedNotification is documented as firing
			// for real top-level windows, but in practice some apps
			// (notably Safari) fire it for things that have
			// AXWindow role in the app's AX tree but are not
			// "true" windows the user perceives as standalone:
			// URL bar autocomplete dropdowns, popovers, detached
			// panels, and tabs. Tabs in particular are exposed by
			// Safari as top-level windows in the AX tree (with a
			// close button, title, and the ability to be focused)
			// even though you perceive them as sub-windows of a
			// larger Safari window.
			//
			// We layer multiple signals:
			//
			//   - role == AXWindow                  (fast-path filter)
			//   - element in kAXWindowsAttribute    (catches most)
			//   - element's parent is the app element (catches
			//                                       tabs that have a
			//                                       tab group as
			//                                       their parent in
			//                                       the AX tree)
			//   - element has a close button        (catches transient
			//                                       overlays like URL
			//                                       bar autocomplete
			//                                       that have the app
			//                                       as parent)
			//
			// On unreadable parent or close button we drop
			// conservatively. A real top-level window has both, so
			// the unreadable cases are rare in practice and not
			// worth risking a false positive.
			//
			// Once all three checks pass, we add the element's
			// AXUIElementRef to entry.knownRealWindows. The destroy
			// handler uses that set as its source of truth, since
			// by the time the destroy notification fires, the
			// element's role, parent, and close button are all
			// unreadable (the element has been torn down).
			AXEntry *entry = gEntries[@(pid)];
			if (!entry) {
				return;
			}

			if (!axElementHasWindowRole(element)) {
				return;
			}

			CFTypeRef parentRef = NULL;
			AXError parentErr = AXUIElementCopyAttributeValue(element, kAXParentAttribute, &parentRef);
			if (parentErr != kAXErrorSuccess || !parentRef) {
				return;
			}
			bool parentIsApp = CFEqual(parentRef, entry.appElement);
			CFRelease(parentRef);
			if (!parentIsApp) {
				// Parent is something other than the app — a tab
				// group, sheet, drawer, popover, sub-view. Drop.
				return;
			}

			CFTypeRef closeButtonRef = NULL;
			AXError closeErr = AXUIElementCopyAttributeValue(element, kAXCloseButtonAttribute, &closeButtonRef);
			bool hasCloseButton = (closeErr == kAXErrorSuccess && closeButtonRef != NULL);
			if (closeButtonRef) {
				CFRelease(closeButtonRef);
			}
			if (!hasCloseButton) {
				// AXWindow role, parent is the app, but no close
				// button — a transient overlay (URL bar
				// autocomplete). Drop.
				return;
			}

			// All three signals confirm this is a real top-level
			// window. Record it so the destroy handler can fire
			// window_closed for it (at destroy time, the
			// element's own attributes are gone).
			if (entry.knownRealWindows) {
				CFSetAddValue(entry.knownRealWindows, element);
			}
			dispatchAXEvent(MIMI_KIND_WINDOW_CREATED, pid, element);

			return;
		}

		if (CFEqual(notification, kAXUIElementDestroyedNotification)) {
			// The OS fires kAXUIElementDestroyedNotification for any
			// element in the app's accessibility subtree that gets
			// torn down — that includes top-level windows, sheets,
			// popovers, web content, toolbars, sidebars, URL bar
			// autocomplete, etc. We only want to fire window_closed
			// for windows the user was interacting with — true
			// top-level windows the user can interact with and drag
			// around.
			//
			// Filtering on the destroyed element's own attributes
			// does not work: by the time this callback fires, the
			// element is already torn down and AXUIElementCopyAttributeValue
			// returns kAXErrorAPIDisabled / kAXErrorInvalidUIElement
			// (-25202) for role, title, parent, close button —
			// everything. We saw this in the diagnostic log:
			// "destroy: role=UNREADABLE … dropping on parent
			// unreadable, err=-25202".
			//
			// Instead, the create handler records every element
			// it confirms to be a real top-level window in
			// entry.knownRealWindows (a CFMutableSetRef of
			// AXUIElementRef). At destroy time we look the
			// destroyed element up in that set. A hit means
			// "we saw this element get created as a real
			// window"; a miss means "we never saw this
			// element, or it was a tab/transient overlay and
			// was correctly rejected on the create path".
			//
			// This is the only way to get a deterministic close
			// signal in the current macOS accessibility API.
			AXEntry *entry = gEntries[@(pid)];
			if (!entry) {
				return;
			}

			if (!entry.knownRealWindows) {
				return;
			}

			if (!CFSetContainsValue(entry.knownRealWindows, element)) {
				// Never confirmed as a real window on the create
				// path (or already removed by an earlier destroy).
				// Drop.
				return;
			}

			CFSetRemoveValue(entry.knownRealWindows, element);
			dispatchAXEvent(MIMI_KIND_WINDOW_CLOSED, pid, element);

			return;
		}

		if (CFEqual(notification, kAXFocusedWindowChangedNotification)) {
			// Delivered on the app element directly when the app's
			// focused window attribute changes. Not subject to
			// descendant fan-out for unrelated sub-views.
			dispatchAXEvent(MIMI_KIND_WINDOW_FOCUS, pid, element);

			return;
		}

		if (CFEqual(notification, kAXTitleChangedNotification)) {
			// Title changes fan out to descendants. Filter to real
			// top-level windows by role.
			if (!axElementHasWindowRole(element)) {
				return;
			}
			dispatchAXEvent(MIMI_KIND_WINDOW_TITLE_CHANGE, pid, element);

			return;
		}

		if (CFEqual(notification, kAXWindowResizedNotification)) {
			// Same fan-out; filter by role.
			if (!axElementHasWindowRole(element)) {
				return;
			}
			dispatchAXEvent(MIMI_KIND_WINDOW_RESIZING, pid, element);

			return;
		}
	}
}

static void axInstallBlock(int pid) {
	if (!gEntries)
		gEntries = [NSMutableDictionary new];

	NSNumber *key = @(pid);
	if (gEntries[key])
		return;

	AXUIElementRef appElement = AXUIElementCreateApplication(pid);
	if (!appElement)
		return;

	AXObserverRef observer = NULL;
	AXError err = AXObserverCreate(pid, axCallback, &observer);
	if (err != kAXErrorSuccess) {
		CFRelease(appElement);

		return;
	}

	CFStringRef notifications[] = {
	    kAXWindowCreatedNotification, kAXUIElementDestroyedNotification, kAXFocusedWindowChangedNotification,
	    kAXTitleChangedNotification,  kAXWindowResizedNotification,
	};
	size_t notifCount = sizeof(notifications) / sizeof(notifications[0]);
	for (size_t i = 0; i < notifCount; i++) {
		AXError addErr = AXObserverAddNotification(observer, appElement, notifications[i], (void *)(intptr_t)pid);
		(void)addErr;
	}

	CFRunLoopRef rl = GetRunLoop();
	if (rl) {
		CFRunLoopAddSource(rl, AXObserverGetRunLoopSource(observer), kCFRunLoopDefaultMode);
	}

	AXEntry *entry = [AXEntry new];
	entry.observer = observer;
	entry.appElement = appElement;
	entry.pid = pid;
	entry.knownRealWindows = CFSetCreateMutable(NULL, 0, &kCFTypeSetCallBacks);
	gEntries[key] = entry;
}

static void axRemoveBlock(int pid) {
	NSNumber *key = @(pid);
	AXEntry *entry = gEntries[key];
	if (!entry)
		return;

	CFRunLoopRef rl = GetRunLoop();
	if (rl) {
		CFRunLoopRemoveSource(rl, AXObserverGetRunLoopSource(entry.observer), kCFRunLoopDefaultMode);
	}
	CFRelease(entry.observer);
	CFRelease(entry.appElement);
	if (entry.knownRealWindows) {
		CFRelease(entry.knownRealWindows);
		entry.knownRealWindows = NULL;
	}
	[gEntries removeObjectForKey:key];
}

bool AXInstallObserver(int pid) {
	__block bool ok = false;
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return false;

	CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
		axInstallBlock(pid);
		ok = true;
		dispatch_semaphore_signal(sem);
	});
	CFRunLoopWakeUp(rl);
	dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);

	return ok;
}

void AXRemoveObserver(int pid) {
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return;

	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
		axRemoveBlock(pid);
		dispatch_semaphore_signal(sem);
	});
	CFRunLoopWakeUp(rl);
	dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
}

void AXRemoveAllObservers(void) {
	if (!gEntries)
		return;

	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return;

	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
		NSArray *keys = [gEntries allKeys];
		for (NSNumber *key in keys) {
			axRemoveBlock([key intValue]);
		}
		dispatch_semaphore_signal(sem);
	});
	CFRunLoopWakeUp(rl);
	dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
	gEntries = nil;
}
