//nolint:testpackage // exercises handleReloadConfig, an unexported method, to reach the logger call
package systray

import (
	"context"
	"errors"
	"testing"
)

// errReloadFailed drives handleReloadConfig down its Warnw branch. Shared with
// reload_test.go, which pins what that branch logs.
var errReloadFailed = errors.New("reload failed")

// TestNewComponent_NilLogger pins the contract stated in AGENTS.md and
// docs/CODING_STANDARDS.md: a constructor that accepts a *zap.SugaredLogger
// tolerates nil by falling back to zap.NewNop(). Before this guard existed,
// passing nil stored the nil pointer and the first log call panicked.
//
// NewComponent is easy to miss when auditing this contract — its signature is
// multi-line, so the logger parameter does not sit on the `func` line where a
// grep for it would land.
//
// The test passes by not panicking; there is no value to assert, because the
// contract promises a logger that deliberately records nothing.
func TestNewComponent_NilLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// construct builds the value under test with a nil logger and then
		// drives it through a code path that logs. It must not panic.
		construct func(t *testing.T)
	}{
		{
			name: "NewComponent",
			construct: func(t *testing.T) {
				t.Helper()

				// A failing reload is the cheapest route to a log call that
				// does not touch Cocoa.
				reload := func(context.Context, string) error { return errReloadFailed }

				c := NewComponent("v0", "config.toml", reload, func() {}, false, nil)
				defer c.Close()

				c.handleReloadConfig()
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
