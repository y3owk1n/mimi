package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/y3owk1n/mimi/configs"
	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/paths"
)

// DefaultConfigPath is the default path for the mimi config file.
const DefaultConfigPath = "~/.config/mimi/config.toml"

// DefaultPIDPath is the default path for the mimi PID file.
const DefaultPIDPath = "~/.local/share/mimi/mimi.pid"

// DefaultSocketPath is the default Unix socket path for daemon IPC.
const DefaultSocketPath = "~/.local/share/mimi/mimi.sock"

// Exists returns true if the config file exists.
func Exists(path string) bool {
	_, err := os.Stat(paths.ExpandHome(path))

	return err == nil
}

// ResolvePath resolves the config path based on CLI override and priority list:
// 1. CLI flag override (if non-empty)
// 2. $XDG_CONFIG_HOME/mimi/config.toml (if env set and file exists)
// 3. ~/.config/mimi/config.toml (if file exists)
// 4. mimi.toml in current directory (if file exists)
// If none exists and CLI override is empty, it returns the default fallback:
// $XDG_CONFIG_HOME/mimi/config.toml (if env set) or ~/.config/mimi/config.toml.
func ResolvePath(cliPath string) string {
	if cliPath != "" {
		return paths.ExpandHome(cliPath)
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		p := filepath.Join(xdg, "mimi/config.toml")
		if Exists(p) {
			return paths.ExpandHome(p)
		}
	}

	p2 := "~/.config/mimi/config.toml"
	if Exists(p2) {
		return paths.ExpandHome(p2)
	}

	altPath := "mimi.toml"
	if Exists(altPath) {
		abs, err := filepath.Abs(altPath)
		if err == nil {
			return abs
		}

		return altPath
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return paths.ExpandHome(filepath.Join(xdg, "mimi/config.toml"))
	}

	return paths.ExpandHome("~/.config/mimi/config.toml")
}

// WriteDefault writes the default config to the given path.
func WriteDefault(path string) error {
	path = paths.ExpandHome(path)

	err := os.MkdirAll(filepath.Dir(path), 0o755) //nolint:mnd
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "creating config directory")
	}

	err = os.WriteFile(path, configs.DefaultConfig, 0o644) //nolint:mnd
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeConfigIOFailed, "writing default config")
	}

	return nil
}

// Load parses and validates the config from a TOML file.
//
// A validation failure returns the config it managed to build alongside the
// error, so a caller whose job is reporting problems -- `mimi config validate`
// -- can show everything wrong in one pass instead of one problem per run.
// That matters for unrecognized hook keys in particular: they are recorded on
// the config rather than raised as errors, so a config that also fails
// validation would otherwise hide them until the other error was fixed.
//
// Callers that want a usable config must check the error first, as all of them
// do. Failures before validation -- an unreadable file, malformed TOML, a hook
// key holding something that is not a list -- return a nil config.
func Load(path string) (*Config, error) {
	path = paths.ExpandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
	}

	var raw rawConfig

	_, err = toml.Decode(string(data), &raw)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeSerializationFailed, "parsing config")
	}

	hooks, unknownHookKeys, err := decodeHooks(raw.Hooks)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeInvalidConfig, "decoding hooks")
	}

	cfg := &Config{
		Settings:        raw.Settings,
		Hooks:           hooks,
		Tiling:          raw.Tiling,
		Border:          raw.Border,
		UnknownHookKeys: unknownHookKeys,
	}

	systrayEnabledSet := raw.Systray.Enabled != nil
	if systrayEnabledSet {
		cfg.Systray.Enabled = *raw.Systray.Enabled
	}

	if raw.Systray.ShowWorkspaceNumber != nil {
		cfg.Systray.ShowWorkspaceNumber = *raw.Systray.ShowWorkspaceNumber
	}

	applyDefaults(cfg, systrayEnabledSet)

	err = validate(cfg)
	if err != nil {
		return cfg, err
	}

	expandPaths(cfg)

	return cfg, nil
}

