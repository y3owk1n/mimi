//nolint:testpackage // tests reloadConfig, the daemon's unexported reload reporting
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/systray"
)

// reloadReportMarker is a value only a user's config file would contain. It
// stands in for every user payload AGENTS.md keeps out of the log: the log
// may name settings.log_file, never what the user put in it.
const reloadReportMarker = "/tmp/mimi-user-payload-marker.log"

// The TOML keys the reload tests expect to be named back to them, spelled the
// way config.RestartOnlyChanges reports them.
const (
	keyLogFile        = "settings.log_file"
	keyLogLevel       = "settings.log_level"
	keyMaxHookWorkers = "settings.max_hook_workers"
)

const reloadReportBaseConfig = `[settings]
log_level = "info"
hook_shell = "/bin/sh"

[hooks]
on_window_focus = ["echo old"]
`

// newTestReloadConfig writes contents to a config file and builds a reloader
// running that config, returning the path, the reloader, the logger a reload
// reports through, and the entries it produces.
func newTestReloadConfig(
	t *testing.T,
	contents string,
) (string, *reloader, *zap.SugaredLogger, *observer.ObservedLogs) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")

	writeReloadTestConfig(t, path, contents)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("failed to load the starting config: %v", err)
	}

	cfgReloader, _, _ := newTestReloader(t, cfg)

	core, logs := observer.New(zapcore.DebugLevel)

	return path, cfgReloader, zap.New(core).Sugar(), logs
}

func writeReloadTestConfig(t *testing.T, path, contents string) {
	t.Helper()

	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}

// entryText renders a log entry the way a reader sees it — message and
// fields together — so an assertion about what does or does not reach the log
// covers both.
func entryText(entry observer.LoggedEntry) string {
	var text strings.Builder

	text.WriteString(entry.Message)

	for _, field := range entry.Context {
		// A zap field carries its value in String or in Interface depending
		// on its type; both go in, so no value can hide from an assertion.
		fmt.Fprintf(&text, " %s=%v=%v", field.Key, field.String, field.Interface)
	}

	return text.String()
}

// discardReloadOutcome stands in for the tray's callback in tests that assert
// about the log rather than the outcome.
func discardReloadOutcome(systray.ReloadOutcome) {}

func TestReloadConfig_ReportsTheOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contents    string
		wantLevel   zapcore.Level
		wantMessage string
		wantInEntry []string
		wantOutcome systray.ReloadOutcome
	}{
		{
			name:        "a config that does not parse",
			contents:    "not valid toml [[[",
			wantLevel:   zapcore.WarnLevel,
			wantMessage: reloadFailedMessage,
			wantOutcome: systray.ReloadOutcomeFailed,
		},
		{
			name:        "a config that parses but cannot be applied",
			contents:    "[[hooks.on_window_focus]]\nrun = \"echo new\"\ntitle = \"[\"\n",
			wantLevel:   zapcore.WarnLevel,
			wantMessage: reloadFailedMessage,
			wantOutcome: systray.ReloadOutcomeFailed,
		},
		{
			name:        "only reloadable settings changed",
			contents:    "[settings]\nlog_level = \"info\"\nhook_shell = \"/bin/bash\"\n",
			wantLevel:   zapcore.InfoLevel,
			wantMessage: reloadedMessage,
			wantOutcome: systray.ReloadOutcomeApplied,
		},
		{
			name:        "a restart-only setting changed",
			contents:    "[settings]\nlog_level = \"debug\"\n",
			wantLevel:   zapcore.WarnLevel,
			wantMessage: reloadRestartRequiredMessage,
			wantInEntry: []string{keyLogLevel},
			wantOutcome: systray.ReloadOutcomeRestartRequired,
		},
		{
			name:        "several restart-only settings changed",
			contents:    "[settings]\nlog_level = \"debug\"\nmax_hook_workers = 8\n",
			wantLevel:   zapcore.WarnLevel,
			wantMessage: reloadRestartRequiredMessage,
			wantInEntry: []string{keyLogLevel, keyMaxHookWorkers},
			wantOutcome: systray.ReloadOutcomeRestartRequired,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path, cfgReloader, logger, logs := newTestReloadConfig(t, reloadReportBaseConfig)

			writeReloadTestConfig(t, path, testCase.contents)

			var reported []systray.ReloadOutcome

			report := func(outcome systray.ReloadOutcome) {
				reported = append(reported, outcome)
			}

			reloadConfig(path, cfgReloader, reloadTriggerSighup, report, logger)

			if len(reported) != 1 {
				t.Fatalf("reported %d outcomes, want 1: %v", len(reported), reported)
			}

			if reported[0] != testCase.wantOutcome {
				t.Errorf("reported outcome = %v, want %v", reported[0], testCase.wantOutcome)
			}

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("got %d log entries, want 1: %+v", len(entries), entries)
			}

			entry := entries[0]

			if entry.Message != testCase.wantMessage {
				t.Errorf("log message = %q, want %q", entry.Message, testCase.wantMessage)
			}

			if entry.Level != testCase.wantLevel {
				t.Errorf("log level = %v, want %v", entry.Level, testCase.wantLevel)
			}

			text := entryText(entry)

			if !strings.Contains(text, string(reloadTriggerSighup)) {
				t.Errorf("log entry %q does not name the trigger that fired the reload", text)
			}

			for _, want := range testCase.wantInEntry {
				if !strings.Contains(text, want) {
					t.Errorf("log entry %q does not name %q", text, want)
				}
			}
		})
	}
}

// TestReloadConfig_KeepsUserConfigValuesOutOfTheLog pins AGENTS.md's rule on
// the one line this change adds: naming a restart-only setting means naming
// the setting, never the value the user gave it.
func TestReloadConfig_KeepsUserConfigValuesOutOfTheLog(t *testing.T) {
	t.Parallel()

	path, cfgReloader, logger, logs := newTestReloadConfig(t, reloadReportBaseConfig)

	writeReloadTestConfig(t, path, "[settings]\nlog_file = \""+reloadReportMarker+"\"\n")

	reloadConfig(path, cfgReloader, reloadTriggerFsnotify, discardReloadOutcome, logger)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %+v", len(entries), entries)
	}

	text := entryText(entries[0])

	if !strings.Contains(text, keyLogFile) {
		t.Errorf("log entry %q does not name the restart-only setting that changed", text)
	}

	if strings.Contains(text, reloadReportMarker) {
		t.Errorf("log entry %q carries the user's config value", text)
	}
}

// TestReloadReporter_WithoutATrayReportsNowhere pins that the reload path does
// not depend on the tray existing. With [systray] enabled = false there is no
// component to hold an outcome, and a reload still has to report through
// something — so the reporter is a no-op rather than a nil the reload path
// would have to remember to check.
func TestReloadReporter_WithoutATrayReportsNowhere(t *testing.T) {
	t.Parallel()

	report := reloadReporter(nil)
	if report == nil {
		t.Fatal("reloadReporter(nil) = nil, want a callback the reload path can call")
	}

	report(systray.ReloadOutcomeApplied)
}
