#import "border.h"

#import "mimi.h"

#import <Cocoa/Cocoa.h>
#import <stdatomic.h>
#import <unistd.h>

extern int SLSMainConnectionID(void);
extern CGError SLSGetWindowBounds(int cid, uint32_t wid, CGRect *bounds);
extern int SLSSpaceGetType(int cid, uint64_t sid);

// A border is a window the window server makes for the daemon directly,
// not an AppKit window. A click on an application window does not raise it
// over a border of this kind, as it does over an AppKit window ordered
// above it, and an order set on the border against the window holds.
extern CGError CGSNewRegionWithRect(const CGRect *rect, CFTypeRef *region);
extern CGError SLSNewWindow(int cid, int type, float x, float y, CFTypeRef region, uint32_t *wid);
extern CGError SLSReleaseWindow(int cid, uint32_t wid);
extern CGError SLSSetWindowTags(int cid, uint32_t wid, const uint64_t *tags, int size);
extern CGError SLSSetWindowResolution(int cid, uint32_t wid, double resolution);
extern CGError SLSSetWindowOpacity(int cid, uint32_t wid, bool opaque);
extern CGError SLSSetWindowLevel(int cid, uint32_t wid, int level);
extern CGError SLSMoveWindowsToManagedSpace(int cid, CFArrayRef windows, uint64_t sid);
extern CGContextRef SLWindowContextCreate(int cid, uint32_t wid, CFDictionaryRef options);
extern CGError SLSFlushWindowContentRegion(int cid, uint32_t wid, void *dirty);
extern CFTypeRef SLSTransactionCreate(int cid);
extern CGError SLSTransactionMoveWindowWithGroup(CFTypeRef transaction, uint32_t wid, CGPoint origin);
extern CGError SLSTransactionOrderWindow(CFTypeRef transaction, uint32_t wid, int mode, uint32_t relative);
extern CGError SLSTransactionCommit(CFTypeRef transaction, int synchronous);
extern CGError SLSTransactionSetWindowLevel(CFTypeRef transaction, uint32_t wid, int level);

// The window server tells a connection about the windows it asked after, as
// each event happens: a move at every step of a drag, where Accessibility
// tells the daemon a few times a second, and a reorder, which Accessibility
// reports only when it moves the focus. It also tells every connection as
// windows are added and removed, the Dock's among them.
typedef void (*MimiWindowServerProc)(uint32_t event, void *data, size_t length, void *context, int cid);
extern CGError SLSRegisterConnectionNotifyProc(int cid, MimiWindowServerProc proc, uint32_t event, void *context);
extern CGError SLSRequestNotificationsForWindows(int cid, const uint32_t *windows, int count);

enum {
	kMimiWindowServerMoved = 806,
	kMimiWindowServerResized = 807,
	kMimiWindowServerReordered = 808,
	kMimiWindowServerAdded = 1325,
	kMimiWindowServerRemoved = 1326,
};

// The level of the window the Dock lays over each display while Mission
// Control or App Expose is up.
static const int kMimiDockOverlayLevel = 20;

// SLSSpaceGetType's answer for a full-screen application's space, the same
// "type" MimiDisplaySpaceIsFullScreen reads. Asking one space is cheap
// enough for a sync at every step of a drag.
static const int kMimiSpaceFullScreen = 4;

// The tags a border window is made with.
static const uint64_t kMimiBorderTags = (1ULL << 1) | (1ULL << 9);

// The display under frame, or the main one when frame is off every display.
static CGDirectDisplayID mimiDisplayUnder(CGRect frame) {
	CGDirectDisplayID display;
	uint32_t count = 0;
	if (CGGetDisplaysWithRect(frame, 1, &display, &count) != kCGErrorSuccess || !count)
		return CGMainDisplayID();
	return display;
}

// The scale of the display under frame, the resolution a border there is
// drawn at. A border keeps the resolution it was made with, and is made
// again when its size changes.
static double mimiScaleUnder(CGRect frame) {
	CGDisplayModeRef mode = CGDisplayCopyDisplayMode(mimiDisplayUnder(frame));
	if (!mode)
		return 1;
	double scale = (double)CGDisplayModeGetPixelWidth(mode) / (double)CGDisplayModeGetWidth(mode);
	CGDisplayModeRelease(mode);
	return scale >= 2 ? 2 : 1;
}

