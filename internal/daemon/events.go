package daemon

import (
	"context"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// eventStreamBufSize is how many events a slow client may fall behind by
// before the bus drops the rest for it.
const eventStreamBufSize = 64

// hookableKinds is the events a client may subscribe to: every kind a hook
// can bind, and none of the daemon's internal ones.
//
//nolint:gochecknoglobals // a fixed set
var hookableKinds = func() map[events.EventKind]bool {
	kinds := make(map[events.EventKind]bool, len(events.AllKinds))
	for _, kind := range events.AllKinds {
		kinds[kind] = true
	}

	return kinds
}()

// eventStream answers the events request: every hookable event the bus
// publishes, as it happens, until the client hangs up.
func eventStream(bus *events.Bus) ipc.StreamHandler {
	return func(ctx context.Context, _ action.Command, send func(v any) error) error {
		sub := bus.SubscribeWithFilter(eventStreamBufSize, func(kind events.EventKind) bool {
			return hookableKinds[kind]
		})
		defer bus.Unsubscribe(sub)

		for {
			select {
			case <-ctx.Done():
				return nil
			case evt, ok := <-sub:
				if !ok {
					return nil
				}

				err := send(evt)
				if err != nil {
					return err
				}
			}
		}
	}
}
