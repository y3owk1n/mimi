#import "animate.h"

#import "mimi_log.h"
#import "workspace.h"

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <pthread.h>
#import <unistd.h>

// The animation never touches the real windows beyond the one frame write the
// caller makes: the window server refuses alpha, transform and ordering on
// another process's windows from an ordinary connection. It is drawn with
// windows of our own instead. A backdrop per display, a still of everything
// on screen but the moving windows, covers the real windows while they jump
// to their new frames. Over it, one proxy per window carries a picture of that
// window, and the compositor moves it with a transform, so no application
// does any work while the animation runs. Windows in front of every moving
// window, a floating one say, get a still of their own over the proxies, so
// the stacking order is kept. One transaction removes everything when it
// ends.
//
// What the setup costs is what the user waits before anything moves, so the
// windows are pooled rather than made and released each time, the mask that
// clips a window's picture to its shape is kept between animations, every
// paint runs at once on its own thread, and an animation that starts while
// one runs keeps the backdrops whose windows have not changed.

#pragma mark - SkyLight External Declarations

extern int SLSMainConnectionID(void);
extern CGError SLSNewWindow(int cid, int type, float x, float y, CFTypeRef region, uint32_t *wid);
extern CGError SLSReleaseWindow(int cid, uint32_t wid);
extern CGError SLSSetWindowOpacity(int cid, uint32_t wid, bool opaque);
extern CGError SLSSetWindowLevel(int cid, uint32_t wid, int level);
extern CGError SLSSetWindowTags(int cid, uint32_t wid, uint64_t *tags, int size);
extern CGError SLSSetWindowResolution(int cid, uint32_t wid, double res);
extern CGError SLSGetWindowBounds(int cid, uint32_t wid, CGRect *out);
extern CGContextRef SLWindowContextCreate(int cid, uint32_t wid, CFDictionaryRef options);
extern CGError CGSNewRegionWithRect(CGRect *rect, CFTypeRef *region);
extern CFTypeRef SLSTransactionCreate(int cid);
extern CGError SLSTransactionCommit(CFTypeRef transaction, int synchronous);
extern CGError SLSTransactionOrderWindow(CFTypeRef transaction, uint32_t wid, int mode, uint32_t relative);
extern CGError SLSTransactionSetWindowTransform(
    CFTypeRef transaction, uint32_t wid, int unused1, int unused2, CGAffineTransform transform);

// The window server's backing store type for SLSNewWindow, and the window
// tags and level the proxies carry: ignore-for-events plus no-shadow, at the
// floating level so they sit over every ordinary window and under the Dock
// and menu bar.
static const int kMimiProxyBackingBuffered = 2;
static const uint64_t kMimiProxyTags = (1ULL << 1) | (1ULL << 9);
static const int kMimiProxyLevel = 3;
// Window layers at and above the Dock's are never covered, so they are left
// out of the backdrop.
static const int kMimiDockLayer = 20;
static const int kMimiOrderAbove = 1;
static const int kMimiOrderOut = 0;

// How many windows the pool keeps between animations, and how many window
// masks are remembered. A pooled backdrop holds a display's worth of pixels.
enum {
	kMimiPoolSize = 8,
	kMimiMaskCacheSize = 32,
	kMimiMaxBackdrops = 32,
};

// One on-screen window, as the window list reports it. A backdrop remembers
// the entries it shows, so a later animation can tell whether it still
// shows the screen.
typedef struct {
	uint32_t number;
	CGRect bounds;
} MimiEntry;

typedef struct {
	uint32_t number;
	uint32_t proxy;
	CGRect from;
	CGRect to;
	// alone is whether no other animating window overlaps this one where it
	// starts, so its picture can be cropped from a composite; picture, mask
	// and scale hold the capture between the phases of MimiAnimationBegin.
	// mask, when there is one, is the window's own capture, whose alpha
	// clips the picture to the window's shape.
	BOOL alone;
	CGImageRef picture;
	CGImageRef mask;
	double scale;
	// depth is the window's place in the on-screen list, front first, so
	// the proxies stack as the windows do.
	int depth;
	// carried is a proxy taken over from the running animation, already
	// painted and on screen, since its window was on its way to the same
	// frame. taken marks the running animation's entry it came from, which
	// its teardown then leaves alone.
	BOOL carried;
	BOOL taken;
	// listed is whether the window is in the on-screen list: one entirely
	// off the display is not, and is captured on its own.
	BOOL listed;
	// drawn is the size the proxy was painted at, the window's size when
	// it was captured, which the placement scales from.
	CGSize drawn;
} MimiProxy;

