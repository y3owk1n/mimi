#import "stackbar.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>

extern int SLSMainConnectionID(void);
extern CGError SLSOrderWindow(int cid, uint32_t wid, int mode, uint32_t relativeTo);

/// SLSOrderWindow's mode for ordering under the relative window, as the
/// border engine uses it.
static const int kMimiStackbarOrderBelow = -1;

// Windows a layout stacked all sit in one frame, so only the one in front is
// seen and nothing says the others are there. This draws them as a deck. The
// window in front holds the middle of the frame, the windows before it show
// above, and the ones after it below, each a little narrower than the one in
// front of it.
//
// The whole deck stays inside the frame the layout set aside. The engine
// takes the room the cards need out of the window in front rather than adding
// it around the outside, so a stack takes up exactly what one window would
// and never lands on a neighbour or runs off the screen.
//
// It draws the windows behind rather than marking the place with a bar or a
// row of icons because that is what macOS itself shows for windows in one
// place: Stage Manager draws a group as offset cards, and Apple's guidance
// puts depth in layering and shadow rather than in a label. A mark of mimi's
// own would sit on a window whose title bar, toolbar and corner radius belong
// to the application and differ in every one of them.
//
// One window per stack, kept in a pool: a sync shows as many as it was given
// and orders the rest out rather than releasing them, since a desktop settles
// on a stack count and a released window would be rebuilt on the next pass.
// Every call goes to the main thread, where the daemon runs its application
// loop.

static NSMutableArray<NSWindow *> *gStacks;

/// The most of a frame the deck behind may take: a quarter of its height for
/// the steps, and a quarter of its width for the taper on both sides.
///
/// The bound is the frame rather than a number of cards, because how many
/// cards fit is a question about the space, not about the stack. A full-height
/// column has room for twenty and will never reach it. A short row in a busy
/// tree has room for three, and drawing five there would leave the window in
/// front too small and the furthest card too narrow to see.
static const double kMimiStackbarMostOfHeight = 0.25;
static const double kMimiStackbarMostOfWidth = 0.25;

/// The corner radius used when the window's own cannot be read.
static const double kMimiStackbarFallbackRadius = 10.0;

void MimiStackbarCards(
    int windows, int active, double width, double height, double step, double taper, int *above, int *below) {
	*above = 0;
	*below = 0;

	if (windows < 2 || step <= 0) {
		return;
	}

	if (active < 0) {
		active = 0;
	}
	if (active > windows - 1) {
		active = windows - 1;
	}

	// What the frame can spare altogether, and how deep one side may go
	// before its furthest card is too narrow to see.
	int budget = (int)(height * kMimiStackbarMostOfHeight / step);
	int deepest = taper > 0 ? (int)(width * kMimiStackbarMostOfWidth / (2 * taper)) : windows;

	int wantAbove = MIN(active, deepest);
	int wantBelow = MIN(windows - 1 - active, deepest);

	// Both sides fit, or the side that wanted less keeps what it asked for
	// and leaves the rest of the budget to the other.
	if (wantAbove + wantBelow > budget) {
		int half = budget / 2;
		if (wantAbove <= half) {
			wantBelow = budget - wantAbove;
		} else if (wantBelow <= half) {
			wantAbove = budget - wantBelow;
		} else {
			wantAbove = half;
			wantBelow = budget - half;
		}
	}

	*above = MAX(wantAbove, 0);
	*below = MAX(wantBelow, 0);
}

static NSRect mimiStackbarCocoaRect(CGRect rect) {
	double primaryHeight = CGDisplayBounds(CGMainDisplayID()).size.height;
	return NSMakeRect(
	    rect.origin.x, primaryHeight - rect.origin.y - rect.size.height, rect.size.width, rect.size.height);
}

static NSWindow *mimiStackbarWindow(void) {
	NSWindow *stack = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1, 1)
	                                              styleMask:NSWindowStyleMaskBorderless
	                                                backing:NSBackingStoreBuffered
	                                                  defer:NO];
	stack.level = NSNormalWindowLevel;
	stack.opaque = NO;
	stack.backgroundColor = [NSColor clearColor];
	stack.hasShadow = NO;
	stack.ignoresMouseEvents = YES;
	stack.releasedWhenClosed = NO;
	stack.animationBehavior = NSWindowAnimationBehaviorNone;
	stack.collectionBehavior = NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle;
	stack.contentView.wantsLayer = YES;
	return stack;
}

/// The corner radius to follow: the window in front's own, as the border
/// under it already reads from the window server, or the style's.
static double mimiStackbarRadius(uint32_t front, MimiStackbarStyle style) {
	double width = 0;
	double radius = 0;
	MimiColor color;
	if (MimiBorderRing(front, &width, &radius, &color) && radius >= 0) {
		return radius;
	}
	return style.radius >= 0 ? style.radius : kMimiStackbarFallbackRadius;
}