// Outside, a border is a little larger than the window it belongs to and
// ordered right under it, so the window covers the middle and the ring
// around it shows. Nothing is drawn over another application's content, and
// a window that overlaps a bordered one covers the border as it covers the
// window. Inside, it is the window's own size and ordered right over it, so
// the ring covers the window's edge and nothing else. A window over the
// bordered one covers both. Either way the ring is drawn with the middle
// cleared, so the corners under the window's own rounded ones stay clear.
//
// The window server is asked which windows are real and on the spaces in
// front, the same test the queries use, and Accessibility which is focused,
// once per sync from the thread that asks. Every border lives on the main
// thread, where the daemon runs its application loop.

#pragma mark - Types

// An edge is one of the windows a border is made of. At rest there are
// four, each a strip along one side of the ring. The top and bottom ones
// run the ring's whole width and hold its corners. The left and right ones
// fill the height between them. A window the size of the ring would carry a
// backing the size of the window it borders, nearly all of it clear.
typedef struct {
	// number is the edge window's own number, 0 when the strip is empty.
	uint32_t number;
	// context draws into the edge window.
	CGContextRef context;
	// frame is where the edge window is, in screen coordinates, y down.
	CGRect frame;
	// ring is where the ring was drawn, from the window's top-left corner,
	// y down.
	CGRect ring;
} MimiEdge;

enum { kMimiEdgeTop, kMimiEdgeBottom, kMimiEdgeLeft, kMimiEdgeRight, kMimiEdgeCount };

@interface MimiBorder : NSObject {
  @public
	MimiEdge edges[kMimiEdgeCount];
}
// targetBounds is the target's frame the border was last drawn for, in screen
// coordinates, y down.
@property(nonatomic) CGRect targetBounds;
// radius is the window's own corner radius, or -1 when the window server
// does not say.
@property(nonatomic) double radius;
@property(nonatomic) BOOL active;
// painted is whether the edges hold the ring in the current style and
// colour, drawn with paintedRadius.
@property(nonatomic) BOOL painted;
@property(nonatomic) double paintedRadius;
// space is the space the border was shown on.
@property(nonatomic) uint64_t space;
// stretched is whether the border is one window the size of its display,
// as it is while its window is being resized, with the ring drawn at the
// window's place inside it, and stretch is the display's frame then.
@property(nonatomic) BOOL stretched;
@property(nonatomic) CGRect stretch;
// drag counts the resizes followed, so that a settle scheduled for one
// resize is ignored once another has come.
@property(nonatomic) NSUInteger drag;
// target is the number of the window the border belongs to.
@property(nonatomic) uint32_t target;
@end

@implementation MimiBorder
@end

// The corner radius drawn for a window whose own the window server does not
// report, when the style follows the window: what macOS gives a document
// window.
static const double kMimiFallbackRadius = 12;

// The borders by the number of the window each belongs to, main thread only.
static NSMutableDictionary<NSNumber *, MimiBorder *> *gBorders;
static MimiBorderStyle gStyle;
static BOOL gEnabled;
static uint32_t gFocused;
// hidden is whether the borders are ordered out for Mission Control.
static BOOL gHidden;

// A sync asked for while one waits to run is folded into it, since a drag
// asks faster than the main thread draws.
static atomic_int gQueued;
static atomic_int gWantRefocus;

#pragma mark - Helpers

// Order border above (1) or below (-1) window number, or off the screen (0).
static void mimiOrder(MimiBorder *border, int mode, uint32_t number) {
	CFTypeRef transaction = SLSTransactionCreate(SLSMainConnectionID());
	for (int i = 0; i < kMimiEdgeCount; i++) {
		if (border->edges[i].number)
			SLSTransactionOrderWindow(transaction, border->edges[i].number, mode, number);
	}
	SLSTransactionCommit(transaction, 1);
	CFRelease(transaction);
}

// Put border right next to its window: under it outside, over it inside.
// The focused window's inside border goes a level up instead. An
// application that raises its window cannot put it over a border there.
// The focused window is in front, so a ring a level up draws over nothing
// else, unless a window of another application floats over it.
static void mimiOrderBeside(MimiBorder *border, uint32_t number) {
	int level = gStyle.inside && border.active ? kCGNormalWindowLevel + 1 : kCGNormalWindowLevel;
	CFTypeRef transaction = SLSTransactionCreate(SLSMainConnectionID());
	for (int i = 0; i < kMimiEdgeCount; i++) {
		uint32_t edge = border->edges[i].number;
		if (!edge)
			continue;
		SLSTransactionSetWindowLevel(transaction, edge, level);
		SLSTransactionOrderWindow(transaction, edge, gStyle.inside ? 1 : -1, number);
	}
	SLSTransactionCommit(transaction, 1);
	CFRelease(transaction);
}

