//
//  element.m
//  mimi
//

#import "mimi.h"

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
