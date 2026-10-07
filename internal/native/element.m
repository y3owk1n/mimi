//
//  element.m
//  mimi
//

#import "mimi.h"
#import "mimi_log.h"

#import <Cocoa/Cocoa.h>

void *MimiGetFocusedApplication(void) {
	@autoreleasepool {
		AXUIElementRef systemWide = AXUIElementCreateSystemWide();
		if (systemWide) {
			AXUIElementRef focusedApp = NULL;
			AXError error =
			    AXUIElementCopyAttributeValue(systemWide, kAXFocusedApplicationAttribute, (CFTypeRef *)&focusedApp);

			CFRelease(systemWide);

			if (error == kAXErrorSuccess && focusedApp) {
				return (void *)focusedApp;
			}

			MimiLog(
			    MimiLogLevelDebug, @"AX focused application unavailable, asking NSWorkspace",
			    @{@"ax_error" : @(error)});
		}

		// Last resort only. NSWorkspace answers this out of state it refreshes
		// from the main thread's run loop, which neither the CLI nor an
		// action-serving daemon thread pumps, so it can name an application
		// that is no longer frontmost. The system-wide Accessibility query
		// above is the one that is always current.
		NSRunningApplication *front = [NSWorkspace sharedWorkspace].frontmostApplication;
		if (!front)
			return NULL;

		pid_t pid = front.processIdentifier;
		AXUIElementRef axApp = AXUIElementCreateApplication(pid);
		return (void *)axApp;
	}
}

int MimiFrontmostPid(void) {
	void *app = MimiGetFocusedApplication();
	if (!app)
		return 0;
	pid_t pid = 0;
	AXUIElementGetPid((AXUIElementRef)app, &pid);
	CFRelease((AXUIElementRef)app);
	return pid;
}

void MimiReleaseElement(void *element) {
	if (element) {
		CFRelease((AXUIElementRef)element);
	}
}

void MimiRetainElement(void *element) {
	if (element) {
		CFRetain((AXUIElementRef)element);
	}
}

int MimiAreElementsEqual(void *element1, void *element2) {
	if (!element1 || !element2)
		return element1 == element2;

	return CFEqual((AXUIElementRef)element1, (AXUIElementRef)element2) ? 1 : 0;
}

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

// MimiAXIsRealWindow reports whether the element is a top-level window the
// user sees as standalone. It needs the AXWindow role, the app element as
// its AX parent, and a close button. Transient AXWindow-role elements fail
// at least one check. Safari's tab-hover preview (subrole AXUnknown) and
// Chromium's extension popup have the app as parent but no close button.
// Tabs have a tab group as parent. URL bar autocomplete has no close button.
// An unreadable parent or close button fails the check, because a real
// top-level window has both and a false positive costs more than a rare
// miss. The answer holds only while the element is alive, since its
// attributes are unreadable at destroy time (see knownRealWindows in
// axobserver.m).
bool MimiAXIsRealWindow(AXUIElementRef element, AXUIElementRef appElement) {
	if (!axElementHasWindowRole(element)) {
		return false;
	}

	CFTypeRef parentRef = NULL;
	AXError parentErr = AXUIElementCopyAttributeValue(element, kAXParentAttribute, &parentRef);
	if (parentErr != kAXErrorSuccess || !parentRef) {
		return false;
	}
	bool parentIsApp = CFEqual(parentRef, appElement);
	CFRelease(parentRef);
	if (!parentIsApp) {
		return false;
	}

	CFTypeRef closeButtonRef = NULL;
	AXError closeErr = AXUIElementCopyAttributeValue(element, kAXCloseButtonAttribute, &closeButtonRef);
	bool hasCloseButton = (closeErr == kAXErrorSuccess && closeButtonRef != NULL);
	if (closeButtonRef) {
		CFRelease(closeButtonRef);
	}

	return hasCloseButton;
}
