#pragma once
#include <CoreFoundation/CoreFoundation.h>

// Must be called from the main thread once at startup, before any other
// Cocoa calls. Initialises NSApplication so the process can receive
// NSWorkspace notifications.
void InitCocoaApp(void);

void WorkspaceObserverStart(void);

void WorkspaceObserverStop(void);

CFRunLoopRef GetRunLoop(void);
