#import "animate.h"

#import "mimi_log.h"
#import "workspace.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <unistd.h>

// The animation never touches the real windows beyond the one frame write the
// caller makes: the window server refuses alpha, transform and ordering on
// another process's windows from an ordinary connection. It is drawn with a
// window of our own per display instead, a transparent layer-backed one the
// size of the display and whatever starts off it. In it, a backdrop layer
// per display, a still of everything on screen but the moving windows,
// covers the real windows while they jump to their new frames. Over it, one
// proxy layer per window carries a picture of that window, and Core
// Animation moves it, so no application does any work while the animation
// runs. Windows in front of every moving window, a floating one say, get a
// still of their own over the proxies, so the stacking order is kept.
//
// A capture is an image the compositor can show as it is. Handing it to a
// layer copies nothing, where painting it into a window of our own copied
// every pixel, four times as many on a Retina display; so what the setup
// costs is the captures, a fixed round trip each. Core Animation runs the
// motion on the render server and ends it with a callback, so nothing ticks
// here. Every layer and window lives on the main thread, where the daemon
// runs its application loop, and the calls below go there.

// Window layers at and above the Dock's are never covered, so they are left
// out of the backdrop.
static const int kMimiDockLayer = 20;

// How many window masks are remembered.
enum {
	kMimiMaskCacheSize = 32,
};

#pragma mark - Types

// One on-screen window, as the window list reports it. A backdrop remembers
// the entries it shows, so a later animation can tell whether it still
// shows the screen.
typedef struct {
	uint32_t number;
	CGRect bounds;
} MimiEntry;

@interface MimiProxy : NSObject
@property(nonatomic) uint32_t number;
@property(nonatomic) CGRect from;
@property(nonatomic) CGRect to;
// alone is whether no other animating window overlaps this one where it
// starts, so its picture can be cropped from a composite. listed is whether
// the window is in the on-screen list: one entirely off the display is not,
// and is captured on its own.
@property(nonatomic) BOOL alone;
@property(nonatomic) BOOL listed;
// carried is a proxy taken over from the running animation, its layer
// already on screen, since its window was on its way to the same frame.
@property(nonatomic) BOOL carried;
// depth is the window's place in the on-screen list, front first, so the
// proxies stack as the windows do.
@property(nonatomic) int depth;
@property(nonatomic) double scale;
// picture and mask hold the capture between the phases of Begin; layer is
// the proxy once made.
@property(nonatomic) CGImageRef picture;
@property(nonatomic) CGImageRef mask;
@property(nonatomic, strong) CALayer *layer;
@end

@implementation MimiProxy

- (void)dealloc {
	if (_picture) {
		CGImageRelease(_picture);
	}
	if (_mask) {
		CGImageRelease(_mask);
	}
}

@end

@interface MimiBackdrop : NSObject
@property(nonatomic) CGRect bounds;
@property(nonatomic) CGDirectDisplayID display;
// over is whether the backdrop sits over the proxies, showing the windows in
// front of every moving one, rather than under them.
@property(nonatomic) BOOL over;
// shows is what the backdrop is a still of, front to back.
@property(nonatomic) MimiEntry *shows;
@property(nonatomic) int showsCount;
@property(nonatomic) CGImageRef still;
@property(nonatomic, strong) CALayer *layer;
// reused is a backdrop kept from the animation before, already on screen.
@property(nonatomic) BOOL reused;
@end

@implementation MimiBackdrop

- (void)dealloc {
	free(_shows);
	if (_still) {
		CGImageRelease(_still);
	}
}

@end

// The window per display the layers live in. Its root layer's bounds are the
// window's frame in screen coordinates, y down, so every sublayer is placed
// in screen coordinates whatever the window's frame is.
@interface MimiHost : NSWindow
@property(nonatomic) CGDirectDisplayID display;
// root holds the layers, under the content view's own layer, whose bounds
// AppKit resets as it lays the view out.
@property(nonatomic, strong) CALayer *root;
@end

@implementation MimiHost

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

// The animation, main thread only.
static NSMutableDictionary<NSNumber *, MimiHost *> *gHosts;
static NSMutableArray<MimiProxy *> *gProxies;
static NSMutableArray<MimiBackdrop *> *gBackdrops;
static BOOL gRunning;
static double gDuration;
static int gEasing;
// generation tells an animation's end callback whether it is still the one
// on screen.
static unsigned gGeneration;

