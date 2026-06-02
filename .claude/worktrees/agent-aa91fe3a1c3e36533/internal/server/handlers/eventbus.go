package handlers

import "sync"

type Event struct {
	Type string
	Data any
}

// EventBus is a tiny fan-out for SSE subscribers. Buffered channels prevent
// a slow client from blocking the publisher; if a buffer is full, the event
// is dropped for that client (we never block).
type EventBus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
	done chan struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{subs: map[chan Event]struct{}{}, done: make(chan struct{})}
}

// Done returns a channel closed when the bus is shutting down. SSE handlers
// select on this so http.Server.Shutdown isn't forced to wait the full
// graceful-shutdown timeout for long-lived event streams to drain.
func (b *EventBus) Done() <-chan struct{} { return b.done }

// Close signals all subscribers to exit. Safe to call multiple times.
func (b *EventBus) Close() {
	b.mu.Lock()
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	b.mu.Unlock()
}

func (b *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *EventBus) Publish(t string, data any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- Event{Type: t, Data: data}:
		default:
			// drop on full
		}
	}
}
