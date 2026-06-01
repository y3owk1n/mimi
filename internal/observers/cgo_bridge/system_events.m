#import "system_events.h"

#include "_cgo_export.h"

#import <Cocoa/Cocoa.h>
#import <CoreAudio/CoreAudio.h>
#import <IOKit/IOKitLib.h>
#import <IOKit/ps/IOPSKeys.h>
#import <IOKit/ps/IOPowerSources.h>
#import <SystemConfiguration/SystemConfiguration.h>
#import <netinet/in.h>

#pragma mark - Power/Battery Observer

static void PowerSourceCallback(void *context);

@interface PowerObserver : NSObject
- (void)startObserving;
- (void)stopObserving;
- (void)checkPowerSources;
@property(nonatomic, strong) NSString *lastPowerState;
@property(nonatomic) BOOL lastBatteryLow;
@property(nonatomic) BOOL lastBatteryCritical;
@end

@implementation PowerObserver {
	CFRunLoopSourceRef _runLoopSource;
}

- (void)startObserving {
	_runLoopSource = IOPSNotificationCreateRunLoopSource(PowerSourceCallback, (__bridge void *)(self));
	if (_runLoopSource) {
		CFRunLoopAddSource(CFRunLoopGetCurrent(), _runLoopSource, kCFRunLoopDefaultMode);
	}
	// Do an initial check to set the baseline state
	[self checkPowerSources];
}

- (void)stopObserving {
	if (_runLoopSource) {
		CFRunLoopRemoveSource(CFRunLoopGetCurrent(), _runLoopSource, kCFRunLoopDefaultMode);
		CFRelease(_runLoopSource);
		_runLoopSource = NULL;
	}
}

- (void)checkPowerSources {
	CFTypeRef snapshot = IOPSCopyPowerSourcesInfo();
	if (!snapshot)
		return;
	CFArrayRef sources = IOPSCopyPowerSourcesList(snapshot);
	if (!sources) {
		CFRelease(snapshot);
		return;
	}

	CFIndex count = CFArrayGetCount(sources);
	for (CFIndex i = 0; i < count; i++) {
		CFTypeRef ps = CFArrayGetValueAtIndex(sources, i);
		CFDictionaryRef desc = IOPSGetPowerSourceDescription(snapshot, ps);
		if (!desc)
			continue;

		// 1. Check Power Source State
		NSString *stateKey = @kIOPSPowerSourceStateKey;
		CFStringRef state = CFDictionaryGetValue(desc, (__bridge CFStringRef)stateKey);
		if (state) {
			NSString *stateStr = (__bridge NSString *)state;
			NSString *acValue = @kIOPSACPowerValue;
			NSString *batteryValue = @kIOPSBatteryPowerValue;
			if (self.lastPowerState && ![stateStr isEqualToString:self.lastPowerState]) {
				if ([stateStr isEqualToString:acValue]) {
					goSystemEvent(50);  // PowerAdapterConnected
				} else if ([stateStr isEqualToString:batteryValue]) {
					goSystemEvent(51);  // PowerAdapterDisconnected
				}
			}
			self.lastPowerState = stateStr;
		}

		// 2. Check Battery Low / Critical
		NSString *lowKey = @kIOPSIsChargingKey;
		CFBooleanRef isLow = CFDictionaryGetValue(desc, (__bridge CFStringRef)lowKey);
		BOOL low = NO;

		// Check capacity to determine low/critical
		NSString *curCapKey = @kIOPSCurrentCapacityKey;
		NSString *maxCapKey = @kIOPSMaxCapacityKey;
		CFNumberRef curCapRef = CFDictionaryGetValue(desc, (__bridge CFStringRef)curCapKey);
		CFNumberRef maxCapRef = CFDictionaryGetValue(desc, (__bridge CFStringRef)maxCapKey);
		int curCap = 0, maxCap = 100;
		if (curCapRef)
			CFNumberGetValue(curCapRef, kCFNumberIntType, &curCap);
		if (maxCapRef)
			CFNumberGetValue(maxCapRef, kCFNumberIntType, &maxCap);
		double pct = maxCap > 0 ? ((double)curCap / maxCap) * 100.0 : 0.0;
		low = (pct <= 20.0);

		BOOL critical = (low && pct <= 5.0);

		if (low && !self.lastBatteryLow) {
			goSystemEvent(52);  // BatteryLow
		}
		if (critical && !self.lastBatteryCritical) {
			goSystemEvent(53);  // BatteryCritical
		}

		self.lastBatteryLow = low;
		self.lastBatteryCritical = critical;
	}

	CFRelease(sources);
	CFRelease(snapshot);
}

@end

static void PowerSourceCallback(void *context) {
	PowerObserver *observer = (__bridge PowerObserver *)context;
	[observer checkPowerSources];
}

static PowerObserver *gPowerObserver = nil;

void PowerObserverStart(void) {
	@autoreleasepool {
		gPowerObserver = [[PowerObserver alloc] init];
		[gPowerObserver startObserving];
	}
}

