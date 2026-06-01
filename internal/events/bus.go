package events

import "sync"

// Subscriber is a channel that receives events.
type Subscriber chan Event

// Bus is a pub-sub event bus that fans out events to subscribers.
type Bus struct {
	mu   sync.RWMutex
	subs []Subscriber
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{}
}

// Subscribe adds a new subscriber with the given buffer size.
func (b *Bus) Subscribe(bufSize int) Subscriber {
	subCh := make(Subscriber, bufSize)

	b.mu.Lock()
	b.subs = append(b.subs, subCh)
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

			close(s)

			return
		}
	}
}

// Publish fans an event out to all subscribers (non-blocking).
func (b *Bus) Publish(evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, s := range b.subs {
		select {
		case s <- evt:
		default:
		}
	}
}
