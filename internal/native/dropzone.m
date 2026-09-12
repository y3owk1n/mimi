#import "dropzone.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>

// The drop zone is one transparent window the size of the display the
// dragged window is over, with a rounded layer that slides to wherever the
// layout says the window would land. Every call goes to the main thread,
// where the daemon runs its application loop.

static NSWindow *gZone;
static CALayer *gShape;

static NSRect mimiDropzoneCocoaRect(CGRect rect) {
	double primaryHeight = CGDisplayBounds(CGMainDisplayID()).size.height;
	return NSMakeRect(
	    rect.origin.x, primaryHeight - rect.origin.y - rect.size.height, rect.size.width, rect.size.height);
}

static NSWindow *mimiDropzoneWindow(void) {
	if (gZone) {
		return gZone;
	}
	NSWindow *zone = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1, 1)
	                                             styleMask:NSWindowStyleMaskBorderless
	                                               backing:NSBackingStoreBuffered
	                                                 defer:NO];
	zone.level = NSFloatingWindowLevel;
	zone.opaque = NO;
	zone.backgroundColor = [NSColor clearColor];
	zone.hasShadow = NO;
	zone.ignoresMouseEvents = YES;
	zone.releasedWhenClosed = NO;
	zone.animationBehavior = NSWindowAnimationBehaviorNone;
	zone.collectionBehavior = NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle |
	                          NSWindowCollectionBehaviorCanJoinAllSpaces;
	NSView *view = zone.contentView;
	view.wantsLayer = YES;
	CALayer *shape = [CALayer layer];
	shape.anchorPoint = CGPointZero;
	[view.layer addSublayer:shape];
	gShape = shape;
	gZone = zone;
	return zone;
}

void MimiDropzoneShow(const MimiDropzoneStyle *style, double x, double y, double w, double h) {
	MimiDropzoneStyle copy = *style;
	CGRect frame = CGRectMake(x, y, w, h);
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *zone = mimiDropzoneWindow();
		CGDirectDisplayID display = CGMainDisplayID();
		uint32_t count = 0;
		CGGetDisplaysWithPoint(CGPointMake(CGRectGetMidX(frame), CGRectGetMidY(frame)), 1, &display, &count);
		CGRect bounds = CGDisplayBounds(display);
		BOOL shown = zone.visible;
		if (!shown || !NSEqualRects(zone.frame, mimiDropzoneCocoaRect(bounds))) {
			[zone setFrame:mimiDropzoneCocoaRect(bounds) display:NO];
		}
		CGRect local = CGRectMake(
		    frame.origin.x - bounds.origin.x,
		    bounds.size.height - (frame.origin.y - bounds.origin.y) - frame.size.height, frame.size.width,
		    frame.size.height);
		[CATransaction begin];
		// The zone jumps to the first frame and slides to the ones after.
		[CATransaction setDisableActions:!shown];
		gShape.frame = local;
		gShape.cornerRadius = copy.radius;
		gShape.borderWidth = copy.width;
		CGColorRef fill = CGColorCreateSRGB(copy.fill.red, copy.fill.green, copy.fill.blue, copy.fill.alpha);
		CGColorRef outline =
		    CGColorCreateSRGB(copy.outline.red, copy.outline.green, copy.outline.blue, copy.outline.alpha);
		gShape.backgroundColor = fill;
		gShape.borderColor = outline;
		CGColorRelease(fill);
		CGColorRelease(outline);
		[CATransaction commit];
		if (!shown) {
			[zone orderFrontRegardless];
		}
	});
}

void MimiDropzoneHide(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gZone) {
			[gZone orderOut:nil];
		}
	});
}

int MimiLeftMouseButtonDown(void) {
	return CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft) ? 1 : 0;
}