// Whether Mission Control or App Expose is up: the Dock then has a window
// over a whole display. They lay the windows out without their borders, so
// the borders come down for them.
static BOOL mimiMissionControlUp(void) {
	NSArray *list = CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID));
	for (NSDictionary *info in list) {
		if ([info[(id)kCGWindowLayer] intValue] != kMimiDockOverlayLevel)
			continue;
		if (![info[(id)kCGWindowOwnerName] isEqualToString:@"Dock"])
			continue;
		CGRect bounds;
		if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], &bounds))
			continue;
		CGDirectDisplayID display;
		uint32_t count = 0;
		if (CGGetDisplaysWithRect(bounds, 1, &display, &count) == kCGErrorSuccess && count &&
		    CGRectEqualToRect(bounds, CGDisplayBounds(display)))
			return YES;
	}
	return NO;
}

// Move every edge of border by offset, in one transaction.
static void mimiShiftBorder(MimiBorder *border, CGPoint offset) {
	CFTypeRef transaction = SLSTransactionCreate(SLSMainConnectionID());
	for (int i = 0; i < kMimiEdgeCount; i++) {
		MimiEdge *edge = &border->edges[i];
		edge->frame = CGRectOffset(edge->frame, offset.x, offset.y);
		if (edge->number)
			SLSTransactionMoveWindowWithGroup(transaction, edge->number, edge->frame.origin);
	}
	SLSTransactionCommit(transaction, 1);
	CFRelease(transaction);
}

// The border window's frame for a window at bounds: the bounds grown by the
// width outside, the bounds themselves inside.
static CGRect mimiBorderFrame(CGRect bounds) {
	if (gStyle.inside)
		return bounds;
	return CGRectInset(bounds, -gStyle.width, -gStyle.width);
}

static CGColorRef mimiBorderColor(MimiColor color) {
	return CGColorCreateSRGB(color.red, color.green, color.blue, color.alpha);
}

// The space ids in front on every display. displays and front get each
// display's bounds and the id of the space in front of it, and fullScreen
// whether that space is a full-screen application's.
static NSArray<NSNumber *> *mimiSpacesInFront(
    CGRect *displays, uint64_t *front, BOOL *fullScreen, uint32_t *displayCount) {
	NSMutableArray<NSNumber *> *spaces = [NSMutableArray array];
	CGDirectDisplayID ids[16];
	uint32_t count = 0;
	CGGetActiveDisplayList(16, ids, &count);
	for (uint32_t i = 0; i < count; i++) {
		displays[i] = CGDisplayBounds(ids[i]);
		front[i] = MimiDisplayActiveSpaceID(ids[i]);
		fullScreen[i] = front[i] && SLSSpaceGetType(SLSMainConnectionID(), front[i]) == kMimiSpaceFullScreen;
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

// Whether a window at frame is full screen, so a border under it has nothing
// to show. It is when the window fills a display, or when it sits on a display
// whose space in front belongs to a full-screen application.
//
// Checking the space matters because an application in full screen may split
// into several windows. Brave splits into a title strip, a toolbar and the
// page, and none of them fills the display. Checking the bounds covers the
// moment on the way in, when the window already fills the display and the
// space is not yet full screen.
static BOOL mimiFullScreen(CGRect frame, const CGRect *displays, const BOOL *fullScreen, uint32_t displayCount) {
	double best = 0;
	BOOL under = NO;
	for (uint32_t i = 0; i < displayCount; i++) {
		if (CGRectEqualToRect(frame, displays[i]))
			return YES;
		CGRect overlap = CGRectIntersection(frame, displays[i]);
		double area = overlap.size.width * overlap.size.height;
		if (area > best) {
			best = area;
			under = fullScreen[i];
		}
	}
	return under;
}

// Whether the window described by info gets a border, and where it is: not
// one of ours, on the screen, with bounds the window server gives, and not
// full screen. Finder keeps a window closed with Command-W rather than
// destroying it, and it stays listed on its space, off the screen.
static BOOL mimiBordered(
    NSDictionary *info, pid_t self, const CGRect *displays, const BOOL *fullScreen, uint32_t displayCount,
    CGRect *bounds) {
	if ([info[(id)kCGWindowOwnerPID] intValue] == self || ![info[(id)kCGWindowIsOnscreen] boolValue])
		return NO;
	if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], bounds))
		return NO;
	return !CGRectIsEmpty(*bounds) && !mimiFullScreen(*bounds, displays, fullScreen, displayCount);
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

