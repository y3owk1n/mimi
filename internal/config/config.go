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
	Enabled             bool `json:"enabled"             toml:"enabled"`
	ShowWorkspaceNumber bool `json:"showWorkspaceNumber" toml:"show_workspace_number"`
}

// HooksConfig holds window and workspace hook entries.
type HooksConfig struct {
	WindowFocus       []HookEntry `json:"onWindowFocus"       toml:"on_window_focus"`
	WindowTitleChange []HookEntry `json:"onWindowTitleChange" toml:"on_window_title_change"`
	WindowCreated     []HookEntry `json:"onWindowCreated"     toml:"on_window_created"`
	WindowClosed      []HookEntry `json:"onWindowClosed"      toml:"on_window_closed"`
	WindowResize      []HookEntry `json:"onWindowResize"      toml:"on_window_resize"`
	WorkspaceChanged  []HookEntry `json:"onWorkspaceChanged"  toml:"on_workspace_changed"`
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
	WindowFocus       []any `json:"onWindowFocus"       toml:"on_window_focus"`
	WindowTitleChange []any `json:"onWindowTitleChange" toml:"on_window_title_change"`
	WindowCreated     []any `json:"onWindowCreated"     toml:"on_window_created"`
	WindowClosed      []any `json:"onWindowClosed"      toml:"on_window_closed"`
	WindowResize      []any `json:"onWindowResize"      toml:"on_window_resize"`
	WorkspaceChanged  []any `json:"onWorkspaceChanged"  toml:"on_workspace_changed"`
}

type rawConfig struct {
	Settings SettingsConfig   `json:"settings" toml:"settings"`
	Hooks    rawHooksConfig   `json:"hooks"    toml:"hooks"`
	Systray  rawSystrayConfig `json:"systray"  toml:"systray"`
}

type rawSystrayConfig struct {
	Enabled             *bool `json:"enabled"             toml:"enabled"`
	ShowWorkspaceNumber *bool `json:"showWorkspaceNumber" toml:"show_workspace_number"`
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

	hooksCfg.WindowFocus = decodeField("on_window_focus", raw.WindowFocus)
	hooksCfg.WindowTitleChange = decodeField("on_window_title_change", raw.WindowTitleChange)
	hooksCfg.WindowCreated = decodeField("on_window_created", raw.WindowCreated)
	hooksCfg.WindowClosed = decodeField("on_window_closed", raw.WindowClosed)
	hooksCfg.WindowResize = decodeField("on_window_resize", raw.WindowResize)
	hooksCfg.WorkspaceChanged = decodeField("on_workspace_changed", raw.WorkspaceChanged)

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
