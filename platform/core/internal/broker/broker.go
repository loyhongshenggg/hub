// Package broker is a tiny fan-out pub/sub for SSE streams.
package broker

import "sync"

// Broker delivers each published value to every subscriber. A subscriber
// that falls behind is dropped (its channel closes) and must resubscribe —
// SSE clients reconnect and get a fresh snapshot.
type Broker[T any] struct {
	mu   sync.Mutex
	subs map[chan T]struct{}
}

func New[T any]() *Broker[T] { return &Broker[T]{subs: map[chan T]struct{}{}} }

// Subscribe returns a channel of values and a func to unsubscribe.
func (b *Broker[T]) Subscribe() (<-chan T, func()) {
	ch := make(chan T, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *Broker[T]) Publish(v T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- v:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}
