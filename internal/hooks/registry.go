package hooks

import (
	"regexp"
	"strings"
	"sync"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// Hook wraps a HookEntry with its compiled title regex and any precomputed
// filter regexes so that per-event matching is allocation-free.
type Hook struct {
	Entry       config.HookEntry
	titleRegexp *regexp.Regexp
	// appRegexp is the compiled glob from Entry.App, or nil when not set.
	appRegexp *regexp.Regexp
	// bundleRegexp is the compiled glob from Entry.BundleID, or nil when
	// not set. Glob is used for symmetry with Entry.App.
	bundleRegexp *regexp.Regexp
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

// KindFilter returns a predicate that reports whether the registry has at
// least one hook registered for a given event kind. It is intended for use
// with events.Bus.SubscribeWithFilter so the bus can drop events that the
// executor would otherwise ignore.
func (r *Registry) KindFilter() events.KindFilter {
	return func(kind events.EventKind) bool {
		r.mu.RLock()
		defer r.mu.RUnlock()

		return len(r.m[kind]) > 0
	}
}

// Matches checks whether a hook's filters (app, bundle_id, title) match an event.
func (h *Hook) Matches(evt events.Event) (bool, string) {
	if h.appRegexp != nil && !h.appRegexp.MatchString(evt.AppName) {
		return false, "app filter mismatch"
	}

	if h.bundleRegexp != nil && !h.bundleRegexp.MatchString(evt.BundleID) {
		return false, "bundle_id filter mismatch"
	}

	if h.titleRegexp != nil && !h.titleRegexp.MatchString(evt.WindowTitle) {
		return false, "title filter mismatch"
	}

	return true, ""
}

func buildMap(cfg *config.Config) (map[events.EventKind][]Hook, error) {
	hookMap := make(map[events.EventKind][]Hook)

	// Folding over config.HookKinds rather than a locally built map also
	// settles which error a config with several bad patterns reports. This
	// returns on the first failure; ranging a map made "first" mean first in
	// the map's random iteration order, so the same config could blame a
	// different pattern on each run. It is now the first one in the file.
	for _, kind := range config.HookKinds {
		var hooks []Hook
		for _, entry := range *kind.Entries(&cfg.Hooks) {
			hook := Hook{Entry: entry}
			if entry.Title != "" {
				re, err := regexp.Compile(entry.Title)
				if err != nil {
					return nil, err
				}

				hook.titleRegexp = re
			}

			if entry.App != "" {
				re, err := compileGlob(entry.App)
				if err != nil {
					return nil, err
				}

				hook.appRegexp = re
			}

			if entry.BundleID != "" {
				re, err := compileGlob(entry.BundleID)
				if err != nil {
					return nil, err
				}

				hook.bundleRegexp = re
			}

			hooks = append(hooks, hook)
		}

		if len(hooks) > 0 {
			hookMap[kind.Kind] = hooks
		}
	}

	return hookMap, nil
}

// compileGlob converts a glob-style pattern (with `*` wildcards) to an
// anchored *regexp.Regexp. Returns nil and a nil error for empty input or
// the catch-all "*", so callers can use a single `if re != nil` check.
func compileGlob(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || pattern == "*" {
		return nil, nil //nolint:nilnil // intentional: signals "no filter"
	}

	quoted := regexp.QuoteMeta(pattern)
	// QuoteMeta escapes `*` to `\*`; convert it back to the regex wildcard.
	body := strings.ReplaceAll(quoted, `\*`, ".*")

	return regexp.Compile("^" + body + "$")
}
