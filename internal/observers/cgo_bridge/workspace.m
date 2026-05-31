#import <Cocoa/Cocoa.h>
#import "workspace.h"
#include "_cgo_export.h"

static CFRunLoopRef gRunLoop = NULL;

@interface WorkspaceObserver : NSObject
@property (nonatomic, strong) NSDictionary *notifToKind;
- (void)startObserving;
- (int)kindForNotificationName:(NSString *)name;
@end

@implementation WorkspaceObserver

- (void)startObserving {
    self.notifToKind = @{
        // App lifecycle
        NSWorkspaceDidActivateApplicationNotification:   @(0),
        NSWorkspaceDidDeactivateApplicationNotification: @(1),
        NSWorkspaceDidLaunchApplicationNotification:     @(2),
        NSWorkspaceDidTerminateApplicationNotification:  @(3),
        NSWorkspaceDidHideApplicationNotification:       @(4),
        NSWorkspaceDidUnhideApplicationNotification:     @(5),
        // System events
        NSWorkspaceWillSleepNotification:                @(10),
        NSWorkspaceDidWakeNotification:                  @(11),
        NSWorkspaceSessionDidResignActiveNotification:   @(12),
        NSWorkspaceSessionDidBecomeActiveNotification:   @(13),
        NSWorkspaceWillPowerOffNotification:             @(14),
        // Storage
        NSWorkspaceDidMountNotification:                 @(20),
        NSWorkspaceDidUnmountNotification:               @(21),
        // Workspace (spaces/desktops)
        NSWorkspaceActiveSpaceDidChangeNotification:     @(70),
    };

    NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
    for (NSString *notifName in self.notifToKind) {
        [wsnc addObserver:self
                 selector:@selector(handleNotification:)
                     name:notifName
                   object:nil];
    }
}

- (void)appearanceChanged:(NSNotification *)note {
    // Dark/light mode changed - send appearance_changed event (kind 42)
    goWorkspaceEvent(42, "", "", 0, "", "");
}

- (int)kindForNotificationName:(NSString *)name {
    NSNumber *kind = self.notifToKind[name];
    return kind ? [kind intValue] : -1;
}

- (void)handleNotification:(NSNotification *)note {
    int kind = [self kindForNotificationName:note.name];

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

    goWorkspaceEvent(kind, appName, bundleID, pid, volPath, volName);
}

@end

static WorkspaceObserver *gObserver = nil;

CFRunLoopRef GetRunLoop(void) {
    return gRunLoop;
}

void InitCocoaApp(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        // Register as accessory so the process can receive workspace
        // notifications without showing a Dock icon or menu bar.
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    }
}

void WorkspaceObserverStart(void) {
    @autoreleasepool {
        gRunLoop = CFRunLoopGetCurrent();
        gObserver = [[WorkspaceObserver alloc] init];
        [gObserver startObserving];
        
        // Register for dark mode changes via distributed notification center
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
        NSNotificationCenter *wsnc = [[NSWorkspace sharedWorkspace] notificationCenter];
        [wsnc removeObserver:gObserver];
        [[NSDistributedNotificationCenter defaultCenter] removeObserver:gObserver];
    }
    CFRunLoopStop(CFRunLoopGetCurrent());
}
