#import "workspace.h"

#include "_cgo_export.h"
#import "eventkinds.h"

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
		tempNotifToKind[NSWorkspaceDidActivateApplicationNotification] = @(MIMI_KIND_APP_ACTIVATE);
		tempNotifToKind[NSWorkspaceDidDeactivateApplicationNotification] = @(MIMI_KIND_APP_DEACTIVATE);
		tempNotifToKind[NSWorkspaceDidLaunchApplicationNotification] = @(MIMI_KIND_APP_LAUNCH);
		tempNotifToKind[NSWorkspaceDidTerminateApplicationNotification] = @(MIMI_KIND_APP_QUIT);
		tempNotifToKind[NSWorkspaceDidHideApplicationNotification] = @(MIMI_KIND_APP_HIDE);
		tempNotifToKind[NSWorkspaceDidUnhideApplicationNotification] = @(MIMI_KIND_APP_UNHIDE);
	}

	if (systemState) {
		tempNotifToKind[NSWorkspaceWillSleepNotification] = @(MIMI_KIND_WILL_SLEEP);
		tempNotifToKind[NSWorkspaceDidWakeNotification] = @(MIMI_KIND_DID_WAKE);
		tempNotifToKind[NSWorkspaceSessionDidResignActiveNotification] = @(MIMI_KIND_SESSION_RESIGN);
		tempNotifToKind[NSWorkspaceSessionDidBecomeActiveNotification] = @(MIMI_KIND_SESSION_BECOME);
		tempNotifToKind[NSWorkspaceWillPowerOffNotification] = @(MIMI_KIND_WILL_POWER_OFF);
	}

	if (volume) {
		tempNotifToKind[NSWorkspaceDidMountNotification] = @(MIMI_KIND_VOLUME_MOUNT);
		tempNotifToKind[NSWorkspaceDidUnmountNotification] = @(MIMI_KIND_VOLUME_UNMOUNT);
	}

	if (workspace) {
		tempNotifToKind[NSWorkspaceActiveSpaceDidChangeNotification] = @(MIMI_KIND_WORKSPACE_CHANGED);
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
		self.spacePollTimer = [NSTimer scheduledTimerWithTimeInterval:2.0
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
	goWorkspaceChangeEvent(MIMI_KIND_WORKSPACE_CHANGED, (int)[currentIDs count], (char *)[infoJSON UTF8String]);
}

- (void)appearanceChanged:(NSNotification *)note {
	goWorkspaceEvent(MIMI_KIND_APPEARANCE_CHANGED, "", "", 0, "", "");
}

- (int)kindForNotificationName:(NSString *)name {
	NSNumber *kind = self.notifToKind[name];
	return kind ? [kind intValue] : -1;
}

- (void)handleNotification:(NSNotification *)note {
	int kind = [self kindForNotificationName:note.name];
	if (kind == MIMI_KIND_WORKSPACE_CHANGED) {
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

void WorkspaceObserverStart(int appLifecycle, int systemState, int volume, int workspace, int appearance) {
	@autoreleasepool {
		InitBridgeRunLoop();
		gObserver = [[WorkspaceObserver alloc] init];
		[gObserver updateObserversWithAppLifecycle:appLifecycle != 0
		                               systemState:systemState != 0
		                                    volume:volume != 0
		                                 workspace:workspace != 0
		                                appearance:appearance != 0];

		CFRunLoopSourceContext context = {0};
		CFRunLoopSourceRef dummySource = CFRunLoopSourceCreate(NULL, 0, &context);
		CFRunLoopAddSource(CFRunLoopGetCurrent(), dummySource, kCFRunLoopDefaultMode);

		CFRunLoopRun();

		CFRunLoopRemoveSource(CFRunLoopGetCurrent(), dummySource, kCFRunLoopDefaultMode);
		CFRelease(dummySource);

		atomic_store(&gRunLoop, NULL);
	}
}

void WorkspaceObserverUpdate(int appLifecycle, int systemState, int volume, int workspace, int appearance) {
	CFRunLoopRef rl = GetRunLoop();
	if (!rl)
		return;

	if (CFRunLoopGetCurrent() == rl) {
		if (gObserver) {
			[gObserver updateObserversWithAppLifecycle:appLifecycle != 0
			                               systemState:systemState != 0
			                                    volume:volume != 0
			                                 workspace:workspace != 0
			                                appearance:appearance != 0];
		}
		return;
	}

	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
		if (gObserver) {
			[gObserver updateObserversWithAppLifecycle:appLifecycle != 0
			                               systemState:systemState != 0
			                                    volume:volume != 0
			                                 workspace:workspace != 0
			                                appearance:appearance != 0];
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
