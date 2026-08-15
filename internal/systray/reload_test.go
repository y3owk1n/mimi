//nolint:testpackage // exercises handleReloadConfig, an unexported method, to reach its log calls
package systray

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestComponent_HandleReloadConfig_ReportsTheRequestNotTheOutcome pins what the
// systray's reload menu item is allowed to say. As a reload trigger it only asks the
// daemon to reload — it signals and returns, and never learns whether the new
// config was applied (docs/adr/0002-reload-is-signal-mediated.md). So a
// delivered ask reports a *requested* reload, matching what `mimi config
// reload` prints for the same reason; claiming the configuration was reloaded
// put a success line in the log next to the daemon's own failure line for the
// same click, on an invalid config.
//
// Failing to ask at all is a different thing entirely, and stays a warning.
func TestComponent_HandleReloadConfig_ReportsTheRequestNotTheOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// requestReload stands in for the closure that signals the daemon.
		requestReload func(context.Context, string) error
		wantLevel     zapcore.Level
		wantMessage   string
	}{
		{
			name:          "asking for a reload succeeds",
			requestReload: func(context.Context, string) error { return nil },
			wantLevel:     zapcore.InfoLevel,
			wantMessage:   "config reload requested from systray",
		},
		{
			name:          "asking for a reload fails",
			requestReload: func(context.Context, string) error { return errReloadFailed },
			wantLevel:     zapcore.WarnLevel,
			wantMessage:   "failed to request config reload from systray",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			core, logs := observer.New(zapcore.DebugLevel)

			component := NewComponent(
				"v0",
				"config.toml",
				testCase.requestReload,
				func() {},
				false,
				zap.New(core).Sugar(),
			)
			defer component.Close()

			component.handleReloadConfig()

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("handleReloadConfig() logged %d entries, want 1", len(entries))
			}

			if entries[0].Level != testCase.wantLevel {
				t.Errorf("logged at %v, want %v", entries[0].Level, testCase.wantLevel)
			}

			if entries[0].Message != testCase.wantMessage {
				t.Errorf("logged %q, want %q", entries[0].Message, testCase.wantMessage)
			}
		})
	}
}
