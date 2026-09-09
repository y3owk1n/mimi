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

typedef struct {
	uint32_t number;
	uint32_t proxy;
	CGRect from;
	CGRect to;
	// alone is whether no other animating window overlaps this one where it
	// starts, so its picture can be cropped from a composite; picture and
	// scale hold the capture between the two phases of MimiAnimationBegin.
	BOOL alone;
	CGImageRef picture;
	double scale;
} MimiProxy;

static struct {
	pthread_mutex_t lock;
	BOOL running;
	MimiProxy *proxies;
	int proxyCount;
	uint32_t *backdrops;
	int backdropCount;
	double start;
	double duration;
	int easing;
} gAnim = {.lock = PTHREAD_MUTEX_INITIALIZER};

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

// Where the proxies are at this instant of the running animation, for the
// windows a new animation starts from. The caller holds the lock.
static double mimiProgressLocked(void) {
	if (gAnim.duration <= 0) {
		return 1;
	}
	double t = (CACurrentMediaTime() - gAnim.start) / gAnim.duration;
	return mimiEase(gAnim.easing, t < 0 ? 0 : (t > 1 ? 1 : t));
}

static BOOL mimiInFlightLocked(uint32_t number, CGRect *out) {
	if (!gAnim.running) {
		return NO;
	}
	double p = mimiProgressLocked();
	for (int i = 0; i < gAnim.proxyCount; i++) {
		if (gAnim.proxies[i].number == number) {
			*out = mimiLerp(gAnim.proxies[i].from, gAnim.proxies[i].to, p);
			return YES;
		}
	}
	return NO;
}

