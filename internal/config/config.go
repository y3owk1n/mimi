package config

import (
	"fmt"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Config holds the full mimi configuration.
type Config struct {
	Settings SettingsConfig `json:"settings" toml:"settings"`
	Hooks    HooksConfig    `json:"hooks"    toml:"hooks"`
	Systray  SystrayConfig  `json:"systray"  toml:"systray"`
}

// SettingsConfig holds the [settings] section of the config.
type SettingsConfig struct {
	LogFile         string `json:"logFile"         toml:"log_file"`
	LogLevel        string `json:"logLevel"        toml:"log_level"`
	LogFormat       string `json:"logFormat"       toml:"log_format"`
	HookTimeoutSecs int    `json:"hookTimeoutSecs" toml:"hook_timeout_secs"`
	HookShell       string `json:"hookShell"       toml:"hook_shell"`
	MaxHookWorkers  int    `json:"maxHookWorkers"  toml:"max_hook_workers"`
	PIDFile         string `json:"pidFile"         toml:"pid_file"`
}

// SystrayConfig holds the [systray] section of the config.
type SystrayConfig struct {
	Enabled bool `json:"enabled" toml:"enabled"`
}

// HooksConfig holds all hook entries grouped by event kind.
type HooksConfig struct {
	AppActivate                 []HookEntry `json:"onAppActivate"                 toml:"on_app_activate"`
	AppDeactivate               []HookEntry `json:"onAppDeactivate"               toml:"on_app_deactivate"`
	AppLaunch                   []HookEntry `json:"onAppLaunch"                   toml:"on_app_launch"`
	AppQuit                     []HookEntry `json:"onAppQuit"                     toml:"on_app_quit"`
	AppHide                     []HookEntry `json:"onAppHide"                     toml:"on_app_hide"`
	AppUnhide                   []HookEntry `json:"onAppUnhide"                   toml:"on_app_unhide"`
	WindowFocus                 []HookEntry `json:"onWindowFocus"                 toml:"on_window_focus"`
	WindowTitleChange           []HookEntry `json:"onWindowTitleChange"           toml:"on_window_title_change"`
	WindowCreated               []HookEntry `json:"onWindowCreated"               toml:"on_window_created"`
	WindowClosed                []HookEntry `json:"onWindowClosed"                toml:"on_window_closed"`
	SystemSleep                 []HookEntry `json:"onSystemSleep"                 toml:"on_system_sleep"`
	SystemWake                  []HookEntry `json:"onSystemWake"                  toml:"on_system_wake"`
	ScreenLock                  []HookEntry `json:"onScreenLock"                  toml:"on_screen_lock"`
	ScreenUnlock                []HookEntry `json:"onScreenUnlock"                toml:"on_screen_unlock"`
	SystemShutdown              []HookEntry `json:"onSystemShutdown"              toml:"on_system_shutdown"`
	UserSessionEnd              []HookEntry `json:"onUserSessionEnd"              toml:"on_user_session_end"`
	VolumeMount                 []HookEntry `json:"onVolumeMount"                 toml:"on_volume_mount"`
	VolumeUnmount               []HookEntry `json:"onVolumeUnmount"               toml:"on_volume_unmount"`
	ExternalDisplayConnected    []HookEntry `json:"onExternalDisplayConnected"    toml:"on_external_display_connected"`
	ExternalDisplayDisconnected []HookEntry `json:"onExternalDisplayDisconnected" toml:"on_external_display_disconnected"`
	AppearanceChanged           []HookEntry `json:"onAppearanceChanged"           toml:"on_appearance_changed"`
	PowerAdapterConnected       []HookEntry `json:"onPowerAdapterConnected"       toml:"on_power_adapter_connected"`
	PowerAdapterDisconnected    []HookEntry `json:"onPowerAdapterDisconnected"    toml:"on_power_adapter_disconnected"`
	BatteryLow                  []HookEntry `json:"onBatteryLow"                  toml:"on_battery_low"`
	BatteryCritical             []HookEntry `json:"onBatteryCritical"             toml:"on_battery_critical"`
	AudioDeviceChanged          []HookEntry `json:"onAudioDeviceChanged"          toml:"on_audio_device_changed"`
	WorkspaceChanged            []HookEntry `json:"onWorkspaceChanged"            toml:"on_workspace_changed"`
	USBDeviceConnected          []HookEntry `json:"onUsbDeviceConnected"          toml:"on_usb_device_connected"`
	USBDeviceDisconnected       []HookEntry `json:"onUsbDeviceDisconnected"       toml:"on_usb_device_disconnected"`
	NetworkUp                   []HookEntry `json:"onNetworkUp"                   toml:"on_network_up"`
	NetworkDown                 []HookEntry `json:"onNetworkDown"                 toml:"on_network_down"`
	ClipboardChanged            []HookEntry `json:"onClipboardChanged"            toml:"on_clipboard_changed"`
}

// HookEntry defines a single hook command and its optional filters.
type HookEntry struct {
	Run         string `json:"run"         toml:"run"`
	App         string `json:"app"         toml:"app"`
	BundleID    string `json:"bundleId"    toml:"bundle_id"`
	Title       string `json:"title"       toml:"title"`
	TimeoutSecs int    `json:"timeoutSecs" toml:"timeout_secs"`
	Async       bool   `json:"async"       toml:"async"`
}

type rawHooksConfig struct {
	AppActivate                 []any `json:"onAppActivate"                 toml:"on_app_activate"`
	AppDeactivate               []any `json:"onAppDeactivate"               toml:"on_app_deactivate"`
	AppLaunch                   []any `json:"onAppLaunch"                   toml:"on_app_launch"`
	AppQuit                     []any `json:"onAppQuit"                     toml:"on_app_quit"`
	AppHide                     []any `json:"onAppHide"                     toml:"on_app_hide"`
	AppUnhide                   []any `json:"onAppUnhide"                   toml:"on_app_unhide"`
	WindowFocus                 []any `json:"onWindowFocus"                 toml:"on_window_focus"`
	WindowTitleChange           []any `json:"onWindowTitleChange"           toml:"on_window_title_change"`
	WindowCreated               []any `json:"onWindowCreated"               toml:"on_window_created"`
	WindowClosed                []any `json:"onWindowClosed"                toml:"on_window_closed"`
	SystemSleep                 []any `json:"onSystemSleep"                 toml:"on_system_sleep"`
	SystemWake                  []any `json:"onSystemWake"                  toml:"on_system_wake"`
	ScreenLock                  []any `json:"onScreenLock"                  toml:"on_screen_lock"`
	ScreenUnlock                []any `json:"onScreenUnlock"                toml:"on_screen_unlock"`
	SystemShutdown              []any `json:"onSystemShutdown"              toml:"on_system_shutdown"`
	UserSessionEnd              []any `json:"onUserSessionEnd"              toml:"on_user_session_end"`
	VolumeMount                 []any `json:"onVolumeMount"                 toml:"on_volume_mount"`
	VolumeUnmount               []any `json:"onVolumeUnmount"               toml:"on_volume_unmount"`
	ExternalDisplayConnected    []any `json:"onExternalDisplayConnected"    toml:"on_external_display_connected"`
	ExternalDisplayDisconnected []any `json:"onExternalDisplayDisconnected" toml:"on_external_display_disconnected"`
	AppearanceChanged           []any `json:"onAppearanceChanged"           toml:"on_appearance_changed"`
	PowerAdapterConnected       []any `json:"onPowerAdapterConnected"       toml:"on_power_adapter_connected"`
	PowerAdapterDisconnected    []any `json:"onPowerAdapterDisconnected"    toml:"on_power_adapter_disconnected"`
	BatteryLow                  []any `json:"onBatteryLow"                  toml:"on_battery_low"`
	BatteryCritical             []any `json:"onBatteryCritical"             toml:"on_battery_critical"`
	AudioDeviceChanged          []any `json:"onAudioDeviceChanged"          toml:"on_audio_device_changed"`
	WorkspaceChanged            []any `json:"onWorkspaceChanged"            toml:"on_workspace_changed"`
	USBDeviceConnected          []any `json:"onUsbDeviceConnected"          toml:"on_usb_device_connected"`
	USBDeviceDisconnected       []any `json:"onUsbDeviceDisconnected"       toml:"on_usb_device_disconnected"`
	NetworkUp                   []any `json:"onNetworkUp"                   toml:"on_network_up"`
	NetworkDown                 []any `json:"onNetworkDown"                 toml:"on_network_down"`
	ClipboardChanged            []any `json:"onClipboardChanged"            toml:"on_clipboard_changed"`
}

type rawConfig struct {
	Settings SettingsConfig   `json:"settings" toml:"settings"`
	Hooks    rawHooksConfig   `json:"hooks"    toml:"hooks"`
	Systray  rawSystrayConfig `json:"systray"  toml:"systray"`
}

type rawSystrayConfig struct {
	Enabled *bool `json:"enabled" toml:"enabled"`
}

func decodeHooks(raw rawHooksConfig) (HooksConfig, error) {
	var (
		hooksCfg HooksConfig
		errs     []string
	)

	decodeField := func(field string, rawItems []any) []HookEntry {
		var entries []HookEntry
		for idx, item := range rawItems {
			switch val := item.(type) {
			case string:
				entries = append(entries, HookEntry{Run: val})
			case map[string]any:
				entry := HookEntry{
					Run:      getString(val, "run"),
					App:      getString(val, "app"),
					BundleID: getString(val, "bundle_id"),
					Title:    getString(val, "title"),
				}
				if timeout, ok := getInt(val, "timeout_secs"); ok {
					entry.TimeoutSecs = timeout
				}

				if async, ok := getBool(val, "async"); ok {
					entry.Async = async
				}

				if entry.Run == "" {
					errs = append(errs, fmt.Sprintf("%s[%d]: 'run' field is required", field, idx))
				}

				entries = append(entries, entry)
			default:
				errs = append(
					errs,
					fmt.Sprintf("%s[%d]: expected string or table, got %T", field, idx, item),
				)
			}
		}

		return entries
	}

	hooksCfg.AppActivate = decodeField("on_app_activate", raw.AppActivate)
	hooksCfg.AppDeactivate = decodeField("on_app_deactivate", raw.AppDeactivate)
	hooksCfg.AppLaunch = decodeField("on_app_launch", raw.AppLaunch)
	hooksCfg.AppQuit = decodeField("on_app_quit", raw.AppQuit)
	hooksCfg.AppHide = decodeField("on_app_hide", raw.AppHide)
	hooksCfg.AppUnhide = decodeField("on_app_unhide", raw.AppUnhide)
	hooksCfg.WindowFocus = decodeField("on_window_focus", raw.WindowFocus)
	hooksCfg.WindowTitleChange = decodeField("on_window_title_change", raw.WindowTitleChange)
	hooksCfg.WindowCreated = decodeField("on_window_created", raw.WindowCreated)
	hooksCfg.WindowClosed = decodeField("on_window_closed", raw.WindowClosed)
	hooksCfg.SystemSleep = decodeField("on_system_sleep", raw.SystemSleep)
	hooksCfg.SystemWake = decodeField("on_system_wake", raw.SystemWake)
	hooksCfg.ScreenLock = decodeField("on_screen_lock", raw.ScreenLock)
	hooksCfg.ScreenUnlock = decodeField("on_screen_unlock", raw.ScreenUnlock)
	hooksCfg.SystemShutdown = decodeField("on_system_shutdown", raw.SystemShutdown)
	hooksCfg.UserSessionEnd = decodeField("on_user_session_end", raw.UserSessionEnd)
	hooksCfg.VolumeMount = decodeField("on_volume_mount", raw.VolumeMount)
	hooksCfg.VolumeUnmount = decodeField("on_volume_unmount", raw.VolumeUnmount)
	hooksCfg.ExternalDisplayConnected = decodeField(
		"on_external_display_connected",
		raw.ExternalDisplayConnected,
	)
	hooksCfg.ExternalDisplayDisconnected = decodeField(
		"on_external_display_disconnected",
		raw.ExternalDisplayDisconnected,
	)
	hooksCfg.AppearanceChanged = decodeField("on_appearance_changed", raw.AppearanceChanged)
	hooksCfg.PowerAdapterConnected = decodeField(
		"on_power_adapter_connected",
		raw.PowerAdapterConnected,
	)
	hooksCfg.PowerAdapterDisconnected = decodeField(
		"on_power_adapter_disconnected",
		raw.PowerAdapterDisconnected,
	)
	hooksCfg.BatteryLow = decodeField("on_battery_low", raw.BatteryLow)
	hooksCfg.BatteryCritical = decodeField("on_battery_critical", raw.BatteryCritical)
	hooksCfg.AudioDeviceChanged = decodeField("on_audio_device_changed", raw.AudioDeviceChanged)
	hooksCfg.WorkspaceChanged = decodeField("on_workspace_changed", raw.WorkspaceChanged)
	hooksCfg.USBDeviceConnected = decodeField("on_usb_device_connected", raw.USBDeviceConnected)
	hooksCfg.USBDeviceDisconnected = decodeField(
		"on_usb_device_disconnected",
		raw.USBDeviceDisconnected,
	)
	hooksCfg.NetworkUp = decodeField("on_network_up", raw.NetworkUp)
	hooksCfg.NetworkDown = decodeField("on_network_down", raw.NetworkDown)
	hooksCfg.ClipboardChanged = decodeField("on_clipboard_changed", raw.ClipboardChanged)

	if len(errs) > 0 {
		return hooksCfg, derrors.Newf(
			derrors.CodeInvalidConfig,
			"hook decode errors:\n  - %s",
			strings.Join(errs, "\n  - "),
		)
	}

	return hooksCfg, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}

func getInt(m map[string]any, key string) (int, bool) {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		}
	}

	return 0, false
}

func getBool(m map[string]any, key string) (bool, bool) {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}

	return false, false
}