/// One part of the way from the nearest card to the furthest, so a deep deck
/// fades away rather than ending on a hard edge.
static CGColorRef mimiStackbarCardColor(MimiStackbarStyle style, double along) {
	double red = style.color.red + (style.farColor.red - style.color.red) * along;
	double green = style.color.green + (style.farColor.green - style.color.green) * along;
	double blue = style.color.blue + (style.farColor.blue - style.color.blue) * along;
	double alpha = style.color.alpha + (style.farColor.alpha - style.color.alpha) * along;
	return CGColorCreateSRGB(red, green, blue, alpha);
}

/// Draw the cards behind the window in front, furthest first so each is drawn
/// over the one behind it and its shadow falls on that one's exposed top.
/// One card, drawn behind the window in front. deep is how far back it
/// stands, which decides how narrow and how faded it is.
static void mimiStackbarCard(
    CALayer *root, CGRect card, int deep, int deepest, double radius, MimiStackbarStyle style) {
	if (card.size.width <= 0 || card.size.height <= 0) {
		return;
	}

	CALayer *layer = [CALayer layer];
	layer.anchorPoint = CGPointZero;
	layer.frame = card;
	layer.cornerRadius = radius;

	double along = deepest <= 1 ? 0 : (double)(deep - 1) / (deepest - 1);
	CGColorRef fill = mimiStackbarCardColor(style, along);
	layer.backgroundColor = fill;
	CGColorRelease(fill);

	// Its own shadow, falling on the card behind it, is what separates one
	// from the next.
	layer.shadowColor = CGColorGetConstantColor(kCGColorBlack);
	layer.shadowOpacity = 0.35f;
	layer.shadowRadius = 4;
	layer.shadowOffset = CGSizeMake(0, -2);

	[root addSublayer:layer];
}

/// Draw the deck: the windows before the one in front above it, the ones
/// after it below, furthest first on each side.
///
/// Where the window in front sits between the two is where it sits in the
/// stack, so the deck says which one is being looked at and which way the
/// others lie, which is what focusing up and down moves through.
static void mimiStackbarDraw(NSWindow *stack, MimiStackbar spec, int above, int below, MimiStackbarStyle style) {
	CALayer *root = stack.contentView.layer;
	root.sublayers = nil;

	if (above <= 0 && below <= 0) {
		return;
	}

	double radius = mimiStackbarRadius(spec.front, style);
	int deepest = MAX(above, below);

	// The layer tree is in Cocoa coordinates, where y counts up. The window
	// in front holds the frame less the steps taken off each end.
	double front = below * style.step;
	double top = spec.height - above * style.step;

	for (int deep = above; deep >= 1; deep--) {
		double in = deep * style.taper;
		CGRect card = CGRectMake(in, front, spec.width - 2 * in, top + deep * style.step - front);
		mimiStackbarCard(root, card, deep, deepest, radius, style);
	}

	for (int deep = below; deep >= 1; deep--) {
		double in = deep * style.taper;
		double bottom = front - deep * style.step;
		CGRect card = CGRectMake(in, bottom, spec.width - 2 * in, top - bottom);
		mimiStackbarCard(root, card, deep, deepest, radius, style);
	}
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
		if (!gStacks) {
			gStacks = [NSMutableArray array];
		}

		while (gStacks.count < specs.count) {
			[gStacks addObject:mimiStackbarWindow()];
		}

		for (NSUInteger index = 0; index < specs.count; index++) {
			MimiStackbar spec;
			[specs[index] getValue:&spec size:sizeof(spec)];

			NSWindow *stack = gStacks[index];
			int above = 0;
			int below = 0;
			MimiStackbarCards(
			    spec.count, spec.active, spec.width, spec.height, styleCopy.step, styleCopy.taper, &above, &below);
			if ((above <= 0 && below <= 0) || spec.width <= 0 || spec.height <= 0) {
				[stack orderOut:nil];
				continue;
			}

			// Exactly the frame the layout set aside.
			CGRect whole = CGRectMake(spec.x, spec.y, spec.width, spec.height);

			[stack setFrame:mimiStackbarCocoaRect(whole) display:NO];
			mimiStackbarDraw(stack, spec, above, below, styleCopy);

			// Shown only when it is not already, then put under the window
			// in front so that only the cards' tops show. Ordering a shown
			// window to the front again races the push back under it, and
			// the cards land over the whole column until the next pass. The
			// border engine orders its rings this way for the same reason.
			//
			// Under the window itself rather than over its ring. The
			// window in front sits inside the frame now, so the border
			// engine draws its ring around that smaller window and the
			// ring never reaches the cards.
			if (!stack.visible) {
				[stack orderFront:nil];
			}
			SLSOrderWindow(SLSMainConnectionID(), (uint32_t)stack.windowNumber, kMimiStackbarOrderBelow, spec.front);
		}

		for (NSUInteger index = specs.count; index < gStacks.count; index++) {
			[gStacks[index] orderOut:nil];
		}
	});
}

void MimiStackbarsClear(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		for (NSWindow *stack in gStacks) {
			[stack orderOut:nil];
		}
		[gStacks removeAllObjects];
	});
}
