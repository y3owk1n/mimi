package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/y3owk1n/mimi/configs"
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
		for i, e := range *kind.Entries(&cfg.Hooks) {
			if e.Run == "" {
				errs = append(
					errs,
					fmt.Sprintf("hooks.%s[%d]: run command is empty", kind.TOMLKey, i),
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

func expandPaths(cfg *Config) {
	cfg.Settings.LogFile = paths.ExpandHome(cfg.Settings.LogFile)
	cfg.Settings.PIDFile = paths.ExpandHome(cfg.Settings.PIDFile)
	cfg.Settings.SocketFile = paths.ExpandHome(cfg.Settings.SocketFile)
}
