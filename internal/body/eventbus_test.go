package body

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeEvent(eventType string) Event {
	return Event{
		Type:      eventType,
		Payload:   json.RawMessage(`{"hello":"world"}`),
		TraceID:   "trace-123",
		Timestamp: time.Now().UTC(),
	}
}

func recvWithTimeout(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func TestInMemoryEventBus_PublishSubscribe(t *testing.T) {
	bus := NewInMemoryEventBus()
	defer bus.Close()

	ch, err := bus.Subscribe("user.created")
	require.NoError(t, err)

	evt := makeEvent("user.created")
	err = bus.Publish(context.Background(), evt)
	require.NoError(t, err)

	received := recvWithTimeout(t, ch)
	assert.Equal(t, "user.created", received.Type)
	assert.Equal(t, evt.TraceID, received.TraceID)
	assert.WithinDuration(t, evt.Timestamp, received.Timestamp, time.Second)
	assert.JSONEq(t, `{"hello":"world"}`, string(received.Payload))
}

func TestInMemoryEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewInMemoryEventBus()
	defer bus.Close()

	ch1, err := bus.Subscribe("order.placed")
	require.NoError(t, err)
	ch2, err := bus.Subscribe("order.placed")
	require.NoError(t, err)
	ch3, err := bus.Subscribe("order.placed")
	require.NoError(t, err)

	evt := makeEvent("order.placed")
	err = bus.Publish(context.Background(), evt)
	require.NoError(t, err)

	for i, ch := range []<-chan Event{ch1, ch2, ch3} {
		received := recvWithTimeout(t, ch)
		assert.Equal(t, "order.placed", received.Type, "subscriber %d should receive event", i)
	}
}

func TestInMemoryEventBus_Unsubscribe(t *testing.T) {
	bus := NewInMemoryEventBus()
	defer bus.Close()

	ch1, err := bus.Subscribe("payment.processed")
	require.NoError(t, err)
	ch2, err := bus.Subscribe("payment.processed")
	require.NoError(t, err)

	err = bus.Unsubscribe("payment.processed", ch1)
	require.NoError(t, err)

	_, ok := <-ch1
	assert.False(t, ok, "unsubscribed channel should be closed")

	err = bus.Publish(context.Background(), makeEvent("payment.processed"))
	require.NoError(t, err)

	received := recvWithTimeout(t, ch2)
	assert.Equal(t, "payment.processed", received.Type)
}

func TestInMemoryEventBus_NoSubscribers(t *testing.T) {
	bus := NewInMemoryEventBus()
	defer bus.Close()

	err := bus.Publish(context.Background(), makeEvent("orphan.event"))
	assert.NoError(t, err)
}

func TestInMemoryEventBus_Close(t *testing.T) {
	bus := NewInMemoryEventBus()

	ch1, err := bus.Subscribe("a")
	require.NoError(t, err)
	ch2, err := bus.Subscribe("b")
	require.NoError(t, err)

	bus.Close()

	_, ok := <-ch1
	assert.False(t, ok, "channel for type 'a' should be closed")
	_, ok = <-ch2
	assert.False(t, ok, "channel for type 'b' should be closed")

	err = bus.Publish(context.Background(), makeEvent("a"))
	assert.Error(t, err, "publish after close should return error")
}

func TestInMemoryEventBus_DifferentEventTypes(t *testing.T) {
	bus := NewInMemoryEventBus()
	defer bus.Close()

	emailCh, err := bus.Subscribe("notify.email")
	require.NoError(t, err)
	smsCh, err := bus.Subscribe("notify.sms")
	require.NoError(t, err)

	err = bus.Publish(context.Background(), makeEvent("notify.email"))
	require.NoError(t, err)

	received := recvWithTimeout(t, emailCh)
	assert.Equal(t, "notify.email", received.Type)

	select {
	case <-smsCh:
		t.Fatal("sms subscriber should not receive email events")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestInMemoryEventBus_ConcurrentPublish(t *testing.T) {
	bus := NewInMemoryEventBus()
	defer bus.Close()

	ch, err := bus.Subscribe("concurrent.test")
	require.NoError(t, err)

	var wg sync.WaitGroup
	const numPublishers = 100
	const eventsPerPublisher = 50

	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				evt := Event{
					Type:    "concurrent.test",
					Payload: json.RawMessage(`{"ok":true}`),
				}
				_ = bus.Publish(context.Background(), evt)
			}
		}(i)
	}
	wg.Wait()

	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	received := 0
	for time.Now().Before(deadline) {
		select {
		case <-ch:
			received++
		default:
			if received > 0 {
				goto done
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
done:

	assert.Positive(t, received, "should receive at least some events from concurrent publishers")
}
