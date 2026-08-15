package hooks //nolint:testpackage // tests unexported buildMap/compileGlob alongside the exported API

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
)

// Shared literals for hook entries and events built throughout this file.
const (
	testHookRun      = "true"
	testAppGlob      = "Fire*"
	testAppName      = "Firefox"
	testAppNameMiss  = "Terminal"
	testTitleRegex   = "^Inbox"
	testWindowTitle  = "Inbox - Mail"
	testWindowTitle2 = "Sent - Mail"
	testBundleGlob   = "org.example.*"
	testBundleIDHit  = "org.example.App"
	testBundleIDMiss = "com.other.App"
)

// allHookableKinds mirrors the twelve entries buildMap's literal map wires
// up. Kept local (rather than reusing events.AllKinds) so a future kind
// added to events.AllKinds without a matching buildMap entry fails loudly
// here instead of silently expanding both lists together.
var allHookableKinds = []events.EventKind{
	events.AppActivate,
	events.AppDeactivate,
	events.AppLaunch,
	events.AppQuit,
	events.AppHide,
	events.AppUnhide,
	events.WindowFocus,
	events.WindowTitleChange,
	events.WindowCreated,
	events.WindowClosed,
	events.WindowResize,
	events.WorkspaceChanged,
}

// cfgWithHook returns a *config.Config with a single hook entry registered
// for the given kind. Panics on an unknown kind since that's a test bug,
// not a runtime condition.
func cfgWithHook(kind events.EventKind, entry config.HookEntry) *config.Config {
	cfg := &config.Config{}

	switch kind { //nolint:exhaustive // events.WindowResizing has no HooksConfig field; it's internal-only
	case events.AppActivate:
		cfg.Hooks.AppActivate = []config.HookEntry{entry}
	case events.AppDeactivate:
		cfg.Hooks.AppDeactivate = []config.HookEntry{entry}
	case events.AppLaunch:
		cfg.Hooks.AppLaunch = []config.HookEntry{entry}
	case events.AppQuit:
		cfg.Hooks.AppQuit = []config.HookEntry{entry}
	case events.AppHide:
		cfg.Hooks.AppHide = []config.HookEntry{entry}
	case events.AppUnhide:
		cfg.Hooks.AppUnhide = []config.HookEntry{entry}
	case events.WindowFocus:
		cfg.Hooks.WindowFocus = []config.HookEntry{entry}
	case events.WindowTitleChange:
		cfg.Hooks.WindowTitleChange = []config.HookEntry{entry}
	case events.WindowCreated:
		cfg.Hooks.WindowCreated = []config.HookEntry{entry}
	case events.WindowClosed:
		cfg.Hooks.WindowClosed = []config.HookEntry{entry}
	case events.WindowResize:
		cfg.Hooks.WindowResize = []config.HookEntry{entry}
	case events.WorkspaceChanged:
		cfg.Hooks.WorkspaceChanged = []config.HookEntry{entry}
	default:
		panic("cfgWithHook: unknown kind " + string(kind))
	}

	return cfg
}

func TestBuildMap_EveryKindProducesHooks(t *testing.T) {
	t.Parallel()

	for _, kind := range allHookableKinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			cfg := cfgWithHook(kind, config.HookEntry{Run: testHookRun})

			hookMap, err := buildMap(cfg)
			if err != nil {
				t.Fatalf("buildMap() unexpected error: %v", err)
			}

			hooks, ok := hookMap[kind]
			if !ok || len(hooks) != 1 {
				t.Fatalf("buildMap()[%s] = %v (ok=%v), want exactly 1 hook", kind, hooks, ok)
			}

			if hooks[0].Entry.Run != testHookRun {
				t.Errorf("hook.Entry.Run = %q, want %q", hooks[0].Entry.Run, testHookRun)
			}
		})
	}
}

func TestBuildMap_InvalidTitleRegexReturnsError(t *testing.T) {
	t.Parallel()

	cfg := cfgWithHook(events.AppActivate, config.HookEntry{Run: testHookRun, Title: "["})

	hookMap, err := buildMap(cfg)
	if err == nil {
		t.Fatal("buildMap() expected error for invalid title regex, got nil")
	}

	if hookMap != nil {
		t.Errorf("buildMap() expected nil map on error, got %v", hookMap)
	}
}