// A region of size at the origin, the shape a border window takes.
static CFTypeRef mimiRegion(CGSize size) {
	CGRect rect = CGRectMake(0, 0, size.width, size.height);
	CFTypeRef region = NULL;
	CGSNewRegionWithRect(&rect, &region);
	return region;
}

// Make edge a window of size on border's space, off screen until it is
// moved and ordered, or no window when size is empty. It is made on the
// space so that it shows with the windows there and not on whichever space
// is in front of the main display.
static void mimiMakeEdge(MimiBorder *border, MimiEdge *edge, CGSize size, double scale) {
	edge->number = 0;
	edge->context = NULL;
	edge->frame = CGRectMake(-9999, -9999, size.width, size.height);
	if (size.width <= 0 || size.height <= 0)
		return;
	int cid = SLSMainConnectionID();
	CFTypeRef region = mimiRegion(size);
	uint32_t number = 0;
	SLSNewWindow(cid, kCGBackingStoreBuffered, -9999, -9999, region, &number);
	CFRelease(region);
	SLSSetWindowTags(cid, number, &kMimiBorderTags, 64);
	SLSSetWindowResolution(cid, number, scale);
	SLSSetWindowOpacity(cid, number, false);
	SLSSetWindowLevel(cid, number, kCGNormalWindowLevel);
	CFArrayRef windows = CFBridgingRetain(@[ @(number) ]);
	SLSMoveWindowsToManagedSpace(cid, windows, border.space);
	CFRelease(windows);

	edge->number = number;
	edge->context = SLWindowContextCreate(cid, number, NULL);
	CGContextSetInterpolationQuality(edge->context, kCGInterpolationNone);
}

static MimiBorder *mimiNewBorder(uint64_t space, uint32_t target) {
	MimiBorder *border = [MimiBorder new];
	border.space = space;
	border.target = target;
	return border;
}

static void mimiReleaseEdge(MimiEdge edge) {
	if (!edge.number)
		return;
	CGContextRelease(edge.context);
	SLSReleaseWindow(SLSMainConnectionID(), edge.number);
}

static void mimiCloseBorder(MimiBorder *border) {
	for (int i = 0; i < kMimiEdgeCount; i++) {
		mimiReleaseEdge(border->edges[i]);
		border->edges[i] = (MimiEdge){0};
	}
}

