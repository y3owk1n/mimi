#import "border.h"

#import "mimi.h"
#import "mimi_log.h"

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
extern CGError SLSSetWindowShape(int cid, uint32_t wid, float x, float y, CFTypeRef region);
extern CGError SLSSetWindowResolution(int cid, uint32_t wid, double resolution);
extern CGError SLSSetWindowOpacity(int cid, uint32_t wid, bool opaque);
extern CGError SLSSetWindowLevel(int cid, uint32_t wid, int level);
extern CGError SLSMoveWindowsToManagedSpace(int cid, CFArrayRef windows, uint64_t sid);
extern CGContextRef SLWindowContextCreate(int cid, uint32_t wid, CFDictionaryRef options);
extern CGError SLSFlushWindowContentRegion(int cid, uint32_t wid, void *dirty);
extern CGError SLSWindowFreezeWithOptions(int cid, uint32_t wid, CFTypeRef options);
extern CGError SLSWindowThaw(int cid, uint32_t wid);
extern CFTypeRef SLSTransactionCreate(int cid);
extern CGError SLSTransactionMoveWindowWithGroup(CFTypeRef transaction, uint32_t wid, CGPoint origin);
extern CGError SLSTransactionOrderWindow(CFTypeRef transaction, uint32_t wid, int mode, uint32_t relative);
extern CGError SLSTransactionCommit(CFTypeRef transaction, int synchronous);

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

// Borders are drawn at Retina resolution on every display, since a window
// keeps one resolution as it crosses from one display to another.
static const double kMimiBorderResolution = 2;

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

@interface MimiBorder : NSObject
// number is the border window's own number.
@property(nonatomic) uint32_t number;
// context draws into the border window.
@property(nonatomic) CGContextRef context;
// size is the border window's, the size its shape was last set to.
@property(nonatomic) CGSize size;
// targetBounds is the target's frame the border was last drawn for, in screen
// coordinates, y down.
@property(nonatomic) CGRect targetBounds;
// radius is the window's own corner radius, or -1 when the window server
// does not say.
@property(nonatomic) double radius;
@property(nonatomic) BOOL active;
// space is the space the border was shown on.
@property(nonatomic) uint64_t space;
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
static atomic_uint gFocusRequest;

#pragma mark - Helpers

// Order border above (1) or below (-1) window number, or off the screen (0).
static void mimiOrder(MimiBorder *border, int mode, uint32_t number) {
	CFTypeRef transaction = SLSTransactionCreate(SLSMainConnectionID());
	SLSTransactionOrderWindow(transaction, border.number, mode, number);
	SLSTransactionCommit(transaction, 0);
	CFRelease(transaction);
}

// Put border right next to its window: under it outside, over it inside.
static void mimiOrderBeside(MimiBorder *border, uint32_t number) { mimiOrder(border, gStyle.inside ? 1 : -1, number); }

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