func TestBuildMap_InvalidAppGlobReturnsError(t *testing.T) {
	t.Parallel()

	// compileGlob quotes the input with regexp.QuoteMeta before compiling,
	// which escapes every regex metacharacter into a valid literal (and
	// only unescapes "*" back into ".*", itself always valid), so ordinary
	// glob punctuation like "[" or "(" can never make the compiled pattern
	// invalid. The one input regexp.Compile rejects downstream of that
	// quoting is invalid UTF-8.
	cfg := cfgWithHook(events.AppActivate, config.HookEntry{Run: testHookRun, App: "\xff\xfe"})

	hookMap, err := buildMap(cfg)
	if err == nil {
		t.Fatal("buildMap() expected error for invalid app glob, got nil")
	}

	if hookMap != nil {
		t.Errorf("buildMap() expected nil map on error, got %v", hookMap)
	}
}

func TestBuildMap_ErrorReturnsNoPartialMap(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	// A valid hook on one kind and an invalid one on another: buildMap must
	// not return the valid half.
	cfg.Hooks.AppActivate = []config.HookEntry{{Run: testHookRun}}
	cfg.Hooks.AppQuit = []config.HookEntry{{Run: testHookRun, Title: "("}}

	hookMap, err := buildMap(cfg)
	if err == nil {
		t.Fatal("buildMap() expected error, got nil")
	}

	if hookMap != nil {
		t.Errorf("buildMap() expected nil map on error, got %v", hookMap)
	}
}

func TestHookMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry config.HookEntry
		evt   events.Event
		want  bool
	}{
		{
			name:  "empty filters match everything",
			entry: config.HookEntry{Run: testHookRun},
			evt: events.Event{
				AppName:     "Anything",
				BundleID:    "com.anything",
				WindowTitle: "anything",
			},
			want: true,
		},
		{
			name:  "app glob hit",
			entry: config.HookEntry{Run: testHookRun, App: testAppGlob},
			evt:   events.Event{AppName: testAppName},
			want:  true,
		},
		{
			name:  "app glob miss",
			entry: config.HookEntry{Run: testHookRun, App: testAppGlob},
			evt:   events.Event{AppName: testAppNameMiss},
			want:  false,
		},
		{
			name:  "title regexp hit",
			entry: config.HookEntry{Run: testHookRun, Title: testTitleRegex},
			evt:   events.Event{WindowTitle: testWindowTitle},
			want:  true,
		},
		{
			name:  "title regexp miss",
			entry: config.HookEntry{Run: testHookRun, Title: testTitleRegex},
			evt:   events.Event{WindowTitle: testWindowTitle2},
			want:  false,
		},
		{
			name:  "bundle_id glob hit",
			entry: config.HookEntry{Run: testHookRun, BundleID: testBundleGlob},
			evt:   events.Event{BundleID: testBundleIDHit},
			want:  true,
		},
		{
			name:  "bundle_id glob miss",
			entry: config.HookEntry{Run: testHookRun, BundleID: testBundleGlob},
			evt:   events.Event{BundleID: testBundleIDMiss},
			want:  false,
		},
		{
			name:  "app and title filters both match",
			entry: config.HookEntry{Run: testHookRun, App: testAppGlob, Title: testTitleRegex},
			evt:   events.Event{AppName: testAppName, WindowTitle: testWindowTitle},
			want:  true,
		},
		{
			name:  "app matches but title does not: overall miss",
			entry: config.HookEntry{Run: testHookRun, App: testAppGlob, Title: testTitleRegex},
			evt:   events.Event{AppName: testAppName, WindowTitle: testWindowTitle2},
			want:  false,
		},
		{
			name:  "title matches but app does not: overall miss",
			entry: config.HookEntry{Run: testHookRun, App: testAppGlob, Title: testTitleRegex},
			evt:   events.Event{AppName: testAppNameMiss, WindowTitle: testWindowTitle},
			want:  false,
		},
	}

	for _, matchCase := range tests {
		t.Run(matchCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := cfgWithHook(events.AppActivate, matchCase.entry)

			hookMap, err := buildMap(cfg)
			if err != nil {
				t.Fatalf("buildMap() unexpected error: %v", err)
			}

			hook := hookMap[events.AppActivate][0]

			got, reason := hook.Matches(matchCase.evt)
			if got != matchCase.want {
				t.Errorf("Matches() = %v (reason=%q), want %v", got, reason, matchCase.want)
			}

			if got && reason != "" {
				t.Errorf("Matches() returned reason %q on a match, want empty", reason)
			}

			if !got && reason == "" {
				t.Error("Matches() returned empty reason on a mismatch, want a reason string")
			}
		})
	}
}