// Create a proxy window of the given size at the origin, to be painted with
// mimiPaint and placed with a transform in the caller's transaction. Returns
// 0 when the window server refuses.
static uint32_t mimiNewProxy(int cid, CGSize size, double scale) {
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

// Draw picture into the proxy window and release it. A captured image is
// materialised the first time it is read, which is most of a proxy's cost.
static void mimiPaint(int cid, uint32_t wid, CGSize size, CGImageRef picture) {
	CGContextRef context = SLWindowContextCreate(cid, wid, NULL);
	if (context) {
		// Copying the pixels in, alpha included, rather than compositing
		// them over a cleared backing: the picture covers the window.
		CGContextSetBlendMode(context, kCGBlendModeCopy);
		CGContextDrawImage(context, CGRectMake(0, 0, size.width, size.height), picture);
		CGContextFlush(context);
		CGContextRelease(context);
	}
	CGImageRelease(picture);
}

static void mimiReleaseAllLocked(int cid, CFTypeRef transaction) {
	for (int i = 0; i < gAnim.proxyCount; i++) {
		SLSTransactionOrderWindow(transaction, gAnim.proxies[i].proxy, kMimiOrderOut, 0);
	}
	for (int i = 0; i < gAnim.backdropCount; i++) {
		SLSTransactionOrderWindow(transaction, gAnim.backdrops[i], kMimiOrderOut, 0);
	}
	SLSTransactionCommit(transaction, 1);

	for (int i = 0; i < gAnim.proxyCount; i++) {
		SLSReleaseWindow(cid, gAnim.proxies[i].proxy);
	}
	for (int i = 0; i < gAnim.backdropCount; i++) {
		SLSReleaseWindow(cid, gAnim.backdrops[i]);
	}
	free(gAnim.proxies);
	free(gAnim.backdrops);
	gAnim.proxies = NULL;
	gAnim.backdrops = NULL;
	gAnim.proxyCount = 0;
	gAnim.backdropCount = 0;
	gAnim.running = NO;
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
			    transaction, proxy->proxy, 0, 0, mimiPlacement(proxy->from.size, mimiLerp(proxy->from, proxy->to, p)));
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

#pragma mark - API

int MimiScreenCaptureGranted(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }

int MimiAnimationBegin(const MimiAnimationTarget *targets, int count, double duration, int easing) {
	if (!targets || count <= 0) {
		return 0;
	}
	if (!CGPreflightScreenCaptureAccess()) {
		return -1;
	}

	@autoreleasepool {
		int cid = SLSMainConnectionID();
		pthread_mutex_lock(&gAnim.lock);

		// Where each window starts: its current animated frame if it is animating, else
		// where the window server has it.
		MimiProxy *next = calloc((size_t)count, sizeof(MimiProxy));
		int nextCount = 0;
		for (int i = 0; i < count; i++) {
			CGRect from;
			if (!mimiInFlightLocked(targets[i].number, &from)) {
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
			if (fabs(from.origin.x - to.origin.x) < 0.5 && fabs(from.origin.y - to.origin.y) < 0.5 &&
			    fabs(from.size.width - to.size.width) < 0.5 && fabs(from.size.height - to.size.height) < 0.5) {
				continue;
			}
			next[nextCount].number = targets[i].number;
			next[nextCount].from = from;
			next[nextCount].to = to;
			nextCount++;
		}

		// Everything on screen but the animating windows and our own, a
		// previous animation's proxies. The list runs front to back, and is
		// split at the first animating window. Whatever comes before it is
		// in front of every moving window, a floating window say, and is
		// drawn over the proxies. The rest goes under them.
		// CGWindowListCreateImageFromArray reads the ids as raw values, not
		// as numbers.
		CFMutableArrayRef others = CFArrayCreateMutable(NULL, 0, NULL);
		CFMutableArrayRef front = CFArrayCreateMutable(NULL, 0, NULL);
		pid_t self = getpid();
		BOOL passed = NO;
		CFArrayRef onScreen = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
		for (NSDictionary *info in (__bridge NSArray *)onScreen) {
			if ([info[(id)kCGWindowOwnerPID] intValue] == self ||
			    [info[(id)kCGWindowLayer] intValue] >= kMimiDockLayer) {
				continue;
			}
			uint32_t number = [info[(id)kCGWindowNumber] unsignedIntValue];
			BOOL inFlight = NO;
			for (int i = 0; i < nextCount && !inFlight; i++) {
				inFlight = next[i].number == number;
			}
			if (inFlight) {
				passed = YES;
			} else {
				CFArrayAppendValue(passed ? others : front, (const void *)(uintptr_t)number);
			}
		}
		if (onScreen) {
			CFRelease(onScreen);
		}

		// Backdrops: at most two per display, the still of what is under
		// the proxies and, when anything is, the still of what is over them.
		uint32_t *backdrops = calloc(32, sizeof(uint32_t));
		CGRect *backdropBounds = calloc(32, sizeof(CGRect));
		CGImageRef *backdropStills = calloc(32, sizeof(CGImageRef));
		double *backdropScales = calloc(32, sizeof(double));
		BOOL *backdropOver = calloc(32, sizeof(BOOL));
		int backdropCount = 0;
		CGDirectDisplayID displays[16];
		uint32_t displayCount = 0;
		CGGetActiveDisplayList(16, displays, &displayCount);

		// Every capture is taken before any window is made: a capture that
		// follows the creation of a window not yet drawn waits half a second
		// on it.
		for (uint32_t d = 0; d < displayCount; d++) {
			CGRect bounds = CGDisplayBounds(displays[d]);
			BOOL any = NO;
			for (int i = 0; i < nextCount; i++) {
				if (next[i].picture == NULL && mimiOnDisplay(next[i].from, bounds)) {
					any = YES;
					break;
				}
			}
			if (!any) {
				continue;
			}

			// Deprecated for ScreenCaptureKit, whose screenshot is asynchronous
			// and far slower to start; this composes the display in a few
			// milliseconds, which an animation that starts on the same frame
			// as the event needs.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
			// The windows that overlap another animating window, as under a monocle
			// layout, are captured one by one: a composite of the set shows
			// only the top one. The rest are cropped from one composite, one
			// call for all of them, since a capture costs the same however
			// small the window.
			CFMutableArrayRef apart = CFArrayCreateMutable(NULL, 0, NULL);
			for (int i = 0; i < nextCount; i++) {
				if (next[i].picture != NULL || !mimiOnDisplay(next[i].from, bounds)) {
					continue;
				}
				BOOL overlaps = NO;
				for (int j = 0; j < nextCount && !overlaps; j++) {
					if (j != i && next[j].picture == NULL && mimiOnDisplay(next[j].from, bounds)) {
						overlaps = !CGRectIsEmpty(CGRectIntersection(next[i].from, next[j].from));
					}
				}
				next[i].alone = !overlaps;
				if (!overlaps) {
					CFArrayAppendValue(apart, (const void *)(uintptr_t)next[i].number);
				}
			}

			CGImageRef still = CGWindowListCreateImageFromArray(bounds, others, kCGWindowImageBestResolution);
			CGImageRef cover = CFArrayGetCount(front) > 0
			                       ? CGWindowListCreateImageFromArray(bounds, front, kCGWindowImageBestResolution)
			                       : NULL;
			CGImageRef flight = CFArrayGetCount(apart) > 0
			                        ? CGWindowListCreateImageFromArray(bounds, apart, kCGWindowImageBestResolution)
			                        : NULL;
			CFRelease(apart);
			CGImageRef measure = still ? still : (cover ? cover : flight);
			double scale = measure ? (double)CGImageGetWidth(measure) / bounds.size.width : 1;

			// A display with nothing else on it needs no backdrop.
			CGImageRef stills[2] = {still, cover};
			for (int k = 0; k < 2; k++) {
				if (stills[k] && backdropCount < 32) {
					backdropBounds[backdropCount] = bounds;
					backdropStills[backdropCount] = stills[k];
					backdropScales[backdropCount] = scale;
					backdropOver[backdropCount] = k == 1;
					backdropCount++;
				}
			}

			for (int i = 0; i < nextCount; i++) {
				MimiProxy *proxy = &next[i];
				if (proxy->picture != NULL || !mimiOnDisplay(proxy->from, bounds)) {
					continue;
				}
				if (proxy->alone && flight) {
					CGRect crop = CGRectMake(
					    (proxy->from.origin.x - bounds.origin.x) * scale,
					    (proxy->from.origin.y - bounds.origin.y) * scale, proxy->from.size.width * scale,
					    proxy->from.size.height * scale);
					proxy->picture = CGImageCreateWithImageInRect(flight, crop);
				} else if (!proxy->alone) {
					proxy->picture = CGWindowListCreateImage(
					    proxy->from, kCGWindowListOptionIncludingWindow, proxy->number, kCGWindowImageBestResolution);
				}
				proxy->scale = scale;
				if (!proxy->picture) {
					MIMI_LOG("animation: capturing window %u failed", proxy->number);
				}
			}
#pragma clang diagnostic pop

			if (flight) {
				CGImageRelease(flight);
			}
		}
		CFRelease(others);
		CFRelease(front);

		// Then the windows, each painted the moment it exists: the window
		// server stalls for half a second on a request that arrives while
		// windows it made are still waiting for their first pixels.
		int kept = 0;
		for (int i = 0; i < backdropCount; i++) {
			uint32_t backdrop = mimiNewProxy(cid, backdropBounds[i].size, backdropScales[i]);
			if (backdrop) {
				mimiPaint(cid, backdrop, backdropBounds[i].size, backdropStills[i]);
				backdrops[kept] = backdrop;
				backdropBounds[kept] = backdropBounds[i];
				backdropOver[kept] = backdropOver[i];
				kept++;
			} else {
				CGImageRelease(backdropStills[i]);
			}
		}
		backdropCount = kept;

		for (int i = 0; i < nextCount; i++) {
			if (next[i].picture) {
				next[i].proxy = mimiNewProxy(cid, next[i].from.size, next[i].scale);
				if (next[i].proxy) {
					mimiPaint(cid, next[i].proxy, next[i].from.size, next[i].picture);
				} else {
					CGImageRelease(next[i].picture);
				}
				next[i].picture = NULL;
			}
		}
		free(backdropStills);
		free(backdropScales);

		// Keep the proxies that were made, in order.
		kept = 0;
		for (int i = 0; i < nextCount; i++) {
			if (next[i].proxy != 0) {
				next[kept++] = next[i];
			}
		}

		// One commit: the under backdrops come in, the new proxies over
		// them, the over backdrops on top, and the previous animation, if
		// any, goes out.
		CFTypeRef transaction = SLSTransactionCreate(cid);
		for (int i = 0; i < backdropCount; i++) {
			SLSTransactionSetWindowTransform(
			    transaction, backdrops[i], 0, 0, mimiPlacement(backdropBounds[i].size, backdropBounds[i]));
			if (!backdropOver[i]) {
				SLSTransactionOrderWindow(transaction, backdrops[i], kMimiOrderAbove, 0);
			}
		}
		for (int i = 0; i < kept; i++) {
			SLSTransactionSetWindowTransform(
			    transaction, next[i].proxy, 0, 0, mimiPlacement(next[i].from.size, next[i].from));
			SLSTransactionOrderWindow(transaction, next[i].proxy, kMimiOrderAbove, 0);
		}
		for (int i = 0; i < backdropCount; i++) {
			if (backdropOver[i]) {
				SLSTransactionOrderWindow(transaction, backdrops[i], kMimiOrderAbove, 0);
			}
		}
		mimiReleaseAllLocked(cid, transaction);
		CFRelease(transaction);
		free(backdropBounds);
		free(backdropOver);

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
			SLSReleaseWindow(cid, gAnim.proxies[i].proxy);
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
