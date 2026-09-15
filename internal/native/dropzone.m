#import "dropzone.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>

// The drop zone is one transparent window the size of the display the
// dragged window is over, with a rounded layer that slides to wherever the
// layout says the window would land. Every call goes to the main thread,
// where the daemon runs its application loop.

static NSWindow *gZone;
static CALayer *gShape;
static CALayer *gTarget;

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
	CALayer *target = [CALayer layer];
	target.anchorPoint = CGPointZero;
	target.hidden = YES;
	[view.layer addSublayer:target];
	gTarget = target;
	gZone = zone;
	return zone;
}

// mimiDropzoneStyleLayer gives a layer the frame and look of one mark, in
// the zone window's own coordinates, sliding it there when it is up.
static void mimiDropzoneStyleLayer(CALayer *layer, const MimiDropzoneStyle *style, CGRect frame, BOOL slide) {
	NSRect bounds = gZone.frame;
	CGRect local = CGRectMake(
	    frame.origin.x - bounds.origin.x, bounds.size.height - (frame.origin.y - bounds.origin.y) - frame.size.height,
	    frame.size.width, frame.size.height);
	[CATransaction begin];
	[CATransaction setDisableActions:!slide];
	layer.frame = local;
	layer.cornerRadius = style->radius;
	layer.borderWidth = style->width;
	CGColorRef fill = CGColorCreateSRGB(style->fill.red, style->fill.green, style->fill.blue, style->fill.alpha);
	CGColorRef outline =
	    CGColorCreateSRGB(style->outline.red, style->outline.green, style->outline.blue, style->outline.alpha);
	layer.backgroundColor = fill;
	layer.borderColor = outline;
	CGColorRelease(fill);
	CGColorRelease(outline);
	[CATransaction commit];
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
		// The zone jumps to the first frame and slides to the ones after.
		mimiDropzoneStyleLayer(gShape, &copy, frame, shown);
		if (!shown) {
			[zone orderFrontRegardless];
		}
	});
}

void MimiDropzoneShowTarget(const MimiDropzoneStyle *style, double x, double y, double w, double h) {
	MimiDropzoneStyle copy = *style;
	CGRect frame = CGRectMake(x, y, w, h);
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!gZone) {
			return;
		}
		BOOL shown = !gTarget.hidden;
		mimiDropzoneStyleLayer(gTarget, &copy, frame, shown);
		gTarget.hidden = NO;
	});
}

void MimiDropzoneHideTarget(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gTarget) {
			gTarget.hidden = YES;
		}
	});
}

void MimiDropzoneHide(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gZone) {
			[gZone orderOut:nil];
			gTarget.hidden = YES;
		}
	});
}

int MimiLeftMouseButtonDown(void) {
	return CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft) ? 1 : 0;
}
