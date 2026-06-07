package hooks

import (
	"regexp"
	"sync"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// Hook wraps a HookEntry with its compiled title regex.
type Hook struct {
	Entry       config.HookEntry
	titleRegexp *regexp.Regexp
}

// Registry maps event kinds to their registered hooks.
type Registry struct {
	mu sync.RWMutex
	m  map[events.EventKind][]Hook
}

// NewRegistry creates an empty hook registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[events.EventKind][]Hook)}
}

// Reload rebuilds the hook map from a config.
func (r *Registry) Reload(cfg *config.Config) error {
	hookMap, err := buildMap(cfg)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.m = hookMap
	r.mu.Unlock()

	return nil
}

// HooksFor returns all hooks registered for the given event kind.
func (r *Registry) HooksFor(kind events.EventKind) []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.m[kind]
}

// Matches checks whether a hook's filters (app, bundle_id, title) match an event.
func (h *Hook) Matches(evt events.Event) (bool, string) {
	if h.Entry.App != "" {
		if !globMatch(h.Entry.App, evt.AppName) {
			return false, "app filter mismatch"
		}
	}

	if h.Entry.BundleID != "" && h.Entry.BundleID != evt.BundleID {
		return false, "bundle_id filter mismatch"
	}

	if h.titleRegexp != nil && !h.titleRegexp.MatchString(evt.WindowTitle) {
		return false, "title filter mismatch"
	}

	return true, ""
}

func buildMap(cfg *config.Config) (map[events.EventKind][]Hook, error) {
	hookMap := make(map[events.EventKind][]Hook)
	entries := map[events.EventKind][]config.HookEntry{
		events.WindowFocus:       cfg.Hooks.WindowFocus,
		events.WindowTitleChange: cfg.Hooks.WindowTitleChange,
		events.WindowCreated:     cfg.Hooks.WindowCreated,
		events.WindowClosed:      cfg.Hooks.WindowClosed,
		events.WindowResize:      cfg.Hooks.WindowResize,
		events.WorkspaceChanged:  cfg.Hooks.WorkspaceChanged,
	}

	for kind, hookEntries := range entries {
		var hooks []Hook
		for _, e := range hookEntries {
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
			hookMap[kind] = hooks
		}
	}

	return hookMap, nil
}

func globMatch(pattern, str string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}

	re := regexp.QuoteMeta(pattern)
	re = stringsReplaceAll(re, "\\*", ".*")
	matched, _ := regexp.MatchString("^"+re+"$", str)

	return matched
}

func stringsReplaceAll(str, old, replacement string) string {
	var result []byte
	for idx := 0; idx < len(str); {
		if str[idx] == old[0] && idx+len(old) <= len(str) && str[idx:idx+len(old)] == old {
			result = append(result, []byte(replacement)...)
			idx += len(old)
		} else {
			result = append(result, str[idx])
			idx++
		}
	}

	return string(result)
}
