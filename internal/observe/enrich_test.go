//nolint:testpackage // drives Router.handle, which is unexported
package observe

import (
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/events"
)

func TestSetEnricher_RunsBeforeAnEventOfItsKindIsPublished(t *testing.T) {
	router, sub := newTestRouter(t)
	router.SetEnricher(events.DisplayChanged, func(evt events.Event) events.Event {
		evt.Extra = map[string]string{"displays_count": "2"}

		return evt
	})

	router.handle(events.Event{Kind: events.DisplayChanged})
	router.handle(events.Event{Kind: events.SystemWake})

	got := receive(t, sub)
	if got.Kind != events.DisplayChanged || got.Extra["displays_count"] != "2" {
		t.Fatalf("got %+v", got)
	}

	if other := receive(t, sub); other.Extra != nil {
		t.Fatalf("an event of another kind was enriched: %+v", other)
	}
}

func receive(t *testing.T, sub <-chan events.Event) events.Event {
	t.Helper()

	select {
	case evt := <-sub:
		return evt
	case <-time.After(time.Second):
		t.Fatal("no event published")

		return events.Event{}
	}
}

func TestSetEnricher_RunsOnASettledDragToo(t *testing.T) {
	router, sub := newTestRouter(t)
	router.SetEnricher(events.WindowMove, func(evt events.Event) events.Event {
		evt.Extra["display_index"] = "1"

		return evt
	})

	router.handle(
		events.Event{
			Kind:     events.WindowMoving,
			PID:      5,
			WindowID: 9,
			Extra:    map[string]string{"window_center": "1,2"},
		},
	)

	got := receive(t, sub)
	if got.Kind != events.WindowMove || got.Extra["display_index"] != "1" ||
		got.Extra["window_center"] != "1,2" {
		t.Fatalf("got %+v", got)
	}
}
