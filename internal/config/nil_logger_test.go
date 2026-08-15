//nolint:testpackage // exercises notifyChange, an unexported method, to reach the logger call
package config

import (
	"path/filepath"
	"testing"
)

// TestNewWatcher_NilLogger pins the contract stated in AGENTS.md and
// docs/CODING_STANDARDS.md: a constructor that accepts a *zap.SugaredLogger
// tolerates nil by falling back to zap.NewNop(). Before this guard existed,
// passing nil stored the nil pointer and the first log call panicked.
//
// The test passes by not panicking; there is no value to assert, because the
// contract promises a logger that deliberately records nothing.
func TestNewWatcher_NilLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// construct builds the value under test with a nil logger and then
		// drives it through a code path that logs. It must not panic.
		construct func(t *testing.T)
	}{
		{
			name: "NewWatcher",
			construct: func(t *testing.T) {
				t.Helper()

				// notifyChange logs before it calls onChange, and it never
				// reads the file, so the path need not exist.
				missing := filepath.Join(t.TempDir(), "does-not-exist.toml")

				w := NewWatcher(missing, func() {}, nil)
				w.notifyChange()
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