// The [tiling] defaults: how long a burst of window events settles before one
// relayout runs, how long the layout program may take, and how the frames
// move when animated.
const (
	defaultTilingDebounceMS      = 100
	defaultTilingTimeoutSecs     = 5
	defaultTilingCommandSecs     = 1
	defaultTilingAnimationMS     = 150
	defaultTilingAnimationEasing = "ease-out"
)

// The [border] defaults: a ring a few points wide, light on the focused
// window and dark on the rest.
const (
	defaultBorderWidth         = 4.0
	defaultBorderActiveColor   = "#e2e2e3"
	defaultBorderInactiveColor = "#414141"
	maxBorderWidth             = 32.0
)

// The [tiling.dropzone] defaults: a faint fill of the border's light color
// with a thin outline of it, rounded like a document window.
const (
	defaultDropzoneColor        = "#30e2e2e3"
	defaultDropzoneOutlineColor = "#e2e2e3"
	defaultDropzoneOutlineWidth = 2.0
	defaultDropzoneRadius       = 12.0
	defaultStackbarColor        = "#b0636366"
	defaultStackbarFarColor     = "#30636366"
	defaultStackbarStep         = 10.0
	defaultStackbarTaper        = 6.0
	defaultStackbarRadius       = -1.0
)

func applyDefaults(cfg *Config, systrayEnabledSet bool) {
	settings := &cfg.Settings
	if settings.LogLevel == "" {
		settings.LogLevel = "info"
	}

	if settings.LogFormat == "" {
		settings.LogFormat = "text"
	}

	if settings.HookTimeoutSecs == 0 {
		settings.HookTimeoutSecs = 10
	}

	if settings.HookShell == "" {
		settings.HookShell = "/bin/sh"
	}

	if settings.MaxHookWorkers == 0 {
		settings.MaxHookWorkers = 4
	}

	if cfg.Tiling.DebounceMS == 0 {
		cfg.Tiling.DebounceMS = defaultTilingDebounceMS
	}

	if cfg.Tiling.TimeoutSecs == 0 {
		cfg.Tiling.TimeoutSecs = defaultTilingTimeoutSecs
	}

	if cfg.Tiling.CommandTimeoutSecs == 0 {
		cfg.Tiling.CommandTimeoutSecs = defaultTilingCommandSecs
	}

	if cfg.Tiling.LayoutMode == "" {
		cfg.Tiling.LayoutMode = LayoutModeOneshot
	}

	if cfg.Tiling.Animation.DurationMS == 0 {
		cfg.Tiling.Animation.DurationMS = defaultTilingAnimationMS
	}

	if cfg.Tiling.Animation.Easing == "" {
		cfg.Tiling.Animation.Easing = defaultTilingAnimationEasing
	}

	if cfg.Tiling.Dropzone.Color == "" {
		cfg.Tiling.Dropzone.Color = defaultDropzoneColor
	}

	if cfg.Tiling.Dropzone.OutlineColor == "" {
		cfg.Tiling.Dropzone.OutlineColor = defaultDropzoneOutlineColor
	}

	if cfg.Tiling.Dropzone.OutlineWidth == 0 {
		cfg.Tiling.Dropzone.OutlineWidth = defaultDropzoneOutlineWidth
	}

	if cfg.Tiling.Dropzone.Radius == 0 {
		cfg.Tiling.Dropzone.Radius = defaultDropzoneRadius
	}

	if cfg.Tiling.Stackbar.Color == "" {
		cfg.Tiling.Stackbar.Color = defaultStackbarColor
	}

	if cfg.Tiling.Stackbar.FarColor == "" {
		cfg.Tiling.Stackbar.FarColor = defaultStackbarFarColor
	}

	if cfg.Tiling.Stackbar.Step == 0 {
		cfg.Tiling.Stackbar.Step = defaultStackbarStep
	}

	if cfg.Tiling.Stackbar.Taper == 0 {
		cfg.Tiling.Stackbar.Taper = defaultStackbarTaper
	}

	if cfg.Tiling.Stackbar.Radius == 0 {
		cfg.Tiling.Stackbar.Radius = defaultStackbarRadius
	}

	if cfg.Border.Width == 0 {
		cfg.Border.Width = defaultBorderWidth
	}

	if cfg.Border.ActiveColor == "" {
		cfg.Border.ActiveColor = defaultBorderActiveColor
	}

	if cfg.Border.InactiveColor == "" {
		cfg.Border.InactiveColor = defaultBorderInactiveColor
	}

	if settings.PIDFile == "" {
		settings.PIDFile = "~/.local/share/mimi/mimi.pid"
	}

	if settings.SocketFile == "" {
		settings.SocketFile = DefaultSocketPath
	}

	if settings.ResizeDebounceMS == 0 {
		settings.ResizeDebounceMS = 250
	}

	if !systrayEnabledSet {
		cfg.Systray.Enabled = true
	}
}