// Move border to origin, in screen coordinates, y down.
static void mimiMoveBorder(MimiBorder *border, CGPoint origin) {
	CFTypeRef transaction = SLSTransactionCreate(SLSMainConnectionID());
	SLSTransactionMoveWindowWithGroup(transaction, border.number, origin);
	SLSTransactionCommit(transaction, 0);
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
// one of ours, with bounds the window server gives, and not full screen.
static BOOL mimiBordered(
    NSDictionary *info, pid_t self, const CGRect *displays, const BOOL *fullScreen, uint32_t displayCount,
    CGRect *bounds) {
	if ([info[(id)kCGWindowOwnerPID] intValue] == self)
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

// A border window of size on space, off screen until it is moved and
// ordered. It is made on the space so that it shows with the windows there
// and not on whichever space is in front of the main display.
static MimiBorder *mimiNewBorder(CGSize size, uint64_t space) {
	int cid = SLSMainConnectionID();
	CFTypeRef region = mimiRegion(size);
	uint32_t number = 0;
	SLSNewWindow(cid, kCGBackingStoreBuffered, -9999, -9999, region, &number);
	CFRelease(region);
	SLSSetWindowTags(cid, number, &kMimiBorderTags, 64);
	SLSSetWindowResolution(cid, number, kMimiBorderResolution);
	SLSSetWindowOpacity(cid, number, false);
	SLSSetWindowLevel(cid, number, kCGNormalWindowLevel);
	CFArrayRef windows = CFBridgingRetain(@[ @(number) ]);
	SLSMoveWindowsToManagedSpace(cid, windows, space);
	CFRelease(windows);

	MimiBorder *border = [MimiBorder new];
	border.number = number;
	border.size = size;
	border.space = space;
	border.context = SLWindowContextCreate(cid, number, NULL);
	CGContextSetInterpolationQuality(border.context, kCGInterpolationNone);
	return border;
}

static void mimiCloseBorder(MimiBorder *border) {
	CGContextRelease(border.context);
	border.context = NULL;
	SLSReleaseWindow(SLSMainConnectionID(), border.number);
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

// Draw border for bounds, in the colour for whether it is active: the
// window frame is bounds grown by the width outside or bounds inside, and
// the ring is the window frame with its middle cut out, both with rounded
// corners. The window is reshaped when the frame's size changed, and moved
// to the frame's origin.
static void mimiDrawBorder(MimiBorder *border, CGRect bounds, BOOL active) {
	int cid = SLSMainConnectionID();
	CGRect outer = mimiBorderFrame(bounds);
	CGRect frame = CGRectMake(0, 0, outer.size.width, outer.size.height);

	// A reshape leaves the last drawing where it was until the next flush,
	// so the window is frozen across the reshape and the redraw.
	SLSWindowFreezeWithOptions(cid, border.number, NULL);
	if (!CGSizeEqualToSize(outer.size, border.size)) {
		CFTypeRef region = mimiRegion(outer.size);
		SLSSetWindowShape(cid, border.number, 0, 0, region);
		CFRelease(region);
		border.size = outer.size;
	}

	CGPathRef path = MimiBorderRingPath(bounds.size, gStyle.width, mimiRadiusFor(border), gStyle.inside);
	CGColorRef color = mimiBorderColor(active ? gStyle.active : gStyle.inactive);
	CGContextRef context = border.context;
	CGContextClearRect(context, frame);
	CGContextAddPath(context, path);
	CGContextSetFillColorWithColor(context, color);
	CGContextEOFillPath(context);
	CGContextFlush(context);
	CGColorRelease(color);
	CGPathRelease(path);
	SLSFlushWindowContentRegion(cid, border.number, NULL);
	SLSWindowThaw(cid, border.number);

	mimiMoveBorder(border, outer.origin);
	border.targetBounds = bounds;
	border.active = active;
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
	mimiMoveBorder(border, mimiBorderFrame(bounds).origin);
}

// Put a window's border back beside it after the window server reordered
// the window, as when the window's application raised it.
static void mimiReorder(uint32_t number) {
	MimiBorder *border = gEnabled ? gBorders[@(number)] : nil;
	if (border)
		mimiOrderBeside(border, number);
}

static void mimiSyncOnMain(uint32_t focused, BOOL refocus);

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
			mimiSyncOnMain(0, NO);
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

// Bring the borders up to date, on the main thread. focused is the window
// Accessibility named, or 0 to keep the last answer.
static void mimiSyncOnMain(uint32_t focused, BOOL refocus) {
	if (!gEnabled)
		return;
	if (gHidden && mimiMissionControlUp())
		return;
	BOOL reshow = gHidden;
	gHidden = NO;

	uint32_t raised = 0;
	if (refocus && focused != gFocused) {
		gFocused = focused;
		raised = focused;
	}

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
			border = mimiNewBorder(mimiBorderFrame(bounds).size, space);
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
		// beside it, so a window that comes to the front leaves its border
		// where it was, and the border follows it. The rest of the stack
		// has not moved, so the other borders are left alone, unless they
		// were all taken down.
		if (fresh || reshow || number == raised) {
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
		// A border drawn on the other side of its window has to change
		// places with it, so a change of placement starts over.
		if (gEnabled && gStyle.inside != copy.inside)
			mimiCloseAll();
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
	return border ? border.number : 0;
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
