package config //nolint:testpackage // exercises unexported decodeHooks directly

// decodeHooks reads the [hooks] table out of a map keyed by whatever the user
// typed, so the shape the TOML decoder hands it is load-bearing in a way the
// compiler cannot check: an inline array arrives as []any, [[hooks.on_x]]
// tables arrive as []map[string]any, and an unrecognized key can hold
// anything at all. Getting that wrong silently drops hooks -- which is the
// failure mode this whole table exists to prevent.

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// decodeTOMLHooks runs src through the same decode path Load uses and returns
// the resulting hooks plus any error.
func decodeTOMLHooks(t *testing.T, src string) (HooksConfig, error) {
	t.Helper()

	var raw rawConfig

	_, err := toml.Decode(src, &raw)
	if err != nil {
		t.Fatalf("toml.Decode rejected the fixture: %v", err)
	}

	return decodeHooks(raw.Hooks)
}

func TestDecodeHooks_AcceptsEveryHookSpelling(t *testing.T) {
	t.Parallel()

	hooks, err := decodeTOMLHooks(t, `
[hooks]
on_app_activate = ["echo inline-string", { run = "echo inline-table", app = "Safari" }]

[[hooks.on_app_quit]]
run = "echo array-of-tables"
async = true
`)
	if err != nil {
		t.Fatalf("decodeHooks: %v", err)
	}

	if len(hooks.AppActivate) != 2 {
		t.Errorf("on_app_activate: got %d entries, want 2", len(hooks.AppActivate))
	}

	if hooks.AppActivate[0].Run != "echo inline-string" {
		t.Errorf("inline string entry: got %q", hooks.AppActivate[0].Run)
	}

	if hooks.AppActivate[1].App != "Safari" {
		t.Errorf("inline table entry lost its app filter: got %q", hooks.AppActivate[1].App)
	}

	// [[hooks.on_x]] decodes to []map[string]any rather than []any. A decoder
	// that only understands []any drops these entirely and reports success.
	if len(hooks.AppQuit) != 1 {
		t.Fatalf(
			"on_app_quit ([[hooks.on_app_quit]] form): got %d entries, want 1",
			len(hooks.AppQuit),
		)
	}

	if hooks.AppQuit[0].Run != "echo array-of-tables" {
		t.Errorf("array-of-tables entry: got %q", hooks.AppQuit[0].Run)
	}

	if !hooks.AppQuit[0].Async {
		t.Error("array-of-tables entry lost async = true")
	}
}

func TestDecodeHooks_ToleratesUnrecognizedKeysWhateverTheyHold(t *testing.T) {
	t.Parallel()

	// An unrecognized hook key is currently ignored, whatever its value. That
	// is a real defect -- the user's typo'd hook never fires and nothing says
	// so -- but rejecting it is a user-visible change reserved for a separate
	// fix. This test pins the current behavior so the decoder does not start
	// hard-failing on it by accident.
	for _, value := range []string{`"echo x"`, `42`, `{ run = "echo x" }`, `["echo x"]`, `true`} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			hooks, err := decodeTOMLHooks(
				t,
				"[hooks]\non_app_activate = [\"echo real\"]\non_bogus = "+value+"\n",
			)
			if err != nil {
				t.Fatalf("unrecognized key %s should be ignored, got: %v", value, err)
			}

			if len(hooks.AppActivate) != 1 {
				t.Errorf(
					"recognized hook alongside %s: got %d entries, want 1",
					value,
					len(hooks.AppActivate),
				)
			}
		})
	}
}

func TestDecodeHooks_RejectsARecognizedKeyThatIsNotAList(t *testing.T) {
	t.Parallel()

	_, err := decodeTOMLHooks(t, "[hooks]\non_app_activate = \"echo not-a-list\"\n")
	if err == nil {
		t.Fatal("expected an error for a non-list hook value, got nil")
	}

	if !strings.Contains(err.Error(), "on_app_activate") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
}

func TestDecodeHooks_AbsentKeysDecodeToNothing(t *testing.T) {
	t.Parallel()

	hooks, err := decodeTOMLHooks(t, "[hooks]\n")
	if err != nil {
		t.Fatalf("an empty [hooks] table is valid: %v", err)
	}

	for _, kind := range HookKinds {
		if entries := *kind.Entries(&hooks); len(entries) != 0 {
			t.Errorf("%s: got %d entries from an empty table, want 0", kind.TOMLKey, len(entries))
		}
	}
}
