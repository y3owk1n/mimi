package config

import (
	"fmt"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Config holds the full mimi configuration.
type Config struct {
	Settings SettingsConfig `toml:"settings"`
	Hooks    HooksConfig    `toml:"hooks"`
	Systray  SystrayConfig  `toml:"systray"`
}

// SettingsConfig holds the [settings] section of the config.
type SettingsConfig struct {
	LogFile         string `toml:"log_file"`
	LogLevel        string `toml:"log_level"`
	LogFormat       string `toml:"log_format"`
	HookTimeoutSecs int    `toml:"hook_timeout_secs"`
	HookShell       string `toml:"hook_shell"`
	MaxHookWorkers  int    `toml:"max_hook_workers"`
	PIDFile         string `toml:"pid_file"`
}

// SystrayConfig holds the [systray] section of the config.
type SystrayConfig struct {
	Enabled bool `toml:"enabled"`
}

// HooksConfig holds all hook entries grouped by event kind.
type HooksConfig struct {
	AppActivate                 []HookEntry `toml:"on_app_activate"`
	AppDeactivate               []HookEntry `toml:"on_app_deactivate"`
	AppLaunch                   []HookEntry `toml:"on_app_launch"`
	AppQuit                     []HookEntry `toml:"on_app_quit"`
	AppHide                     []HookEntry `toml:"on_app_hide"`
	AppUnhide                   []HookEntry `toml:"on_app_unhide"`
	WindowFocus                 []HookEntry `toml:"on_window_focus"`
	WindowTitleChange           []HookEntry `toml:"on_window_title_change"`
	WindowCreated               []HookEntry `toml:"on_window_created"`
	WindowClosed                []HookEntry `toml:"on_window_closed"`
	SystemSleep                 []HookEntry `toml:"on_system_sleep"`
	SystemWake                  []HookEntry `toml:"on_system_wake"`
	ScreenLock                  []HookEntry `toml:"on_screen_lock"`
	ScreenUnlock                []HookEntry `toml:"on_screen_unlock"`
	SystemShutdown              []HookEntry `toml:"on_system_shutdown"`
	UserSessionEnd              []HookEntry `toml:"on_user_session_end"`
	VolumeMount                 []HookEntry `toml:"on_volume_mount"`
	VolumeUnmount               []HookEntry `toml:"on_volume_unmount"`
	ExternalDisplayConnected    []HookEntry `toml:"on_external_display_connected"`
	ExternalDisplayDisconnected []HookEntry `toml:"on_external_display_disconnected"`
	AppearanceChanged           []HookEntry `toml:"on_appearance_changed"`
	PowerAdapterConnected       []HookEntry `toml:"on_power_adapter_connected"`
	PowerAdapterDisconnected    []HookEntry `toml:"on_power_adapter_disconnected"`
	BatteryLow                  []HookEntry `toml:"on_battery_low"`
	BatteryCritical             []HookEntry `toml:"on_battery_critical"`
	AudioDeviceChanged          []HookEntry `toml:"on_audio_device_changed"`
	WorkspaceChanged            []HookEntry `toml:"on_workspace_changed"`
	USBDeviceConnected          []HookEntry `toml:"on_usb_device_connected"`
	USBDeviceDisconnected       []HookEntry `toml:"on_usb_device_disconnected"`
	NetworkUp                   []HookEntry `toml:"on_network_up"`
	NetworkDown                 []HookEntry `toml:"on_network_down"`
	ClipboardChanged            []HookEntry `toml:"on_clipboard_changed"`
}

// HookEntry defines a single hook command and its optional filters.
type HookEntry struct {
	Run         string `toml:"run"`
	App         string `toml:"app"`
	BundleID    string `toml:"bundle_id"`
	Title       string `toml:"title"`
	TimeoutSecs int    `toml:"timeout_secs"`
	Async       bool   `toml:"async"`
}

type rawHooksConfig struct {
	AppActivate                 []any `toml:"on_app_activate"`
	AppDeactivate               []any `toml:"on_app_deactivate"`
	AppLaunch                   []any `toml:"on_app_launch"`
	AppQuit                     []any `toml:"on_app_quit"`
	AppHide                     []any `toml:"on_app_hide"`
	AppUnhide                   []any `toml:"on_app_unhide"`
	WindowFocus                 []any `toml:"on_window_focus"`
	WindowTitleChange           []any `toml:"on_window_title_change"`
	WindowCreated               []any `toml:"on_window_created"`
	WindowClosed                []any `toml:"on_window_closed"`
	SystemSleep                 []any `toml:"on_system_sleep"`
	SystemWake                  []any `toml:"on_system_wake"`
	ScreenLock                  []any `toml:"on_screen_lock"`
	ScreenUnlock                []any `toml:"on_screen_unlock"`
	SystemShutdown              []any `toml:"on_system_shutdown"`
	UserSessionEnd              []any `toml:"on_user_session_end"`
	VolumeMount                 []any `toml:"on_volume_mount"`
	VolumeUnmount               []any `toml:"on_volume_unmount"`
	ExternalDisplayConnected    []any `toml:"on_external_display_connected"`
	ExternalDisplayDisconnected []any `toml:"on_external_display_disconnected"`
	AppearanceChanged           []any `toml:"on_appearance_changed"`
	PowerAdapterConnected       []any `toml:"on_power_adapter_connected"`
	PowerAdapterDisconnected    []any `toml:"on_power_adapter_disconnected"`
	BatteryLow                  []any `toml:"on_battery_low"`
	BatteryCritical             []any `toml:"on_battery_critical"`
	AudioDeviceChanged          []any `toml:"on_audio_device_changed"`
	WorkspaceChanged            []any `toml:"on_workspace_changed"`
	USBDeviceConnected          []any `toml:"on_usb_device_connected"`
	USBDeviceDisconnected       []any `toml:"on_usb_device_disconnected"`
	NetworkUp                   []any `toml:"on_network_up"`
	NetworkDown                 []any `toml:"on_network_down"`
	ClipboardChanged            []any `toml:"on_clipboard_changed"`
}

type rawConfig struct {
	Settings SettingsConfig   `toml:"settings"`
	Hooks    rawHooksConfig   `toml:"hooks"`
	Systray  rawSystrayConfig `toml:"systray"`
}

type rawSystrayConfig struct {
	Enabled *bool `toml:"enabled"`
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
