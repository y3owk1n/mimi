package native

/*
#include "workspace.h"
#include "axobserver.h"
*/
import "C"

import (
	"runtime"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/y3owk1n/mimi/internal/events"
)

const eventChBufSize = 256

var eventCh = make(chan events.Event, eventChBufSize)

// Events returns a read-only channel of events from native observers.
func Events() <-chan events.Event { return eventCh }

// ObserverConfig specifies which macOS event observers are active.
type ObserverConfig struct {
	AppLifecycle bool
	Workspace    bool
}

// StartObservers initializes and starts configured macOS event observers.
func StartObservers(obsCfg ObserverConfig, beforeRunLoop func() bool) bool {
	mainThread := make(chan bool)
	go func() {
		runtime.LockOSThread()
		C.InitCocoaApp()

		if beforeRunLoop != nil && !beforeRunLoop() {
			mainThread <- false

			return
		}

		C.InitBridgeRunLoop()

		mainThread <- true
		C.WorkspaceObserverStart(
			boolToInt(obsCfg.AppLifecycle),
			0,
			0,
			boolToInt(obsCfg.Workspace),
			0,
		)
	}()
	if !<-mainThread {
		return false
	}

	eventCh <- events.Event{
		ID:      uuid.NewString(),
		Kind:    events.EventKind("_startup_"),
		AppName: "mimi",
		At:      time.Now(),
	}

	return true
}

// UpdateObservers dynamically starts or stops workspace observers based on config.
func UpdateObservers(obsCfg ObserverConfig) {
	C.WorkspaceObserverUpdate(
		boolToInt(obsCfg.AppLifecycle),
		0,
		0,
		boolToInt(obsCfg.Workspace),
		0,
	)
}

// StopObservers stops all macOS event observers.
func StopObservers() {
	C.AXRemoveAllObservers()
	C.WorkspaceObserverStop()
}

// InstallAXObserver installs an AX observer for the given PID.
func InstallAXObserver(pid int) bool {
	return bool(C.AXInstallObserver(C.int(pid)))
}

// RemoveAXObserver removes the AX observer for the given PID.
func RemoveAXObserver(pid int) {
	C.AXRemoveObserver(C.int(pid))
}

//export goWorkspaceEvent
func goWorkspaceEvent(kind C.int, appName, bundleID *C.char, pid C.int,
	volPath, volName *C.char,
) {
	_, _ = volPath, volName
	evt := events.Event{
		ID:       uuid.NewString(),
		Kind:     kindFromInt(int(kind)),
		AppName:  C.GoString(appName),
		BundleID: C.GoString(bundleID),
		PID:      int(pid),
		At:       time.Now(),
	}
	select {
	case eventCh <- evt:
	default:
	}
}

//export goWorkspaceChangeEvent
func goWorkspaceChangeEvent(kind C.int, windowCount C.int, infoJSON *C.char) {
	evt := events.Event{
		ID:   uuid.NewString(),
		Kind: kindFromInt(int(kind)),
		At:   time.Now(),
		Extra: map[string]string{
			"windows_count": strconv.Itoa(int(windowCount)),
		},
	}
	if infoJSON != nil {
		jsonStr := C.GoString(infoJSON)
		if jsonStr != "" {
			evt.Extra["info"] = jsonStr
		}
	}
	select {
	case eventCh <- evt:
	default:
	}
}

//export goAXEvent
func goAXEvent(kind C.int, appName, bundleID *C.char, pid C.int, windowTitle *C.char) {
	evt := events.Event{
		ID:          uuid.NewString(),
		Kind:        kindFromInt(int(kind)),
		AppName:     C.GoString(appName),
		BundleID:    C.GoString(bundleID),
		PID:         int(pid),
		WindowTitle: C.GoString(windowTitle),
		At:          time.Now(),
	}
	select {
	case eventCh <- evt:
	default:
	}
}

func kindFromInt(kindInt int) events.EventKind {
	kindMap := map[int]events.EventKind{
		0:  events.AppActivate,
		2:  events.AppLaunch,
		3:  events.AppQuit,
		30: events.WindowFocus,
		31: events.WindowTitleChange,
		32: events.WindowCreated,
		33: events.WindowClosed,
		34: events.WindowResizing,
		70: events.WorkspaceChanged,
	}
	if k, ok := kindMap[kindInt]; ok {
		return k
	}

	return events.EventKind("unknown")
}

func boolToInt(b bool) C.int {
	if b {
		return 1
	}

	return 0
}
