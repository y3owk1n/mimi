//nolint:testpackage // exercises handle, an unexported method, to reach the logger call
package observe

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/events"
)

// TestNewRouter_NilLogger pins the contract stated in AGENTS.md and
// docs/CODING_STANDARDS.md: a constructor that accepts a *zap.SugaredLogger
// tolerates nil by falling back to zap.NewNop(). Before this guard existed,
// passing nil stored the nil pointer and the first log call panicked.
//
// Both of the package's logger-accepting constructors are covered:
// NewRouterWithDebounce holds the guard and NewRouter delegates to it, so the
// NewRouter row would still fail if that delegation were ever replaced by an
// independent struct literal.
func TestNewRouter_NilLogger(t *testing.T) {
	t.Parallel()

	// A default-kind event falls straight through handle's switch to the
	// unconditional Debugw, which is the cheapest route to a log call.
	evt := events.Event{Kind: events.WorkspaceChanged}

	tests := []struct {
		name string
		// newRouter builds a Router with a nil logger.
		newRouter func() *Router
	}{
		{
			name: "NewRouter",
			newRouter: func() *Router {
				return NewRouter(events.NewBus(), NewAXTracker(false), nil)
			},
		},
		{
			name: "NewRouterWithDebounce",
			newRouter: func() *Router {
				return NewRouterWithDebounce(
					events.NewBus(),
					NewAXTracker(false),
					nil,
					testDebounceWindow,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.newRouter().handle(evt)
		})
	}
}
