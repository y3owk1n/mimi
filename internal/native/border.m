#import "border.h"

#import "mimi.h"
#import "mimi_log.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <stdatomic.h>
#import <unistd.h>

extern int SLSMainConnectionID(void);
extern CGError SLSOrderWindow(int cid, uint32_t wid, int mode, uint32_t relativeTo);
extern CGError SLSGetWindowBounds(int cid, uint32_t wid, CGRect *bounds);

// The window server tells a connection about the windows it asked after, as
// each event happens: a move at every step of a drag, where Accessibility
// tells the daemon a few times a second. The events are numbered as yabai
// and JankyBorders read them.
typedef void (*MimiWindowServerProc)(uint32_t event, void *data, size_t length, void *context, int cid);
extern CGError SLSRegisterConnectionNotifyProc(int cid, MimiWindowServerProc proc, uint32_t event, void *context);
extern CGError SLSRequestNotificationsForWindows(int cid, const uint32_t *windows, int count);

enum {
	kMimiWindowServerMoved = 806,
	kMimiWindowServerResized = 807,
};

// SLSOrderWindow's mode for ordering under the relative window.
static const int kMimiOrderBelow = -1;

// A border is a window of our own, a little larger than the window it
// belongs to and ordered right under it, so the window covers the middle and
// the ring around it shows. Nothing is drawn over another application's
// content, and a window that overlaps a bordered one covers the border as it
// covers the window. The ring is a shape layer with the window's corner cut
// out of it, so the corners under the window's own rounded ones stay clear.
//
// The window server is asked which windows are real and on the spaces in
// front, the same test the queries use, and Accessibility which is focused,
// once per sync from the thread that asks. Every window and layer lives on
// the main thread, where the daemon runs its application loop.

#pragma mark - Types

@interface MimiBorder : NSWindow
// targetBounds is the target's frame the border was last drawn for, in screen
// coordinates, y down.
@property(nonatomic) CGRect targetBounds;
// radius is the window's own corner radius, or -1 when the window server
// does not say.
@property(nonatomic) double radius;
@property(nonatomic) BOOL active;
// space is the space the border was shown on.
@property(nonatomic) uint64_t space;
@property(nonatomic, strong) CAShapeLayer *ring;
@end

@implementation MimiBorder

- (BOOL)canBecomeKeyWindow {
	return NO;
}

- (BOOL)canBecomeMainWindow {
	return NO;
}

- (BOOL)isAccessibilityElement {
	return NO;
}

@end

// The corner radius drawn for a window whose own the window server does not
// report, when the style follows the window: what macOS gives a document
// window.
static const double kMimiFallbackRadius = 12;

// Put border right under its window, in one window server call. AppKit's
// own ordering takes a shown window off the screen and back, which flashes.
static void mimiOrderUnder(MimiBorder *border, uint32_t number) {
	// AppKit has to show the window once for the window server to know it.
	if (!border.visible)
		[border orderFront:nil];
	SLSOrderWindow(SLSMainConnectionID(), (uint32_t)border.windowNumber, kMimiOrderBelow, number);
}

// The borders by the number of the window each belongs to, main thread only.
static NSMutableDictionary<NSNumber *, MimiBorder *> *gBorders;
static MimiBorderStyle gStyle;
static BOOL gEnabled;
static uint32_t gFocused;

// A sync asked for while one waits to run is folded into it, since a drag
// asks faster than the main thread draws.
static atomic_int gQueued;
static atomic_int gWantRefocus;
static atomic_uint gFocusRequest;

#pragma mark - Helpers

// NSWindow frames are y up from the primary display's bottom-left.
static NSRect mimiBorderCocoaRect(CGRect rect) {
	double primaryHeight = CGDisplayBounds(CGMainDisplayID()).size.height;
	return NSMakeRect(
	    rect.origin.x, primaryHeight - rect.origin.y - rect.size.height, rect.size.width, rect.size.height);
}

static CGColorRef mimiBorderColor(MimiColor color) {
	return CGColorCreateSRGB(color.red, color.green, color.blue, color.alpha);
}

// The window the user is typing into, by number, or 0 when there is none.
// An Accessibility round trip into the frontmost application.
static uint32_t mimiFocusedWindowNumber(void) {
	void *window = MimiGetFrontmostWindow();
	if (!window)
		return 0;
	uint32_t number = MimiGetWindowNumber(window);
	MimiReleaseElement(window);
	return number;
}

