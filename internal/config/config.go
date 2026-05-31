package config

import (
	"fmt"
	"strings"
)

const DefaultConfig = `# =============================================================================
# mimi — macOS Event Daemon
# =============================================================================
# Documentation: https://github.com/y3owk1n/mimi
# Validate:      mimi config validate
# Re-create:     mimi init
#
# Every hook receives these environment variables:
#   mimi_EVENT        — event kind (e.g. "app_activate")
#   mimi_EVENT_ID     — unique event UUID
#   mimi_APP_NAME     — app display name
#   mimi_BUNDLE_ID    — bundle identifier (e.g. "com.apple.Safari")
#   mimi_PID          — app process ID
#   mimi_WINDOW_TITLE — focused window title (window events only)
#   mimi_VOLUME_PATH  — mount path (volume events only)
#   mimi_VOLUME_NAME  — volume display name
#   mimi_TIMESTAMP    — RFC3339 timestamp
# =============================================================================

[settings]
# Log file path (supports ~/ expansion).
log_file = "~/.local/share/mimi/mimi.log"

# Log level: debug | info | warn | error
log_level = "info"

# Log format: text | json
log_format = "text"

# Default timeout (seconds) for each hook. Can be overridden per-hook.
hook_timeout_secs = 10

# Shell used to execute hook commands.
hook_shell = "/bin/sh"

# Maximum number of hook processes running concurrently.
max_hook_workers = 4

# PID file path (supports ~/ expansion).
pid_file = "~/.local/share/mimi/mimi.pid"

# =============================================================================
# Hooks
# =============================================================================
# Each hook is an array of entries. An entry can be either:
#
#   1) A plain string — the shell command to run:
#        on_app_activate = ["echo hello"]
#
#   2) An inline table with options:
#        on_app_activate = [
#          { run = "echo hello", app = "Slack", async = true }
#        ]
#
# Available entry fields:
#   run          — shell command (required)
#   app          — only fire when app name matches (glob: "Slack", "Code*")
#   bundle_id    — only fire when bundle ID matches exactly
#   title        — only fire when window title matches (regex)
#   timeout_secs — override global hook_timeout_secs for this entry
#   async        — run in background without blocking (default: false)
# =============================================================================

[hooks]

# ── Application Lifecycle ─────────────────────────────────────────────────────

# App comes to foreground.
on_app_activate = [
    "echo 'activated: $mimi_APP_NAME ($mimi_BUNDLE_ID)'"
]

# App loses foreground.
# on_app_deactivate = [
#     "echo 'deactivated: $mimi_APP_NAME'"
# ]

# App process started.
# on_app_launch = [
#     "logger 'launched: $mimi_APP_NAME'"
# ]

# App process terminated.
# on_app_quit = [
#     "echo 'quit: $mimi_APP_NAME'"
# ]

# App hidden (Cmd+H).
# on_app_hide = [
#     "echo 'hidden: $mimi_APP_NAME'"
# ]

# App unhidden.
# on_app_unhide = [
#     "echo 'unhidden: $mimi_APP_NAME'"
# ]

# ── Window Events (requires Accessibility permission) ─────────────────────────

# Focused window changed.
# on_window_focus = [
#     { run = "echo 'focus: $mimi_APP_NAME — $mimi_WINDOW_TITLE'", async = true }
# ]

# Active window title changed.
# on_window_title_change = [
#     "echo 'title: $mimi_WINDOW_TITLE'"
# ]

# New window opened.
# on_window_created = [
#     "echo 'window opened: $mimi_APP_NAME'"
# ]

# Window closed.
# on_window_closed = [
#     "echo 'window closed: $mimi_APP_NAME'"
# ]

# ── System Power Events ────────────────────────────────────────────────────────

# System going to sleep.
on_system_sleep = [
    "logger 'mimi: system sleeping'"
]

# System woke up.
on_system_wake = [
    "logger 'mimi: system woke'"
]

# Screen locked / session resigned active.
# on_screen_lock = []

# Screen unlocked / session became active.
# on_screen_unlock = []

# Shutdown or restart imminent.
# on_system_shutdown = []

# ── Storage Events ─────────────────────────────────────────────────────────────

# Volume / USB drive mounted.
on_volume_mount = [
    "echo 'mounted: $mimi_VOLUME_NAME at $mimi_VOLUME_PATH'"
]

# Volume / USB drive unmounted.
# on_volume_unmount = []

# ── Network Events (placeholder) ───────────────────────────────────────────────

# on_network_up = []
# on_network_down = []
`

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
	AppActivate       []interface{} `toml:"on_app_activate"`
	AppDeactivate     []interface{} `toml:"on_app_deactivate"`
	AppLaunch         []interface{} `toml:"on_app_launch"`
	AppQuit           []interface{} `toml:"on_app_quit"`
	AppHide           []interface{} `toml:"on_app_hide"`
	AppUnhide         []interface{} `toml:"on_app_unhide"`
	WindowFocus       []interface{} `toml:"on_window_focus"`
	WindowTitleChange []interface{} `toml:"on_window_title_change"`
	WindowCreated     []interface{} `toml:"on_window_created"`
	WindowClosed      []interface{} `toml:"on_window_closed"`
	SystemSleep       []interface{} `toml:"on_system_sleep"`
	SystemWake        []interface{} `toml:"on_system_wake"`
	ScreenLock        []interface{} `toml:"on_screen_lock"`
	ScreenUnlock      []interface{} `toml:"on_screen_unlock"`
	SystemShutdown    []interface{} `toml:"on_system_shutdown"`
	VolumeMount       []interface{} `toml:"on_volume_mount"`
	VolumeUnmount     []interface{} `toml:"on_volume_unmount"`
}

type rawConfig struct {
	Settings SettingsConfig `toml:"settings"`
	Hooks    rawHooksConfig `toml:"hooks"`
}

func decodeHooks(raw rawHooksConfig) (HooksConfig, error) {
	var hc HooksConfig
	var errs []string

	decodeField := func(field string, rawItems []interface{}) []HookEntry {
		var entries []HookEntry
		for i, item := range rawItems {
			switch v := item.(type) {
			case string:
				entries = append(entries, HookEntry{Run: v})
			case map[string]interface{}:
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
				errs = append(errs, fmt.Sprintf("%s[%d]: expected string or table, got %T", field, i, item))
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

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(m map[string]interface{}, key string) (int, bool) {
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

func getBool(m map[string]interface{}, key string) (bool, bool) {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}
