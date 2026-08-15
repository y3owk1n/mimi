//nolint:testpackage // tests warnUnknownHookKeys, an unexported function
package daemon

import (
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/y3owk1n/mimi/internal/config"
)

// The daemon warns about hook kinds it does not recognize rather than refusing
// to start: the hooks it did understand still work, and a typo in a config
// should not take mimi down. `mimi config validate` is the command that fails.
//
// The keys themselves are the user's config text, so they stay out of the log
// under the same rule that keeps window titles and hook commands out of it.
func TestWarnUnknownHookKeys_CountsWithoutNamingTheKeys(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.WarnLevel)
	logger := zap.New(core).Sugar()

	const typo = "on_window_focussed"

	warnUnknownHookKeys(&config.Config{UnknownHookKeys: []string{typo, "on_another_typo"}}, logger)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one warning, got %d", len(entries))
	}

	fields := entries[0].ContextMap()

	count, isInt := fields["count"].(int64)
	if !isInt || count != 2 {
		t.Errorf("count field: got %v, want 2", fields["count"])
	}

	// The recognized set is what makes the warning actionable without naming
	// what the user typed.
	recognized, isString := fields["recognized"].(string)
	if !isString || !strings.Contains(recognized, "on_app_activate") {
		t.Errorf("recognized field should list the valid keys, got %v", fields["recognized"])
	}

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, typo) {
			t.Errorf("log message leaked the user's key: %q", entry.Message)
		}

		for key, val := range entry.ContextMap() {
			str, isText := val.(string)
			if isText && strings.Contains(str, typo) {
				t.Errorf("log field %q leaked the user's key: %q", key, str)
			}
		}
	}
}

func TestWarnUnknownHookKeys_SaysNothingWhenThereAreNone(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.WarnLevel)
	logger := zap.New(core).Sugar()

	warnUnknownHookKeys(&config.Config{}, logger)

	if got := logs.Len(); got != 0 {
		t.Errorf("a clean config should log nothing, got %d entries", got)
	}
}
