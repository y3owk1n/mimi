package events

import (
	"sync"
	"sync/atomic"
)

// Subscriber is a channel that receives events.
type Subscriber chan Event

// KindFilter is an optional predicate the bus uses to skip events that the
// subscriber does not care about, avoiding the channel send and giving
// high-frequency observers a free backpressure-free fast path.
type KindFilter func(EventKind) bool

// Bus is a pub-sub event bus that fans out events to subscribers.
type Bus struct {
	mu        sync.RWMutex
	subs      []Subscriber
	filters   []KindFilter
	names     []string
	dropCount atomic.Int64
	onDrop    func(name string, kind EventKind, buffer int)
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{}
}

// SetDropHandler has the bus call onDrop with the subscriber's name, the
// event's kind and the subscriber's buffer size each time Publish discards an
// event because that buffer is full. It is set before anything publishes.
func (b *Bus) SetDropHandler(onDrop func(name string, kind EventKind, buffer int)) {
	b.mu.Lock()
	b.onDrop = onDrop
	b.mu.Unlock()
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
	return b.SubscribeNamed("", bufSize, filter)
}

// SubscribeNamed is SubscribeWithFilter with a name. The bus passes the name to
// the drop handler when this subscriber drops an event.
func (b *Bus) SubscribeNamed(name string, bufSize int, filter KindFilter) Subscriber {
	subCh := make(Subscriber, bufSize)

	b.mu.Lock()
	b.subs = append(b.subs, subCh)
	b.filters = append(b.filters, filter)
	b.names = append(b.names, name)
	b.mu.Unlock()

	return subCh
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Bus) Unsubscribe(sub Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for index, existing := range b.subs {
		if existing == sub {
			b.subs = append(b.subs[:index], b.subs[index+1:]...)
			b.filters = append(b.filters[:index], b.filters[index+1:]...)
			b.names = append(b.names[:index], b.names[index+1:]...)

			close(existing)

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
			b.dropCount.Add(1)

			if b.onDrop != nil {
				b.onDrop(b.names[idx], evt.Kind, cap(sub))
			}
		}
	}
}

// DropCount returns the number of events Publish has discarded because a
// subscriber's buffer was full. It counts every dropped delivery across all
// subscribers, not per-subscriber.
func (b *Bus) DropCount() int64 {
	return b.dropCount.Load()
}