// The masks remembered by window number, each for the size the window had
// and the scale of the display it was on. A window's shape only changes
// with its size, and taking the mask is a capture, the cost of a whole
// display's composite.
typedef struct {
	uint32_t number;
	CGSize size;
	double scale;
	CGImageRef mask;
} MimiMask;

static MimiMask gMasks[kMimiMaskCacheSize];
static int gMaskCount;
static int gMaskNext;

#pragma mark - Helpers

// A window is on the display that has its centre.
static BOOL mimiOnDisplay(CGRect frame, CGRect display) {
	return CGRectContainsPoint(display, CGPointMake(CGRectGetMidX(frame), CGRectGetMidY(frame)));
}

// A window animates on the display it starts on, or, starting off every
// display, as a strip's parked column does, on the one it arrives on.
static BOOL mimiAnimatesOn(MimiProxy *proxy, CGRect display) {
	if (mimiOnDisplay(proxy.from, display)) {
		return YES;
	}
	CGDirectDisplayID starts[16];
	uint32_t startCount = 0;
	CGGetDisplaysWithPoint(CGPointMake(CGRectGetMidX(proxy.from), CGRectGetMidY(proxy.from)), 16, starts, &startCount);
	return startCount == 0 && mimiOnDisplay(proxy.to, display);
}

static BOOL mimiSameSize(CGSize a, CGSize b) {
	return fabs(a.width - b.width) < 0.5 && fabs(a.height - b.height) < 0.5;
}

static BOOL mimiSameRect(CGRect a, CGRect b) {
	return fabs(a.origin.x - b.origin.x) < 0.5 && fabs(a.origin.y - b.origin.y) < 0.5 && mimiSameSize(a.size, b.size);
}

static BOOL mimiSameEntries(const MimiEntry *a, int aCount, const MimiEntry *b, int bCount) {
	if (aCount != bCount) {
		return NO;
	}
	for (int i = 0; i < aCount; i++) {
		if (a[i].number != b[i].number || !mimiSameRect(a[i].bounds, b[i].bounds)) {
			return NO;
		}
	}
	return YES;
}

// The window numbers of entries, as CGWindowListCreateImageFromArray reads
// them: raw values, not CFNumbers.
static CFArrayRef mimiNumbers(const MimiEntry *entries, int count) {
	CFMutableArrayRef numbers = CFArrayCreateMutable(NULL, count, NULL);
	for (int i = 0; i < count; i++) {
		CFArrayAppendValue(numbers, (const void *)(uintptr_t)entries[i].number);
	}
	return numbers;
}

static CAMediaTimingFunction *mimiTiming(int easing) {
	switch (easing) {
	case MimiEasingEaseIn:
		return [CAMediaTimingFunction functionWithControlPoints:0.32:0:0.67:0];
	case MimiEasingEaseOut:
		return [CAMediaTimingFunction functionWithControlPoints:0.33:1:0.68:1];
	case MimiEasingEaseInOut:
		return [CAMediaTimingFunction functionWithControlPoints:0.65:0:0.35:1];
	default:
		return [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionLinear];
	}
}

// Run block on the main thread and wait, or right here when this is it.
static void mimiOnMain(dispatch_block_t block) {
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_sync(dispatch_get_main_queue(), block);
	}
}

// A layer's frame in screen coordinates, as it is on screen this instant.
static CGRect mimiPresented(CALayer *layer) {
	CALayer *shown = layer.presentationLayer ?: layer;
	return shown.frame;
}

// Where the window is now if it is in flight, and the proxy that carries it.
static MimiProxy *mimiInFlight(uint32_t number, CGRect *out) {
	if (!gRunning) {
		return nil;
	}
	for (MimiProxy *proxy in gProxies) {
		if (proxy.number == number) {
			*out = mimiPresented(proxy.layer);
			return proxy;
		}
	}
	return nil;
}

// A window's frame from the window server, in screen coordinates.
static BOOL mimiWindowBounds(uint32_t number, CGRect *out) {
	const void *values[] = {(void *)(uintptr_t)number};
	CFArrayRef ids = CFArrayCreate(NULL, values, 1, NULL);
	CFArrayRef info = CGWindowListCreateDescriptionFromArray(ids);
	CFRelease(ids);
	BOOL found = NO;
	if (info && CFArrayGetCount(info) == 1) {
		NSDictionary *entry = (__bridge NSDictionary *)CFArrayGetValueAtIndex(info, 0);
		found = CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)entry[(id)kCGWindowBounds], out);
	}
	if (info) {
		CFRelease(info);
	}
	return found;
}