func TestRegistry_ReloadReplacesMapWholesale(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	firstCfg := cfgWithHook(events.AppActivate, config.HookEntry{Run: "first"})

	err := reg.Reload(firstCfg)
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	if hooks := reg.HooksFor(events.AppActivate); len(hooks) != 1 || hooks[0].Entry.Run != "first" {
		t.Fatalf("HooksFor(AppActivate) after first reload = %v, want [first]", hooks)
	}

	secondCfg := cfgWithHook(events.AppQuit, config.HookEntry{Run: "second"})

	err = reg.Reload(secondCfg)
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	// The second config has no AppActivate hooks: reload must have replaced
	// the map wholesale rather than merging into the old one.
	if hooks := reg.HooksFor(events.AppActivate); len(hooks) != 0 {
		t.Errorf(
			"HooksFor(AppActivate) after second reload = %v, want empty (map replaced, not merged)",
			hooks,
		)
	}

	if hooks := reg.HooksFor(events.AppQuit); len(hooks) != 1 || hooks[0].Entry.Run != "second" {
		t.Fatalf("HooksFor(AppQuit) after second reload = %v, want [second]", hooks)
	}
}

func TestRegistry_FailedReloadLeavesPriorMapIntact(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	goodCfg := cfgWithHook(events.AppActivate, config.HookEntry{Run: "good"})

	err := reg.Reload(goodCfg)
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	badCfg := cfgWithHook(events.AppActivate, config.HookEntry{Run: "bad", Title: "("})

	err = reg.Reload(badCfg)
	if err == nil {
		t.Fatal("Reload() expected error for invalid title regex, got nil")
	}

	hooks := reg.HooksFor(events.AppActivate)
	if len(hooks) != 1 || hooks[0].Entry.Run != "good" {
		t.Fatalf(
			"HooksFor(AppActivate) after failed reload = %v, want the prior map [good] intact",
			hooks,
		)
	}
}

func TestRegistry_HooksForUnknownKindReturnsEmptyNotPanic(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	err := reg.Reload(
		cfgWithHook(events.AppActivate, config.HookEntry{Run: testHookRun}),
	)
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	hooks := reg.HooksFor(events.EventKind("does_not_exist"))
	if len(hooks) != 0 {
		t.Errorf("HooksFor(unknown) = %v, want empty", hooks)
	}
}

func TestRegistry_HooksForOnFreshRegistryReturnsEmptyNotPanic(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	hooks := reg.HooksFor(events.AppActivate)
	if len(hooks) != 0 {
		t.Errorf("HooksFor() on fresh registry = %v, want empty", hooks)
	}
}

func TestRegistry_KindFilter(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	err := reg.Reload(
		cfgWithHook(events.AppActivate, config.HookEntry{Run: testHookRun}),
	)
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	filter := reg.KindFilter()

	if !filter(events.AppActivate) {
		t.Error("KindFilter()(AppActivate) = false, want true: registry has a hook for this kind")
	}

	if filter(events.AppQuit) {
		t.Error("KindFilter()(AppQuit) = true, want false: registry has no hook for this kind")
	}

	if filter(events.EventKind("does_not_exist")) {
		t.Error("KindFilter()(unknown kind) = true, want false")
	}
}

func TestRegistry_KindFilterReflectsReload(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	err := reg.Reload(
		cfgWithHook(events.AppActivate, config.HookEntry{Run: testHookRun}),
	)
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	filter := reg.KindFilter()
	if !filter(events.AppActivate) {
		t.Fatal("KindFilter()(AppActivate) = false before reload, want true")
	}

	// Reload to a config with no hooks for AppActivate; the filter closure
	// reads the registry live, so it must reflect the new (empty) map.
	err = reg.Reload(&config.Config{})
	if err != nil {
		t.Fatalf("Reload() unexpected error: %v", err)
	}

	if filter(events.AppActivate) {
		t.Error("KindFilter()(AppActivate) = true after reload dropped the hook, want false")
	}
}
