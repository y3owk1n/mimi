package config

import (
	"fmt"
	"slices"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Config holds the full mimi configuration.
type Config struct {
	Settings SettingsConfig `json:"settings" toml:"settings"`
	Hooks    HooksConfig    `json:"hooks"    toml:"hooks"`
	Systray  SystrayConfig  `json:"systray"  toml:"systray"`

	// UnknownHookKeys lists the keys found under [hooks] that name no hook
	// kind, sorted. Loading records them rather than reporting them so each
	// caller can decide: the daemon warns and carries on with the hooks it did
	// understand, while `mimi config validate` names them and fails. It is not
	// part of the config file, so it carries no toml tag, and it is excluded
	// from JSON so `mimi config dump` keeps printing the config as written.
	UnknownHookKeys []string `json:"-" toml:"-"`
}

// SettingsConfig holds the [settings] section of the config.
type SettingsConfig struct {
	LogFile          string `json:"logFile"          toml:"log_file"`
	LogLevel         string `json:"logLevel"         toml:"log_level"`
	LogFormat        string `json:"logFormat"        toml:"log_format"`
	HookTimeoutSecs  int    `json:"hookTimeoutSecs"  toml:"hook_timeout_secs"`
	HookShell        string `json:"hookShell"        toml:"hook_shell"`
	MaxHookWorkers   int    `json:"maxHookWorkers"   toml:"max_hook_workers"`
	PIDFile          string `json:"pidFile"          toml:"pid_file"`
	SocketFile       string `json:"socketFile"       toml:"socket_file"`
	ResizeDebounceMS int    `json:"resizeDebounceMs" toml:"resize_debounce_ms"`
}

// SystrayConfig holds the [systray] section of the config.
type SystrayConfig struct {
	Enabled             bool `json:"enabled"             toml:"enabled"`
	ShowWorkspaceNumber bool `json:"showWorkspaceNumber" toml:"show_workspace_number"`
}

// HooksConfig holds all hook entries grouped by event kind.
type HooksConfig struct {
	AppActivate       []HookEntry `json:"onAppActivate"       toml:"on_app_activate"`
	AppDeactivate     []HookEntry `json:"onAppDeactivate"     toml:"on_app_deactivate"`
	AppLaunch         []HookEntry `json:"onAppLaunch"         toml:"on_app_launch"`
	AppQuit           []HookEntry `json:"onAppQuit"           toml:"on_app_quit"`
	AppHide           []HookEntry `json:"onAppHide"           toml:"on_app_hide"`
	AppUnhide         []HookEntry `json:"onAppUnhide"         toml:"on_app_unhide"`
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

// rawHooksConfig holds the [hooks] table exactly as written, keyed by the TOML
// key the user typed, so decodeHooks can fold over HookKinds to pull the keys
// it recognizes instead of restating the twelve field names.
//
// The value type is any, not []any: a []any map would make the TOML decoder
// itself reject every unrecognized key whose value is not an array, which is
// both a behavior change and the decoder's error rather than one of ours.
// Unrecognized keys are decoded, ignored here, and left for a later change to
// report.
type rawHooksConfig map[string]any

type rawConfig struct {
	Settings SettingsConfig   `json:"settings" toml:"settings"`
	Hooks    rawHooksConfig   `json:"hooks"    toml:"hooks"`
	Systray  rawSystrayConfig `json:"systray"  toml:"systray"`
}

type rawSystrayConfig struct {
	Enabled             *bool `json:"enabled"             toml:"enabled"`
	ShowWorkspaceNumber *bool `json:"showWorkspaceNumber" toml:"show_workspace_number"`
}

// decodeHooks turns the raw [hooks] table into a HooksConfig, and also returns
// the keys it did not recognize so the caller can decide what to do about them.
//
// It reports only structural problems -- a key that is not a list, an entry
// that is neither a string nor a table. Whether a decoded entry actually says
// what to run is validate's business, so that a hook with no command reads the
// same however it was written.
func decodeHooks(raw rawHooksConfig) (HooksConfig, []string, error) {
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

				entries = append(entries, entry)
			default:
				errs = append(
					errs,
					fmt.Sprintf("hooks.%s[%d]: expected string or table, got %T", field, idx, item),
				)
			}
		}

		return entries
	}

	recognized := make(map[string]struct{}, len(HookKinds))
	for _, name := range HookKindNames() {
		recognized[name] = struct{}{}
	}

	for _, kind := range HookKinds {
		items, isList := rawHookItems(raw[kind.TOMLKey])
		if !isList {
			errs = append(
				errs,
				fmt.Sprintf(
					"hooks.%s: expected an array of hooks, got %T",
					kind.TOMLKey,
					raw[kind.TOMLKey],
				),
			)

			continue
		}

		*kind.Entries(&hooksCfg) = decodeField(kind.TOMLKey, items)
	}

	var unknown []string

	for key := range raw {
		if _, ok := recognized[key]; !ok {
			unknown = append(unknown, key)
		}
	}

	// Map iteration order is random; sort so a config with several typos
	// reports them the same way on every run.
	slices.Sort(unknown)

	if len(errs) > 0 {
		return hooksCfg, unknown, derrors.Newf(
			derrors.CodeInvalidConfig,
			"hook decode errors:\n  - %s",
			strings.Join(errs, "\n  - "),
		)
	}

	return hooksCfg, unknown, nil
}

// rawHookItems normalizes the two shapes the TOML decoder produces for a hook
// list into the one decodeField walks: an inline array (on_x = [...]) arrives
// as []any, while a sequence of [[hooks.on_x]] tables arrives as
// []map[string]any.
//
// A missing key is an absent list, not a malformed one. The second result is
// false only when the key is present and holds something that is not a list.
func rawHookItems(value any) ([]any, bool) {
	switch list := value.(type) {
	case nil:
		return nil, true
	case []any:
		return list, true
	case []map[string]any:
		items := make([]any, 0, len(list))
		for _, table := range list {
			items = append(items, table)
		}

		return items, true
	default:
		return nil, false
	}
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