CGPathRef MimiBorderRingPath(CGSize size, double width, double radius, int inside) {
	double grow = inside ? 0 : width;
	CGRect frame = CGRectMake(0, 0, size.width + 2 * grow, size.height + 2 * grow);
	CGRect hole = CGRectInset(frame, width, width);
	// The window's own corner is the hole's edge outside and the frame's
	// edge inside. The other edge runs parallel to it, width away.
	double outer = MIN(radius + grow, MIN(frame.size.width, frame.size.height) / 2);
	double inner = MIN(MAX(outer - width, 0), MIN(hole.size.width, hole.size.height) / 2);
	CGMutablePathRef path = CGPathCreateMutable();
	CGPathAddRoundedRect(path, NULL, frame, outer, outer);
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

// The depth of the top and bottom edges of a ring around frame. It holds the
// rounded corners and is at most half the frame, so the two never overlap.
// The outer radius is the one MimiBorderRingPath rounds by.
static double mimiEdgeDepth(CGRect frame, double radius) {
	double shorter = MIN(frame.size.width, frame.size.height);
	double outer = MIN(radius + (gStyle.inside ? 0 : gStyle.width), shorter / 2);
	return MIN(ceil(MAX(gStyle.width, outer)), floor(shorter / 2));
}

// The frames of border's edges for a ring around frame, depth deep, in
// screen coordinates, y down, and own, the part of the ring each draws. At
// rest they are four strips. The top and bottom span the ring's whole width,
// and the left and right sit between them. Stretched, the first edge is the
// whole of stretch and draws the whole ring, and the rest are empty. A
// resize step then redraws one window and moves none. When some strips move
// while others are redrawn, the window server shows the two on different
// frames and the corners between them tear.
static void mimiEdgeFrames(CGRect frame, double depth, const CGRect *stretch, CGRect *frames, CGRect *own) {
	if (stretch) {
		frames[0] = *stretch;
		own[0] = frame;
		for (int i = 1; i < kMimiEdgeCount; i++) {
			frames[i] = own[i] = CGRectZero;
		}
		return;
	}
	double side = frame.size.height - 2 * depth;
	frames[kMimiEdgeTop] = CGRectMake(frame.origin.x, frame.origin.y, frame.size.width, depth);
	frames[kMimiEdgeBottom] = CGRectMake(frame.origin.x, CGRectGetMaxY(frame) - depth, frame.size.width, depth);
	frames[kMimiEdgeLeft] = CGRectMake(frame.origin.x, frame.origin.y + depth, depth, side);
	frames[kMimiEdgeRight] = CGRectMake(CGRectGetMaxX(frame) - depth, frame.origin.y + depth, depth, side);
	for (int i = 0; i < kMimiEdgeCount; i++) {
		own[i] = frames[i];
	}
}

// Clear the four strips of a ring drawn in box, each thick deep. That is
// where its sides and its rounded corners were. The middle is clear already.
static void mimiClearRing(CGContextRef context, CGRect box, double thick) {
	CGContextClearRect(context, CGRectMake(box.origin.x, box.origin.y, box.size.width, thick));
	CGContextClearRect(context, CGRectMake(box.origin.x, CGRectGetMaxY(box) - thick, box.size.width, thick));
	CGContextClearRect(context, CGRectMake(box.origin.x, box.origin.y, thick, box.size.height));
	CGContextClearRect(context, CGRectMake(CGRectGetMaxX(box) - thick, box.origin.y, thick, box.size.height));
}

// Draw the part of ring that falls in own into edge, a window at frame.
// ring has its origin at outer's bottom-left corner, and the context's is
// the window's bottom-left corner, y up. Each strip draws only its own part,
// so a translucent corner is not drawn twice. The ring is filled once for
// each of strips that own overlaps. One fill of the whole ring makes
// CoreGraphics allocate a buffer the size of the ring, middle included. The
// allocator keeps those pages after CoreGraphics frees the buffer. When was
// is null, the whole window is cleared first. Otherwise only the ring last
// drawn at was is cleared, since a stretched edge is the size of the
// display. was is measured from the window's top-left corner, y down.
static void mimiDrawEdge(
    MimiEdge *edge, CGPathRef ring, CGRect outer, CGRect frame, CGRect own, const CGRect *strips, CGColorRef color,
    CGRect was, double thick) {
	CGContextRef context = edge->context;
	if (CGRectIsNull(was))
		CGContextClearRect(context, CGRectMake(0, 0, frame.size.width, frame.size.height));
	else
		mimiClearRing(
		    context, CGRectMake(was.origin.x, frame.size.height - CGRectGetMaxY(was), was.size.width, was.size.height),
		    thick);
	CGContextSetFillColorWithColor(context, color);
	for (int i = 0; i < kMimiEdgeCount; i++) {
		CGRect part = CGRectIntersection(own, strips[i]);
		if (CGRectIsEmpty(part))
			continue;
		CGContextSaveGState(context);
		CGContextClipToRect(
		    context, CGRectMake(
		                 part.origin.x - frame.origin.x, CGRectGetMaxY(frame) - CGRectGetMaxY(part), part.size.width,
		                 part.size.height));
		CGContextTranslateCTM(context, outer.origin.x - frame.origin.x, CGRectGetMaxY(frame) - CGRectGetMaxY(outer));
		CGContextAddPath(context, ring);
		CGContextEOFillPath(context);
		CGContextRestoreGState(context);
	}
	CGContextFlush(context);
}

// Draw border for bounds, in the colour for whether it is active: the ring
// is bounds grown by the width outside or bounds inside, with its middle
// cut out, both with rounded corners, split across the edges.
static const int64_t kMimiSwapOverlap = 2 * NSEC_PER_SEC / 60;

static void mimiDrawBorder(MimiBorder *border, CGRect bounds, BOOL active) {
	int cid = SLSMainConnectionID();
	CGRect outer = mimiBorderFrame(bounds);
	double radius = mimiRadiusFor(border);
	double depth = mimiEdgeDepth(outer, radius);
	CGRect own[kMimiEdgeCount];
	CGRect frames[kMimiEdgeCount];
	CGRect strips[kMimiEdgeCount];
	CGRect stretch = border.stretch;
	mimiEdgeFrames(outer, depth, border.stretched ? &stretch : NULL, frames, own);
	mimiEdgeFrames(outer, depth, NULL, strips, strips);
	double scale = mimiScaleUnder(outer);
	CGPathRef ring = MimiBorderRingPath(bounds.size, gStyle.width, radius, gStyle.inside);
	CGColorRef color = mimiBorderColor(active ? gStyle.active : gStyle.inactive);

	// An edge whose size changes gets a new window, drawn and put in place
	// before the old one goes. A window reshaped in place shows its last
	// drawing in the new shape for a frame, even with screen updates
	// disabled and the window frozen. The old window stays up two frames
	// more, since the new one is not on the screen until the window server
	// draws it. An edge that holds the same drawing, as a strip of a window
	// that only moved does, is only moved. Drawing into a window waits for
	// the window server to take the last drawing.
	BOOL repaint = !border.painted || active != border.active || radius != border.paintedRadius;
	MimiEdge old[kMimiEdgeCount] = {0};
	BOOL made = NO;
	CFTypeRef transaction = SLSTransactionCreate(cid);
	for (int i = 0; i < kMimiEdgeCount; i++) {
		MimiEdge *edge = &border->edges[i];
		if (!CGSizeEqualToSize(frames[i].size, edge->frame.size)) {
			old[i] = *edge;
			mimiMakeEdge(border, edge, frames[i].size, scale);
			made = made || edge->number;
		}
		CGPoint origin = frames[i].origin;
		CGRect at = CGRectOffset(outer, -origin.x, -origin.y);
		if (edge->number && (repaint || old[i].number || !CGRectEqualToRect(at, edge->ring))) {
			CGRect was = repaint || old[i].number || !border.stretched ? CGRectNull : edge->ring;
			mimiDrawEdge(edge, ring, outer, frames[i], own[i], strips, color, was, gStyle.width + radius + 1);
			SLSFlushWindowContentRegion(cid, edge->number, NULL);
		}
		if (edge->number && !CGPointEqualToPoint(origin, edge->frame.origin))
			SLSTransactionMoveWindowWithGroup(transaction, edge->number, origin);
		edge->frame = frames[i];
		edge->ring = at;
	}
	SLSTransactionCommit(transaction, 1);
	CFRelease(transaction);
	if (made)
		mimiOrderBeside(border, border.target);
	CGColorRelease(color);
	CGPathRelease(ring);

	for (int i = 0; i < kMimiEdgeCount; i++) {
		MimiEdge edge = old[i];
		if (!edge.number)
			continue;
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, kMimiSwapOverlap), dispatch_get_main_queue(), ^{
			mimiReleaseEdge(edge);
		});
	}
	border.targetBounds = bounds;
	border.active = active;
	border.painted = YES;
	border.paintedRadius = radius;
}