#pragma mark - Hosts and layers

// NSWindow frames are y up from the primary display's bottom-left.
static NSRect mimiCocoaRect(CGRect rect) {
	double primaryHeight = CGDisplayBounds(CGMainDisplayID()).size.height;
	return NSMakeRect(
	    rect.origin.x, primaryHeight - rect.origin.y - rect.size.height, rect.size.width, rect.size.height);
}

// The host for a display, made on first use, at the floating level so it
// sits over every ordinary window and under the Dock and menu bar, passing
// events through.
// A host covers its display and half a display's width to either side,
// where a strip parks the column beside the one on screen, so a proxy
// starting there is drawn. It is never resized: moving a window that shows
// an animation would show it shifted for a frame.
static CGRect mimiHostFrame(CGRect bounds) { return CGRectInset(bounds, -bounds.size.width / 2, 0); }

static MimiHost *mimiHost(CGDirectDisplayID display, CGRect bounds) {
	if (!gHosts) {
		gHosts = [NSMutableDictionary new];
	}
	MimiHost *host = gHosts[@(display)];
	if (host) {
		return host;
	}
	host = [[MimiHost alloc] initWithContentRect:mimiCocoaRect(mimiHostFrame(bounds))
	                                   styleMask:NSWindowStyleMaskBorderless
	                                     backing:NSBackingStoreBuffered
	                                       defer:NO];
	host.display = display;
	host.level = NSFloatingWindowLevel;
	host.opaque = NO;
	host.backgroundColor = [NSColor clearColor];
	host.hasShadow = NO;
	host.ignoresMouseEvents = YES;
	host.releasedWhenClosed = NO;
	host.animationBehavior = NSWindowAnimationBehaviorNone;
	host.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary |
	                          NSWindowCollectionBehaviorIgnoresCycle | NSWindowCollectionBehaviorFullScreenAuxiliary;
	NSView *view = host.contentView;
	view.wantsLayer = YES;
	// The root layer fills the view, and its coordinate space is screen
	// coordinates, y down.
	CGRect frame = mimiHostFrame(bounds);
	CALayer *root = [CALayer layer];
	root.geometryFlipped = YES;
	root.masksToBounds = NO;
	root.anchorPoint = CGPointZero;
	root.position = CGPointZero;
	root.bounds = CGRectMake(frame.origin.x, frame.origin.y, frame.size.width, frame.size.height);
	root.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;
	[view.layer addSublayer:root];
	host.root = root;
	gHosts[@(display)] = host;
	return host;
}

// Push the committed transaction to the render server and wait for the
// display to show it. A commit made on the main thread from a dispatched
// block is otherwise held until the run loop turns, and the caller writes
// the real frames as soon as this returns: they must already be covered.
static void mimiShow(void) {
	[CATransaction flush];
	double refresh = 60;
	CGDisplayModeRef mode = CGDisplayCopyDisplayMode(CGMainDisplayID());
	if (mode) {
		if (CGDisplayModeGetRefreshRate(mode) > 0) {
			refresh = CGDisplayModeGetRefreshRate(mode);
		}
		CGDisplayModeRelease(mode);
	}
	usleep((useconds_t)(1e6 / refresh));
}

// A layer showing image over `frame`, in screen coordinates, clipped to
// mask's alpha when there is one. The image is shown as it is, without a
// copy.
static CALayer *mimiImageLayer(CGImageRef image, CGImageRef mask, CGRect frame, double scale) {
	CALayer *layer = [CALayer layer];
	layer.contents = (__bridge id)image;
	layer.contentsGravity = kCAGravityResize;
	layer.contentsScale = scale;
	layer.frame = frame;
	if (mask) {
		CALayer *clip = [CALayer layer];
		clip.contents = (__bridge id)mask;
		clip.contentsGravity = kCAGravityResize;
		clip.contentsScale = scale;
		clip.frame = layer.bounds;
		clip.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;
		layer.mask = clip;
	}
	return layer;
}

#pragma mark - Masks

// The remembered mask for a window of this size, retained, or NULL.
static CGImageRef mimiCachedMask(uint32_t number, CGSize size, double scale) {
	for (int i = 0; i < gMaskCount; i++) {
		if (gMasks[i].number == number && mimiSameSize(gMasks[i].size, size) && gMasks[i].scale == scale) {
			return CGImageRetain(gMasks[i].mask);
		}
	}
	return NULL;
}

