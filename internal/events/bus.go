package events

import "sync"

type Subscriber chan Event

type Bus struct {
	mu   sync.RWMutex
	subs []Subscriber
}

func NewBus() *Bus {
	return &Bus{}
}

func (b *Bus) Subscribe(bufSize int) Subscriber {
	ch := make(Subscriber, bufSize)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

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

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		select {
		case s <- e:
		default:
		}
	}
}