typedef struct {
	uint32_t window;
	CGRect bounds;
	double scale;
	// over is whether the backdrop sits over the proxies, showing the
	// windows in front of every moving one, rather than under them.
	BOOL over;
	// shows is what the backdrop is a still of, front to back.
	MimiEntry *shows;
	int showsCount;
	// still is the capture waiting to be painted, between the phases of
	// MimiAnimationBegin. reused is a backdrop kept from the animation
	// before, already painted and on screen.
	CGImageRef still;
	BOOL reused;
} MimiBackdrop;

static struct {
	pthread_mutex_t lock;
	BOOL running;
	MimiProxy *proxies;
	int proxyCount;
	MimiBackdrop *backdrops;
	int backdropCount;
	double start;
	double duration;
	int easing;
} gAnim = {.lock = PTHREAD_MUTEX_INITIALIZER};

// The pool of windows not on screen, each made for a size and scale, ready
// to be painted and ordered in again. Making a window is a round trip, and
// a request that reaches the window server while a window it just made
// waits for its first pixels stalls for half a second, so the same windows
// serve every animation. The lock covers it.
typedef struct {
	uint32_t window;
	CGSize size;
	double scale;
} MimiPooled;

static MimiPooled gPool[kMimiPoolSize];
static int gPoolCount;

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

static double mimiEase(int easing, double t) {
	switch (easing) {
	case MimiEasingEaseIn:
		return t * t * t;
	case MimiEasingEaseOut: {
		double u = 1 - t;
		return 1 - u * u * u;
	}
	case MimiEasingEaseInOut:
		if (t < 0.5) {
			return 4 * t * t * t;
		} else {
			double u = -2 * t + 2;
			return 1 - u * u * u / 2;
		}
	default:
		return t;
	}
}

// A window is on the display that has its centre.
static BOOL mimiOnDisplay(CGRect frame, CGRect display) {
	return CGRectContainsPoint(display, CGPointMake(CGRectGetMidX(frame), CGRectGetMidY(frame)));
}

// A window animates on the display it starts on, or, starting off every
// display, as a strip's parked column does, on the one it arrives on.
static BOOL mimiAnimatesOn(const MimiProxy *proxy, CGRect display, CGDirectDisplayID display_id) {
	if (mimiOnDisplay(proxy->from, display)) {
		return YES;
	}
	CGDirectDisplayID starts[16];
	uint32_t startCount = 0;
	CGGetDisplaysWithPoint(
	    CGPointMake(CGRectGetMidX(proxy->from), CGRectGetMidY(proxy->from)), 16, starts, &startCount);
	return startCount == 0 && mimiOnDisplay(proxy->to, display);
}

static CGRect mimiLerp(CGRect from, CGRect to, double p) {
	return CGRectMake(
	    from.origin.x + (to.origin.x - from.origin.x) * p, from.origin.y + (to.origin.y - from.origin.y) * p,
	    from.size.width + (to.size.width - from.size.width) * p,
	    from.size.height + (to.size.height - from.size.height) * p);
}