// Remember a mask, copied into memory of its own so the capture it was
// cropped from can go. The window's older mask, if any, is replaced;
// otherwise the oldest entry is.
static void mimiRememberMask(uint32_t number, CGSize size, double scale, CGImageRef mask) {
	size_t width = CGImageGetWidth(mask);
	size_t height = CGImageGetHeight(mask);
	CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
	CGContextRef context = CGBitmapContextCreate(
	    NULL, width, height, 8, 0, space, kCGImageAlphaPremultipliedFirst | kCGBitmapByteOrder32Little);
	CGColorSpaceRelease(space);
	if (!context) {
		return;
	}
	CGContextSetBlendMode(context, kCGBlendModeCopy);
	CGContextDrawImage(context, CGRectMake(0, 0, width, height), mask);
	CGImageRef copy = CGBitmapContextCreateImage(context);
	CGContextRelease(context);
	if (!copy) {
		return;
	}

	int slot = -1;
	for (int i = 0; i < gMaskCount; i++) {
		if (gMasks[i].number == number) {
			slot = i;
			break;
		}
	}
	if (slot < 0) {
		if (gMaskCount < kMimiMaskCacheSize) {
			slot = gMaskCount++;
		} else {
			slot = gMaskNext;
			gMaskNext = (gMaskNext + 1) % kMimiMaskCacheSize;
		}
	}
	if (gMasks[slot].mask) {
		CGImageRelease(gMasks[slot].mask);
	}
	gMasks[slot] = (MimiMask){.number = number, .size = size, .scale = scale, .mask = copy};
}

#pragma mark - Teardown

// Take every layer of the current animation off screen, but for the
// backdrops marked reused and the proxies the next animation carries, and
// hide the hosts left empty.
static void mimiRetire(NSArray<MimiProxy *> *carried) {
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	for (MimiProxy *proxy in gProxies) {
		if (![carried containsObject:proxy]) {
			[proxy.layer removeFromSuperlayer];
		}
	}
	for (MimiBackdrop *backdrop in gBackdrops) {
		if (!backdrop.reused) {
			[backdrop.layer removeFromSuperlayer];
		}
	}
	[CATransaction commit];
	for (MimiHost *host in gHosts.allValues) {
		if (host.root.sublayers.count == 0) {
			[host orderOut:nil];
		}
	}
	gProxies = nil;
	gBackdrops = nil;
	gRunning = NO;
}

static void mimiReleaseAll(void) {
	for (MimiBackdrop *backdrop in gBackdrops) {
		backdrop.reused = NO;
	}
	mimiRetire(nil);
}

#pragma mark - Begin

// A backdrop of the running animation that still shows the screen: on this
// display, on this side of the proxies, and a still of these very windows
// where they are now. Its picture is at most one animation old.
static MimiBackdrop *mimiReusable(CGDirectDisplayID display, BOOL over, const MimiEntry *shows, int showsCount) {
	if (!gRunning) {
		return nil;
	}
	for (MimiBackdrop *backdrop in gBackdrops) {
		if (backdrop.display == display && backdrop.over == over &&
		    mimiSameEntries(backdrop.shows, backdrop.showsCount, shows, showsCount)) {
			return backdrop;
		}
	}
	return nil;
}

static MimiBackdrop *mimiNewBackdrop(
    CGDirectDisplayID display, CGRect bounds, BOOL over, const MimiEntry *shows, int showsCount, CGImageRef still,
    MimiBackdrop *reused) {
	MimiBackdrop *backdrop = [MimiBackdrop new];
	backdrop.display = display;
	backdrop.bounds = bounds;
	backdrop.over = over;
	backdrop.still = still;
	backdrop.shows = calloc((size_t)showsCount + 1, sizeof(MimiEntry));
	memcpy(backdrop.shows, shows, (size_t)showsCount * sizeof(MimiEntry));
	backdrop.showsCount = showsCount;
	if (reused) {
		backdrop.layer = reused.layer;
		backdrop.reused = YES;
		reused.reused = YES;
	}
	return backdrop;
}

int MimiScreenCaptureGranted(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }

