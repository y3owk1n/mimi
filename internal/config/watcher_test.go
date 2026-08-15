//nolint:testpackage // tests notifyChange, an unexported method exercising Watcher's private log/dispatch ordering
package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newTestWatcher writes contents to a config file in dir and returns a
// Watcher over it, wired to onChange, plus the log entries notifyChange will
// produce on it.
func newTestWatcher(
	t *testing.T,
	dir, contents string,
	onChange func(),
) (*Watcher, *observer.ObservedLogs) {
	t.Helper()

	path := filepath.Join(dir, "config.toml")

	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core).Sugar()

	return NewWatcher(path, onChange, logger), logs
}

// TestWatcher_NotifyChange_ReportsTheChangeAndNothingMore pins the fix for
// #107 and the split it grew into: the watcher says the file changed, and
// claims nothing about the reload. It does not read the file, so a config
// that will not parse reaches the daemon's one reload path — which loads it,
// applies it, and logs the single authoritative outcome line — instead of
// being reported here, in a second voice that cannot name the trigger.
func TestWatcher_NotifyChange_ReportsTheChangeAndNothingMore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{name: "a config that parses", contents: "[settings]\nlog_level = \"info\"\n"},
		{name: "a config that does not parse", contents: "not valid toml [[["},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			called := 0

			onChange := func() { called++ }

			watcher, logs := newTestWatcher(t, t.TempDir(), testCase.contents, onChange)
			watcher.notifyChange()

			if called != 1 {
				t.Fatalf("onChange was called %d times, want 1", called)
			}

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("got %d log entries, want 1: %+v", len(entries), entries)
			}

			entry := entries[0]

			if entry.Message != "config file changed" {
				t.Errorf("log message = %q, want %q", entry.Message, "config file changed")
			}

			if entry.Level != zapcore.DebugLevel {
				t.Errorf(
					"watcher logged %q at %v, want Debug; the daemon's reloadConfig owns the reload outcome line",
					entry.Message,
					entry.Level,
				)
			}
		})
	}
}