static void mimiCloseAll(void) {
	for (MimiBorder *border in gBorders.allValues) {
		mimiCloseBorder(border);
	}
	[gBorders removeAllObjects];
}

#pragma mark - Following

// followed is whether the window server has been asked for its move and
// resize events, once per process.
static BOOL gFollowing;

// A window resized live gets its border stretched to the display it is
// on, so that each step is a redraw and not a reshape. The window server
// shows a window reshaped in height with its drawing anchored at the
// bottom for a frame, and a resize reshapes at every step. Half a second
// after the last resize the border goes back to its four strips.
static const int64_t kMimiResizeSettle = 500 * NSEC_PER_MSEC;

static void mimiSettle(MimiBorder *border, NSUInteger drag) {
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, kMimiResizeSettle), dispatch_get_main_queue(), ^{
		if (!gEnabled || gBorders[@(border.target)] != border || border.drag != drag)
			return;
		border.stretched = NO;
		mimiDrawBorder(border, border.targetBounds, border.active);
	});
}

// Move a window's border to where the window server has the window now,
// at once and on its own, as one step of a drag. A size change redraws the
// ring, in a window stretched to the display; a move alone just moves the
// edges, the cheap case a drag makes at every frame.
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
		border.stretched = YES;
		border.stretch = CGDisplayBounds(mimiDisplayUnder(bounds));
		border.drag++;
		mimiDrawBorder(border, bounds, border.active);
		mimiSettle(border, border.drag);
		return;
	}
	if (border.stretched) {
		mimiDrawBorder(border, bounds, border.active);
		return;
	}
	CGPoint offset =
	    CGPointMake(bounds.origin.x - border.targetBounds.origin.x, bounds.origin.y - border.targetBounds.origin.y);
	border.targetBounds = bounds;
	mimiShiftBorder(border, offset);
}