void MimiAnimationWarm(void) {
	if (![NSApp isRunning] && ![NSThread isMainThread]) {
		return;
	}
	mimiOnMain(^{
		CGDirectDisplayID displays[16];
		uint32_t displayCount = 0;
		CGGetActiveDisplayList(16, displays, &displayCount);
		for (uint32_t d = 0; d < displayCount; d++) {
			mimiHost(displays[d], CGDisplayBounds(displays[d]));
		}
	});
}

static int mimiBeginOnMain(const MimiAnimationTarget *targets, int count, double duration, int easing);

int MimiAnimationBegin(const MimiAnimationTarget *targets, int count, double duration, int easing) {
	if (!targets || count <= 0) {
		return 0;
	}
	// The check is a round trip that was measured at 8 ms, and a grant only
	// takes effect when the process restarts, so a yes is kept.
	static BOOL granted = NO;
	if (!granted) {
		granted = CGPreflightScreenCaptureAccess();
	}
	if (!granted) {
		return -1;
	}
	// Without an application loop, as in the CLI, there is nothing to run
	// the animation on, and the frames move at once.
	if (![NSApp isRunning]) {
		return 0;
	}

	__block int kept = 0;
	mimiOnMain(^{
		@autoreleasepool {
			kept = mimiBeginOnMain(targets, count, duration, easing);
		}
	});
	return kept;
}

