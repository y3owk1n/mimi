# Objective-C Guidelines

## File Organization

### CGO and Go Files

Native bridge implementations belong in `.m` / `.h` files under `internal/observers/cgo_bridge/`, not in Go CGO comment blocks.

Go files in `internal/observers/cgo_bridge/` use a minimal CGO preamble (`#include` headers, `#cgo` flags, and `extern` declarations for `//export` callbacks only).

Bridge `.m` files must `#include` their matching header and must **not** re-declare structs or typedefs already defined in that header (duplicate definitions cause `conflicting types` errors when CGO includes the same header).

### Header Files (.h)

- Minimal public interface
- Use `@class` forward declarations when possible
- Group related declarations with `#pragma mark`

```objc
#import <Foundation/Foundation.h>

void InitCocoaApp(void);
void WorkspaceObserverStart(void);
void WorkspaceObserverStop(void);
```

### Implementation Files (.m)

Standard structure:

1. Imports
2. `#pragma mark` sections
3. Interface declarations (private)
4. Implementation
5. C interface functions

```objc
#import "workspace.h"
#import <Cocoa/Cocoa.h>

#pragma mark - Workspace Observer

static id s_workspaceObserver = nil;

#pragma mark - C Interface

void InitCocoaApp(void) {
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

void WorkspaceObserverStart(void) {
    // Implementation
}
```

## Naming Conventions

### C Bridge Exports

Functions declared in `.h` files and called from Go via CGO use a descriptive prefix related to their observer (e.g., `Workspace`, `AX`, `Power`):

```objc
void InitCocoaApp(void);
void WorkspaceObserverStart(void);
void WorkspaceObserverStop(void);
CFRunLoopRef GetRunLoop(void);
bool AXInstallObserver(int pid);
void AXRemoveObserver(int pid);
```

### Objective-C Methods

- Use descriptive names with clear intent
- Follow Apple's naming conventions
- Start with lowercase letter, use camelCase

```objc
- (void)startObserving;
- (void)stopObserving;
```

## Property Attributes

- `strong` for object ownership
- `weak` for delegates and to avoid retain cycles
- `assign` for primitive types
- `copy` for NSString and blocks

```objc
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, weak) id<NSWorkspaceDelegate> delegate;
@property(nonatomic, assign) NSInteger eventCount;
```

## Memory Management

### ARC

mimi uses Automatic Reference Counting (ARC) for Objective-C code. The compiler handles `retain`/`release` automatically.

### C Interface Objects

For objects passed across the C/Go boundary, use toll-free bridging or `__bridge` casts:

```objc
CFRunLoopRef GetRunLoop(void) {
    return (__bridge CFRunLoopRef)[NSRunLoop mainRunLoop];
}
```

## Comments

Use HeaderDoc-style comments for public API:

```objc
/// Initialise the Cocoa application with background-only activation policy.
void InitCocoaApp(void);

/// Start observing NSWorkspace notifications.
void WorkspaceObserverStart(void);
```

Inline comments for non-obvious logic:

```objc
// Polling is used because NSWorkspaceActiveSpaceDidChangeNotification
// is not delivered to NSApplicationActivationPolicyAccessory processes.
```

## Code Organization

Use `#pragma mark` to organize code:

```objc
#pragma mark - Workspace Observer

#pragma mark - C Interface

#pragma mark - Power Observer
```

## Threading

All Cocoa/UI code must run on the main thread:

```objc
if ([NSThread isMainThread]) {
    [self startObserving];
} else {
    dispatch_async(dispatch_get_main_queue(), ^{
        [self startObserving];
    });
}
```

## See Also

- [CONVENTIONS.md](./CONVENTIONS.md)
- [TESTING_PATTERNS.md](../testing/TESTING_PATTERNS.md)
