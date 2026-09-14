package hooks

import (
	"sync"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// Hook wraps a HookEntry with its compiled title regex and any precomputed
// filter regexes so that per-event matching is allocation-free.
type Hook struct {
	Entry config.HookEntry
	title config.Filter
	app   config.Filter
	// bundle is the compiled glob from Entry.BundleID. Glob is used for
	// symmetry with Entry.App.
	bundle config.Filter
	// space is the space index the hook is filtered to, in the form
	// workspace events carry it, or "" for no filter.
	space        string
	spaceNegated bool
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

// Matches checks whether a hook's filters (app, bundle_id, title, space)
// match an event.
//
// The space filter reads the index a workspace event carries. An event that
// carries none, because Mission Control could not be enumerated when it was
// published, matches no positive space filter and every negated one: the
// event does not say it is on that space.
func (h *Hook) Matches(evt events.Event) (bool, string) {
	if !h.app.Matches(evt.AppName) {
		return false, "app filter mismatch"
	}

	if !h.bundle.Matches(evt.BundleID) {
		return false, "bundle_id filter mismatch"
	}

	if !h.title.Matches(evt.WindowTitle) {
		return false, "title filter mismatch"
	}

	if h.space != "" && (evt.Extra["space_index"] == h.space) == h.spaceNegated {
		return false, "space filter mismatch"
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

			var err error

			hook.title, err = config.CompileRegexpFilter(entry.Title)
			if err != nil {
				return nil, err
			}

			hook.app, err = config.CompileGlobFilter(entry.App)
			if err != nil {
				return nil, err
			}

			hook.bundle, err = config.CompileGlobFilter(entry.BundleID)
			if err != nil {
				return nil, err
			}

			if entry.Space != "" {
				hook.space, hook.spaceNegated, err = config.ParseSpaceFilter(entry.Space)
				if err != nil {
					return nil, err
				}
			}

			hooks = append(hooks, hook)
		}

		if len(hooks) > 0 {
			hookMap[kind.Kind] = hooks
		}
	}

	return hookMap, nil
}
