package body

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// ErrBusClosed is returned when a publish is attempted on a closed bus.
var ErrBusClosed = errors.New("event bus closed")

// Event is a discrete occurrence that heads can publish or subscribe to,
// enabling loose coupling between components.
type Event struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	TraceID   string          `json:"trace_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// EventBus is the contract for pub/sub messaging between heads.
type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType string) (<-chan Event, error)
	Unsubscribe(eventType string, ch <-chan Event) error
	Close()
}

// InMemoryEventBus is a non-durable, in-process implementation of EventBus.
// Events are delivered to subscribers via buffered channels; slow consumers
// have events silently dropped to avoid blocking publishers.
type InMemoryEventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
	closed      bool
}

// NewInMemoryEventBus creates a ready-to-use in-memory event bus.
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		subscribers: make(map[string][]chan Event),
	}
}

// Publish sends an event to all subscribers of its type. Drops the event for
// any subscriber whose buffer is full, so publishers are never blocked.
func (b *InMemoryEventBus) Publish(_ context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	subs := b.subscribers[event.Type]
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
	return nil
}

// Subscribe returns a buffered channel that receives events of the given type.
func (b *InMemoryEventBus) Subscribe(eventType string) (<-chan Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 64)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch, nil
}

// Unsubscribe removes and closes the given channel from the subscriber list.
func (b *InMemoryEventBus) Unsubscribe(eventType string, ch <-chan Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[eventType]
	for i, sub := range subs {
		if sub == ch {
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			close(sub)
			break
		}
	}

	if len(b.subscribers[eventType]) == 0 {
		delete(b.subscribers, eventType)
	}
	return nil
}

// Close shuts down the bus, closing all subscriber channels and rejecting
// future publishes.
func (b *InMemoryEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for eventType, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
		delete(b.subscribers, eventType)
	}
	b.closed = true
}