// The space ids in front on every display. displays and front get each
// display's bounds and the id of the space in front of it.
static NSArray<NSNumber *> *mimiSpacesInFront(CGRect *displays, uint64_t *front, uint32_t *displayCount) {
	NSMutableArray<NSNumber *> *spaces = [NSMutableArray array];
	CGDirectDisplayID ids[16];
	uint32_t count = 0;
	CGGetActiveDisplayList(16, ids, &count);
	for (uint32_t i = 0; i < count; i++) {
		displays[i] = CGDisplayBounds(ids[i]);
		front[i] = MimiDisplayActiveSpaceID(ids[i]);
		if (front[i])
			[spaces addObject:@(front[i])];
	}
	*displayCount = count;
	return spaces;
}

// The id of the space in front on the display that frame overlaps most, or
// 0 when frame overlaps no display.
static uint64_t mimiSpaceUnder(CGRect frame, const CGRect *displays, const uint64_t *front, uint32_t displayCount) {
	uint64_t space = 0;
	double best = 0;
	for (uint32_t i = 0; i < displayCount; i++) {
		CGRect overlap = CGRectIntersection(frame, displays[i]);
		double area = overlap.size.width * overlap.size.height;
		if (area > best) {
			best = area;
			space = front[i];
		}
	}
	return space;
}

// Whether frame fills a display, which is how a full-screen window sits: a
// border under one has nothing to show.
static BOOL mimiFillsADisplay(CGRect frame, const CGRect *displays, uint32_t displayCount) {
	for (uint32_t i = 0; i < displayCount; i++) {
		if (CGRectEqualToRect(frame, displays[i]))
			return YES;
	}
	return NO;
}

// The window server's description of each window: its owner and bounds.
// The ids go in as the pointers CGWindowListCreateDescriptionFromArray
// reads, not as numbers.
static CFArrayRef mimiDescribe(NSArray<NSNumber *> *numbers) {
	CFIndex count = (CFIndex)numbers.count;
	const void **ids = calloc((size_t)count + 1, sizeof(void *));
	for (CFIndex i = 0; i < count; i++) {
		ids[i] = (const void *)(uintptr_t)numbers[(NSUInteger)i].unsignedIntValue;
	}
	CFArrayRef list = CFArrayCreate(NULL, ids, count, NULL);
	free(ids);
	CFArrayRef described = list ? CGWindowListCreateDescriptionFromArray(list) : NULL;
	if (list)
		CFRelease(list);
	return described;
}

#pragma mark - Drawing

static MimiBorder *mimiNewBorder(void) {
	MimiBorder *border = [[MimiBorder alloc] initWithContentRect:NSMakeRect(0, 0, 1, 1)
	                                                   styleMask:NSWindowStyleMaskBorderless
	                                                     backing:NSBackingStoreBuffered
	                                                       defer:NO];
	border.level = NSNormalWindowLevel;
	border.opaque = NO;
	border.backgroundColor = [NSColor clearColor];
	border.hasShadow = NO;
	border.ignoresMouseEvents = YES;
	border.releasedWhenClosed = NO;
	border.animationBehavior = NSWindowAnimationBehaviorNone;
	border.collectionBehavior = NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle;
	NSView *view = border.contentView;
	view.wantsLayer = YES;
	CAShapeLayer *ring = [CAShapeLayer layer];
	ring.fillRule = kCAFillRuleEvenOdd;
	ring.anchorPoint = CGPointZero;
	ring.position = CGPointZero;
	ring.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;
	[view.layer addSublayer:ring];
	border.ring = ring;
	return border;
}

CGPathRef MimiBorderRingPath(CGSize size, double width, double radius) {
	CGRect frame = CGRectMake(0, 0, size.width + 2 * width, size.height + 2 * width);
	CGRect hole = CGRectInset(frame, width, width);
	double inner = MIN(radius, MIN(hole.size.width, hole.size.height) / 2);
	CGMutablePathRef path = CGPathCreateMutable();
	CGPathAddRoundedRect(path, NULL, frame, inner + width, inner + width);
	// The hole is a rounded rect even when square, so every ring path has
	// the same elements and one animates into another.
	CGPathAddRoundedRect(path, NULL, hole, MAX(inner, 0.001), MAX(inner, 0.001));
	return path;
}

