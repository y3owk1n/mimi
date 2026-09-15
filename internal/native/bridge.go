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
	"sync"
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
	// SystemState is sleep, wake, the display set changing, and the screen
	// locking and unlocking.
	SystemState bool
	// Appearance is the system switching between light and dark mode.
	Appearance bool
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
			boolToInt(obsCfg.SystemState),
			0,
			boolToInt(obsCfg.Workspace),
			boolToInt(obsCfg.Appearance),
		)
	}()
	if !<-mainThread {
		return false
	}

	eventCh <- events.Event{
		ID:      uuid.NewString(),
		Kind:    events.Startup,
		AppName: "mimi",
		At:      time.Now(),
	}

	return true
}

// UpdateObservers dynamically starts or stops workspace observers based on config.
func UpdateObservers(obsCfg ObserverConfig) {
	C.WorkspaceObserverUpdate(
		boolToInt(obsCfg.AppLifecycle),
		boolToInt(obsCfg.SystemState),
		0,
		boolToInt(obsCfg.Workspace),
		boolToInt(obsCfg.Appearance),
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

//export goAppearanceEvent
func goAppearanceEvent(dark C.int) {
	trySend(appearanceEvent(dark != 0))
}

// appearanceEvent is the event one light or dark mode switch publishes,
// carrying the mode now in effect as mimi_APPEARANCE.
func appearanceEvent(dark bool) events.Event {
	appearance := "light"
	if dark {
		appearance = "dark"
	}

	return events.Event{
		ID:    uuid.NewString(),
		Kind:  events.AppearanceChanged,
		At:    time.Now(),
		Extra: map[string]string{"appearance": appearance},
	}
}

//export goWorkspaceChangeEvent
func goWorkspaceChangeEvent(kind C.int, windowCount C.int, infoJSON *C.char) {
	info := ""
	if infoJSON != nil {
		info = C.GoString(infoJSON)
	}

	evt := workspaceChangeEvent(kindFromInt(int(kind)), int(windowCount), info, activeSpace)

	if did := changedDisplay(); did != 0 {
		evt.Extra[DisplayIDKey] = strconv.FormatUint(uint64(did), 10)
	}

	trySend(evt)
}

// The extras the native layer puts on an event for the daemon to turn into
// the display index a user counts before a hook sees them. A window event
// carries its window's center, in window coordinates as "x,y". The daemon
// resolves it to the display holding the center, or the nearest one when
// the center is off every display. A space change carries the display
// whose space changed, by the identifier macOS uses.
const (
	WindowCenterKey = "window_center"
	DisplayIDKey    = "display_id"
)

// lastActive is the space in front on each display at the last space
// change, so the next change can say which display it was on.
//
//nolint:gochecknoglobals // one desktop per process
var (
	lastActive   = map[uint32]uint64{}
	lastActiveMu sync.Mutex
)

// changedDisplay is the display whose space in front changed since the
// last space change, or the display under the cursor when none did or this
// is the first change seen. 0 when the displays cannot be read.
func changedDisplay() uint32 {
	displays, err := Displays()
	if err != nil {
		return 0
	}

	ids := make([]uint32, len(displays))
	for index, display := range displays {
		ids[index] = display.ID
	}

	active := ActiveSpaceIDs(ids)

	lastActiveMu.Lock()
	defer lastActiveMu.Unlock()

	changed := uint32(0)

	for did, sid := range active {
		if was, seen := lastActive[did]; seen && was != sid {
			changed = did

			break
		}
	}

	lastActive = active

	if changed == 0 {
		return CursorDisplayID()
	}

	return changed
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
	hasCenter C.int,
	centerX, centerY C.double,
) {
	evt := events.Event{
		ID:          uuid.NewString(),
		Kind:        kindFromInt(int(kind)),
		AppName:     C.GoString(appName),
		BundleID:    C.GoString(bundleID),
		PID:         int(pid),
		WindowTitle: C.GoString(windowTitle),
		WindowID:    uint64(windowID),
		At:          time.Now(),
	}

	if hasCenter != 0 {
		evt.Extra = map[string]string{
			WindowCenterKey: strconv.FormatFloat(float64(centerX), 'f', -1, 64) + "," +
				strconv.FormatFloat(float64(centerY), 'f', -1, 64),
		}
	}

	trySend(evt)
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
	case int(C.MIMI_KIND_WINDOW_MOVING):
		return events.WindowMoving
	case int(C.MIMI_KIND_WINDOW_MINIMIZE):
		return events.WindowMinimize
	case int(C.MIMI_KIND_WINDOW_UNMINIMIZE):
		return events.WindowUnminimize
	case int(C.MIMI_KIND_WORKSPACE_CHANGED):
		return events.WorkspaceChanged
	case int(C.MIMI_KIND_WILL_SLEEP):
		return events.SystemSleep
	case int(C.MIMI_KIND_DID_WAKE):
		return events.SystemWake
	case int(C.MIMI_KIND_DISPLAY_CHANGED):
		return events.DisplayChanged
	case int(C.MIMI_KIND_APPEARANCE_CHANGED):
		return events.AppearanceChanged
	case int(C.MIMI_KIND_SCREEN_LOCKED):
		return events.ScreenLocked
	case int(C.MIMI_KIND_SCREEN_UNLOCKED):
		return events.ScreenUnlocked
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
