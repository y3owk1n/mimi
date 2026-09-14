#import "mouse.h"

#include "_cgo_export.h"
#import "mimi_log.h"
#import "workspace.h"

#include <CoreGraphics/CoreGraphics.h>

// The monitor is a listen-only event tap on mouse moves, which is what an
// NSEvent global monitor is underneath. It never swallows or delays an
// event. The tap runs on the daemon's main run loop, so it is installed and
// removed there.

static CFMachPortRef gMouseTap;
static CFRunLoopSourceRef gMouseSource;

static CGEventRef mimiMouseMoved(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *info) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		if (gMouseTap)
			CGEventTapEnable(gMouseTap, true);
		return event;
	}

	CGPoint point = CGEventGetLocation(event);
	goMouseMoved(point.x, point.y);
	return event;
}

static void mimiMouseMonitorStartOnRunLoop(void) {
	if (gMouseTap)
		return;

	gMouseTap = CGEventTapCreate(
	    kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionListenOnly, CGEventMaskBit(kCGEventMouseMoved),
	    mimiMouseMoved, NULL);
	if (!gMouseTap) {
		MIMI_LOG("CGEventTapCreate for mouse moves failed");
		return;
	}

	gMouseSource = CFMachPortCreateRunLoopSource(NULL, gMouseTap, 0);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), gMouseSource, kCFRunLoopCommonModes);
	CGEventTapEnable(gMouseTap, true);
}

static void mimiMouseMonitorStopOnRunLoop(void) {
	if (!gMouseTap)
		return;

	CGEventTapEnable(gMouseTap, false);
	CFRunLoopRemoveSource(CFRunLoopGetCurrent(), gMouseSource, kCFRunLoopCommonModes);
	CFRelease(gMouseSource);
	CFRelease(gMouseTap);
	gMouseSource = NULL;
	gMouseTap = NULL;
}

int MimiMouseMonitorStart(void) {
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return 0;

	if (CFRunLoopGetCurrent() == rl) {
		mimiMouseMonitorStartOnRunLoop();
		return gMouseTap != NULL;
	}

	__block int started = 0;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	CFRunLoopPerformBlock(rl, kCFRunLoopCommonModes, ^{
		mimiMouseMonitorStartOnRunLoop();
		started = gMouseTap != NULL;
		dispatch_semaphore_signal(done);
	});
	CFRunLoopWakeUp(rl);
	dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC));
	return started;
}

void MimiMouseMonitorStop(void) {
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return;

	if (CFRunLoopGetCurrent() == rl) {
		mimiMouseMonitorStopOnRunLoop();
		return;
	}

	CFRunLoopPerformBlock(rl, kCFRunLoopCommonModes, ^{
		mimiMouseMonitorStopOnRunLoop();
	});
	CFRunLoopWakeUp(rl);
}
