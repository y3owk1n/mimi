package events

import "time"

type EventKind string

const (
	// App lifecycle
	AppActivate       EventKind = "app_activate"
	AppDeactivate     EventKind = "app_deactivate"
	AppLaunch         EventKind = "app_launch"
	AppQuit           EventKind = "app_quit"
	AppHide           EventKind = "app_hide"
	AppUnhide         EventKind = "app_unhide"

	// Window events
	WindowFocus       EventKind = "window_focus"
	WindowTitleChange EventKind = "window_title_change"
	WindowCreated     EventKind = "window_created"
	WindowClosed      EventKind = "window_closed"

	// System events
	SystemSleep      EventKind = "system_sleep"
	SystemWake       EventKind = "system_wake"
	ScreenLock       EventKind = "screen_lock"
	ScreenUnlock     EventKind = "screen_unlock"
	SystemShutdown   EventKind = "system_shutdown"
	UserSessionEnd   EventKind = "user_session_end"

	// Storage events
	VolumeMount      EventKind = "volume_mount"
	VolumeUnmount    EventKind = "volume_unmount"

	// Display/Appearance events
	ExternalDisplayConnected    EventKind = "external_display_connected"
	ExternalDisplayDisconnected EventKind = "external_display_disconnected"
	AppearanceChanged           EventKind = "appearance_changed"

	// Power/Battery events
	PowerAdapterConnected    EventKind = "power_adapter_connected"
	PowerAdapterDisconnected EventKind = "power_adapter_disconnected"
	BatteryLow              EventKind = "battery_low"
	BatteryCritical         EventKind = "battery_critical"

	// Audio events
	AudioDeviceChanged EventKind = "audio_device_changed"

	// Workspace/Desktop events
	WorkspaceChanged EventKind = "workspace_changed"

	// USB/Peripheral events
	USBDeviceConnected    EventKind = "usb_device_connected"
	USBDeviceDisconnected EventKind = "usb_device_disconnected"

	// Network events
	NetworkUp   EventKind = "network_up"
	NetworkDown EventKind = "network_down"

	// Clipboard events
	ClipboardChanged EventKind = "clipboard_changed"
)

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

type Event struct {
	ID          string            `json:"id"`
	Kind        EventKind         `json:"kind"`
	AppName     string            `json:"app_name,omitempty"`
	BundleID    string            `json:"bundle_id,omitempty"`
	PID         int               `json:"pid,omitempty"`
	WindowTitle string            `json:"window_title,omitempty"`
	VolumePath  string            `json:"volume_path,omitempty"`
	VolumeName  string            `json:"volume_name,omitempty"`
	At          time.Time         `json:"at"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type Publisher interface {
	Publish(Event)
}