static int mimiBeginOnMain(const MimiAnimationTarget *targets, int count, double duration, int easing) {
	// Where each window starts: its current animated frame if it is
	// animating, else where the window server has it. A window in flight
	// to the very frame asked for again, as when the pass that follows a
	// focus change repeats the one that made it, keeps the proxy it has,
	// and when no window does more than that the running animation is
	// left to finish.
	NSMutableArray<MimiProxy *> *next = [NSMutableArray arrayWithCapacity:(NSUInteger)count];
	NSMutableArray<MimiProxy *> *carried = [NSMutableArray array];
	for (int i = 0; i < count; i++) {
		CGRect from;
		MimiProxy *flight = mimiInFlight(targets[i].number, &from);
		if (!flight && !mimiWindowBounds(targets[i].number, &from)) {
			continue;
		}
		if (from.size.width <= 0 || from.size.height <= 0 || targets[i].w <= 0 || targets[i].h <= 0) {
			continue;
		}
		// A window already at its frame is skipped. Most passes move few
		// windows, or none, and cost nothing here.
		CGRect to = CGRectMake(targets[i].x, targets[i].y, targets[i].w, targets[i].h);
		if (mimiSameRect(from, to)) {
			continue;
		}
		MimiProxy *proxy = [MimiProxy new];
		proxy.number = targets[i].number;
		proxy.from = from;
		proxy.to = to;
		if (flight && mimiSameRect(flight.to, to)) {
			proxy.carried = YES;
			proxy.layer = flight.layer;
			proxy.scale = flight.scale;
			[carried addObject:flight];
		}
		[next addObject:proxy];
	}
	if (next.count == carried.count) {
		return 0;
	}

	// Everything on screen but the animating windows and our own. The list
	// runs front to back, and is split at the first animating window.
	// Whatever comes before it is in front of every moving window, a
	// floating window say, and is drawn over the proxies. The rest goes
	// under them. under is the scene from the first animating window down,
	// moving windows included, that the apart windows' pictures are cropped
	// from. CGWindowListCreateImageFromArray composites a list in the order
	// given, first on top, so every list keeps the on-screen order.
	CFArrayRef onScreen = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
	CFIndex onScreenCount = onScreen ? CFArrayGetCount(onScreen) : 0;
	MimiEntry *others = calloc((size_t)onScreenCount + 1, sizeof(MimiEntry));
	MimiEntry *front = calloc((size_t)onScreenCount + 1, sizeof(MimiEntry));
	MimiEntry *under = calloc((size_t)onScreenCount + 1, sizeof(MimiEntry));
	int othersCount = 0, frontCount = 0, underCount = 0;
	pid_t self = getpid();
	BOOL passed = NO;
	int depth = 0;
	for (NSDictionary *info in (__bridge NSArray *)onScreen) {
		if ([info[(id)kCGWindowOwnerPID] intValue] == self || [info[(id)kCGWindowLayer] intValue] >= kMimiDockLayer) {
			continue;
		}
		MimiEntry entry = {.number = [info[(id)kCGWindowNumber] unsignedIntValue]};
		CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], &entry.bounds);
		BOOL inFlight = NO;
		for (MimiProxy *proxy in next) {
			if (proxy.number == entry.number) {
				proxy.depth = depth;
				proxy.listed = YES;
				inFlight = YES;
				break;
			}
		}
		depth++;
		if (inFlight) {
			passed = YES;
		} else if (passed) {
			others[othersCount++] = entry;
		} else {
			front[frontCount++] = entry;
		}
		if (passed) {
			under[underCount++] = entry;
		}
	}
	if (onScreen) {
		CFRelease(onScreen);
	}

	// Backdrops: at most two per display, the still of what is under the
	// proxies and, when anything is, the still of what is over them.
	NSMutableArray<MimiBackdrop *> *backdrops = [NSMutableArray array];
	CGDirectDisplayID displays[16];
	uint32_t displayCount = 0;
	CGGetActiveDisplayList(16, displays, &displayCount);

	for (uint32_t d = 0; d < displayCount; d++) {
		CGRect bounds = CGDisplayBounds(displays[d]);
		BOOL any = NO;
		for (MimiProxy *proxy in next) {
			if (!proxy.carried && !proxy.picture && mimiAnimatesOn(proxy, bounds)) {
				any = YES;
				break;
			}
		}
		if (!any) {
			continue;
		}
		double scale = bounds.size.width > 0 ? CGDisplayPixelsWide(displays[d]) / bounds.size.width : 1;

		// The windows that overlap another animating window, as under a
		// monocle layout, are captured one by one: a composite of the set
		// shows only the top one. The rest are cropped from one composite
		// of the scene. Their masks come from one composite of those
		// windows alone, taken only for the windows whose mask is not
		// remembered from an earlier animation.
		MimiEntry *apart = calloc(next.count, sizeof(MimiEntry));
		int apartCount = 0;
		BOOL anyAlone = NO;
		for (MimiProxy *proxy in next) {
			if (proxy.carried || proxy.picture || !mimiAnimatesOn(proxy, bounds)) {
				continue;
			}
			// A window off the display is not in the scene, and one not in
			// the list has no place in it either.
			BOOL overlaps = !proxy.listed || !mimiOnDisplay(proxy.from, bounds);
			for (MimiProxy *other in next) {
				if (overlaps) {
					break;
				}
				if (other != proxy && !other.picture && mimiAnimatesOn(other, bounds)) {
					overlaps = !CGRectIsEmpty(CGRectIntersection(proxy.from, other.from));
				}
			}
			proxy.alone = !overlaps;
			if (!overlaps) {
				anyAlone = YES;
				proxy.mask = mimiCachedMask(proxy.number, proxy.from.size, scale);
				if (!proxy.mask) {
					apart[apartCount++] = (MimiEntry){.number = proxy.number, .bounds = proxy.from};
				}
			}
		}

		// Deprecated for ScreenCaptureKit, whose screenshot is asynchronous
		// and far slower to start; this composes the display in some ten
		// milliseconds, which an animation that starts on the same frame as
		// the event needs. SLSHWCaptureWindowList, the hardware capture
		// yabai uses, was measured on macOS 26 at 6 to 10 ms per window,
		// the cost of one of these composites of a whole display, and
		// returns one clipped image for a list, so it captures nothing
		// faster here. The window server serialises captures, so issuing
		// them concurrently was measured to gain nothing, and a smaller
		// area or a lower resolution was measured to cost the same.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		MimiBackdrop *keptUnder = mimiReusable(displays[d], NO, others, othersCount);
		if (keptUnder) {
			[backdrops addObject:mimiNewBackdrop(displays[d], bounds, NO, others, othersCount, NULL, keptUnder)];
		} else if (othersCount > 0) {
			CFArrayRef list = mimiNumbers(others, othersCount);
			CGImageRef still = CGWindowListCreateImageFromArray(bounds, list, kCGWindowImageBestResolution);
			CFRelease(list);
			if (still) {
				[backdrops addObject:mimiNewBackdrop(displays[d], bounds, NO, others, othersCount, still, nil)];
			}
		}
		MimiBackdrop *keptOver = frontCount > 0 ? mimiReusable(displays[d], YES, front, frontCount) : nil;
		if (keptOver) {
			[backdrops addObject:mimiNewBackdrop(displays[d], bounds, YES, front, frontCount, NULL, keptOver)];
		} else if (frontCount > 0) {
			CFArrayRef list = mimiNumbers(front, frontCount);
			CGImageRef cover = CGWindowListCreateImageFromArray(bounds, list, kCGWindowImageBestResolution);
			CFRelease(list);
			if (cover) {
				[backdrops addObject:mimiNewBackdrop(displays[d], bounds, YES, front, frontCount, cover, nil)];
			}
		}
		CGImageRef flight = NULL;
		if (apartCount > 0) {
			CFArrayRef list = mimiNumbers(apart, apartCount);
			flight = CGWindowListCreateImageFromArray(bounds, list, kCGWindowImageBestResolution);
			CFRelease(list);
		}
		// A window captured on its own comes out opaque, with its
		// translucent parts a flat tint, since the window server has
		// nothing behind it to blend with. Composited with what is under
		// it, it looks as it does on screen, so the apart windows take
		// their picture from that scene, cropped, and their own capture
		// only clips it to the window's shape.
		CGImageRef scene = NULL;
		if (anyAlone) {
			CFArrayRef list = mimiNumbers(under, underCount);
			scene = CGWindowListCreateImageFromArray(bounds, list, kCGWindowImageBestResolution);
			CFRelease(list);
		}
		free(apart);

		for (MimiProxy *proxy in next) {
			if (proxy.carried || proxy.picture || !mimiAnimatesOn(proxy, bounds)) {
				continue;
			}
			if (proxy.alone) {
				CGRect crop = CGRectMake(
				    (proxy.from.origin.x - bounds.origin.x) * scale, (proxy.from.origin.y - bounds.origin.y) * scale,
				    proxy.from.size.width * scale, proxy.from.size.height * scale);
				if (scene) {
					proxy.picture = CGImageCreateWithImageInRect(scene, crop);
				}
				if (!proxy.mask && flight) {
					proxy.mask = CGImageCreateWithImageInRect(flight, crop);
					if (proxy.mask) {
						mimiRememberMask(proxy.number, proxy.from.size, scale, proxy.mask);
					}
				}
				if (!proxy.picture && proxy.mask) {
					// No scene: the window's own capture is the picture.
					proxy.picture = proxy.mask;
					proxy.mask = NULL;
				}
			} else if (mimiOnDisplay(proxy.from, bounds)) {
				proxy.picture = CGWindowListCreateImage(
				    proxy.from, kCGWindowListOptionIncludingWindow, proxy.number, kCGWindowImageBestResolution);
			} else {
				// Off the display, a window has pixels only when asked for
				// by its own bounds: a rectangle there comes back empty. Its
				// picture is opaque, a flat tint where the window is
				// translucent, since nothing is under it to blend with.
				proxy.picture = CGWindowListCreateImage(
				    CGRectNull, kCGWindowListOptionIncludingWindow, proxy.number,
				    kCGWindowImageBoundsIgnoreFraming | kCGWindowImageBestResolution);
			}
			proxy.scale = scale;
			if (!proxy.picture) {
				MIMI_LOG("animation: capturing window %u failed", proxy.number);
				if (proxy.mask) {
					CGImageRelease(proxy.mask);
					proxy.mask = NULL;
				}
			}
		}
#pragma clang diagnostic pop

		if (flight) {
			CGImageRelease(flight);
		}
		if (scene) {
			CGImageRelease(scene);
		}

		mimiHost(displays[d], bounds);
	}
	free(others);
	free(front);
	free(under);

	// Then the layers, in one transaction: the under backdrops come in, the
	// proxies over them back to front, the over backdrops on top, and the
	// previous animation, if any, goes out, but for what this one keeps.
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	for (MimiBackdrop *backdrop in backdrops) {
		if (backdrop.reused || backdrop.over) {
			continue;
		}
		backdrop.layer = mimiImageLayer(backdrop.still, NULL, backdrop.bounds, 1);
		CGImageRelease(backdrop.still);
		backdrop.still = NULL;
		[mimiHost(backdrop.display, backdrop.bounds).root addSublayer:backdrop.layer];
	}
	NSMutableArray<MimiProxy *> *made = [NSMutableArray array];
	for (MimiProxy *proxy in next) {
		if (proxy.carried || proxy.picture) {
			[made addObject:proxy];
		}
	}
	[made sortUsingComparator:^NSComparisonResult(MimiProxy *a, MimiProxy *b) {
		return a.depth > b.depth ? NSOrderedAscending : (a.depth < b.depth ? NSOrderedDescending : NSOrderedSame);
	}];
	for (MimiProxy *proxy in made) {
		if (!proxy.carried) {
			proxy.layer = mimiImageLayer(proxy.picture, proxy.mask, proxy.from, proxy.scale);
			CGImageRelease(proxy.picture);
			proxy.picture = NULL;
			if (proxy.mask) {
				CGImageRelease(proxy.mask);
				proxy.mask = NULL;
			}
		}
		// Every proxy goes to the top of its host, carried ones included,
		// so they stack as their windows do.
		CALayer *root = proxy.layer.superlayer;
		if (!root) {
			for (MimiHost *host in gHosts.allValues) {
				if (mimiAnimatesOn(proxy, CGDisplayBounds(host.display))) {
					root = host.root;
					break;
				}
			}
		}
		[proxy.layer removeFromSuperlayer];
		[root addSublayer:proxy.layer];
	}
	for (MimiBackdrop *backdrop in backdrops) {
		if (!backdrop.over) {
			continue;
		}
		if (!backdrop.reused) {
			backdrop.layer = mimiImageLayer(backdrop.still, NULL, backdrop.bounds, 1);
			CGImageRelease(backdrop.still);
			backdrop.still = NULL;
		}
		[backdrop.layer removeFromSuperlayer];
		[mimiHost(backdrop.display, backdrop.bounds).root addSublayer:backdrop.layer];
	}
	[CATransaction commit];
	mimiRetire(carried);
	for (MimiHost *host in gHosts.allValues) {
		if (host.root.sublayers.count > 0 && !host.visible) {
			[host orderFrontRegardless];
		}
	}
	mimiShow();

	gProxies = made;
	gBackdrops = backdrops;
	gDuration = duration;
	gEasing = easing;
	gRunning = NO;
	return (int)made.count;
}