func validate(cfg *Config) error {
	var errs []string
	if cfg.Settings.HookTimeoutSecs < 1 {
		errs = append(errs, "settings.hook_timeout_secs must be >= 1")
	}

	if cfg.Settings.MaxHookWorkers < 1 {
		errs = append(errs, "settings.max_hook_workers must be >= 1")
	}

	if cfg.Settings.ResizeDebounceMS < 0 {
		errs = append(errs, "settings.resize_debounce_ms must be >= 0")
	}

	if cfg.Tiling.Enabled && strings.TrimSpace(cfg.Tiling.Layout) == "" {
		errs = append(errs, "tiling.layout is required when tiling.enabled is true")
	}

	if cfg.Tiling.DebounceMS < 0 {
		errs = append(errs, "tiling.debounce_ms must be >= 0")
	}

	if cfg.Tiling.Gap != nil && *cfg.Tiling.Gap < 0 {
		errs = append(errs, "tiling.gap must be >= 0")
	}

	if cfg.Tiling.TimeoutSecs < 1 {
		errs = append(errs, "tiling.timeout_secs must be >= 1")
	}

	if cfg.Tiling.CommandTimeoutSecs < 1 {
		errs = append(errs, "tiling.command_timeout_secs must be >= 1")
	}

	if cfg.Tiling.LayoutMode != LayoutModeOneshot && cfg.Tiling.LayoutMode != LayoutModeResident {
		errs = append(errs, "tiling.layout_mode must be oneshot or resident")
	}

	animationMS := cfg.Tiling.Animation.DurationMS
	if animationMS < 1 || animationMS > action.MaxAnimationMS {
		errs = append(
			errs,
			fmt.Sprintf(
				"tiling.animation.duration_ms must be between 1 and %d",
				action.MaxAnimationMS,
			),
		)
	}

	if !slices.Contains(action.Easings, cfg.Tiling.Animation.Easing) {
		errs = append(
			errs,
			"tiling.animation.easing must be one of "+strings.Join(action.Easings, ", "),
		)
	}

	errs = append(errs, validateBorder(cfg.Border)...)
	errs = append(errs, validateDropzone(cfg.Tiling)...)
	errs = append(errs, validateStackbar(cfg.Tiling)...)

	// HookKinds is a slice, so these errors come out in its declared order.
	// The map this replaced meant validate reported the same broken config in
	// a different order on every run.
	//
	// Entries are named by the key the user typed, not by the event kind they
	// publish as, so the error points at a line they can find in their file.
	// This is the only check on a hook's command: decoding deliberately leaves
	// it alone so that a hook written as a bare string and one written as a
	// table report the same way.
	for _, kind := range HookKinds {
		for index, entry := range *kind.Entries(&cfg.Hooks) {
			if entry.Run == "" {
				errs = append(
					errs,
					fmt.Sprintf("hooks.%s[%d]: run command is empty", kind.TOMLKey, index),
				)
			}

			for _, problem := range validateFilters(kind, entry) {
				errs = append(
					errs,
					fmt.Sprintf("hooks.%s[%d]: %s", kind.TOMLKey, index, problem),
				)
			}
		}
	}

	if len(errs) > 0 {
		return derrors.Newf(
			derrors.CodeInvalidConfig,
			"config validation failed:\n  - %s",
			strings.Join(errs, "\n  - "),
		)
	}

	return nil
}