// The corner radius drawn for border: the style's, or the window's own.
static double mimiRadiusFor(MimiBorder *border) {
	if (gStyle.radius >= 0)
		return gStyle.radius;
	return border.radius >= 0 ? border.radius : kMimiFallbackRadius;
}

// Draw border for bounds, in the colour for whether it is active: the
// window frame is bounds grown by the width, and the ring is the window
// frame with bounds cut out, both with rounded corners.
static void mimiDrawBorder(MimiBorder *border, CGRect bounds, BOOL active) {
	double width = gStyle.width;
	CGRect outer = CGRectInset(bounds, -width, -width);
	[border setFrame:mimiBorderCocoaRect(outer) display:NO];

	CGRect frame = CGRectMake(0, 0, outer.size.width, outer.size.height);
	CGPathRef path = MimiBorderRingPath(bounds.size, width, mimiRadiusFor(border));

	CGColorRef color = mimiBorderColor(active ? gStyle.active : gStyle.inactive);
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	border.ring.contentsScale = border.backingScaleFactor;
	border.ring.bounds = frame;
	border.ring.path = path;
	border.ring.fillColor = color;
	[CATransaction commit];
	CGColorRelease(color);
	CGPathRelease(path);

	border.targetBounds = bounds;
	border.active = active;
}

static void mimiCloseAll(void) {
	for (MimiBorder *border in gBorders.allValues) {
		[border close];
	}
	[gBorders removeAllObjects];
}

#pragma mark - Following

// followed is whether the window server has been asked for its move and
// resize events, once per process.
static BOOL gFollowing;

// Move a window's border to where the window server has the window now,
// at once and on its own, as one step of a drag. A size change redraws the
// ring; a move alone just moves the window, the cheap case a drag makes
// at every frame.
static void mimiFollow(uint32_t number) {
	MimiBorder *border = gEnabled ? gBorders[@(number)] : nil;
	if (!border)
		return;
	CGRect bounds;
	if (SLSGetWindowBounds(SLSMainConnectionID(), number, &bounds) != kCGErrorSuccess || CGRectIsEmpty(bounds))
		return;
	if (CGRectEqualToRect(bounds, border.targetBounds))
		return;
	if (!CGSizeEqualToSize(bounds.size, border.targetBounds.size)) {
		mimiDrawBorder(border, bounds, border.active);
		return;
	}
	border.targetBounds = bounds;
	[border setFrame:mimiBorderCocoaRect(CGRectInset(bounds, -gStyle.width, -gStyle.width)) display:NO];
}

static void mimiWindowServerEvent(uint32_t event, void *data, size_t length, void *context, int cid) {
	(void)event;
	(void)context;
	(void)cid;
	if (!data || length < sizeof(uint32_t))
		return;
	uint32_t number;
	memcpy(&number, data, sizeof(number));
	if ([NSThread isMainThread]) {
		mimiFollow(number);
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		mimiFollow(number);
	});
}

// Ask the window server for the events of every bordered window. It keeps
// the last list given, so the whole list goes each time it changes.
static void mimiFollowBordered(void) {
	int cid = SLSMainConnectionID();
	if (!gFollowing) {
		gFollowing = YES;
		SLSRegisterConnectionNotifyProc(cid, mimiWindowServerEvent, kMimiWindowServerMoved, NULL);
		SLSRegisterConnectionNotifyProc(cid, mimiWindowServerEvent, kMimiWindowServerResized, NULL);
	}
	NSArray<NSNumber *> *keys = gBorders.allKeys;
	uint32_t *numbers = calloc(keys.count + 1, sizeof(uint32_t));
	int count = 0;
	for (NSNumber *key in keys) {
		numbers[count++] = key.unsignedIntValue;
	}
	SLSRequestNotificationsForWindows(cid, numbers, count);
	free(numbers);
}

#pragma mark - Sync

