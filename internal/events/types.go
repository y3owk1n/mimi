package events

import "time"

// EventKind classifies an event.
type EventKind string

// Hookable event kinds — exposed to users via config and hooks.
const (
	// WindowFocus fires when the focused window changes.
	WindowFocus EventKind = "window_focus"
	// WindowTitleChange fires when the active window title changes.
	WindowTitleChange EventKind = "window_title_change"
	// WindowCreated fires when a new window opens.
	WindowCreated EventKind = "window_created"
	// WindowClosed fires when a window closes.
	WindowClosed EventKind = "window_closed"
	// WindowResizing fires when a window is being resized (raw, internal).
	WindowResizing EventKind = "_window_resizing"
	// WindowResize fires when a window resize is completed (debounced).
	WindowResize EventKind = "window_resize"

	// WorkspaceChanged fires when the active macOS Space/Desktop changes.
	WorkspaceChanged EventKind = "workspace_changed"
)

// Internal event kinds used by observers for AX lifecycle management.
// These are not hookable and do not appear in AllKinds.
const (
	AppActivate EventKind = "app_activate"
	AppLaunch   EventKind = "app_launch"
	AppQuit     EventKind = "app_quit"
)

// AllKinds lists every hookable event kind.
var AllKinds = []EventKind{
	WindowFocus,
	WindowTitleChange,
	WindowCreated,
	WindowClosed,
	WindowResize,
	WorkspaceChanged,
}

// Event carries information about a system event through the bus.
type Event struct {
	ID          string            `json:"id"`
	Kind        EventKind         `json:"kind"`
	AppName     string            `json:"appName,omitempty"`
	BundleID    string            `json:"bundleId,omitempty"`
	PID         int               `json:"pid,omitempty"`
	WindowTitle string            `json:"windowTitle,omitempty"`
	At          time.Time         `json:"at"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// Publisher is the interface for publishing events to the bus.
type Publisher interface {
	Publish(e Event)
}
