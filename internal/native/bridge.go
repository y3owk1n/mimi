package native

/*
#include "workspace.h"
#include "axobserver.h"
#include "eventkinds.h"
*/
import "C"

import (
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/y3owk1n/mimi/internal/events"
)

const eventChBufSize = 4096

var (
	eventCh      = make(chan events.Event, eventChBufSize)
	eventDropped atomic.Int64
)

// Events returns a read-only channel of events from native observers.
func Events() <-chan events.Event { return eventCh }

// EventDropCount returns the number of events dropped due to channel congestion.
func EventDropCount() int64 { return eventDropped.Load() }

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

func trySend(evt events.Event) {
	select {
	case eventCh <- evt:
	default:
		eventDropped.Add(1)
	}
}

//export goWorkspaceEvent
func goWorkspaceEvent(kind C.int, appName, bundleID *C.char, pid C.int,
	volPath, volName *C.char,
) {
	_, _ = volPath, volName
	trySend(events.Event{
		ID:       uuid.NewString(),
		Kind:     kindFromInt(int(kind)),
		AppName:  C.GoString(appName),
		BundleID: C.GoString(bundleID),
		PID:      int(pid),
		At:       time.Now(),
	})
}

//export goWorkspaceChangeEvent
func goWorkspaceChangeEvent(kind C.int, windowCount C.int, infoJSON *C.char) {
	info := ""
	if infoJSON != nil {
		info = C.GoString(infoJSON)
	}

	trySend(workspaceChangeEvent(kindFromInt(int(kind)), int(windowCount), info, activeSpace))
}

// activeSpace is the read a workspace change carries to its hooks: the 1-based
// index of the space now in front and how many spaces there are. ok is false
// when Mission Control could not be enumerated, in which case the event says
// nothing about the space rather than naming a wrong one.
func activeSpace() (int, int, bool) {
	index, err := ActiveSpaceIndex()
	if err != nil {
		return 0, 0, false
	}

	return index, SpaceCount(), true
}

// workspaceChangeEvent builds the event one space change publishes. space is
// the read behind mimi_SPACE_INDEX and mimi_SPACE_COUNT, passed in so the
// shape of the event can be pinned without a desktop under it.
func workspaceChangeEvent(
	kind events.EventKind,
	windowCount int,
	infoJSON string,
	space func() (int, int, bool),
) events.Event {
	evt := events.Event{
		ID:   uuid.NewString(),
		Kind: kind,
		At:   time.Now(),
		Extra: map[string]string{
			"windows_count": strconv.Itoa(windowCount),
		},
	}

	if infoJSON != "" {
		evt.Extra["info"] = infoJSON
	}

	if index, count, ok := space(); ok {
		evt.Extra["space_index"] = strconv.Itoa(index)
		evt.Extra["space_count"] = strconv.Itoa(count)
	}

	return evt
}

//export goAXEvent
func goAXEvent(
	kind C.int,
	appName, bundleID *C.char,
	pid C.int,
	windowTitle *C.char,
	windowID C.ulonglong,
) {
	trySend(events.Event{
		ID:          uuid.NewString(),
		Kind:        kindFromInt(int(kind)),
		AppName:     C.GoString(appName),
		BundleID:    C.GoString(bundleID),
		PID:         int(pid),
		WindowTitle: C.GoString(windowTitle),
		WindowID:    uint64(windowID),
		At:          time.Now(),
	})
}

func kindFromInt(kindInt int) events.EventKind {
	switch kindInt {
	case int(C.MIMI_KIND_APP_ACTIVATE):
		return events.AppActivate
	case int(C.MIMI_KIND_APP_DEACTIVATE):
		return events.AppDeactivate
	case int(C.MIMI_KIND_APP_LAUNCH):
		return events.AppLaunch
	case int(C.MIMI_KIND_APP_QUIT):
		return events.AppQuit
	case int(C.MIMI_KIND_APP_HIDE):
		return events.AppHide
	case int(C.MIMI_KIND_APP_UNHIDE):
		return events.AppUnhide
	case int(C.MIMI_KIND_WINDOW_FOCUS):
		return events.WindowFocus
	case int(C.MIMI_KIND_WINDOW_TITLE_CHANGE):
		return events.WindowTitleChange
	case int(C.MIMI_KIND_WINDOW_CREATED):
		return events.WindowCreated
	case int(C.MIMI_KIND_WINDOW_CLOSED):
		return events.WindowClosed
	case int(C.MIMI_KIND_WINDOW_RESIZING):
		return events.WindowResizing
	case int(C.MIMI_KIND_WORKSPACE_CHANGED):
		return events.WorkspaceChanged
	default:
		return events.EventKind("unknown")
	}
}

func boolToInt(b bool) C.int {
	if b {
		return 1
	}

	return 0
}
