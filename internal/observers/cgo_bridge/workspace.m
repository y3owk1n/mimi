#import "workspace.h"

#include "_cgo_export.h"

#import <Cocoa/Cocoa.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdatomic.h>

static _Atomic(CFRunLoopRef) gRunLoop = NULL;

@interface WorkspaceObserver : NSObject
@property(nonatomic, strong) NSDictionary *notifToKind;
@property(nonatomic, strong) NSTimer *spacePollTimer;
@property(nonatomic, strong) NSSet *lastWindowIDs;
- (void)updateObserversWithAppLifecycle:(BOOL)appLifecycle
                            systemState:(BOOL)systemState
                                 volume:(BOOL)volume
                              workspace:(BOOL)workspace
                             appearance:(BOOL)appearance;
- (int)kindForNotificationName:(NSString *)name;
- (NSArray *)currentWindowList;
- (NSSet *)currentWindowIDs;
- (void)checkSpaceChange:(NSTimer *)timer;
@end

@implementation WorkspaceObserver

- (NSSet *)currentWindowIDs {
	CFArrayRef windowList = CGWindowListCopyWindowInfo(
	    kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!windowList)
		return [NSSet set];

	NSMutableSet *ids = [NSMutableSet setWithCapacity:CFArrayGetCount(windowList)];
	for (NSDictionary *info in (__bridge NSArray *)windowList) {
		NSNumber *winID = info[(__bridge NSString *)kCGWindowNumber];
		if (winID)
			[ids addObject:winID];
	}
	CFRelease(windowList);
	return ids;
}

- (NSArray *)currentWindowList {
	CFArrayRef windowList = CGWindowListCopyWindowInfo(
	    kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!windowList)
		return nil;

	return CFBridgingRelease(windowList);
}

