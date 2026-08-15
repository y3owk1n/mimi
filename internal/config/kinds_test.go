package config //nolint:testpackage // asserts HookKinds against unexported decode internals

// HookKinds is a hand-written slice, so a kind added to HooksConfig without a
// matching row compiles fine and then silently never fires -- the same failure
// mode that once left half of a user's hooks uncounted. These tests are the
// check the compiler cannot give: they pin the table against the two
// independent enumerations of the same twelve kinds, HooksConfig's fields and
// events.AllKinds.
//
// internal/hooks and internal/observe deliberately keep their own local kind
// lists in tests for the same reason; folding those onto HookKinds would leave
// the table checking itself.

import (
	"reflect"
	"slices"
	"testing"

	"github.com/y3owk1n/mimi/internal/events"
)

// runTrue is a harmless hook command; these tests only care that an entry
// exists, never what it runs.
const runTrue = "true"

func TestHookKinds_CoversEveryHooksConfigField(t *testing.T) {
	t.Parallel()

	hooksType := reflect.TypeFor[HooksConfig]()

	if len(HookKinds) != hooksType.NumField() {
		t.Fatalf(
			"HookKinds has %d rows but HooksConfig has %d fields; a kind was added to one and not the other",
			len(HookKinds),
			hooksType.NumField(),
		)
	}

	// Every row must point at a distinct field, so a copy-pasted row that
	// forgot to change its accessor fails here rather than shadowing a kind.
	var cfg HooksConfig

	seen := make(map[*[]HookEntry]events.EventKind, len(HookKinds))

	for _, kind := range HookKinds {
		ptr := kind.Entries(&cfg)
		if other, dup := seen[ptr]; dup {
			t.Errorf("%q and %q resolve to the same HooksConfig field", kind.Kind, other)
		}

		seen[ptr] = kind.Kind
	}
}

func TestHookKinds_RowOrderMatchesHooksConfigFieldOrder(t *testing.T) {
	t.Parallel()

	// The order is user-visible: hook errors are reported in it. Pin it to
	// HooksConfig's field order, which is also the order of
	// configs/default-config.toml and docs/CONFIGURATION.md.
	hooksType := reflect.TypeFor[HooksConfig]()

	for idx, kind := range HookKinds {
		if idx >= hooksType.NumField() {
			break
		}

		field := hooksType.Field(idx)

		got := field.Tag.Get("toml")
		if got != kind.TOMLKey {
			t.Errorf(
				"HookKinds[%d] is %q but HooksConfig field %d (%s) has toml tag %q",
				idx, kind.TOMLKey, idx, field.Name, got,
			)
		}
	}
}

func TestHookKinds_CoversEveryHookableEventKind(t *testing.T) {
	t.Parallel()

	rows := make(map[events.EventKind]struct{}, len(HookKinds))
	for _, kind := range HookKinds {
		rows[kind.Kind] = struct{}{}
	}

	for _, kind := range events.AllKinds {
		if _, ok := rows[kind]; !ok {
			t.Errorf("events.AllKinds has %q, which has no row in HookKinds", kind)
		}
	}

	for kind := range rows {
		found := slices.Contains(events.AllKinds, kind)

		if !found {
			t.Errorf("HookKinds has %q, which is not in events.AllKinds", kind)
		}
	}
}

func TestHookKinds_TOMLKeysAreUniqueAndPrefixed(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, len(HookKinds))

	for _, kind := range HookKinds {
		if _, dup := seen[kind.TOMLKey]; dup {
			t.Errorf("duplicate TOML key %q", kind.TOMLKey)
		}

		seen[kind.TOMLKey] = struct{}{}

		if want := "on_" + string(kind.Kind); want != kind.TOMLKey {
			t.Errorf("kind %q has TOML key %q, want %q", kind.Kind, kind.TOMLKey, want)
		}
	}
}

func TestHooksConfig_HasGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		hooks HooksConfig
		group HookGroup
		want  bool
	}{
		{"empty config, app group", HooksConfig{}, GroupApp, false},
		{"empty config, window group", HooksConfig{}, GroupWindow, false},
		{
			"app hook counts for app group",
			HooksConfig{AppActivate: []HookEntry{{Run: runTrue}}},
			GroupApp,
			true,
		},
		{
			"app hook does not count for window group",
			HooksConfig{AppActivate: []HookEntry{{Run: runTrue}}},
			GroupWindow,
			false,
		},
		{
			"window resize counts for window group",
			HooksConfig{WindowResize: []HookEntry{{Run: runTrue}}},
			GroupWindow,
			true,
		},
		{
			"workspace hook counts for workspace group",
			HooksConfig{WorkspaceChanged: []HookEntry{{Run: runTrue}}},
			GroupWorkspace,
			true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hooks := testCase.hooks
			if got := hooks.HasGroup(testCase.group); got != testCase.want {
				t.Errorf("HasGroup(%v) = %v, want %v", testCase.group, got, testCase.want)
			}
		})
	}
}