void PowerObserverStop(void) {
	if (gPowerObserver) {
		[gPowerObserver stopObserving];
		gPowerObserver = nil;
	}
}

#pragma mark - Audio Device Observer

static OSStatus AudioPropertyListener(
    AudioObjectID inObjectID, UInt32 inNumberAddresses, const AudioObjectPropertyAddress *inAddresses,
    void *inClientData) {
	goSystemEvent(60);  // audio_device_changed
	return noErr;
}

void AudioObserverStart(void) {
	AudioObjectPropertyAddress address1 = {
	    kAudioHardwarePropertyDevices, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	AudioObjectPropertyAddress address2 = {
	    kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	AudioObjectPropertyAddress address3 = {
	    kAudioHardwarePropertyDefaultInputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};

	AudioObjectAddPropertyListener(kAudioObjectSystemObject, &address1, AudioPropertyListener, NULL);
	AudioObjectAddPropertyListener(kAudioObjectSystemObject, &address2, AudioPropertyListener, NULL);
	AudioObjectAddPropertyListener(kAudioObjectSystemObject, &address3, AudioPropertyListener, NULL);
}

void AudioObserverStop(void) {
	AudioObjectPropertyAddress address1 = {
	    kAudioHardwarePropertyDevices, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	AudioObjectPropertyAddress address2 = {
	    kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	AudioObjectPropertyAddress address3 = {
	    kAudioHardwarePropertyDefaultInputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};

	AudioObjectRemovePropertyListener(kAudioObjectSystemObject, &address1, AudioPropertyListener, NULL);
	AudioObjectRemovePropertyListener(kAudioObjectSystemObject, &address2, AudioPropertyListener, NULL);
	AudioObjectRemovePropertyListener(kAudioObjectSystemObject, &address3, AudioPropertyListener, NULL);
}

#pragma mark - Clipboard Observer

@interface ClipboardObserver : NSObject
- (void)startObserving;
- (void)stopObserving;
- (void)clipboardChanged:(NSTimer *)timer;
@end

@implementation ClipboardObserver {
	NSTimer *_timer;
	NSInteger _lastChangeCount;
}

- (void)startObserving {
	_lastChangeCount = [[NSPasteboard generalPasteboard] changeCount];
	_timer = [NSTimer scheduledTimerWithTimeInterval:0.5
	                                          target:self
	                                        selector:@selector(clipboardChanged:)
	                                        userInfo:nil
	                                         repeats:YES];
}

- (void)stopObserving {
	[_timer invalidate];
	_timer = nil;
}

- (void)clipboardChanged:(NSTimer *)timer {
	NSInteger changeCount = [[NSPasteboard generalPasteboard] changeCount];
	if (changeCount != _lastChangeCount) {
		_lastChangeCount = changeCount;
		goSystemEvent(100);  // clipboard_changed
	}
}

@end

static ClipboardObserver *gClipboardObserver = nil;

void ClipboardObserverStart(void) {
	@autoreleasepool {
		gClipboardObserver = [[ClipboardObserver alloc] init];
		[gClipboardObserver startObserving];
	}
}

void ClipboardObserverStop(void) {
	if (gClipboardObserver) {
		[gClipboardObserver stopObserving];
		gClipboardObserver = nil;
	}
}

#pragma mark - USB/Peripheral Observer

static void USBDeviceAdded(void *refCon, io_iterator_t iterator);
static void USBDeviceRemoved(void *refCon, io_iterator_t iterator);

@interface USBObserver : NSObject
- (void)startObserving;
- (void)stopObserving;
@property(nonatomic) IONotificationPortRef notifyPort;
@property(nonatomic) io_iterator_t addedIter;
@property(nonatomic) io_iterator_t removedIter;
@end

@implementation USBObserver

- (void)startObserving {
	self.notifyPort = IONotificationPortCreate(kIOMainPortDefault);
	if (!self.notifyPort)
		return;

	CFRunLoopSourceRef runLoopSource = IONotificationPortGetRunLoopSource(self.notifyPort);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), runLoopSource, kCFRunLoopDefaultMode);

	CFMutableDictionaryRef matchingDict = IOServiceMatching("IOUSBDevice");
	CFRetain(matchingDict);  // Retain since we use it twice

	// Monitor additions
	kern_return_t kr = IOServiceAddMatchingNotification(
	    self.notifyPort, kIOFirstMatchNotification, matchingDict, USBDeviceAdded, (__bridge void *)(self), &_addedIter);
	if (kr == kIOReturnSuccess) {
		// Drain iterator to arm and ignore initial set of already connected devices
		io_service_t device;
		while ((device = IOIteratorNext(self.addedIter))) {
			IOObjectRelease(device);
		}
	}

	// Monitor removals
	kr = IOServiceAddMatchingNotification(
	    self.notifyPort, kIOTerminatedNotification, matchingDict, USBDeviceRemoved, (__bridge void *)(self),
	    &_removedIter);
	if (kr == kIOReturnSuccess) {
		io_service_t device;
		while ((device = IOIteratorNext(self.removedIter))) {
			IOObjectRelease(device);
		}
	}
}

- (void)stopObserving {
	if (self.addedIter) {
		IOObjectRelease(self.addedIter);
		self.addedIter = 0;
	}
	if (self.removedIter) {
		IOObjectRelease(self.removedIter);
		self.removedIter = 0;
	}
	if (self.notifyPort) {
		IONotificationPortDestroy(self.notifyPort);
		self.notifyPort = NULL;
	}
}

@end

static void USBDeviceAdded(void *refCon, io_iterator_t iterator) {
	io_service_t device;
	while ((device = IOIteratorNext(iterator))) {
		goSystemEvent(80);  // USBDeviceConnected
		IOObjectRelease(device);
	}
}

static void USBDeviceRemoved(void *refCon, io_iterator_t iterator) {
	io_service_t device;
	while ((device = IOIteratorNext(iterator))) {
		goSystemEvent(81);  // USBDeviceDisconnected
		IOObjectRelease(device);
	}
}

static USBObserver *gUSBObserver = nil;

void USBObserverStart(void) {
	@autoreleasepool {
		gUSBObserver = [[USBObserver alloc] init];
		[gUSBObserver startObserving];
	}
}

void USBObserverStop(void) {
	if (gUSBObserver) {
		[gUSBObserver stopObserving];
		gUSBObserver = nil;
	}
}

#pragma mark - Network Observer

static void ReachabilityCallback(SCNetworkReachabilityRef target, SCNetworkReachabilityFlags flags, void *info);

@interface NetworkObserver : NSObject
- (void)startObserving;
- (void)stopObserving;
- (void)updateStatus:(SCNetworkReachabilityFlags)flags;
@property(nonatomic) SCNetworkReachabilityRef reachability;
@property(nonatomic) BOOL lastReachable;
@property(nonatomic) BOOL initialized;
@end

@implementation NetworkObserver

- (void)startObserving {
	struct sockaddr_in zeroAddress;
	bzero(&zeroAddress, sizeof(zeroAddress));
	zeroAddress.sin_len = sizeof(zeroAddress);
	zeroAddress.sin_family = AF_INET;

	self.reachability =
	    SCNetworkReachabilityCreateWithAddress(kCFAllocatorDefault, (const struct sockaddr *)&zeroAddress);
	if (!self.reachability)
		return;

	SCNetworkReachabilityContext context = {0, (__bridge void *)self, NULL, NULL, NULL};
	if (SCNetworkReachabilitySetCallback(self.reachability, ReachabilityCallback, &context)) {
		SCNetworkReachabilityScheduleWithRunLoop(self.reachability, CFRunLoopGetCurrent(), kCFRunLoopDefaultMode);
	}

	SCNetworkReachabilityFlags flags;
	if (SCNetworkReachabilityGetFlags(self.reachability, &flags)) {
		[self updateStatus:flags];
	}
	self.initialized = YES;
}

- (void)stopObserving {
	if (self.reachability) {
		SCNetworkReachabilityUnscheduleFromRunLoop(self.reachability, CFRunLoopGetCurrent(), kCFRunLoopDefaultMode);
		CFRelease(self.reachability);
		self.reachability = NULL;
	}
}

- (void)updateStatus:(SCNetworkReachabilityFlags)flags {
	BOOL reachable =
	    (flags & kSCNetworkReachabilityFlagsReachable) && !(flags & kSCNetworkReachabilityFlagsConnectionRequired);
	if (self.initialized) {
		if (reachable != self.lastReachable) {
			if (reachable) {
				goSystemEvent(90);  // NetworkUp
			} else {
				goSystemEvent(91);  // NetworkDown
			}
		}
	}
	self.lastReachable = reachable;
}

@end

static void ReachabilityCallback(SCNetworkReachabilityRef target, SCNetworkReachabilityFlags flags, void *info) {
	NetworkObserver *observer = (__bridge NetworkObserver *)info;
	[observer updateStatus:flags];
}

static NetworkObserver *gNetworkObserver = nil;

void NetworkObserverStart(void) {
	@autoreleasepool {
		gNetworkObserver = [[NetworkObserver alloc] init];
		[gNetworkObserver startObserving];
	}
}

void NetworkObserverStop(void) {
	if (gNetworkObserver) {
		[gNetworkObserver stopObserving];
		gNetworkObserver = nil;
	}
}

#pragma mark - Display Observer

static void DisplayReconfigurationCallBack(
    CGDirectDisplayID display, CGDisplayChangeSummaryFlags flags, void *userInfo) {
	if (flags & kCGDisplayBeginConfigurationFlag) {
		return;  // Process only when configuration has finalized
	}

	if (flags & kCGDisplayAddFlag) {
		goSystemEvent(40);  // ExternalDisplayConnected
	} else if (flags & kCGDisplayRemoveFlag) {
		goSystemEvent(41);  // ExternalDisplayDisconnected
	}
}

void DisplayObserverStart(void) { CGDisplayRegisterReconfigurationCallback(DisplayReconfigurationCallBack, NULL); }

void DisplayObserverStop(void) { CGDisplayRemoveReconfigurationCallback(DisplayReconfigurationCallBack, NULL); }