- (NSString *)windowInfoJSON {
	NSArray *windows = [self currentWindowList];
	if (!windows)
		return @"";

	NSInteger totalCount = [windows count];
	NSMutableArray *items = [NSMutableArray arrayWithCapacity:totalCount];
	NSInteger realCount = 0;
	for (NSDictionary *info in windows) {
		NSNumber *layer = info[(__bridge NSString *)kCGWindowLayer];
		int l = layer ? [layer intValue] : 0;
		if (l == 0)
			realCount++;

		NSDictionary *bounds = info[(__bridge NSString *)kCGWindowBounds];
		CGFloat x = 0, y = 0, w = 0, h = 0;
		if (bounds) {
			x = [bounds[@"X"] doubleValue];
			y = [bounds[@"Y"] doubleValue];
			w = [bounds[@"Width"] doubleValue];
			h = [bounds[@"Height"] doubleValue];
		}

		[items addObject:@{
			@"app" : info[(__bridge NSString *)kCGWindowOwnerName] ?: @"",
			@"title" : info[(__bridge NSString *)kCGWindowName] ?: @"",
			@"pid" : info[(__bridge NSString *)kCGWindowOwnerPID] ?: @(0),
			@"layer" : @(l),
			@"x" : @(x),
			@"y" : @(y),
			@"w" : @(w),
			@"h" : @(h),
		}];
	}

	NSDictionary *payload = @{
		@"total_count" : @(totalCount),
		@"real_count" : @(realCount),
		@"windows" : items,
	};

	NSError *err = nil;
	NSData *json = [NSJSONSerialization dataWithJSONObject:payload options:0 error:&err];
	if (!json)
		return @"";
	return [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
}

- (void)updateObserversWithAppLifecycle:(BOOL)appLifecycle
                            systemState:(BOOL)systemState
                                 volume:(BOOL)volume
                              workspace:(BOOL)workspace
                             appearance:(BOOL)appearance {
	NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
	[wsnc removeObserver:self];
	[[NSDistributedNotificationCenter defaultCenter] removeObserver:self];

	if (self.spacePollTimer) {
		[self.spacePollTimer invalidate];
		self.spacePollTimer = nil;
	}

	NSMutableDictionary *tempNotifToKind = [NSMutableDictionary dictionary];

	if (appLifecycle) {
		tempNotifToKind[NSWorkspaceDidActivateApplicationNotification] = @(0);
		tempNotifToKind[NSWorkspaceDidDeactivateApplicationNotification] = @(1);
		tempNotifToKind[NSWorkspaceDidLaunchApplicationNotification] = @(2);
		tempNotifToKind[NSWorkspaceDidTerminateApplicationNotification] = @(3);
		tempNotifToKind[NSWorkspaceDidHideApplicationNotification] = @(4);
		tempNotifToKind[NSWorkspaceDidUnhideApplicationNotification] = @(5);
	}

	if (systemState) {
		tempNotifToKind[NSWorkspaceWillSleepNotification] = @(10);
		tempNotifToKind[NSWorkspaceDidWakeNotification] = @(11);
		tempNotifToKind[NSWorkspaceSessionDidResignActiveNotification] = @(12);
		tempNotifToKind[NSWorkspaceSessionDidBecomeActiveNotification] = @(13);
		tempNotifToKind[NSWorkspaceWillPowerOffNotification] = @(14);
	}

	if (volume) {
		tempNotifToKind[NSWorkspaceDidMountNotification] = @(20);
		tempNotifToKind[NSWorkspaceDidUnmountNotification] = @(21);
	}

	if (workspace) {
		tempNotifToKind[NSWorkspaceActiveSpaceDidChangeNotification] = @(70);
	}

	self.notifToKind = tempNotifToKind;

	for (NSString *notifName in self.notifToKind) {
		[wsnc addObserver:self selector:@selector(handleNotification:) name:notifName object:nil];
	}

	if (appearance) {
		NSDistributedNotificationCenter *dnc = [NSDistributedNotificationCenter defaultCenter];
		[dnc addObserver:self
		        selector:@selector(appearanceChanged:)
		            name:@"AppleInterfaceThemeChangedNotification"
		          object:nil];
	}

	if (workspace) {
		self.lastWindowIDs = [self currentWindowIDs];
		self.spacePollTimer = [NSTimer scheduledTimerWithTimeInterval:0.2
		                                                       target:self
		                                                     selector:@selector(checkSpaceChange:)
		                                                     userInfo:nil
		                                                      repeats:YES];
	}
}

- (void)checkSpaceChange:(NSTimer *)timer {
	NSSet *currentIDs = [self currentWindowIDs];
	if ([currentIDs isEqualToSet:self.lastWindowIDs]) {
		return;
	}
	self.lastWindowIDs = currentIDs;
	NSString *infoJSON = [self windowInfoJSON];
	goWorkspaceChangeEvent(70, (int)[currentIDs count], (char *)[infoJSON UTF8String]);
}

- (void)appearanceChanged:(NSNotification *)note {
	goWorkspaceEvent(42, "", "", 0, "", "");
}

- (int)kindForNotificationName:(NSString *)name {
	NSNumber *kind = self.notifToKind[name];
	return kind ? [kind intValue] : -1;
}

- (void)handleNotification:(NSNotification *)note {
	int kind = [self kindForNotificationName:note.name];
	if (kind == 70) {
		NSSet *windows = [self currentWindowIDs];
		self.lastWindowIDs = windows;
		NSString *infoJSON = [self windowInfoJSON];
		goWorkspaceChangeEvent(kind, (int)[windows count], (char *)[infoJSON UTF8String]);
		return;
	}

	NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
	const char *appName = app ? [app.localizedName UTF8String] : "";
	const char *bundleID = app ? [app.bundleIdentifier UTF8String] : "";
	int pid = app ? (int)app.processIdentifier : 0;

	const char *volPath = "";
	const char *volName = "";
	if (note.userInfo[@"NSDevicePath"]) {
		volPath = [note.userInfo[@"NSDevicePath"] UTF8String];
		volName = [[[note.userInfo[@"NSDevicePath"] lastPathComponent] stringByDeletingPathExtension] UTF8String];
	}

	goWorkspaceEvent(kind, (char *)appName, (char *)bundleID, pid, (char *)volPath, (char *)volName);
}

@end

static WorkspaceObserver *gObserver = nil;

CFRunLoopRef GetRunLoop(void) { return atomic_load(&gRunLoop); }

void InitCocoaApp(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}

void InitBridgeRunLoop(void) { atomic_store(&gRunLoop, CFRunLoopGetCurrent()); }

void WorkspaceObserverStart(bool appLifecycle, bool systemState, bool volume, bool workspace, bool appearance) {
	@autoreleasepool {
		InitBridgeRunLoop();
		gObserver = [[WorkspaceObserver alloc] init];
		[gObserver updateObserversWithAppLifecycle:appLifecycle
		                               systemState:systemState
		                                    volume:volume
		                                 workspace:workspace
		                                appearance:appearance];

		CFRunLoopRun();
		atomic_store(&gRunLoop, NULL);
	}
}

void WorkspaceObserverUpdate(bool appLifecycle, bool systemState, bool volume, bool workspace, bool appearance) {
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return;

	if (CFRunLoopGetCurrent() == rl) {
		if (gObserver) {
			[gObserver updateObserversWithAppLifecycle:appLifecycle
			                               systemState:systemState
			                                    volume:volume
			                                 workspace:workspace
			                                appearance:appearance];
		}
		return;
	}

	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
		if (gObserver) {
			[gObserver updateObserversWithAppLifecycle:appLifecycle
			                               systemState:systemState
			                                    volume:volume
			                                 workspace:workspace
			                                appearance:appearance];
		}
		dispatch_semaphore_signal(sem);
	});
	CFRunLoopWakeUp(rl);
	dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
}

void WorkspaceObserverStop(void) {
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return;

	if (CFRunLoopGetCurrent() == rl) {
		if (gObserver) {
			[gObserver.spacePollTimer invalidate];
			gObserver.spacePollTimer = nil;
			NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
			[wsnc removeObserver:gObserver];
			[[NSDistributedNotificationCenter defaultCenter] removeObserver:gObserver];
			gObserver = nil;
		}
		CFRunLoopStop(rl);
		return;
	}

	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
		if (gObserver) {
			[gObserver.spacePollTimer invalidate];
			gObserver.spacePollTimer = nil;
			NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
			[wsnc removeObserver:gObserver];
			[[NSDistributedNotificationCenter defaultCenter] removeObserver:gObserver];
			gObserver = nil;
		}
		CFRunLoopStop(rl);
		dispatch_semaphore_signal(sem);
	});
	CFRunLoopWakeUp(rl);
	dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
}
