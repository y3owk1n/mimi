#import "stackbar.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>

// A stack indicator is one transparent window sitting on the top edge of the
// frame a stack's windows share, holding one segment per window with the
// active one in its own colour. It says how many windows are in that place
// and which one is meant to be seen, which is the only thing about a stack
// that is otherwise invisible.
//
// One window per stack, kept in a pool: a sync shows as many as it was given
// and orders the rest out rather than releasing them, since a desktop settles
// on a stack count and a released window would be rebuilt on the next pass.
// Every call goes to the main thread, where the daemon runs its application
// loop.

static NSMutableArray<NSWindow *> *gBars;

/// Gap between one segment and the next, in points.
static const double kMimiStackbarGap = 2.0;

/// Inset of the whole bar from the sides of the frame it marks, in points.
static const double kMimiStackbarInset = 4.0;

static NSRect mimiStackbarCocoaRect(CGRect rect) {
	double primaryHeight = CGDisplayBounds(CGMainDisplayID()).size.height;
	return NSMakeRect(
	    rect.origin.x, primaryHeight - rect.origin.y - rect.size.height, rect.size.width, rect.size.height);
}

static NSWindow *mimiStackbarWindow(void) {
	NSWindow *bar = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1, 1)
	                                            styleMask:NSWindowStyleMaskBorderless
	                                              backing:NSBackingStoreBuffered
	                                                defer:NO];
	bar.level = NSFloatingWindowLevel;
	bar.opaque = NO;
	bar.backgroundColor = [NSColor clearColor];
	bar.hasShadow = NO;
	bar.ignoresMouseEvents = YES;
	bar.releasedWhenClosed = NO;
	bar.animationBehavior = NSWindowAnimationBehaviorNone;
	bar.collectionBehavior = NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle |
	                         NSWindowCollectionBehaviorCanJoinAllSpaces;
	bar.contentView.wantsLayer = YES;
	return bar;
}

/// Lay one bar's segments out across its width, the active one coloured apart.
static void mimiStackbarDraw(NSWindow *bar, MimiStackbar spec, MimiStackbarStyle style) {
	CALayer *root = bar.contentView.layer;
	root.sublayers = nil;

	if (spec.count <= 0) {
		return;
	}

	double width = bar.frame.size.width;
	double each = (width - kMimiStackbarGap * (spec.count - 1)) / spec.count;
	if (each <= 0) {
		return;
	}

	CGColorRef plain = CGColorCreateSRGB(style.color.red, style.color.green, style.color.blue, style.color.alpha);
	CGColorRef active = CGColorCreateSRGB(
	    style.activeColor.red, style.activeColor.green, style.activeColor.blue, style.activeColor.alpha);

	for (int index = 0; index < spec.count; index++) {
		CALayer *segment = [CALayer layer];
		segment.anchorPoint = CGPointZero;
		segment.frame = CGRectMake(index * (each + kMimiStackbarGap), 0, each, style.height);
		segment.cornerRadius = style.radius;
		segment.backgroundColor = index == spec.active ? active : plain;
		[root addSublayer:segment];
	}

	CGColorRelease(plain);
	CGColorRelease(active);
}

void MimiStackbarsSync(const MimiStackbar *bars, int count, const MimiStackbarStyle *style) {
	if (count < 0) {
		count = 0;
	}

	MimiStackbarStyle styleCopy = *style;
	NSMutableArray *specs = [NSMutableArray arrayWithCapacity:(NSUInteger)count];
	for (int index = 0; index < count; index++) {
		[specs addObject:[NSValue valueWithBytes:&bars[index] objCType:@encode(MimiStackbar)]];
	}

	dispatch_async(dispatch_get_main_queue(), ^{
		if (!gBars) {
			gBars = [NSMutableArray array];
		}

		while (gBars.count < specs.count) {
			[gBars addObject:mimiStackbarWindow()];
		}

		for (NSUInteger index = 0; index < specs.count; index++) {
			MimiStackbar spec;
			[specs[index] getValue:&spec size:sizeof(spec)];

			NSWindow *bar = gBars[index];
			CGRect frame =
			    CGRectMake(spec.x + kMimiStackbarInset, spec.y, spec.width - 2 * kMimiStackbarInset, styleCopy.height);
			if (frame.size.width <= 0) {
				[bar orderOut:nil];
				continue;
			}

			[bar setFrame:mimiStackbarCocoaRect(frame) display:NO];
			mimiStackbarDraw(bar, spec, styleCopy);
			[bar orderFrontRegardless];
		}

		// The windows past the ones just drawn belong to stacks that are
		// gone. They are kept for the next sync rather than released.
		for (NSUInteger index = specs.count; index < gBars.count; index++) {
			[gBars[index] orderOut:nil];
		}
	});
}

void MimiStackbarsClear(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		for (NSWindow *bar in gBars) {
			[bar orderOut:nil];
		}
		[gBars removeAllObjects];
	});
}
