package hooks

import (
	"regexp"
	"sync"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

type Hook struct {
	Entry       config.HookEntry
	titleRegexp *regexp.Regexp
}

type Registry struct {
	mu sync.RWMutex
	m  map[events.EventKind][]Hook
}

func NewRegistry() *Registry {
	return &Registry{m: make(map[events.EventKind][]Hook)}
}

func (r *Registry) Reload(cfg *config.Config) error {
	m, err := buildMap(cfg)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.m = m
	r.mu.Unlock()
	return nil
}

func (r *Registry) HooksFor(kind events.EventKind) []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[kind]
}

func (h *Hook) Matches(e events.Event) bool {
	if h.Entry.App != "" {
		if !globMatch(h.Entry.App, e.AppName) {
			return false
		}
	}
	if h.Entry.BundleID != "" && h.Entry.BundleID != e.BundleID {
		return false
	}
	if h.titleRegexp != nil && !h.titleRegexp.MatchString(e.WindowTitle) {
		return false
	}
	return true
}

func buildMap(cfg *config.Config) (map[events.EventKind][]Hook, error) {
	m := make(map[events.EventKind][]Hook)
	entries := map[events.EventKind][]config.HookEntry{
		events.AppActivate:       cfg.Hooks.AppActivate,
		events.AppDeactivate:     cfg.Hooks.AppDeactivate,
		events.AppLaunch:         cfg.Hooks.AppLaunch,
		events.AppQuit:           cfg.Hooks.AppQuit,
		events.AppHide:           cfg.Hooks.AppHide,
		events.AppUnhide:         cfg.Hooks.AppUnhide,
		events.WindowFocus:       cfg.Hooks.WindowFocus,
		events.WindowTitleChange: cfg.Hooks.WindowTitleChange,
		events.WindowCreated:     cfg.Hooks.WindowCreated,
		events.WindowClosed:      cfg.Hooks.WindowClosed,
		events.SystemSleep:       cfg.Hooks.SystemSleep,
		events.SystemWake:        cfg.Hooks.SystemWake,
		events.ScreenLock:        cfg.Hooks.ScreenLock,
		events.ScreenUnlock:      cfg.Hooks.ScreenUnlock,
		events.SystemShutdown:    cfg.Hooks.SystemShutdown,
		events.VolumeMount:       cfg.Hooks.VolumeMount,
		events.VolumeUnmount:     cfg.Hooks.VolumeUnmount,
	}

	for kind, entries := range entries {
		var hooks []Hook
		for _, e := range entries {
			hook := Hook{Entry: e}
			if e.Title != "" {
				re, err := regexp.Compile(e.Title)
				if err != nil {
					return nil, err
				}
				hook.titleRegexp = re
			}
			hooks = append(hooks, hook)
		}
		if len(hooks) > 0 {
			m[kind] = hooks
		}
	}
	return m, nil
}

func globMatch(pattern, s string) bool {
	if pattern == "" {
		return true
	}
	if pattern == "*" {
		return true
	}
	// Simple glob: support * wildcard
	re := regexp.QuoteMeta(pattern)
	re = stringsReplaceAll(re, "\\*", ".*")
	matched, _ := regexp.MatchString("^"+re+"$", s)
	return matched
}

func stringsReplaceAll(s, old, new string) string {
	var result []byte
	for i := 0; i < len(s); {
		if s[i] == old[0] && i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result = append(result, []byte(new)...)
			i += len(old)
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return string(result)
}
