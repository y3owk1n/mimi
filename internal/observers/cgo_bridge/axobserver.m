#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import "axobserver.h"
#import "workspace.h"
#include "_cgo_export.h"

@interface AXEntry : NSObject
@property AXObserverRef observer;
@property AXUIElementRef appElement;
@property pid_t pid;
@end
@implementation AXEntry
@end

static NSMutableDictionary<NSNumber*, AXEntry*> *gEntries;
static dispatch_queue_t gAxQueue;

static void axCallback(AXObserverRef observer,
                       AXUIElementRef element,
                       CFStringRef notification,
                       void *refcon) {
    @autoreleasepool {
        pid_t pid = (pid_t)(intptr_t)refcon;

        CFTypeRef titleRef = NULL;
        AXUIElementCopyAttributeValue(element, kAXTitleAttribute, &titleRef);
        const char *title = "";
        if (titleRef) {
            if (CFGetTypeID(titleRef) == CFStringGetTypeID()) {
                title = [(__bridge NSString *)titleRef UTF8String];
                if (!title) title = "";
            }
            CFRelease(titleRef);
        }

        int kind = -1;
        if (CFEqual(notification, kAXFocusedWindowChangedNotification))        kind = 30;
        else if (CFEqual(notification, kAXTitleChangedNotification))           kind = 31;
        else if (CFEqual(notification, kAXWindowCreatedNotification))          kind = 32;
        else if (CFEqual(notification, kAXUIElementDestroyedNotification))     kind = 33;

        if (kind >= 0) {
            NSRunningApplication *app =
                [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
            const char *appName  = app ? [app.localizedName UTF8String] : "";
            const char *bundleID = app ? [app.bundleIdentifier UTF8String] : "";
            goAXEvent(kind, appName, bundleID, (int)pid, title);
        }
    }
}

static void axInstallBlock(int pid) {
    if (!gEntries) gEntries = [NSMutableDictionary new];

    NSNumber *key = @(pid);
    if (gEntries[key]) return;

    AXUIElementRef appElement = AXUIElementCreateApplication(pid);
    if (!appElement) return;

    AXObserverRef observer = NULL;
    AXError err = AXObserverCreate(pid, axCallback, &observer);
    if (err != kAXErrorSuccess) {
        CFRelease(appElement);
        return;
    }

    CFStringRef notifications[] = {
        kAXFocusedWindowChangedNotification,
        kAXTitleChangedNotification,
        kAXWindowCreatedNotification,
        kAXUIElementDestroyedNotification,
    };
    size_t notifCount = sizeof(notifications) / sizeof(notifications[0]);
    for (size_t i = 0; i < notifCount; i++) {
        AXObserverAddNotification(observer, appElement,
                                  notifications[i], (void*)(intptr_t)pid);
    }

    CFRunLoopRef rl = GetRunLoop();
    if (rl) {
        CFRunLoopAddSource(rl,
                           AXObserverGetRunLoopSource(observer),
                           kCFRunLoopDefaultMode);
    }

    AXEntry *entry = [AXEntry new];
    entry.observer    = observer;
    entry.appElement  = appElement;
    entry.pid         = pid;
    gEntries[key]     = entry;

    // Post initial focused-window notification
    CFTypeRef focusedRef = NULL;
    AXUIElementCopyAttributeValue(appElement, kAXFocusedWindowAttribute, &focusedRef);
    if (focusedRef) {
        axCallback(observer, focusedRef, kAXFocusedWindowChangedNotification, (void*)(intptr_t)pid);
        CFRelease(focusedRef);
    }
}

static void axRemoveBlock(int pid) {
    NSNumber *key = @(pid);
    AXEntry *entry = gEntries[key];
    if (!entry) return;

    CFRunLoopRef rl = GetRunLoop();
    if (rl) {
        CFRunLoopRemoveSource(rl,
                              AXObserverGetRunLoopSource(entry.observer),
                              kCFRunLoopDefaultMode);
    }
    CFRelease(entry.observer);
    CFRelease(entry.appElement);
    [gEntries removeObjectForKey:key];
}

bool AXInstallObserver(int pid) {
    if (!gAxQueue) gAxQueue = dispatch_queue_create("mimi.ax", DISPATCH_QUEUE_SERIAL);
    __block bool ok = false;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    CFRunLoopRef rl = GetRunLoop();
    if (!rl) return false;

    CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
        axInstallBlock(pid);
        ok = true;
        dispatch_semaphore_signal(sem);
    });
    CFRunLoopWakeUp(rl);
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    return ok;
}

void AXRemoveObserver(int pid) {
    CFRunLoopRef rl = GetRunLoop();
    if (!rl) return;

    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    CFRunLoopPerformBlock(rl, kCFRunLoopDefaultMode, ^{
        axRemoveBlock(pid);
        dispatch_semaphore_signal(sem);
    });
    CFRunLoopWakeUp(rl);
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
}

void AXRemoveAllObservers(void) {
    if (!gEntries) return;
    for (NSNumber *key in [gEntries allKeys]) {
        AXRemoveObserver([key intValue]);
    }
}
