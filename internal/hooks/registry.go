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
	Entry config.HookEntry
	title filter
	app   filter
	// bundle is the compiled glob from Entry.BundleID. Glob is used for
	// symmetry with Entry.App.
	bundle filter
	// space is the space index the hook is filtered to, in the form
	// workspace events carry it, or "" for no filter.
	space        string
	spaceNegated bool
}

// filter is one compiled pattern filter: nil when the entry set none, and
// inverted when the entry negated it.
type filter struct {
	re      *regexp.Regexp
	negated bool
}

// matches reports whether the filter admits value. An unset filter, or the
// catch-all "*", admits everything; negated, the catch-all admits nothing.
func (f filter) matches(value string) bool {
	if f.re == nil {
		return !f.negated
	}

	return f.re.MatchString(value) != f.negated
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
	if !h.app.matches(evt.AppName) {
		return false, "app filter mismatch"
	}

	if !h.bundle.matches(evt.BundleID) {
		return false, "bundle_id filter mismatch"
	}

	if !h.title.matches(evt.WindowTitle) {
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

			hook.title, err = compileFilter(entry.Title, regexp.Compile)
			if err != nil {
				return nil, err
			}

			hook.app, err = compileFilter(entry.App, compileGlob)
			if err != nil {
				return nil, err
			}

			hook.bundle, err = compileFilter(entry.BundleID, compileGlob)
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

// compileFilter compiles one pattern filter with the given compiler, reading
// the negation prefix off it first. An empty pattern is no filter.
func compileFilter(
	pattern string,
	compile func(string) (*regexp.Regexp, error),
) (filter, error) {
	if pattern == "" {
		return filter{}, nil
	}

	rest, negated := config.SplitNegation(pattern)

	re, err := compile(rest)
	if err != nil {
		return filter{}, err
	}

	return filter{re: re, negated: negated}, nil
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
