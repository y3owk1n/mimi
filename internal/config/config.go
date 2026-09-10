package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Config holds the full mimi configuration.
//
// The reload tags classify each part of the file as reloadable, restart-only
// or reinstall-only; see reloadability.go for what they mean and who reads
// them. A section tagged per-field is classified one field at a time instead.
type Config struct {
	Settings SettingsConfig `json:"settings" reload:"per-field"  toml:"settings"`
	Hooks    HooksConfig    `json:"hooks"    reload:"reloadable" toml:"hooks"`
	Systray  SystrayConfig  `json:"systray"  reload:"per-field"  toml:"systray"`
	Tiling   TilingConfig   `json:"tiling"   reload:"reloadable" toml:"tiling"`
	Border   BorderConfig   `json:"border"   reload:"reloadable" toml:"border"`

	// UnknownHookKeys lists the keys found under [hooks] that name no hook
	// kind, sorted. Loading records them rather than reporting them so each
	// caller can decide: the daemon warns and carries on with the hooks it did
	// understand, while `mimi config validate` names them and fails. It is not
	// part of the config file, so it carries no toml tag, and it is excluded
	// from JSON so `mimi config dump` keeps printing the config as written.
	UnknownHookKeys []string `json:"-" toml:"-"`
}

// SettingsConfig holds the [settings] section of the config.
//
// Every field carries a reload tag: the logger, the pid file and the socket
// are opened once at startup, and the hook worker limit sizes a channel the
// executor never resizes, so changing any of them takes a restart. The three
// the reload path re-reads are tagged reloadable.
//
// ServicePath is read by nobody in the daemon at all. It is the PATH `mimi
// services install` bakes into the launchd plist, so the value in effect is
// the one on disk in the plist rather than the one in this struct, and only
// installing the service again changes it — see
// docs/adr/0003-a-setting-the-daemon-never-reads-is-reinstall-only.md. It is a
// PATH rather than a path, so expandPaths leaves it alone: it is a list, and
// launchd expands no "~" in it whatever mimi does.
type SettingsConfig struct {
	LogFile          string `json:"logFile"          reload:"restart-only"   toml:"log_file"`
	LogLevel         string `json:"logLevel"         reload:"restart-only"   toml:"log_level"`
	LogFormat        string `json:"logFormat"        reload:"restart-only"   toml:"log_format"`
	HookTimeoutSecs  int    `json:"hookTimeoutSecs"  reload:"reloadable"     toml:"hook_timeout_secs"`
	HookShell        string `json:"hookShell"        reload:"reloadable"     toml:"hook_shell"`
	MaxHookWorkers   int    `json:"maxHookWorkers"   reload:"restart-only"   toml:"max_hook_workers"`
	PIDFile          string `json:"pidFile"          reload:"restart-only"   toml:"pid_file"`
	SocketFile       string `json:"socketFile"       reload:"restart-only"   toml:"socket_file"`
	ResizeDebounceMS int    `json:"resizeDebounceMs" reload:"reloadable"     toml:"resize_debounce_ms"`
	ServicePath      string `json:"servicePath"      reload:"reinstall-only" toml:"service_path"`
}

// SystrayConfig holds the [systray] section of the config.
//
// The systray is built before the daemon's reload path exists and is never
// rebuilt, so both fields are restart-only.
type SystrayConfig struct {
	Enabled             bool `json:"enabled"             reload:"restart-only" toml:"enabled"`
	ShowWorkspaceNumber bool `json:"showWorkspaceNumber" reload:"restart-only" toml:"show_workspace_number"`
}

// The values tiling.layout_mode takes.
const (
	LayoutModeOneshot  = "oneshot"
	LayoutModeResident = "resident"
)

// TilingConfig holds the [tiling] section of the config: whether the daemon
// runs a layout at all, and the program that is the layout.
//
// mimi ships no layout. Layout is a command line run through
// settings.hook_shell; it reads the tiling input as JSON on stdin and prints
// the frames to apply as JSON on stdout (see docs/CONFIGURATION.md and
// examples/tiling/). The whole section is reloadable: the engine re-reads it
// on every reload, and enabling it at runtime is how a layout is tried out.
type TilingConfig struct {
	Enabled bool   `json:"enabled" toml:"enabled"`
	Layout  string `json:"layout"  toml:"layout"`
	// LayoutMode is how the layout runs: LayoutModeOneshot, a process per
	// pass given the input on stdin and read to exit, or
	// LayoutModeResident, one process kept running that reads an input
	// line and prints an output line per pass.
	LayoutMode  string `json:"layoutMode"  toml:"layout_mode"`
	DebounceMS  int    `json:"debounceMs"  toml:"debounce_ms"`
	TimeoutSecs int    `json:"timeoutSecs" toml:"timeout_secs"`
	// CommandTimeoutSecs bounds each before and after command line a
	// layout returns. A before line holds the frames until it finishes,
	// so its bound is its own, and tighter than the layout's.
	CommandTimeoutSecs int  `json:"commandTimeoutSecs" toml:"command_timeout_secs"`
	RelayoutOnDrag     bool `json:"relayoutOnDrag"     toml:"relayout_on_drag"`
	// Gap is the space between tiled windows and at the display's edges, in
	// points. Left unset, the macOS tiled-window margin applies, the same
	// setting resize_window honors; set, it replaces that setting for
	// layouts, 0 included.
	Gap *int `json:"gap" toml:"gap"`
	// Animation is how the frames a layout returns move into place.
	Animation AnimationConfig `json:"animation" toml:"animation"`
}

