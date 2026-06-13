package events

import "sync"

// Subscriber is a channel that receives events.
type Subscriber chan Event

// KindFilter is an optional predicate the bus uses to skip events that the
// subscriber does not care about, avoiding the channel send and giving
// high-frequency observers a free backpressure-free fast path.
type KindFilter func(EventKind) bool

// Bus is a pub-sub event bus that fans out events to subscribers.
type Bus struct {
	mu      sync.RWMutex
	subs    []Subscriber
	filters []KindFilter
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{}
}

// Subscribe adds a new subscriber with the given buffer size.
func (b *Bus) Subscribe(bufSize int) Subscriber {
	return b.SubscribeWithFilter(bufSize, nil)
}

// SubscribeWithFilter adds a new subscriber with the given buffer size and
// an optional kind filter. When a filter is provided, the bus will skip
// events whose kind returns false from the filter, avoiding the channel
// send entirely. Pass nil to receive every event.
func (b *Bus) SubscribeWithFilter(bufSize int, filter KindFilter) Subscriber {
	subCh := make(Subscriber, bufSize)

	b.mu.Lock()
	b.subs = append(b.subs, subCh)
	b.filters = append(b.filters, filter)
	b.mu.Unlock()

	return subCh
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Bus) Unsubscribe(sub Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, s := range b.subs {
		if s == sub {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			b.filters = append(b.filters[:i], b.filters[i+1:]...)

			close(s)

			return
		}
	}
}

// Publish fans an event out to all subscribers (non-blocking).
// Subscribers with a kind filter that returns false for this event kind
// are skipped entirely.
func (b *Bus) Publish(evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for idx, sub := range b.subs {
		if filter := b.filters[idx]; filter != nil && !filter(evt.Kind) {
			continue
		}

		select {
		case sub <- evt:
		default:
		}
	}
}