// Put a window's border back beside it after the window server reordered
// the window, as when the window's application raised it at the end of a
// resize. The border is ordered at once, without checking the stacking
// first. The event arrives before the window server lists the window in
// its new place, and every frame the window spends over its border is a
// frame without a ring.
static void mimiReorder(uint32_t number) {
	MimiBorder *border = gEnabled ? gBorders[@(number)] : nil;
	if (border)
		mimiOrderBeside(border, number);
}

static void mimiSyncOnMain(BOOL refocus);

// Take the borders down while Mission Control is up and bring them back
// after. The Dock's windows come and go in a burst, which runs one check.
// The Dock takes its overlay down after the burst, so while the borders are
// down the check runs again a little later.
static atomic_int gCheckQueued;
static const int64_t kMimiMissionControlRecheck = 200 * NSEC_PER_MSEC;

static void mimiCheckMissionControl(void) {
	if (atomic_exchange(&gCheckQueued, 1))
		return;
	dispatch_async(dispatch_get_main_queue(), ^{
		atomic_store(&gCheckQueued, 0);
		if (!gEnabled)
			return;
		BOOL up = mimiMissionControlUp();
		if (gHidden && !up) {
			mimiSyncOnMain(NO);
			return;
		}
		if (up && !gHidden) {
			for (MimiBorder *border in gBorders.allValues) {
				mimiOrder(border, 0, 0);
			}
			gHidden = YES;
		}
		if (gHidden) {
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, kMimiMissionControlRecheck), dispatch_get_main_queue(), ^{
				mimiCheckMissionControl();
			});
		}
	});
}

static void mimiApply(uint32_t event, uint32_t number) {
	if (event == kMimiWindowServerReordered)
		mimiReorder(number);
	else
		mimiFollow(number);
}

