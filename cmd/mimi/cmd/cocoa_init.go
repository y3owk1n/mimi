package cmd

import (
	"sync"

	"github.com/y3owk1n/mimi/internal/space"
)

// cocoaInitOnce ensures we only call [NSApplication
// sharedApplication] once per process. Action subcommands need
// an initialized NSApplication to use AppKit helpers such as
// NSRunningApplication and NSWorkspace from the CGo bridge.
var cocoaInitOnce sync.Once

// ensureCocoaInitialized brings up NSApplication on the calling
// thread. Safe to call from multiple goroutines; only the first
// call does real work. main() already locks the main OS thread,
// which is the thread NSApplication expects.
func ensureCocoaInitialized() {
	cocoaInitOnce.Do(space.InitCocoa)
}
