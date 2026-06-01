package cgo_bridge

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices -framework IOKit -framework CoreAudio -framework SystemConfiguration

#include "workspace.h"
#include "axobserver.h"
#include "system_events.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"time"

	"github.com/google/uuid"

	"github.com/y3owk1n/mimi/internal/events"
)

var eventCh = make(chan events.Event, 256)

func EventCh() <-chan events.Event { return eventCh }

func Start() {
	// Lock to main OS thread to initialize NSApplication properly.
	// NSWorkspace notifications require proper Cocoa initialization.
	mainThread := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		C.InitCocoaApp()
		close(mainThread)
		C.WorkspaceObserverStart()
	}()
	<-mainThread

	// Start system observers
	C.PowerObserverStart()
	C.AudioObserverStart()
	C.ClipboardObserverStart()
	C.USBObserverStart()
	C.NetworkObserverStart()
	C.DisplayObserverStart()

	// Push a synthetic startup event as a proof-of-life for the pipeline.
	// Its kind ("_startup_") won't match any user hook.
	eventCh <- events.Event{
		ID:      uuid.NewString(),
		Kind:    events.EventKind("_startup_"),
		AppName: "mimi",
		At:      time.Now(),
	}
}

func Stop() {
	C.WorkspaceObserverStop()
	C.AXRemoveAllObservers()
	C.PowerObserverStop()
	C.AudioObserverStop()
	C.ClipboardObserverStop()
	C.USBObserverStop()
	C.NetworkObserverStop()
	C.DisplayObserverStop()
}

func InstallAXObserver(pid int) bool {
	return bool(C.AXInstallObserver(C.int(pid)))
}

func RemoveAXObserver(pid int) {
	C.AXRemoveObserver(C.int(pid))
}

//export goWorkspaceEvent
func goWorkspaceEvent(kind C.int, appName, bundleID *C.char, pid C.int,
	volPath, volName *C.char,
) {
	e := events.Event{
		ID:         uuid.NewString(),
		Kind:       kindFromInt(int(kind)),
		AppName:    C.GoString(appName),
		BundleID:   C.GoString(bundleID),
		PID:        int(pid),
		VolumePath: C.GoString(volPath),
		VolumeName: C.GoString(volName),
		At:         time.Now(),
	}
	select {
	case eventCh <- e:
	default:
	}
}

//export goWorkspaceChangeEvent
func goWorkspaceChangeEvent(kind C.int, windowCount C.int, infoJSON *C.char) {
	e := events.Event{
		ID:   uuid.NewString(),
		Kind: kindFromInt(int(kind)),
		At:   time.Now(),
		Extra: map[string]string{
			"windows_count": fmt.Sprintf("%d", int(windowCount)),
		},
	}
	if infoJSON != nil {
		jsonStr := C.GoString(infoJSON)
		if jsonStr != "" {
			e.Extra["info"] = jsonStr
		}
	}
	select {
	case eventCh <- e:
	default:
	}
}

//export goAXEvent
func goAXEvent(kind C.int, appName, bundleID *C.char, pid C.int, windowTitle *C.char) {
	e := events.Event{
		ID:          uuid.NewString(),
		Kind:        kindFromInt(int(kind)),
		AppName:     C.GoString(appName),
		BundleID:    C.GoString(bundleID),
		PID:         int(pid),
		WindowTitle: C.GoString(windowTitle),
		At:          time.Now(),
	}
	select {
	case eventCh <- e:
	default:
	}
}

//export goSystemEvent
func goSystemEvent(kind C.int) {
	e := events.Event{
		ID:   uuid.NewString(),
		Kind: kindFromInt(int(kind)),
		At:   time.Now(),
	}
	select {
	case eventCh <- e:
	default:
	}
}

func kindFromInt(n int) events.EventKind {
	m := map[int]events.EventKind{
		// App lifecycle
		0: events.AppActivate, 1: events.AppDeactivate,
		2: events.AppLaunch, 3: events.AppQuit,
		4: events.AppHide, 5: events.AppUnhide,
		// System events
		10: events.SystemSleep, 11: events.SystemWake,
		12: events.ScreenLock, 13: events.ScreenUnlock,
		14: events.SystemShutdown,
		// Storage events
		20: events.VolumeMount, 21: events.VolumeUnmount,
		// Window events
		30: events.WindowFocus, 31: events.WindowTitleChange,
		32: events.WindowCreated, 33: events.WindowClosed,
		// Display/Appearance events
		40: events.ExternalDisplayConnected, 41: events.ExternalDisplayDisconnected,
		42: events.AppearanceChanged,
		// Power/Battery events
		50: events.PowerAdapterConnected, 51: events.PowerAdapterDisconnected,
		52: events.BatteryLow, 53: events.BatteryCritical,
		// Audio events
		60: events.AudioDeviceChanged,
		// Workspace events
		70: events.WorkspaceChanged,
		// USB/Peripheral events
		80: events.USBDeviceConnected, 81: events.USBDeviceDisconnected,
		// Network events
		90: events.NetworkUp, 91: events.NetworkDown,
		// Clipboard events
		100: events.ClipboardChanged,
	}
	if k, ok := m[n]; ok {
		return k
	}
	return events.EventKind("unknown")
}