static void mimiWindowServerEvent(uint32_t event, void *data, size_t length, void *context, int cid) {
	(void)context;
	(void)cid;
	if (!data || length < sizeof(uint32_t))
		return;
	if (event == kMimiWindowServerAdded || event == kMimiWindowServerRemoved) {
		mimiCheckMissionControl();
		return;
	}
	uint32_t number;
	memcpy(&number, data, sizeof(number));
	if ([NSThread isMainThread]) {
		mimiApply(event, number);
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		mimiApply(event, number);
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
		SLSRegisterConnectionNotifyProc(cid, mimiWindowServerEvent, kMimiWindowServerReordered, NULL);
		SLSRegisterConnectionNotifyProc(cid, mimiWindowServerEvent, kMimiWindowServerAdded, NULL);
		SLSRegisterConnectionNotifyProc(cid, mimiWindowServerEvent, kMimiWindowServerRemoved, NULL);
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

// Bring the borders up to date, on the main thread. With refocus, the
// focused window is found again. It is the front process's window nearest
// the front, among the windows the borders are drawn for. Accessibility's
// focused window is not asked. Safari keeps reporting the window that was
// focused before when the focus moved without a click.
static void mimiSyncOnMain(BOOL refocus) {
	if (!gEnabled)
		return;
	if (gHidden && mimiMissionControlUp())
		return;
	BOOL reshow = gHidden;
	gHidden = NO;

	CGRect displays[16];
	uint64_t front[16];
	BOOL fullScreen[16];
	uint32_t displayCount = 0;
	NSArray<NSNumber *> *spaces = mimiSpacesInFront(displays, front, fullScreen, &displayCount);
	CFArrayRef radiiRef = NULL;
	NSArray<NSNumber *> *numbers =
	    CFBridgingRelease(MimiCopyRealWindowsOnSpaces((__bridge CFArrayRef)spaces, &radiiRef));
	NSArray<NSNumber *> *radii = CFBridgingRelease(radiiRef);
	CFArrayRef described = mimiDescribe(numbers);

	pid_t self = getpid();
	// The window server lists the windows on the screen front to back.
	uint32_t raised = 0;
	if (refocus) {
		pid_t frontPid = MimiFrontmostPid();
		uint32_t focused = 0;
		NSSet<NSNumber *> *real = [NSSet setWithArray:numbers];
		NSArray *onScreen =
		    CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID));
		for (NSDictionary *info in onScreen) {
			if ([info[(id)kCGWindowOwnerPID] intValue] == frontPid && [real containsObject:info[(id)kCGWindowNumber]]) {
				focused = [info[(id)kCGWindowNumber] unsignedIntValue];
				break;
			}
		}
		if (focused != gFocused) {
			gFocused = focused;
			raised = focused;
		}
	}
	// How many windows get a border on each space, when a window alone on
	// its space goes without.
	NSCountedSet<NSNumber *> *crowd = nil;
	if (gStyle.hideWhenSingle) {
		crowd = [NSCountedSet set];
		for (NSDictionary *info in (__bridge NSArray *)described) {
			CGRect bounds;
			if (mimiBordered(info, self, displays, fullScreen, displayCount, &bounds))
				[crowd addObject:@(mimiSpaceUnder(bounds, displays, front, displayCount))];
		}
	}

	NSMutableSet<NSNumber *> *seen = [NSMutableSet set];
	int made = 0;
	int moved = 0;
	for (NSDictionary *info in (__bridge NSArray *)described) {
		CGRect bounds;
		if (!mimiBordered(info, self, displays, fullScreen, displayCount, &bounds))
			continue;

		NSNumber *key = info[(id)kCGWindowNumber];
		uint32_t number = key.unsignedIntValue;
		BOOL active = number == gFocused;
		uint64_t space = mimiSpaceUnder(bounds, displays, front, displayCount);
		if (crowd && [crowd countForObject:@(space)] < 2)
			continue;
		MimiBorder *border = gBorders[key];
		// A border stays on the space it was shown on, so a window that
		// moved to another space left its border behind. Close that one
		// and make a new one on the window's space.
		if (border && border.space != space) {
			mimiCloseBorder(border);
			border = nil;
			moved++;
		}
		BOOL fresh = border == nil;
		if (fresh) {
			border = mimiNewBorder(space, number);
			gBorders[key] = border;
			made++;
		}
		NSUInteger at = [numbers indexOfObject:key];
		double radius = at < radii.count ? radii[at].doubleValue : -1;
		BOOL reshaped = radius != border.radius;
		BOOL releveled = border.active != active;
		border.radius = radius;
		if (fresh || reshaped || !CGRectEqualToRect(border.targetBounds, bounds) || releveled) {
			mimiDrawBorder(border, bounds, active);
		}
		// The window server keeps no tie between a window and the border
		// beside it, so a window that comes to the front leaves its border
		// where it was, and the border follows it. The rest of the stack
		// has not moved, so the other borders are left alone, unless they
		// were all taken down. A border whose window took or lost the focus
		// changes level, and is ordered again with it.
		if (fresh || reshow || releveled || number == raised) {
			mimiOrderBeside(border, number);
		}
		[seen addObject:key];
	}
	if (described)
		CFRelease(described);

	int dropped = 0;
	for (NSNumber *key in gBorders.allKeys) {
		if ([seen containsObject:key])
			continue;
		mimiCloseBorder(gBorders[key]);
		[gBorders removeObjectForKey:key];
		dropped++;
	}

	if (made || dropped || moved)
		mimiFollowBordered();
}

#pragma mark - C Interface

void MimiBordersSetStyle(const MimiBorderStyle *style) {
	MimiBorderStyle copy = *style;
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!gBorders)
			gBorders = [NSMutableDictionary new];
		// A border drawn on the other side of its window has to change
		// places with it, so a change of placement starts over.
		if (gEnabled && gStyle.inside != copy.inside)
			mimiCloseAll();
		gStyle = copy;
		gEnabled = YES;
		for (MimiBorder *border in gBorders.allValues) {
			border.painted = NO;
			mimiDrawBorder(border, border.targetBounds, border.active);
		}
		mimiSyncOnMain(YES);
	});
}

void MimiBordersSync(int refocus) {
	if (refocus)
		atomic_store(&gWantRefocus, 1);
	if (atomic_exchange(&gQueued, 1))
		return;
	dispatch_async(dispatch_get_main_queue(), ^{
		atomic_store(&gQueued, 0);
		BOOL want = atomic_exchange(&gWantRefocus, 0) != 0;
		mimiSyncOnMain(want);
	});
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