// AnimationConfig holds the [tiling.animation] section: whether windows
// move to the frames a layout returns over time rather than at once, over
// how long, and along which curve. Off, the daemon neither captures the
// screen nor asks for the Screen Recording permission that takes.
type AnimationConfig struct {
	Enabled    bool   `json:"enabled"    toml:"enabled"`
	DurationMS int    `json:"durationMs" toml:"duration_ms"`
	Easing     string `json:"easing"     toml:"easing"`
}

// BorderConfig holds the [border] section: whether the daemon draws a
// border around every window on the spaces in front, how wide, the radius
// of the window corner it follows, and the colors for the focused window
// and for the rest. Colors are written as #rrggbb or #rrggbbaa. The whole
// section is reloadable; it needs Accessibility, like the window events it
// follows.
type BorderConfig struct {
	Enabled bool    `json:"enabled" toml:"enabled"`
	Width   float64 `json:"width"   toml:"width"`
	// Radius is the corner radius of the window the border follows on its
	// inside, in points. Left unset, each border follows its own window's
	// corner as the window server reports it; 0 draws square corners.
	Radius        *float64 `json:"radius"        toml:"radius"`
	ActiveColor   string   `json:"activeColor"   toml:"active_color"`
	InactiveColor string   `json:"inactiveColor" toml:"inactive_color"`
}

// CornerRadius is the radius the border follows: Radius when set, else
// FollowWindowRadius, each window's own.
func (b BorderConfig) CornerRadius() float64 {
	if b.Radius != nil {
		return *b.Radius
	}

	return FollowWindowRadius
}

// FollowWindowRadius is the CornerRadius of a [border] section with no
// radius set: every border follows its own window's corner.
const FollowWindowRadius = -1

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
	WindowMove        []HookEntry `json:"onWindowMove"        toml:"on_window_move"`
	WindowMinimize    []HookEntry `json:"onWindowMinimize"    toml:"on_window_minimize"`
	WindowUnminimize  []HookEntry `json:"onWindowUnminimize"  toml:"on_window_unminimize"`
	WorkspaceChanged  []HookEntry `json:"onWorkspaceChanged"  toml:"on_workspace_changed"`
}

// HookEntry defines a single hook command and its optional filters.
//
// App, BundleID, Title and Space may each begin with "!" to match everything
// the pattern does not; see SplitNegation. Space is a 1-based space number
// kept as a string so that the negated form has somewhere to live, and is
// accepted as a bare number in TOML as well.
type HookEntry struct {
	Run         string `json:"run"         toml:"run"`
	App         string `json:"app"         toml:"app"`
	BundleID    string `json:"bundleId"    toml:"bundle_id"`
	Title       string `json:"title"       toml:"title"`
	Space       string `json:"space"       toml:"space"`
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
	Tiling   TilingConfig     `json:"tiling"   toml:"tiling"`
	Border   BorderConfig     `json:"border"   toml:"border"`
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

				space, ok := getSpace(val)
				if !ok {
					errs = append(
						errs,
						fmt.Sprintf(
							"hooks.%s[%d]: space must be a number or a string, got %T",
							field,
							idx,
							val["space"],
						),
					)
				}

				entry.Space = space

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

// getSpace reads a hook's space filter, which TOML delivers as a number when
// written bare (space = 2) and as a string when negated (space = "!2"). Both
// spell the same filter. An absent key is the empty filter; a value of any
// other type is reported as false.
func getSpace(m map[string]any) (string, bool) {
	value, ok := m["space"]
	if !ok {
		return "", true
	}

	switch space := value.(type) {
	case string:
		return space, true
	case int64:
		return strconv.FormatInt(space, 10), true
	case float64:
		return strconv.Itoa(int(space)), true
	default:
		return "", false
	}
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