void MimiAnimationStart(const uint32_t *dropped, int count) {
	if (![NSApp isRunning]) {
		return;
	}
	mimiOnMain(^{
		if (gRunning || (gProxies.count == 0 && gBackdrops.count == 0)) {
			return;
		}

		NSMutableArray<MimiProxy *> *kept = [NSMutableArray array];
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		for (MimiProxy *proxy in gProxies) {
			BOOL drop = NO;
			for (int j = 0; j < count; j++) {
				if (dropped[j] == proxy.number) {
					drop = YES;
					break;
				}
			}
			if (drop) {
				[proxy.layer removeFromSuperlayer];
			} else {
				[kept addObject:proxy];
			}
		}
		[CATransaction commit];
		gProxies = kept;

		if (kept.count == 0) {
			mimiReleaseAll();
			return;
		}

		// The motion runs on the render server from here: one transaction
		// gives every proxy its destination, from wherever it is shown this
		// instant, and the callback at its end takes the animation down,
		// unless another has replaced it since.
		unsigned generation = ++gGeneration;
		[CATransaction begin];
		[CATransaction setAnimationDuration:gDuration];
		[CATransaction setAnimationTimingFunction:mimiTiming(gEasing)];
		[CATransaction setCompletionBlock:^{
			if (gGeneration == generation && gRunning) {
				mimiReleaseAll();
			}
		}];
		for (MimiProxy *proxy in kept) {
			CALayer *layer = proxy.layer;
			CGRect shown = mimiPresented(layer);
			CGRect to = proxy.to;
			CGPoint start = CGPointMake(CGRectGetMidX(shown), CGRectGetMidY(shown));
			CGPoint end = CGPointMake(CGRectGetMidX(to), CGRectGetMidY(to));

			CABasicAnimation *move = [CABasicAnimation animationWithKeyPath:@"position"];
			move.fromValue = [NSValue valueWithPoint:NSPointFromCGPoint(start)];
			move.toValue = [NSValue valueWithPoint:NSPointFromCGPoint(end)];
			CABasicAnimation *size = [CABasicAnimation animationWithKeyPath:@"bounds"];
			size.fromValue = [NSValue valueWithRect:NSMakeRect(0, 0, shown.size.width, shown.size.height)];
			size.toValue = [NSValue valueWithRect:NSMakeRect(0, 0, to.size.width, to.size.height)];

			layer.position = end;
			layer.bounds = CGRectMake(0, 0, to.size.width, to.size.height);
			[layer addAnimation:move forKey:@"position"];
			[layer addAnimation:size forKey:@"bounds"];
		}
		[CATransaction commit];
		[CATransaction flush];
		gRunning = YES;
	});
}
