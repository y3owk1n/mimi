# Objective-C guidelines

## File organization

### CGO and Go files

Native implementations belong in `.m` / `.h` files, in three packages only:

- `internal/native/`: window and space actions, hook daemon observers, borders, drop zone, and stack bar drawing (SkyLight, Accessibility, NSWorkspace)
- `internal/systray/`: menu bar UI
- `internal/permissions/`: Accessibility permission prompts

The CGO preamble in a Go file holds only `#cgo` flags and `#include` lines. Every package compiles Objective-C with `-x objective-c -fobjc-arc`.

A `.m` file reaches a Go `//export` callback either by including `_cgo_export.h` (as `workspace.m` and `axobserver.m` do) or by declaring it `extern` under a `#pragma mark - External Function Declarations` section (as `systray.m` does).

A bridge `.m` file must `#import` the header that declares its functions (`mimi.h` is shared by several) and must not re-declare structs or typedefs already defined in that header. CGO includes the same header, so a duplicate definition causes `conflicting types` errors.

### Header files (.h)

- Guard against double inclusion with `#pragma once` or an `#ifndef` guard
- Keep the public interface minimal
- Use `@class` forward declarations when possible
- Group related declarations with `#pragma mark`

```objc
#pragma once
#include <CoreFoundation/CoreFoundation.h>

void InitCocoaApp(void);

void WorkspaceObserverStart(int appLifecycle, int systemState, int volume, int workspace, int appearance);

void WorkspaceObserverStop(void);
```

### Implementation files (.m)

Standard structure:

1. Imports, own header first
2. `#pragma mark` sections
3. Private interface declarations
4. Implementation
5. C interface functions

```objc
#import "workspace.h"

#include "_cgo_export.h"
#import "mimi_log.h"

#import <Cocoa/Cocoa.h>

static WorkspaceObserver *gObserver = nil;

void InitCocoaApp(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}
```

## Naming conventions

### C bridge exports

Functions declared in `.h` files and called from Go use a prefix naming their subsystem (e.g., `Workspace`, `AX`):

```objc
void InitCocoaApp(void);
void WorkspaceObserverStart(int appLifecycle, int systemState, int volume, int workspace, int appearance);
void WorkspaceObserverStop(void);
CFRunLoopRef GetRunLoop(void);
bool AXInstallObserver(int pid);
void AXRemoveObserver(int pid);
```

### File-local symbols

- Static variables use a `g` prefix: `gObserver`, `gRunLoop`
- Static functions use a `mimi` prefix: `mimiWindowIsReal`
- Static constants use a `kMimi` prefix: `kMimiRaiseAttempts`

### Objective-C methods

- Follow Apple's naming conventions
- Start with a lowercase letter and use camelCase

```objc
- (int)kindForNotificationName:(NSString *)name;
- (NSArray *)currentWindowList;
```

## Property attributes

- `strong` for object ownership
- `weak` for delegates and to avoid retain cycles
- `assign` for primitive types
- `copy` for NSString and blocks

```objc
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, weak) id<NSWorkspaceDelegate> delegate;
@property(nonatomic, assign) NSInteger eventCount;
```

## Memory management

### ARC

mimi compiles Objective-C with Automatic Reference Counting (`-fobjc-arc`). The compiler inserts `retain` and `release`.

### Core Foundation objects

Cross between Core Foundation and Objective-C objects with `__bridge` casts or `CFBridgingRelease`:

```objc
CFArrayRef windowList = CGWindowListCopyWindowInfo(
    kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
if (!windowList) {
	return nil;
}

return CFBridgingRelease(windowList);
```

## Comments

Use `///` comments for public API:

```objc
/// Initialise the Cocoa application with background-only activation policy.
void InitCocoaApp(void);

/// Start observing NSWorkspace notifications.
void WorkspaceObserverStart(int appLifecycle, int systemState, int volume, int workspace, int appearance);
```

Use inline comments for non-obvious logic:

```objc
// NSWorkspaceActiveSpaceDidChangeNotification is the deterministic
// source for active Space/Desktop changes. A previous
// implementation also polled CGWindowListCopyWindowInfo every 2s
// and diffed the result, but that fired on any ephemeral change
// to the on-screen window set.
```

## Code organization

Use `#pragma mark` to divide a file into sections:

```objc
#pragma mark - SkyLight External Declarations

#pragma mark - Helpers

#pragma mark - C Interface
```

## Threading

Cocoa and UI code must run on the main thread:

```objc
if ([NSThread isMainThread]) {
	mimiFollow(number);
	return;
}
dispatch_async(dispatch_get_main_queue(), ^{
	mimiFollow(number);
});
```

## See also

- [CONVENTIONS.md](./CONVENTIONS.md)
- [TESTING_PATTERNS.md](../testing/TESTING_PATTERNS.md)
