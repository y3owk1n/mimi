//nolint:testpackage // tests reload, an unexported method exercising Watcher's private log/dispatch ordering
package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const watcherTestMarker = "watcher-test-marker.log"

// newTestWatcher writes contents to a config file in dir and returns a
// Watcher over it, wired to onChange, plus the log entries reload will
// produce on it.
func newTestWatcher(
	t *testing.T,
	dir, contents string,
	onChange func(*Config),
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

// TestWatcher_Reload_OnSuccess_DoesNotClaimReloadSucceeded pins the fix for
// #107: a successful config.Load must not log an Info "config reloaded" line,
// because that claims more than has happened at that point — onChange (the
// hook registry reload) hasn't run yet, and it is the one that can still
// fail. The daemon's applyReload is the sole place that logs the outcome of
// a reload; the watcher only logs that it parsed the file.
func TestWatcher_Reload_OnSuccess_DoesNotClaimReloadSucceeded(t *testing.T) {
	t.Parallel()

	var gotCfg *Config

	onChange := func(cfg *Config) { gotCfg = cfg }

	dir := t.TempDir()
	watcher, logs := newTestWatcher(
		t,
		dir,
		"[settings]\nlog_file = \""+watcherTestMarker+"\"\n",
		onChange,
	)
	watcher.reload()

	if gotCfg == nil {
		t.Fatal("onChange was not called")
	}

	if gotCfg.Settings.LogFile != watcherTestMarker {
		t.Errorf("onChange got LogFile = %q, want %q", gotCfg.Settings.LogFile, watcherTestMarker)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %+v", len(entries), entries)
	}

	entry := entries[0]

	if entry.Message != "config file parsed" {
		t.Errorf("log message = %q, want %q", entry.Message, "config file parsed")
	}

	if entry.Level != zapcore.DebugLevel {
		t.Errorf(
			"watcher logged %q at %v, want Debug; the daemon's applyReload owns the Info-level success/failure line",
			entry.Message,
			entry.Level,
		)
	}
}

// TestWatcher_Reload_OnFailure_LogsWarnAndSkipsOnChange pins the existing
// (and unchanged) failure path: a bad config file must not invoke onChange,
// and must log exactly one Warn line so it isn't followed by anything that
// looks like a success.
func TestWatcher_Reload_OnFailure_LogsWarnAndSkipsOnChange(t *testing.T) {
	t.Parallel()

	called := false

	onChange := func(*Config) { called = true }

	dir := t.TempDir()
	watcher, logs := newTestWatcher(t, dir, "not valid toml [[[", onChange)
	watcher.reload()

	if called {
		t.Error("onChange was called despite a failed Load")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %+v", len(entries), entries)
	}

	entry := entries[0]

	if entry.Level != zapcore.WarnLevel {
		t.Errorf("log level = %v, want Warn", entry.Level)
	}

	if entry.Message != "config reload failed" {
		t.Errorf("log message = %q, want %q", entry.Message, "config reload failed")
	}
}
