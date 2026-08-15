package config //nolint:testpackage // exercises unexported decodeHooks directly

// decodeHooks reads the [hooks] table out of a map keyed by whatever the user
// typed, so the shape the TOML decoder hands it is load-bearing in a way the
// compiler cannot check: an inline array arrives as []any, [[hooks.on_x]]
// tables arrive as []map[string]any, and an unrecognized key can hold
// anything at all. Getting that wrong silently drops hooks -- which is the
// failure mode this whole table exists to prevent.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// decodeTOMLHooks runs src through the same decode path Load uses and returns
// the resulting hooks, the keys it did not recognize, and any error.
func decodeTOMLHooks(t *testing.T, src string) (HooksConfig, []string, error) {
	t.Helper()

	var raw rawConfig

	_, err := toml.Decode(src, &raw)
	if err != nil {
		t.Fatalf("toml.Decode rejected the fixture: %v", err)
	}

	return decodeHooks(raw.Hooks)
}

// writeConfig writes src to a temp file and returns its path, for tests that
// need to go through Load rather than decodeHooks directly.
func writeConfig(t *testing.T, src string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")

	err := os.WriteFile(path, []byte(src), 0o600)
	if err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	return path
}

func TestDecodeHooks_AcceptsEveryHookSpelling(t *testing.T) {
	t.Parallel()

	hooks, unknown, err := decodeTOMLHooks(t, `
[hooks]
on_app_activate = ["echo inline-string", { run = "echo inline-table", app = "Safari" }]

[[hooks.on_app_quit]]
run = "echo array-of-tables"
async = true
`)
	if err != nil {
		t.Fatalf("decodeHooks: %v", err)
	}

	if len(unknown) != 0 {
		t.Errorf("recognized keys reported as unknown: %v", unknown)
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

func TestDecodeHooks_ReportsUnrecognizedKeysWhateverTheyHold(t *testing.T) {
	t.Parallel()

	// An unrecognized key is named, not rejected here: decoding records it and
	// leaves the decision to the caller, so the daemon can carry on while
	// validate fails. Its value is irrelevant -- the key is wrong whatever it
	// holds, and decoding must not choke on the shape.
	for _, value := range []string{`"echo x"`, `42`, `{ run = "echo x" }`, `["echo x"]`, `true`} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			hooks, unknown, err := decodeTOMLHooks(
				t,
				"[hooks]\non_app_activate = [\"echo real\"]\non_bogus = "+value+"\n",
			)
			if err != nil {
				t.Fatalf("an unrecognized key should not fail decoding, got: %v", err)
			}

			if len(unknown) != 1 || unknown[0] != "on_bogus" {
				t.Errorf("unknown keys: got %v, want [on_bogus]", unknown)
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

func TestDecodeHooks_SortsUnrecognizedKeys(t *testing.T) {
	t.Parallel()

	_, unknown, err := decodeTOMLHooks(t, `
[hooks]
on_zebra = ["echo z"]
on_apple = ["echo a"]
on_mango = ["echo m"]
`)
	if err != nil {
		t.Fatalf("decodeHooks: %v", err)
	}

	want := []string{"on_apple", "on_mango", "on_zebra"}
	if len(unknown) != len(want) {
		t.Fatalf("unknown keys: got %v, want %v", unknown, want)
	}

	for i, key := range want {
		if unknown[i] != key {
			t.Fatalf("unknown keys are not sorted: got %v, want %v", unknown, want)
		}
	}
}

func TestDecodeHooks_RejectsARecognizedKeyThatIsNotAList(t *testing.T) {
	t.Parallel()

	_, _, err := decodeTOMLHooks(t, "[hooks]\non_app_activate = \"echo not-a-list\"\n")
	if err == nil {
		t.Fatal("expected an error for a non-list hook value, got nil")
	}

	if !strings.Contains(err.Error(), "hooks.on_app_activate") {
		t.Errorf("error should name the offending key as the user typed it, got: %v", err)
	}
}

func TestDecodeHooks_LeavesTheEmptyCommandCheckToValidate(t *testing.T) {
	t.Parallel()

	// Decoding used to reject a table with no run field while a bare empty
	// string fell through to validate, so one mistake had two messages and two
	// error codes depending on how it was written. Decoding now says nothing
	// about it.
	_, _, err := decodeTOMLHooks(t, "[hooks]\non_app_activate = [{ app = \"Safari\" }]\n")
	if err != nil {
		t.Fatalf("decoding should not judge a missing run command, got: %v", err)
	}
}

func TestDecodeHooks_AbsentKeysDecodeToNothing(t *testing.T) {
	t.Parallel()

	hooks, unknown, err := decodeTOMLHooks(t, "[hooks]\n")
	if err != nil {
		t.Fatalf("an empty [hooks] table is valid: %v", err)
	}

	if len(unknown) != 0 {
		t.Errorf("empty table reported unknown keys: %v", unknown)
	}

	for _, kind := range HookKinds {
		if entries := *kind.Entries(&hooks); len(entries) != 0 {
			t.Errorf("%s: got %d entries from an empty table, want 0", kind.TOMLKey, len(entries))
		}
	}
}

func TestLoad_RecordsUnknownHookKeysWithoutFailing(t *testing.T) {
	t.Parallel()

	path := writeConfig(
		t,
		"[hooks]\non_app_activate = [\"echo real\"]\non_window_focussed = [\"echo typo\"]\n",
	)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("a typo'd hook key must not stop the config loading: %v", err)
	}

	if len(cfg.UnknownHookKeys) != 1 || cfg.UnknownHookKeys[0] != "on_window_focussed" {
		t.Errorf("UnknownHookKeys: got %v, want [on_window_focussed]", cfg.UnknownHookKeys)
	}

	if len(cfg.Hooks.AppActivate) != 1 {
		t.Errorf(
			"the recognized hook should still load, got %d entries",
			len(cfg.Hooks.AppActivate),
		)
	}
}

func TestLoad_NamesEmptyCommandsByTheKeyTheUserTyped(t *testing.T) {
	t.Parallel()

	// Both spellings of "a hook with no command" must report identically: the
	// key as written, not the event kind it publishes as.
	for name, src := range map[string]string{
		"bare string": "[hooks]\non_app_activate = [\"\"]\n",
		"table":       "[hooks]\non_app_activate = [{ app = \"Safari\" }]\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, src))
			if err == nil {
				t.Fatal("expected a hook with no command to be rejected")
			}

			if !strings.Contains(err.Error(), "hooks.on_app_activate[0]: run command is empty") {
				t.Errorf("got: %v", err)
			}

			// Same message and same code, whichever way it was written. The
			// table spelling used to fail during decoding under
			// CodeSerializationFailed instead.
			if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
				t.Errorf("code: got %v, want %v", derrors.GetCode(err), derrors.CodeInvalidConfig)
			}
		})
	}
}
