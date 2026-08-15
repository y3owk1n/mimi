package hooks_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/hooks"
)

// TestNewExecutor_NilLogger pins the contract stated in AGENTS.md and
// docs/CODING_STANDARDS.md: a constructor that accepts a *zap.SugaredLogger
// tolerates nil by falling back to zap.NewNop(). Before this guard existed,
// passing nil stored the nil pointer and the first log call panicked.
//
// The test passes by not panicking; there is no value to assert, because the
// contract promises a logger that deliberately records nothing.
func TestNewExecutor_NilLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// construct builds the value under test with a nil logger and then
		// drives it through a code path that logs. It must not panic.
		construct func(t *testing.T)
	}{
		{
			name: "NewExecutor",
			construct: func(t *testing.T) {
				t.Helper()

				// An empty registry means Handle logs its "processing
				// event" line and matches nothing, so no hook shells out.
				cfg := &config.SettingsConfig{MaxHookWorkers: 1}

				ex := hooks.NewExecutor(hooks.NewRegistry(), cfg, nil)
				ex.Handle(events.Event{Kind: events.WorkspaceChanged})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.construct(t)
		})
	}
}
