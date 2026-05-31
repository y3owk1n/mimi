package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/y3owk1n/mimi/internal/events"
)

const DefaultConfigPath = "~/.config/mimi/config.toml"

func Exists(path string) bool {
	_, err := os.Stat(expandHome(path))
	return err == nil
}

func WriteDefault(path string) error {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(DefaultConfig), 0644); err != nil {
		return fmt.Errorf("writing default config: %w", err)
	}
	return nil
}

func Load(path string) (*Config, error) {
	path = expandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var raw rawConfig
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	hooks, err := decodeHooks(raw.Hooks)
	if err != nil {
		return nil, fmt.Errorf("decoding hooks: %w", err)
	}

	cfg := &Config{
		Settings: raw.Settings,
		Hooks:    hooks,
	}
	applyDefaults(cfg)
	if err := validate(cfg); err != nil {
		return nil, err
	}
	expandPaths(cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	s := &cfg.Settings
	if s.LogFile == "" {
		s.LogFile = "~/.local/share/mimi/mimi.log"
	}
	if s.LogLevel == "" {
		s.LogLevel = "info"
	}
	if s.LogFormat == "" {
		s.LogFormat = "text"
	}
	if s.HookTimeoutSecs == 0 {
		s.HookTimeoutSecs = 10
	}
	if s.HookShell == "" {
		s.HookShell = "/bin/sh"
	}
	if s.MaxHookWorkers == 0 {
		s.MaxHookWorkers = 4
	}
	if s.PIDFile == "" {
		s.PIDFile = "~/.local/share/mimi/mimi.pid"
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
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func allHookEntries(cfg *Config) map[string][]HookEntry {
	return map[string][]HookEntry{
		string(events.AppActivate):       cfg.Hooks.AppActivate,
		string(events.AppDeactivate):     cfg.Hooks.AppDeactivate,
		string(events.AppLaunch):         cfg.Hooks.AppLaunch,
		string(events.AppQuit):           cfg.Hooks.AppQuit,
		string(events.AppHide):           cfg.Hooks.AppHide,
		string(events.AppUnhide):         cfg.Hooks.AppUnhide,
		string(events.WindowFocus):       cfg.Hooks.WindowFocus,
		string(events.WindowTitleChange): cfg.Hooks.WindowTitleChange,
		string(events.WindowCreated):     cfg.Hooks.WindowCreated,
		string(events.WindowClosed):      cfg.Hooks.WindowClosed,
		string(events.SystemSleep):       cfg.Hooks.SystemSleep,
		string(events.SystemWake):        cfg.Hooks.SystemWake,
		string(events.ScreenLock):        cfg.Hooks.ScreenLock,
		string(events.ScreenUnlock):      cfg.Hooks.ScreenUnlock,
		string(events.SystemShutdown):    cfg.Hooks.SystemShutdown,
		string(events.VolumeMount):       cfg.Hooks.VolumeMount,
		string(events.VolumeUnmount):     cfg.Hooks.VolumeUnmount,
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
