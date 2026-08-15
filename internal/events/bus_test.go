package events_test

import (
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/events"
)

const (
	testKindA = events.EventKind("kind_a")
	testKindB = events.EventKind("kind_b")
	testKindC = events.EventKind("kind_c")

	testEvtID1 = "evt-1"
)

// onlyKind returns a KindFilter that admits a single kind.
func onlyKind(kind events.EventKind) events.KindFilter {
	return func(k events.EventKind) bool {
		return k == kind
	}
}

func recv(t *testing.T, sub events.Subscriber) (events.Event, bool) {
	t.Helper()

	select {
	case evt, ok := <-sub:
		return evt, ok
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")

		return events.Event{}, false
	}
}

func expectEmpty(t *testing.T, sub events.Subscriber, msg string) {
	t.Helper()

	select {
	case evt, ok := <-sub:
		t.Errorf("%s: got event=%+v ok=%v, want nothing", msg, evt, ok)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBus_Publish_FansOutToMultipleSubscribers(t *testing.T) {
	t.Parallel()

	bus := events.NewBus()
	sub1 := bus.Subscribe(1)
	sub2 := bus.Subscribe(1)

	evt := events.Event{ID: testEvtID1, Kind: testKindA}
	bus.Publish(evt)

	got1, ok1 := recv(t, sub1)
	if !ok1 || got1.ID != testEvtID1 {
		t.Errorf("sub1 got %+v ok=%v, want %s", got1, ok1, testEvtID1)
	}

	got2, ok2 := recv(t, sub2)
	if !ok2 || got2.ID != testEvtID1 {
		t.Errorf("sub2 got %+v ok=%v, want %s", got2, ok2, testEvtID1)
	}
}

func TestBus_SubscribeWithFilter_SkipsNonMatchingKinds(t *testing.T) {
	t.Parallel()

	bus := events.NewBus()
	sub := bus.SubscribeWithFilter(1, onlyKind(testKindA))

	bus.Publish(events.Event{ID: "skip-me", Kind: testKindB})

	expectEmpty(t, sub, "filtered subscriber received a non-matching kind")

	bus.Publish(events.Event{ID: "keep-me", Kind: testKindA})

	got, ok := recv(t, sub)
	if !ok || got.ID != "keep-me" {
		t.Errorf("got %+v ok=%v, want keep-me", got, ok)
	}
}

func TestBus_Publish_AfterUnsubscribeNoDelivery(t *testing.T) {
	t.Parallel()

	bus := events.NewBus()
	sub := bus.Subscribe(1)

	bus.Unsubscribe(sub)
	bus.Publish(events.Event{ID: testEvtID1, Kind: testKindA})

	// Unsubscribe closes the channel, so a receive after it must return
	// immediately with ok=false rather than blocking or delivering anything.
	select {
	case evt, ok := <-sub:
		if ok {
			t.Errorf("received %+v after Unsubscribe, want closed channel", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from an unsubscribed channel; want it closed")
	}
}

func TestBus_Publish_DropsWhenSubscriberBufferFull(t *testing.T) {
	t.Parallel()

	bus := events.NewBus()
	sub := bus.Subscribe(1)

	bus.Publish(events.Event{ID: "first", Kind: testKindA})
	bus.Publish(events.Event{ID: "dropped", Kind: testKindA}) // buffer full; Publish must not block

	got, ok := recv(t, sub)
	if !ok || got.ID != "first" {
		t.Errorf("got %+v ok=%v, want first", got, ok)
	}

	expectEmpty(t, sub, "buffer-full publish should have been dropped, not queued")
}

// TestBus_Unsubscribe_KeepsRemainingFiltersAligned pins the invariant that
// Bus's subs and filters slices are spliced together on Unsubscribe.
// Removing a middle subscriber must not shift a later subscriber's filter
// out of step with its channel — that failure mode would look like a hook
// that "sometimes doesn't fire" with nothing in a diff to point at.
func TestBus_Unsubscribe_KeepsRemainingFiltersAligned(t *testing.T) {
	t.Parallel()

	bus := events.NewBus()
	subA := bus.SubscribeWithFilter(1, onlyKind(testKindA))
	subB := bus.SubscribeWithFilter(1, onlyKind(testKindB))
	subC := bus.SubscribeWithFilter(1, onlyKind(testKindC))

	bus.Unsubscribe(subB)

	// subB must be closed.
	select {
	case _, ok := <-subB:
		if ok {
			t.Error("subB received a value after Unsubscribe, want closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from unsubscribed subB; want it closed")
	}

	// Publishing kindC must still reach subC under subC's own filter. If the
	// splice desynced subs from filters, subC would now be paired with
	// subB's old filter (kindB-only) and silently miss this event.
	bus.Publish(events.Event{ID: "for-c", Kind: testKindC})

	gotC, okC := recv(t, subC)
	if !okC || gotC.ID != "for-c" {
		t.Fatalf(
			"subC got %+v ok=%v, want for-c: its filter likely inherited subB's after Unsubscribe",
			gotC,
			okC,
		)
	}

	// subA's filter must also be untouched by the splice.
	expectEmpty(t, subA, "subA should not receive a kindC event under its own kindA-only filter")

	bus.Publish(events.Event{ID: "for-a", Kind: testKindA})

	gotA, okA := recv(t, subA)
	if !okA || gotA.ID != "for-a" {
		t.Fatalf("subA got %+v ok=%v, want for-a", gotA, okA)
	}
}
