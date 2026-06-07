#pragma once
#include <CoreFoundation/CoreFoundation.h>

// Must be called from the main thread once at startup, before any other
// Cocoa calls. Initialises NSApplication so the process can receive
// NSWorkspace notifications.
void InitCocoaApp(void);

void InitBridgeRunLoop(void);

void WorkspaceObserverStart(int appLifecycle, int systemState, int volume, int workspace, int appearance);

void WorkspaceObserverUpdate(int appLifecycle, int systemState, int volume, int workspace, int appearance);

void WorkspaceObserverStop(void);

CFRunLoopRef GetRunLoop(void);