// validateDropzone holds the [tiling.dropzone] section to its rules: it
// needs relayout_on_drag, its colors parse, and its sizes are sane.
func validateDropzone(tiling TilingConfig) []string {
	var errs []string

	zone := tiling.Dropzone

	if zone.Enabled && !tiling.RelayoutOnDrag {
		errs = append(errs, "tiling.dropzone.enabled needs tiling.relayout_on_drag")
	}

	_, err := ParseColor(zone.Color)
	if err != nil {
		errs = append(errs, "tiling.dropzone.color must be #rrggbb or #aarrggbb")
	}

	_, err = ParseColor(zone.OutlineColor)
	if err != nil {
		errs = append(errs, "tiling.dropzone.outline_color must be #rrggbb or #aarrggbb")
	}

	if zone.OutlineWidth < 0 || zone.OutlineWidth > maxBorderWidth {
		errs = append(
			errs,
			fmt.Sprintf(
				"tiling.dropzone.outline_width must be between 0 and %d",
				int(maxBorderWidth),
			),
		)
	}

	if zone.Radius < 0 {
		errs = append(errs, "tiling.dropzone.radius must be >= 0")
	}

	return errs
}

// validateStackbar holds the [tiling.stackbar] section to its rules: its
// colors parse and its sizes are sane. It needs nothing else switched on,
// since a layout that names no stack simply has nothing marked.
func validateStackbar(tiling TilingConfig) []string {
	var errs []string

	bar := tiling.Stackbar

	_, err := ParseColor(bar.Color)
	if err != nil {
		errs = append(errs, "tiling.stackbar.color must be #rrggbb or #aarrggbb")
	}

	_, err = ParseColor(bar.FarColor)
	if err != nil {
		errs = append(errs, "tiling.stackbar.far_color must be #rrggbb or #aarrggbb")
	}

	if bar.Step <= 0 || bar.Step > maxStackbarStep {
		errs = append(
			errs,
			fmt.Sprintf("tiling.stackbar.step must be between 0 and %d", int(maxStackbarStep)),
		)
	}

	if bar.Taper < 0 || bar.Taper > maxStackbarStep {
		errs = append(
			errs,
			fmt.Sprintf("tiling.stackbar.taper must be between 0 and %d", int(maxStackbarStep)),
		)
	}

	return errs
}

// maxStackbarStep is as much of a window behind as may show, and as much
// narrower as it may be drawn. Past this the cards take more of the frame
// than the window in front can spare.
const maxStackbarStep = 40.0

func validateBorder(border BorderConfig) []string {
	var errs []string

	if border.Width < 1 || border.Width > maxBorderWidth {
		errs = append(
			errs,
			fmt.Sprintf("border.width must be between 1 and %d", int(maxBorderWidth)),
		)
	}

	if border.Radius != nil && *border.Radius < 0 {
		errs = append(errs, "border.radius must be >= 0")
	}

	_, err := ParseColor(border.ActiveColor)
	if err != nil {
		errs = append(errs, "border.active_color must be #rrggbb or #aarrggbb")
	}

	_, err = ParseColor(border.InactiveColor)
	if err != nil {
		errs = append(errs, "border.inactive_color must be #rrggbb or #aarrggbb")
	}

	return errs
}

func expandPaths(cfg *Config) {
	// The layout is a command line, not a path, so only a leading "~" is
	// expanded: the shell it runs through expands nothing in the program
	// position, which is the one place a user writes one.
	cfg.Tiling.Layout = paths.ExpandHome(cfg.Tiling.Layout)
	cfg.Settings.LogFile = paths.ExpandHome(cfg.Settings.LogFile)
	cfg.Settings.PIDFile = paths.ExpandHome(cfg.Settings.PIDFile)
	cfg.Settings.SocketFile = paths.ExpandHome(cfg.Settings.SocketFile)
}
