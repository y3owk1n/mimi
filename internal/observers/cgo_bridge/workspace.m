#import <Cocoa/Cocoa.h>
#include <CoreGraphics/CoreGraphics.h>
#import "workspace.h"
#include "_cgo_export.h"

static CFRunLoopRef gRunLoop = NULL;

@interface WorkspaceObserver : NSObject
@property (nonatomic, strong) NSDictionary *notifToKind;
@property (nonatomic, strong) NSTimer *spacePollTimer;
@property (nonatomic, strong) NSSet *lastWindowIDs;
- (void)startObserving;
- (int)kindForNotificationName:(NSString *)name;
- (NSArray *)currentWindowList;
- (NSSet *)currentWindowIDs;
- (void)checkSpaceChange:(NSTimer *)timer;
@end

@implementation WorkspaceObserver

- (NSSet *)currentWindowIDs {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
        kCGNullWindowID
    );
    NSMutableSet *ids = [NSMutableSet setWithCapacity:CFArrayGetCount(windowList)];
    for (NSDictionary *info in (__bridge NSArray *)windowList) {
        NSNumber *winID = info[(__bridge NSString *)kCGWindowNumber];
        if (winID) [ids addObject:winID];
    }
    CFRelease(windowList);
    return ids;
}

- (NSArray *)currentWindowList {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
        kCGNullWindowID
    );
    return (NSArray *)windowList;
}

- (NSString *)windowInfoJSON {
    NSArray *windows = [self currentWindowList];
    NSInteger totalCount = [windows count];
    NSMutableArray *items = [NSMutableArray arrayWithCapacity:totalCount];
    NSInteger realCount = 0;
    for (NSDictionary *info in windows) {
        NSNumber *layer = info[(__bridge NSString *)kCGWindowLayer];
        int l = layer ? [layer intValue] : 0;
        if (l == 0) realCount++;

        NSDictionary *bounds = info[(__bridge NSString *)kCGWindowBounds];
        CGFloat x = 0, y = 0, w = 0, h = 0;
        if (bounds) {
            x = [bounds[@"X"] doubleValue];
            y = [bounds[@"Y"] doubleValue];
            w = [bounds[@"Width"] doubleValue];
            h = [bounds[@"Height"] doubleValue];
        }

        [items addObject:@{
            @"app":   info[(__bridge NSString *)kCGWindowOwnerName] ?: @"",
            @"title": info[(__bridge NSString *)kCGWindowName] ?: @"",
            @"pid":   info[(__bridge NSString *)kCGWindowOwnerPID] ?: @(0),
            @"layer": @(l),
            @"x": @(x), @"y": @(y), @"w": @(w), @"h": @(h),
        }];
    }
    CFRelease((CFArrayRef)windows);

    NSDictionary *payload = @{
        @"total_count": @(totalCount),
        @"real_count":  @(realCount),
        @"windows": items,
    };

    NSError *err = nil;
    NSData *json = [NSJSONSerialization dataWithJSONObject:payload
                                                   options:0
                                                     error:&err];
    if (!json) return @"";
    return [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
}

- (void)startObserving {
    self.notifToKind = @{
        NSWorkspaceDidActivateApplicationNotification:   @(0),
        NSWorkspaceDidDeactivateApplicationNotification: @(1),
        NSWorkspaceDidLaunchApplicationNotification:     @(2),
        NSWorkspaceDidTerminateApplicationNotification:  @(3),
        NSWorkspaceDidHideApplicationNotification:       @(4),
        NSWorkspaceDidUnhideApplicationNotification:     @(5),
        NSWorkspaceWillSleepNotification:                @(10),
        NSWorkspaceDidWakeNotification:                  @(11),
        NSWorkspaceSessionDidResignActiveNotification:   @(12),
        NSWorkspaceSessionDidBecomeActiveNotification:   @(13),
        NSWorkspaceWillPowerOffNotification:             @(14),
        NSWorkspaceDidMountNotification:                 @(20),
        NSWorkspaceDidUnmountNotification:               @(21),
        NSWorkspaceActiveSpaceDidChangeNotification:     @(70),
    };

    NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
    for (NSString *notifName in self.notifToKind) {
        [wsnc addObserver:self
                 selector:@selector(handleNotification:)
                     name:notifName
                   object:nil];
    }

    self.lastWindowIDs = [self currentWindowIDs];
    self.spacePollTimer = [NSTimer scheduledTimerWithTimeInterval:0.2
                                                           target:self
                                                         selector:@selector(checkSpaceChange:)
                                                         userInfo:nil
                                                          repeats:YES];
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
    const char *appName  = app ? [app.localizedName UTF8String] : "";
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

CFRunLoopRef GetRunLoop(void) {
    return gRunLoop;
}

void InitCocoaApp(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    }
}

void WorkspaceObserverStart(void) {
    @autoreleasepool {
        gRunLoop = CFRunLoopGetCurrent();
        gObserver = [[WorkspaceObserver alloc] init];
        [gObserver startObserving];
        
        NSDistributedNotificationCenter *dnc = [NSDistributedNotificationCenter defaultCenter];
        [dnc addObserver:gObserver
                selector:@selector(appearanceChanged:)
                    name:@"AppleInterfaceThemeChangedNotification"
                  object:nil];

        CFRunLoopRun();
        gRunLoop = NULL;
    }
}

void WorkspaceObserverStop(void) {
	if (gObserver) {
		[gObserver.spacePollTimer invalidate];
		gObserver.spacePollTimer = nil;
		NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
		[wsnc removeObserver:gObserver];
		[[NSDistributedNotificationCenter defaultCenter] removeObserver:gObserver];
	}
	CFRunLoopStop(CFRunLoopGetCurrent());
}
