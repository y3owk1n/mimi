package config

import (
	"fmt"
	"strings"
)

type Config struct {
	Settings SettingsConfig `toml:"settings"`
	Hooks    HooksConfig    `toml:"hooks"`
}

type SettingsConfig struct {
	LogFile         string `toml:"log_file"`
	LogLevel        string `toml:"log_level"`
	LogFormat       string `toml:"log_format"`
	HookTimeoutSecs int    `toml:"hook_timeout_secs"`
	HookShell       string `toml:"hook_shell"`
	MaxHookWorkers  int    `toml:"max_hook_workers"`
	PIDFile         string `toml:"pid_file"`
}

type HooksConfig struct {
	AppActivate       []HookEntry `toml:"on_app_activate"`
	AppDeactivate     []HookEntry `toml:"on_app_deactivate"`
	AppLaunch         []HookEntry `toml:"on_app_launch"`
	AppQuit           []HookEntry `toml:"on_app_quit"`
	AppHide           []HookEntry `toml:"on_app_hide"`
	AppUnhide         []HookEntry `toml:"on_app_unhide"`
	WindowFocus       []HookEntry `toml:"on_window_focus"`
	WindowTitleChange []HookEntry `toml:"on_window_title_change"`
	WindowCreated     []HookEntry `toml:"on_window_created"`
	WindowClosed      []HookEntry `toml:"on_window_closed"`
	SystemSleep       []HookEntry `toml:"on_system_sleep"`
	SystemWake        []HookEntry `toml:"on_system_wake"`
	ScreenLock        []HookEntry `toml:"on_screen_lock"`
	ScreenUnlock      []HookEntry `toml:"on_screen_unlock"`
	SystemShutdown    []HookEntry `toml:"on_system_shutdown"`
	VolumeMount       []HookEntry `toml:"on_volume_mount"`
	VolumeUnmount     []HookEntry `toml:"on_volume_unmount"`
}

type HookEntry struct {
	Run         string `toml:"run"`
	App         string `toml:"app"`
	BundleID    string `toml:"bundle_id"`
	Title       string `toml:"title"`
	TimeoutSecs int    `toml:"timeout_secs"`
	Async       bool   `toml:"async"`
}

type rawHooksConfig struct {
	AppActivate       []any `toml:"on_app_activate"`
	AppDeactivate     []any `toml:"on_app_deactivate"`
	AppLaunch         []any `toml:"on_app_launch"`
	AppQuit           []any `toml:"on_app_quit"`
	AppHide           []any `toml:"on_app_hide"`
	AppUnhide         []any `toml:"on_app_unhide"`
	WindowFocus       []any `toml:"on_window_focus"`
	WindowTitleChange []any `toml:"on_window_title_change"`
	WindowCreated     []any `toml:"on_window_created"`
	WindowClosed      []any `toml:"on_window_closed"`
	SystemSleep       []any `toml:"on_system_sleep"`
	SystemWake        []any `toml:"on_system_wake"`
	ScreenLock        []any `toml:"on_screen_lock"`
	ScreenUnlock      []any `toml:"on_screen_unlock"`
	SystemShutdown    []any `toml:"on_system_shutdown"`
	VolumeMount       []any `toml:"on_volume_mount"`
	VolumeUnmount     []any `toml:"on_volume_unmount"`
}

type rawConfig struct {
	Settings SettingsConfig `toml:"settings"`
	Hooks    rawHooksConfig `toml:"hooks"`
}

func decodeHooks(raw rawHooksConfig) (HooksConfig, error) {
	var (
		hc   HooksConfig
		errs []string
	)

	decodeField := func(field string, rawItems []any) []HookEntry {
		var entries []HookEntry
		for i, item := range rawItems {
			switch v := item.(type) {
			case string:
				entries = append(entries, HookEntry{Run: v})
			case map[string]any:
				entry := HookEntry{
					Run:      getString(v, "run"),
					App:      getString(v, "app"),
					BundleID: getString(v, "bundle_id"),
					Title:    getString(v, "title"),
				}
				if timeout, ok := getInt(v, "timeout_secs"); ok {
					entry.TimeoutSecs = timeout
				}

				if async, ok := getBool(v, "async"); ok {
					entry.Async = async
				}

				if entry.Run == "" {
					errs = append(errs, fmt.Sprintf("%s[%d]: 'run' field is required", field, i))
				}

				entries = append(entries, entry)
			default:
				errs = append(
					errs,
					fmt.Sprintf("%s[%d]: expected string or table, got %T", field, i, item),
				)
			}
		}

		return entries
	}

	hc.AppActivate = decodeField("on_app_activate", raw.AppActivate)
	hc.AppDeactivate = decodeField("on_app_deactivate", raw.AppDeactivate)
	hc.AppLaunch = decodeField("on_app_launch", raw.AppLaunch)
	hc.AppQuit = decodeField("on_app_quit", raw.AppQuit)
	hc.AppHide = decodeField("on_app_hide", raw.AppHide)
	hc.AppUnhide = decodeField("on_app_unhide", raw.AppUnhide)
	hc.WindowFocus = decodeField("on_window_focus", raw.WindowFocus)
	hc.WindowTitleChange = decodeField("on_window_title_change", raw.WindowTitleChange)
	hc.WindowCreated = decodeField("on_window_created", raw.WindowCreated)
	hc.WindowClosed = decodeField("on_window_closed", raw.WindowClosed)
	hc.SystemSleep = decodeField("on_system_sleep", raw.SystemSleep)
	hc.SystemWake = decodeField("on_system_wake", raw.SystemWake)
	hc.ScreenLock = decodeField("on_screen_lock", raw.ScreenLock)
	hc.ScreenUnlock = decodeField("on_screen_unlock", raw.ScreenUnlock)
	hc.SystemShutdown = decodeField("on_system_shutdown", raw.SystemShutdown)
	hc.VolumeMount = decodeField("on_volume_mount", raw.VolumeMount)
	hc.VolumeUnmount = decodeField("on_volume_unmount", raw.VolumeUnmount)

	if len(errs) > 0 {
		return hc, fmt.Errorf("hook decode errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return hc, nil
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