// Bring the borders up to date, on the main thread. focused is the window
// Accessibility named, or 0 to keep the last answer.
static void mimiSyncOnMain(uint32_t focused, BOOL refocus) {
	if (!gEnabled)
		return;
	uint32_t raised = 0;
	if (refocus && focused != gFocused) {
		gFocused = focused;
		raised = focused;
	}

	CGRect displays[16];
	uint64_t front[16];
	uint32_t displayCount = 0;
	NSArray<NSNumber *> *spaces = mimiSpacesInFront(displays, front, &displayCount);
	CFArrayRef radiiRef = NULL;
	NSArray<NSNumber *> *numbers =
	    CFBridgingRelease(MimiCopyRealWindowsOnSpaces((__bridge CFArrayRef)spaces, &radiiRef));
	NSArray<NSNumber *> *radii = CFBridgingRelease(radiiRef);
	CFArrayRef described = mimiDescribe(numbers);

	pid_t self = getpid();
	NSMutableSet<NSNumber *> *seen = [NSMutableSet set];
	int made = 0;
	int moved = 0;
	for (NSDictionary *info in (__bridge NSArray *)described) {
		if ([info[(id)kCGWindowOwnerPID] intValue] == self)
			continue;
		CGRect bounds;
		if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], &bounds))
			continue;
		if (CGRectIsEmpty(bounds) || mimiFillsADisplay(bounds, displays, displayCount))
			continue;

		NSNumber *key = info[(id)kCGWindowNumber];
		uint32_t number = key.unsignedIntValue;
		BOOL active = number == gFocused;
		uint64_t space = mimiSpaceUnder(bounds, displays, front, displayCount);
		MimiBorder *border = gBorders[key];
		// A border stays on the space it was shown on, so a window that
		// moved to another space left its border behind. Close that one
		// and make a new one on the window's space.
		if (border && border.space != space) {
			[border close];
			border = nil;
			moved++;
		}
		BOOL fresh = border == nil;
		if (fresh) {
			border = mimiNewBorder();
			border.space = space;
			gBorders[key] = border;
			made++;
		}
		NSUInteger at = [numbers indexOfObject:key];
		double radius = at < radii.count ? radii[at].doubleValue : -1;
		BOOL reshaped = radius != border.radius;
		border.radius = radius;
		if (fresh || reshaped || !CGRectEqualToRect(border.targetBounds, bounds) || border.active != active) {
			mimiDrawBorder(border, bounds, active);
		}
		// The window server keeps no tie between a window and the border
		// under it, so a window that comes to the front leaves its border
		// where it was, and the border follows it. The rest of the stack
		// has not moved, so the other borders are left alone. Ordering a
		// shown window again is what flashes.
		if (fresh || number == raised) {
			mimiOrderUnder(border, number);
		}
		[seen addObject:key];
	}
	if (described)
		CFRelease(described);

	int dropped = 0;
	for (NSNumber *key in gBorders.allKeys) {
		if ([seen containsObject:key])
			continue;
		[gBorders[key] close];
		[gBorders removeObjectForKey:key];
		dropped++;
	}

	if (made || dropped || moved) {
		mimiFollowBordered();
		MIMI_LOG(
		    "borders synced: %lu shown, %d added, %d dropped, %d moved across spaces", (unsigned long)gBorders.count,
		    made, dropped, moved);
	}
}

#pragma mark - C Interface

void MimiBordersSetStyle(const MimiBorderStyle *style) {
	MimiBorderStyle copy = *style;
	uint32_t focused = mimiFocusedWindowNumber();
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!gBorders)
			gBorders = [NSMutableDictionary new];
		gStyle = copy;
		gEnabled = YES;
		for (MimiBorder *border in gBorders.allValues) {
			mimiDrawBorder(border, border.targetBounds, border.active);
		}
		mimiSyncOnMain(focused, YES);
	});
}

void MimiBordersSync(int refocus) {
	if (refocus) {
		atomic_store(&gFocusRequest, mimiFocusedWindowNumber());
		atomic_store(&gWantRefocus, 1);
	}
	if (atomic_exchange(&gQueued, 1))
		return;
	dispatch_async(dispatch_get_main_queue(), ^{
		atomic_store(&gQueued, 0);
		BOOL want = atomic_exchange(&gWantRefocus, 0) != 0;
		mimiSyncOnMain(atomic_load(&gFocusRequest), want);
	});
}

uint32_t MimiBorderWindowNumber(uint32_t number) {
	MimiBorder *border = gEnabled ? gBorders[@(number)] : nil;
	return border ? (uint32_t)border.windowNumber : 0;
}

int MimiBorderRing(uint32_t number, double *width, double *radius, MimiColor *color) {
	MimiBorder *border = gEnabled ? gBorders[@(number)] : nil;
	if (!border)
		return 0;
	*width = gStyle.width;
	*radius = mimiRadiusFor(border);
	*color = border.active ? gStyle.active : gStyle.inactive;
	return 1;
}

void MimiBordersClear(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		gEnabled = NO;
		mimiCloseAll();
	});
}
