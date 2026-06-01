package events

import "time"

// EventKind classifies an event (e.g. app_activate, workspace_changed).
type EventKind string

const (
	// AppActivate fires when an app becomes the frontmost.
	AppActivate EventKind = "app_activate"
	// AppDeactivate fires when an app loses focus.
	AppDeactivate EventKind = "app_deactivate"
	// AppLaunch fires when a new app process starts.
	AppLaunch EventKind = "app_launch"
	// AppQuit fires when an app process terminates.
	AppQuit EventKind = "app_quit"
	// AppHide fires when an app is hidden (⌘H).
	AppHide EventKind = "app_hide"
	// AppUnhide fires when a hidden app is shown again.
	AppUnhide EventKind = "app_unhide"

	// WindowFocus fires when the focused window changes.
	WindowFocus EventKind = "window_focus"
	// WindowTitleChange fires when the active window title changes.
	WindowTitleChange EventKind = "window_title_change"
	// WindowCreated fires when a new window opens.
	WindowCreated EventKind = "window_created"
	// WindowClosed fires when a window closes.
	WindowClosed EventKind = "window_closed"

	// SystemSleep fires when the system or display goes to sleep.
	SystemSleep EventKind = "system_sleep"
	// SystemWake fires when the system wakes.
	SystemWake EventKind = "system_wake"
	// ScreenLock fires when the screen is locked.
	ScreenLock EventKind = "screen_lock"
	// ScreenUnlock fires when the screen is unlocked.
	ScreenUnlock EventKind = "screen_unlock"
	// SystemShutdown fires when a shutdown or restart is imminent.
	SystemShutdown EventKind = "system_shutdown"
	// UserSessionEnd fires when the user session ends (logout).
	UserSessionEnd EventKind = "user_session_end"

	// VolumeMount fires when a volume or USB drive mounts.
	VolumeMount EventKind = "volume_mount"
	// VolumeUnmount fires when a volume or USB drive unmounts.
	VolumeUnmount EventKind = "volume_unmount"

	// ExternalDisplayConnected fires when an external display is connected.
	ExternalDisplayConnected EventKind = "external_display_connected"
	// ExternalDisplayDisconnected fires when an external display is disconnected.
	ExternalDisplayDisconnected EventKind = "external_display_disconnected"
	// AppearanceChanged fires when the system appearance changes (dark/light mode).
	AppearanceChanged EventKind = "appearance_changed"

	// PowerAdapterConnected fires when AC power is plugged in.
	PowerAdapterConnected EventKind = "power_adapter_connected"
	// PowerAdapterDisconnected fires when AC power is unplugged.
	PowerAdapterDisconnected EventKind = "power_adapter_disconnected"
	// BatteryLow fires when the battery level drops to low.
	BatteryLow EventKind = "battery_low"
	// BatteryCritical fires when the battery is critically low.
	BatteryCritical EventKind = "battery_critical"

	// AudioDeviceChanged fires when the audio device list or default changes.
	AudioDeviceChanged EventKind = "audio_device_changed"

	// WorkspaceChanged fires when the active macOS Space/Desktop changes.
	WorkspaceChanged EventKind = "workspace_changed"

	// USBDeviceConnected fires when a USB device is connected.
	USBDeviceConnected EventKind = "usb_device_connected"
	// USBDeviceDisconnected fires when a USB device is disconnected.
	USBDeviceDisconnected EventKind = "usb_device_disconnected"

	// NetworkUp fires when network connectivity is restored.
	NetworkUp EventKind = "network_up"
	// NetworkDown fires when network connectivity is lost.
	NetworkDown EventKind = "network_down"

	// ClipboardChanged fires when the clipboard content changes.
	ClipboardChanged EventKind = "clipboard_changed"
)

// AllKinds lists every known event kind.
var AllKinds = []EventKind{
	AppActivate, AppDeactivate, AppLaunch, AppQuit, AppHide, AppUnhide,
	WindowFocus, WindowTitleChange, WindowCreated, WindowClosed,
	SystemSleep, SystemWake, ScreenLock, ScreenUnlock, SystemShutdown, UserSessionEnd,
	VolumeMount, VolumeUnmount,
	ExternalDisplayConnected, ExternalDisplayDisconnected, AppearanceChanged,
	PowerAdapterConnected, PowerAdapterDisconnected, BatteryLow, BatteryCritical,
	AudioDeviceChanged,
	WorkspaceChanged,
	USBDeviceConnected, USBDeviceDisconnected,
	NetworkUp, NetworkDown,
	ClipboardChanged,
}

// Event carries information about a system event through the bus.
type Event struct {
	ID          string            `json:"id"`
	Kind        EventKind         `json:"kind"`
	AppName     string            `json:"appName,omitempty"`
	BundleID    string            `json:"bundleId,omitempty"`
	PID         int               `json:"pid,omitempty"`
	WindowTitle string            `json:"windowTitle,omitempty"`
	VolumePath  string            `json:"volumePath,omitempty"`
	VolumeName  string            `json:"volumeName,omitempty"`
	At          time.Time         `json:"at"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// Publisher is the interface for publishing events to the bus.
type Publisher interface {
	Publish(e Event)
}
