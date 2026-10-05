//nolint:testpackage // exercises handle, an unexported method, to reach the logger call
package observe

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/events"
)

// TestNewRouterWithDebounce_NilLogger pins the AGENTS.md and
// docs/CODING_STANDARDS.md rule that a constructor taking a *zap.SugaredLogger
// falls back to zap.NewNop() when given nil. Before this guard existed,
// passing nil stored the nil pointer and the first log call panicked.
func TestNewRouterWithDebounce_NilLogger(t *testing.T) {
	t.Parallel()

	router := NewRouterWithDebounce(events.NewBus(), NewAXTracker(false), nil, testDebounceWindow)

	// A default-kind event falls straight through handle's switch to the
	// unconditional Debugw, which is the cheapest route to a log call.
	router.handle(events.Event{Kind: events.WorkspaceChanged})
}