// The transform that shows a proxy drawn at the origin with size `drawn` at
// `at`: the window server maps screen to window, so the translation is
// negated and the scale inverted.
static CGAffineTransform mimiPlacement(CGSize drawn, CGRect at) {
	CGAffineTransform move = CGAffineTransformMakeTranslation(-at.origin.x, -at.origin.y);
	CGAffineTransform scale = CGAffineTransformMakeScale(drawn.width / at.size.width, drawn.height / at.size.height);
	return CGAffineTransformConcat(move, scale);
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

// Where the proxies are at this instant of the running animation, for the
// windows a new animation starts from. The caller holds the lock.
static double mimiProgressLocked(void) {
	if (gAnim.duration <= 0) {
		return 1;
	}
	double t = (CACurrentMediaTime() - gAnim.start) / gAnim.duration;
	return mimiEase(gAnim.easing, t < 0 ? 0 : (t > 1 ? 1 : t));
}

// Whether the window is in flight, and if so where it is now and which
// proxy carries it.
static BOOL mimiInFlightLocked(uint32_t number, CGRect *out, int *index) {
	if (!gAnim.running) {
		return NO;
	}
	double p = mimiProgressLocked();
	for (int i = 0; i < gAnim.proxyCount; i++) {
		if (gAnim.proxies[i].number == number) {
			*out = mimiLerp(gAnim.proxies[i].from, gAnim.proxies[i].to, p);
			*index = i;
			return YES;
		}
	}
	return NO;
}

#pragma mark - Windows

// Create a window of the given size at the origin, to be painted with
// mimiPaint and placed with a transform in the caller's transaction. Returns
// 0 when the window server refuses.
static uint32_t mimiNewWindow(int cid, CGSize size, double scale) {
	CGRect local = CGRectMake(0, 0, size.width, size.height);
	CFTypeRef region = NULL;
	if (CGSNewRegionWithRect(&local, &region) != kCGErrorSuccess || !region) {
		return 0;
	}

	uint32_t wid = 0;
	CGError err = SLSNewWindow(cid, kMimiProxyBackingBuffered, 0, 0, region, &wid);
	CFRelease(region);
	if (err != kCGErrorSuccess || wid == 0) {
		MIMI_LOG("SLSNewWindow failed with error %d", (int)err);
		return 0;
	}

	uint64_t tags = kMimiProxyTags;
	SLSSetWindowResolution(cid, wid, scale);
	SLSSetWindowTags(cid, wid, &tags, 64);
	SLSSetWindowOpacity(cid, wid, false);
	SLSSetWindowLevel(cid, wid, kMimiProxyLevel);

	return wid;
}

// A window of the given size and scale from the pool, or 0 when it has
// none. The caller holds the lock.
static uint32_t mimiPooledLocked(CGSize size, double scale) {
	for (int i = 0; i < gPoolCount; i++) {
		if (mimiSameSize(gPool[i].size, size) && gPool[i].scale == scale) {
			uint32_t wid = gPool[i].window;
			gPool[i] = gPool[--gPoolCount];
			return wid;
		}
	}
	return 0;
}

// Keep an ordered-out window for the next animation, or release it when the
// pool is full. The caller holds the lock.
static void mimiRecycleLocked(int cid, uint32_t wid, CGSize size, double scale) {
	if (gPoolCount < kMimiPoolSize) {
		gPool[gPoolCount++] = (MimiPooled){.window = wid, .size = size, .scale = scale};
		return;
	}
	SLSReleaseWindow(cid, wid);
}

// Draw picture into the window, clipped to mask's alpha when there is one,
// and release both. A captured image is materialised the first time it is
// read, which is most of a proxy's cost, and this is that read: the paints
// of one animation run at once, each on its own thread.
static void mimiPaint(int cid, uint32_t wid, CGSize size, CGImageRef picture, CGImageRef mask) {
	CGContextRef context = SLWindowContextCreate(cid, wid, NULL);
	if (context) {
		CGRect whole = CGRectMake(0, 0, size.width, size.height);
		// Copying the pixels in, alpha included, rather than compositing
		// them over whatever the window showed last: the picture covers
		// the window.
		CGContextSetBlendMode(context, kCGBlendModeCopy);
		CGContextDrawImage(context, whole, picture);
		if (mask) {
			CGContextSetBlendMode(context, kCGBlendModeDestinationIn);
			CGContextDrawImage(context, whole, mask);
		}
		CGContextFlush(context);
		CGContextRelease(context);
	}
	CGImageRelease(picture);
	if (mask) {
		CGImageRelease(mask);
	}
}

// Paint picture into a pooled window, when the pool has one of the size,
// on the group's queue, and hand the window back in *window with *picture
// and *mask taken. With no pooled window the picture stays for the caller
// to make one for. The caller holds the lock.
static void mimiPaintPooledLocked(
    uint32_t *window, CGSize size, double scale, CGImageRef *picture, CGImageRef *mask, dispatch_group_t group,
    dispatch_queue_t queue, int cid) {
	uint32_t wid = mimiPooledLocked(size, scale);
	if (!wid) {
		return;
	}
	*window = wid;
	CGImageRef image = *picture;
	CGImageRef clip = mask ? *mask : NULL;
	*picture = NULL;
	if (mask) {
		*mask = NULL;
	}
	dispatch_group_async(group, queue, ^{
		mimiPaint(cid, wid, size, image, clip);
	});
}

#pragma mark - Masks

// The remembered mask for a window of this size, retained, or NULL. The
// caller holds the lock.
static CGImageRef mimiCachedMaskLocked(uint32_t number, CGSize size, double scale) {
	for (int i = 0; i < gMaskCount; i++) {
		if (gMasks[i].number == number && mimiSameSize(gMasks[i].size, size) && gMasks[i].scale == scale) {
			return CGImageRetain(gMasks[i].mask);
		}
	}
	return NULL;
}

// Remember a mask, copied into memory of its own so the capture it was
// cropped from can go. The window's older mask, if any, is replaced;
// otherwise the oldest entry is. The caller holds the lock.
static void mimiRememberMaskLocked(uint32_t number, CGSize size, double scale, CGImageRef mask) {
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

static void mimiFreeBackdrop(MimiBackdrop *backdrop) {
	free(backdrop->shows);
	backdrop->shows = NULL;
	backdrop->showsCount = 0;
}

// Order every window of the current animation out in the transaction,
// commit it, and pool the windows, except the backdrops marked reused,
// which the next animation keeps on screen. The caller holds the lock.
static void mimiRetireLocked(int cid, CFTypeRef transaction) {
	for (int i = 0; i < gAnim.proxyCount; i++) {
		if (!gAnim.proxies[i].taken) {
			SLSTransactionOrderWindow(transaction, gAnim.proxies[i].proxy, kMimiOrderOut, 0);
		}
	}
	for (int i = 0; i < gAnim.backdropCount; i++) {
		if (!gAnim.backdrops[i].reused) {
			SLSTransactionOrderWindow(transaction, gAnim.backdrops[i].window, kMimiOrderOut, 0);
		}
	}
	SLSTransactionCommit(transaction, 1);

	for (int i = 0; i < gAnim.proxyCount; i++) {
		if (!gAnim.proxies[i].taken) {
			mimiRecycleLocked(cid, gAnim.proxies[i].proxy, gAnim.proxies[i].drawn, gAnim.proxies[i].scale);
		}
	}
	for (int i = 0; i < gAnim.backdropCount; i++) {
		if (!gAnim.backdrops[i].reused) {
			mimiRecycleLocked(cid, gAnim.backdrops[i].window, gAnim.backdrops[i].bounds.size, gAnim.backdrops[i].scale);
		}
		mimiFreeBackdrop(&gAnim.backdrops[i]);
	}
	free(gAnim.proxies);
	free(gAnim.backdrops);
	gAnim.proxies = NULL;
	gAnim.backdrops = NULL;
	gAnim.proxyCount = 0;
	gAnim.backdropCount = 0;
	gAnim.running = NO;
}

static void mimiReleaseAllLocked(int cid, CFTypeRef transaction) {
	for (int i = 0; i < gAnim.backdropCount; i++) {
		gAnim.backdrops[i].reused = NO;
	}
	for (int i = 0; i < gAnim.proxyCount; i++) {
		gAnim.proxies[i].taken = NO;
	}
	mimiRetireLocked(cid, transaction);
}

#pragma mark - Display link

// The link runs on the daemon's bridge run loop, the thread the workspace
// observer already lives on, and is made and invalidated there.
static CADisplayLink *gLink;

@interface MimiAnimationTicker : NSObject
- (void)tick:(CADisplayLink *)link;
@end

@implementation MimiAnimationTicker

- (void)tick:(CADisplayLink *)link {
	pthread_mutex_lock(&gAnim.lock);
	if (!gAnim.running) {
		pthread_mutex_unlock(&gAnim.lock);
		return;
	}

	int cid = SLSMainConnectionID();
	double t = (CACurrentMediaTime() - gAnim.start) / gAnim.duration;
	BOOL done = t >= 1;
	double p = mimiEase(gAnim.easing, done ? 1 : t);

	CFTypeRef transaction = SLSTransactionCreate(cid);
	if (done) {
		mimiReleaseAllLocked(cid, transaction);
	} else {
		for (int i = 0; i < gAnim.proxyCount; i++) {
			MimiProxy *proxy = &gAnim.proxies[i];
			SLSTransactionSetWindowTransform(
			    transaction, proxy->proxy, 0, 0, mimiPlacement(proxy->drawn, mimiLerp(proxy->from, proxy->to, p)));
		}
		SLSTransactionCommit(transaction, 0);
	}
	CFRelease(transaction);

	// Only this animation's link is ended: a new animation may have made
	// its own since.
	if (done && gLink == link) {
		[link invalidate];
		gLink = nil;
	}
	pthread_mutex_unlock(&gAnim.lock);
}

@end

static MimiAnimationTicker *gTicker;

// The screen the animation is on, for its refresh rate: the one under the
// first window's destination, or the main screen.
static NSScreen *mimiScreenLocked(void) {
	if (gAnim.proxyCount > 0) {
		CGRect to = gAnim.proxies[0].to;
		double primaryHeight = [NSScreen screens].firstObject.frame.size.height;
		// NSScreen frames are y-up from the primary display's bottom-left.
		NSPoint centre = NSMakePoint(CGRectGetMidX(to), primaryHeight - CGRectGetMidY(to));
		for (NSScreen *screen in [NSScreen screens]) {
			if (NSPointInRect(centre, screen.frame)) {
				return screen;
			}
		}
	}
	return [NSScreen mainScreen];
}

// Start a link on the bridge run loop. The caller holds the lock; the link
// is made under it again on that thread, so a Start and a tick never race.
static void mimiStartLinkLocked(void) {
	CFRunLoopRef runLoop = GetRunLoop();
	if (!runLoop) {
		return;
	}
	if (!gTicker) {
		gTicker = [MimiAnimationTicker new];
	}
	NSScreen *screen = mimiScreenLocked();
	CFRunLoopPerformBlock(runLoop, kCFRunLoopCommonModes, ^{
		pthread_mutex_lock(&gAnim.lock);
		if (gLink) {
			[gLink invalidate];
		}
		gLink = [screen displayLinkWithTarget:gTicker selector:@selector(tick:)];
		[gLink addToRunLoop:[NSRunLoop currentRunLoop] forMode:NSRunLoopCommonModes];
		pthread_mutex_unlock(&gAnim.lock);
	});
	CFRunLoopWakeUp(runLoop);
}

#pragma mark - Begin

// A backdrop of the running animation that still shows the screen: on this
// display, on this side of the proxies, and a still of these very windows
// where they are now. Its picture is at most one animation old. The caller
// holds the lock.
static MimiBackdrop *mimiReusableLocked(CGRect bounds, BOOL over, const MimiEntry *shows, int showsCount) {
	if (!gAnim.running) {
		return NULL;
	}
	for (int i = 0; i < gAnim.backdropCount; i++) {
		MimiBackdrop *backdrop = &gAnim.backdrops[i];
		if (backdrop->over == over && mimiSameRect(backdrop->bounds, bounds) &&
		    mimiSameEntries(backdrop->shows, backdrop->showsCount, shows, showsCount)) {
			return backdrop;
		}
	}
	return NULL;
}

// Note a backdrop to make from a still of the given entries, or, when the
// running animation has one that still shows them, to keep from it.
static void mimiAddBackdrop(
    MimiBackdrop *backdrops, int *count, CGRect bounds, double scale, BOOL over, const MimiEntry *shows, int showsCount,
    CGImageRef still, MimiBackdrop *reused) {
	if (*count >= kMimiMaxBackdrops) {
		if (still) {
			CGImageRelease(still);
		}
		return;
	}
	MimiBackdrop *backdrop = &backdrops[(*count)++];
	*backdrop = (MimiBackdrop){.bounds = bounds, .scale = scale, .over = over, .still = still};
	backdrop->shows = calloc((size_t)showsCount + 1, sizeof(MimiEntry));
	memcpy(backdrop->shows, shows, (size_t)showsCount * sizeof(MimiEntry));
	backdrop->showsCount = showsCount;
	if (reused) {
		backdrop->window = reused->window;
		backdrop->reused = YES;
		reused->reused = YES;
	}
}

int MimiScreenCaptureGranted(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }

void MimiAnimationWarm(void) {
	@autoreleasepool {
		int cid = SLSMainConnectionID();
		CGDirectDisplayID displays[16];
		uint32_t displayCount = 0;
		CGGetActiveDisplayList(16, displays, &displayCount);

		pthread_mutex_lock(&gAnim.lock);
		for (uint32_t d = 0; d < displayCount; d++) {
			CGRect bounds = CGDisplayBounds(displays[d]);
			double scale = bounds.size.width > 0 ? CGDisplayPixelsWide(displays[d]) / bounds.size.width : 1;
			// Two per display: the backdrop under the proxies and the one
			// over them.
			for (int k = 0; k < 2 && gPoolCount < kMimiPoolSize; k++) {
				uint32_t wid = mimiNewWindow(cid, bounds.size, scale);
				if (!wid) {
					break;
				}
				CGContextRef context = SLWindowContextCreate(cid, wid, NULL);
				if (context) {
					CGContextClearRect(context, CGRectMake(0, 0, bounds.size.width, bounds.size.height));
					CGContextFlush(context);
					CGContextRelease(context);
				}
				mimiRecycleLocked(cid, wid, bounds.size, scale);
			}
		}
		pthread_mutex_unlock(&gAnim.lock);
	}
}

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

	@autoreleasepool {
		int cid = SLSMainConnectionID();
		pthread_mutex_lock(&gAnim.lock);

		// Where each window starts: its current animated frame if it is
		// animating, else where the window server has it. A window in
		// flight to the very frame asked for again, as when the pass that
		// follows a focus change repeats the one that made it, keeps the
		// proxy it has, and when no window does more than that the
		// running animation is left to finish.
		MimiProxy *next = calloc((size_t)count, sizeof(MimiProxy));
		int nextCount = 0;
		int carriedCount = 0;
		for (int i = 0; i < count; i++) {
			CGRect from;
			int flight = -1;
			if (!mimiInFlightLocked(targets[i].number, &from, &flight)) {
				if (SLSGetWindowBounds(cid, targets[i].number, &from) != kCGErrorSuccess) {
					continue;
				}
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
			next[nextCount].number = targets[i].number;
			next[nextCount].from = from;
			next[nextCount].to = to;
			if (flight >= 0 && mimiSameRect(gAnim.proxies[flight].to, to)) {
				next[nextCount].carried = YES;
				next[nextCount].proxy = gAnim.proxies[flight].proxy;
				next[nextCount].scale = gAnim.proxies[flight].scale;
				next[nextCount].drawn = gAnim.proxies[flight].drawn;
				carriedCount++;
			}
			nextCount++;
		}
		if (nextCount == carriedCount) {
			free(next);
			pthread_mutex_unlock(&gAnim.lock);
			return 0;
		}
		for (int i = 0; i < nextCount; i++) {
			if (next[i].carried) {
				for (int j = 0; j < gAnim.proxyCount; j++) {
					if (gAnim.proxies[j].proxy == next[i].proxy) {
						gAnim.proxies[j].taken = YES;
					}
				}
			}
		}

		// Everything on screen but the animating windows and our own, a
		// previous animation's proxies. The list runs front to back, and is
		// split at the first animating window. Whatever comes before it is
		// in front of every moving window, a floating window say, and is
		// drawn over the proxies. The rest goes under them. under is the
		// scene from the first animating window down, moving windows
		// included, that the apart windows' pictures are cropped from.
		// CGWindowListCreateImageFromArray composites a list in the order
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
			if ([info[(id)kCGWindowOwnerPID] intValue] == self ||
			    [info[(id)kCGWindowLayer] intValue] >= kMimiDockLayer) {
				continue;
			}
			MimiEntry entry = {.number = [info[(id)kCGWindowNumber] unsignedIntValue]};
			CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds], &entry.bounds);
			BOOL inFlight = NO;
			for (int i = 0; i < nextCount && !inFlight; i++) {
				inFlight = next[i].number == entry.number;
				if (inFlight) {
					next[i].depth = depth;
					next[i].listed = YES;
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

		// Backdrops: at most two per display, the still of what is under
		// the proxies and, when anything is, the still of what is over them.
		MimiBackdrop *backdrops = calloc(kMimiMaxBackdrops, sizeof(MimiBackdrop));
		int backdropCount = 0;
		CGDirectDisplayID displays[16];
		uint32_t displayCount = 0;
		CGGetActiveDisplayList(16, displays, &displayCount);

		// A capture costs the same round trip whatever its area, so the
		// captures are what the setup waits on. Each one's paint starts
		// as soon as it is taken, on a thread of its own, when the pool
		// has a window for it, and runs under the captures that follow.
		// A window the pool lacks is made only once every capture is
		// taken, then painted at once: a capture that follows the creation
		// of a window not yet drawn waits half a second on it, as does a
		// request that reaches the window server while such a window
		// waits for its first pixels.
		dispatch_group_t paints = dispatch_group_create();
		dispatch_queue_t queue = dispatch_get_global_queue(QOS_CLASS_USER_INTERACTIVE, 0);

		for (uint32_t d = 0; d < displayCount; d++) {
			CGRect bounds = CGDisplayBounds(displays[d]);
			BOOL any = NO;
			for (int i = 0; i < nextCount; i++) {
				if (!next[i].carried && next[i].picture == NULL && mimiAnimatesOn(&next[i], bounds, displays[d])) {
					any = YES;
					break;
				}
			}
			if (!any) {
				continue;
			}
			double scale = bounds.size.width > 0 ? CGDisplayPixelsWide(displays[d]) / bounds.size.width : 1;

			// The windows that overlap another animating window, as under a
			// monocle layout, are captured one by one: a composite of the
			// set shows only the top one. The rest are cropped from one
			// composite of the scene. Their masks come from one composite of
			// those windows alone, taken only for the windows whose mask is
			// not remembered from an earlier animation.
			MimiEntry *apart = calloc((size_t)nextCount, sizeof(MimiEntry));
			int apartCount = 0;
			BOOL anyAlone = NO;
			for (int i = 0; i < nextCount; i++) {
				if (next[i].carried || next[i].picture != NULL || !mimiAnimatesOn(&next[i], bounds, displays[d])) {
					continue;
				}
				// A window off the display is not in the scene, and one
				// not in the list has no place in it either.
				BOOL overlaps = !next[i].listed || !mimiOnDisplay(next[i].from, bounds);
				for (int j = 0; j < nextCount && !overlaps; j++) {
					if (j != i && next[j].picture == NULL && mimiAnimatesOn(&next[j], bounds, displays[d])) {
						overlaps = !CGRectIsEmpty(CGRectIntersection(next[i].from, next[j].from));
					}
				}
				next[i].alone = !overlaps;
				if (!overlaps) {
					anyAlone = YES;
					next[i].mask = mimiCachedMaskLocked(next[i].number, next[i].from.size, scale);
					if (!next[i].mask) {
						apart[apartCount++] = (MimiEntry){.number = next[i].number, .bounds = next[i].from};
					}
				}
			}

			// Deprecated for ScreenCaptureKit, whose screenshot is asynchronous
			// and far slower to start; this composes the display in some ten
			// milliseconds, which an animation that starts on the same frame
			// as the event needs. SLSHWCaptureWindowList, the hardware
			// capture yabai uses, was measured on macOS 26 at 6 to 10 ms per
			// window, the cost of one of these composites of a whole display,
			// and returns one clipped image for a list, so it captures
			// nothing faster here. The window server serialises captures, so
			// issuing them concurrently was measured to gain nothing, and a
			// smaller area or a lower resolution was measured to cost the
			// same.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
			MimiBackdrop *keptUnder = mimiReusableLocked(bounds, NO, others, othersCount);
			if (keptUnder) {
				mimiAddBackdrop(backdrops, &backdropCount, bounds, scale, NO, others, othersCount, NULL, keptUnder);
			} else if (othersCount > 0) {
				CFArrayRef list = mimiNumbers(others, othersCount);
				CGImageRef still = CGWindowListCreateImageFromArray(bounds, list, kCGWindowImageBestResolution);
				CFRelease(list);
				if (still) {
					mimiAddBackdrop(backdrops, &backdropCount, bounds, scale, NO, others, othersCount, still, NULL);
					mimiPaintPooledLocked(
					    &backdrops[backdropCount - 1].window, bounds.size, scale, &backdrops[backdropCount - 1].still,
					    NULL, paints, queue, cid);
				}
			}
			MimiBackdrop *keptOver = frontCount > 0 ? mimiReusableLocked(bounds, YES, front, frontCount) : NULL;
			if (keptOver) {
				mimiAddBackdrop(backdrops, &backdropCount, bounds, scale, YES, front, frontCount, NULL, keptOver);
			} else if (frontCount > 0) {
				CFArrayRef list = mimiNumbers(front, frontCount);
				CGImageRef cover = CGWindowListCreateImageFromArray(bounds, list, kCGWindowImageBestResolution);
				CFRelease(list);
				if (cover) {
					mimiAddBackdrop(backdrops, &backdropCount, bounds, scale, YES, front, frontCount, cover, NULL);
					mimiPaintPooledLocked(
					    &backdrops[backdropCount - 1].window, bounds.size, scale, &backdrops[backdropCount - 1].still,
					    NULL, paints, queue, cid);
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

			for (int i = 0; i < nextCount; i++) {
				MimiProxy *proxy = &next[i];
				if (proxy->carried || proxy->picture != NULL || !mimiAnimatesOn(proxy, bounds, displays[d])) {
					continue;
				}
				if (proxy->alone) {
					CGRect crop = CGRectMake(
					    (proxy->from.origin.x - bounds.origin.x) * scale,
					    (proxy->from.origin.y - bounds.origin.y) * scale, proxy->from.size.width * scale,
					    proxy->from.size.height * scale);
					if (scene) {
						proxy->picture = CGImageCreateWithImageInRect(scene, crop);
					}
					if (!proxy->mask && flight) {
						proxy->mask = CGImageCreateWithImageInRect(flight, crop);
						if (proxy->mask) {
							mimiRememberMaskLocked(proxy->number, proxy->from.size, scale, proxy->mask);
						}
					}
					if (!proxy->picture && proxy->mask) {
						// No scene: the window's own capture is the picture.
						proxy->picture = proxy->mask;
						proxy->mask = NULL;
					}
				} else if (mimiOnDisplay(proxy->from, bounds)) {
					proxy->picture = CGWindowListCreateImage(
					    proxy->from, kCGWindowListOptionIncludingWindow, proxy->number, kCGWindowImageBestResolution);
				} else {
					// Off the display, a window has pixels only when asked
					// for by its own bounds: a rectangle there comes back
					// empty. Its picture is opaque, a flat tint where the
					// window is translucent, since nothing is under it to
					// blend with.
					proxy->picture = CGWindowListCreateImage(
					    CGRectNull, kCGWindowListOptionIncludingWindow, proxy->number,
					    kCGWindowImageBoundsIgnoreFraming | kCGWindowImageBestResolution);
				}
				proxy->scale = scale;
				proxy->drawn = proxy->from.size;
				if (!proxy->picture) {
					MIMI_LOG("animation: capturing window %u failed", proxy->number);
					if (proxy->mask) {
						CGImageRelease(proxy->mask);
						proxy->mask = NULL;
					}
					continue;
				}
				mimiPaintPooledLocked(
				    &proxy->proxy, proxy->from.size, scale, &proxy->picture, &proxy->mask, paints, queue, cid);
			}
#pragma clang diagnostic pop

			if (flight) {
				CGImageRelease(flight);
			}
			if (scene) {
				CGImageRelease(scene);
			}
		}
		free(others);
		free(front);
		free(under);

		// The windows the pool had none of, made now and painted the moment
		// they exist. A picture still held here is one waiting for such a
		// window.
		int kept = 0;
		for (int i = 0; i < backdropCount; i++) {
			MimiBackdrop *backdrop = &backdrops[i];
			if (backdrop->still) {
				backdrop->window = mimiNewWindow(cid, backdrop->bounds.size, backdrop->scale);
				if (backdrop->window) {
					mimiPaint(cid, backdrop->window, backdrop->bounds.size, backdrop->still, NULL);
				} else {
					CGImageRelease(backdrop->still);
				}
				backdrop->still = NULL;
			}
			if (backdrop->window) {
				backdrops[kept++] = *backdrop;
			} else {
				mimiFreeBackdrop(backdrop);
			}
		}
		backdropCount = kept;
		for (int i = 0; i < nextCount; i++) {
			if (!next[i].picture) {
				continue;
			}
			next[i].proxy = mimiNewWindow(cid, next[i].from.size, next[i].scale);
			if (next[i].proxy) {
				mimiPaint(cid, next[i].proxy, next[i].from.size, next[i].picture, next[i].mask);
			} else {
				CGImageRelease(next[i].picture);
				if (next[i].mask) {
					CGImageRelease(next[i].mask);
				}
			}
			next[i].picture = NULL;
			next[i].mask = NULL;
		}
		dispatch_group_wait(paints, DISPATCH_TIME_FOREVER);

		// Keep the proxies that were made, in order.
		kept = 0;
		for (int i = 0; i < nextCount; i++) {
			if (next[i].proxy != 0) {
				next[kept++] = next[i];
			}
		}

		// One commit: the under backdrops come in, the new proxies over
		// them, the over backdrops on top, and the previous animation, if
		// any, goes out, but for the backdrops this one keeps.
		CFTypeRef transaction = SLSTransactionCreate(cid);
		for (int i = 0; i < backdropCount; i++) {
			if (backdrops[i].reused) {
				continue;
			}
			SLSTransactionSetWindowTransform(
			    transaction, backdrops[i].window, 0, 0, mimiPlacement(backdrops[i].bounds.size, backdrops[i].bounds));
			if (!backdrops[i].over) {
				SLSTransactionOrderWindow(transaction, backdrops[i].window, kMimiOrderAbove, 0);
			}
		}
		// The proxies go in back to front, each above everything, so they
		// stack as their windows do on screen.
		for (int i = 0; i < kept; i++) {
			int back = 0;
			for (int j = 1; j < kept; j++) {
				if (next[j].depth > next[back].depth) {
					back = j;
				}
			}
			SLSTransactionSetWindowTransform(
			    transaction, next[back].proxy, 0, 0, mimiPlacement(next[back].drawn, next[back].from));
			SLSTransactionOrderWindow(transaction, next[back].proxy, kMimiOrderAbove, 0);
			next[back].depth = -1;
		}
		for (int i = 0; i < backdropCount; i++) {
			if (backdrops[i].over) {
				SLSTransactionOrderWindow(transaction, backdrops[i].window, kMimiOrderAbove, 0);
			}
		}
		mimiRetireLocked(cid, transaction);
		CFRelease(transaction);

		gAnim.proxies = next;
		gAnim.proxyCount = kept;
		gAnim.backdrops = backdrops;
		gAnim.backdropCount = backdropCount;
		gAnim.duration = duration;
		gAnim.easing = easing;
		gAnim.running = NO;

		pthread_mutex_unlock(&gAnim.lock);
		return kept;
	}
}

void MimiAnimationStart(const uint32_t *dropped, int count) {
	int cid = SLSMainConnectionID();
	pthread_mutex_lock(&gAnim.lock);
	if (gAnim.running || (gAnim.proxyCount == 0 && gAnim.backdropCount == 0)) {
		pthread_mutex_unlock(&gAnim.lock);
		return;
	}

	CFTypeRef transaction = SLSTransactionCreate(cid);
	int kept = 0;
	for (int i = 0; i < gAnim.proxyCount; i++) {
		BOOL drop = NO;
		for (int j = 0; j < count; j++) {
			if (dropped[j] == gAnim.proxies[i].number) {
				drop = YES;
				break;
			}
		}
		if (drop) {
			SLSTransactionOrderWindow(transaction, gAnim.proxies[i].proxy, kMimiOrderOut, 0);
			mimiRecycleLocked(cid, gAnim.proxies[i].proxy, gAnim.proxies[i].drawn, gAnim.proxies[i].scale);
		} else {
			gAnim.proxies[kept++] = gAnim.proxies[i];
		}
	}
	gAnim.proxyCount = kept;

	if (kept == 0) {
		mimiReleaseAllLocked(cid, transaction);
		CFRelease(transaction);
		pthread_mutex_unlock(&gAnim.lock);
		return;
	}
	SLSTransactionCommit(transaction, 1);
	CFRelease(transaction);

	if (!GetRunLoop()) {
		// No run loop to tick on, as in the CLI, so the proxies are removed
		// at once.
		CFTypeRef teardown = SLSTransactionCreate(cid);
		mimiReleaseAllLocked(cid, teardown);
		CFRelease(teardown);
		pthread_mutex_unlock(&gAnim.lock);
		return;
	}
	gAnim.start = CACurrentMediaTime();
	gAnim.running = YES;
	mimiStartLinkLocked();
	pthread_mutex_unlock(&gAnim.lock);
}
