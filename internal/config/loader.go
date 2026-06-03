package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/y3owk1n/mimi/configs"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/events"
)

// DefaultConfigPath is the default path for the mimi config file.
const DefaultConfigPath = "~/.config/mimi/config.toml"

// Exists returns true if the config file exists.
func Exists(path string) bool {
	_, err := os.Stat(expandHome(path))

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
		return expandHome(cliPath)
	}

	// 1. Check $XDG_CONFIG_HOME/mimi/config.toml
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		p := filepath.Join(xdg, "mimi/config.toml")
		if Exists(p) {
			return expandHome(p)
		}
	}

	// 2. Check ~/.config/mimi/config.toml
	p2 := "~/.config/mimi/config.toml"
	if Exists(p2) {
		return expandHome(p2)
	}

	// 3. Check mimi.toml (current directory)
	altPath := "mimi.toml"
	if Exists(altPath) {
		abs, err := filepath.Abs(altPath)
		if err == nil {
			return abs
		}

		return altPath
	}

	// Fallback when none exists
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return expandHome(filepath.Join(xdg, "mimi/config.toml"))
	}

	return expandHome("~/.config/mimi/config.toml")
}

// WriteDefault writes the default config to the given path.
func WriteDefault(path string) error {
	path = expandHome(path)

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
func Load(path string) (*Config, error) {
	path = expandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeConfigIOFailed, "reading config")
	}

	var raw rawConfig

	_, err = toml.Decode(string(data), &raw)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeSerializationFailed, "parsing config")
	}

	hooks, err := decodeHooks(raw.Hooks)
	if err != nil {
		return nil, derrors.Wrapf(err, derrors.CodeSerializationFailed, "decoding hooks")
	}

	cfg := &Config{
		Settings: raw.Settings,
		Hooks:    hooks,
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
		return nil, err
	}

	expandPaths(cfg)

	return cfg, nil
}

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

	if settings.PIDFile == "" {
		settings.PIDFile = "~/.local/share/mimi/mimi.pid"
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

	for kind, entries := range allHookEntries(cfg) {
		for i, e := range entries {
			if e.Run == "" {
				errs = append(errs, fmt.Sprintf("hooks.%s[%d]: run command is empty", kind, i))
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

func allHookEntries(cfg *Config) map[string][]HookEntry {
	return map[string][]HookEntry{
		string(events.AppActivate):                 cfg.Hooks.AppActivate,
		string(events.AppDeactivate):               cfg.Hooks.AppDeactivate,
		string(events.AppLaunch):                   cfg.Hooks.AppLaunch,
		string(events.AppQuit):                     cfg.Hooks.AppQuit,
		string(events.AppHide):                     cfg.Hooks.AppHide,
		string(events.AppUnhide):                   cfg.Hooks.AppUnhide,
		string(events.WindowFocus):                 cfg.Hooks.WindowFocus,
		string(events.WindowTitleChange):           cfg.Hooks.WindowTitleChange,
		string(events.WindowCreated):               cfg.Hooks.WindowCreated,
		string(events.WindowClosed):                cfg.Hooks.WindowClosed,
		string(events.SystemSleep):                 cfg.Hooks.SystemSleep,
		string(events.SystemWake):                  cfg.Hooks.SystemWake,
		string(events.ScreenLock):                  cfg.Hooks.ScreenLock,
		string(events.ScreenUnlock):                cfg.Hooks.ScreenUnlock,
		string(events.SystemShutdown):              cfg.Hooks.SystemShutdown,
		string(events.UserSessionEnd):              cfg.Hooks.UserSessionEnd,
		string(events.VolumeMount):                 cfg.Hooks.VolumeMount,
		string(events.VolumeUnmount):               cfg.Hooks.VolumeUnmount,
		string(events.ExternalDisplayConnected):    cfg.Hooks.ExternalDisplayConnected,
		string(events.ExternalDisplayDisconnected): cfg.Hooks.ExternalDisplayDisconnected,
		string(events.AppearanceChanged):           cfg.Hooks.AppearanceChanged,
		string(events.PowerAdapterConnected):       cfg.Hooks.PowerAdapterConnected,
		string(events.PowerAdapterDisconnected):    cfg.Hooks.PowerAdapterDisconnected,
		string(events.BatteryLow):                  cfg.Hooks.BatteryLow,
		string(events.BatteryCritical):             cfg.Hooks.BatteryCritical,
		string(events.AudioDeviceChanged):          cfg.Hooks.AudioDeviceChanged,
		string(events.WorkspaceChanged):            cfg.Hooks.WorkspaceChanged,
		string(events.USBDeviceConnected):          cfg.Hooks.USBDeviceConnected,
		string(events.USBDeviceDisconnected):       cfg.Hooks.USBDeviceDisconnected,
		string(events.NetworkUp):                   cfg.Hooks.NetworkUp,
		string(events.NetworkDown):                 cfg.Hooks.NetworkDown,
		string(events.ClipboardChanged):            cfg.Hooks.ClipboardChanged,
	}
}

func expandPaths(cfg *Config) {
	cfg.Settings.LogFile = expandHome(cfg.Settings.LogFile)
	cfg.Settings.PIDFile = expandHome(cfg.Settings.PIDFile)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
}
