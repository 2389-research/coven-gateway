// ABOUTME: Tests for EventBroadcaster fan-out pub/sub system
// ABOUTME: Covers subscribe, publish, unsubscribe, context cancellation, concurrency

package conversation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeEvent(id, convKey string) *store.LedgerEvent {
	text := "hello from " + id
	return &store.LedgerEvent{
		ID:              id,
		ConversationKey: convKey,
		Direction:       store.EventDirectionInbound,
		Author:          "test-user",
		Timestamp:       time.Now(),
		Type:            store.EventTypeMessage,
		Text:            &text,
	}
}

func TestBroadcaster_SingleSubscriberReceivesEvent(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	ch, _ := b.Subscribe(ctx, "agent-1")

	event := makeEvent("evt-1", "agent-1")
	b.Publish("agent-1", event, "")

	select {
	case received := <-ch:
		assert.Equal(t, "evt-1", received.ID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroadcaster_MultipleSubscribersReceiveSameEvent(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	ch1, _ := b.Subscribe(ctx, "agent-1")
	ch2, _ := b.Subscribe(ctx, "agent-1")
	ch3, _ := b.Subscribe(ctx, "agent-1")

	event := makeEvent("evt-2", "agent-1")
	b.Publish("agent-1", event, "")

	for i, ch := range []<-chan *store.LedgerEvent{ch1, ch2, ch3} {
		select {
		case received := <-ch:
			assert.Equal(t, "evt-2", received.ID, "subscriber %d got wrong event", i)
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

func TestBroadcaster_DifferentConversationKeysAreIsolated(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	ch1, _ := b.Subscribe(ctx, "agent-1")
	ch2, _ := b.Subscribe(ctx, "agent-2")

	event := makeEvent("evt-3", "agent-1")
	b.Publish("agent-1", event, "")

	// ch1 should receive the event
	select {
	case received := <-ch1:
		assert.Equal(t, "evt-3", received.ID)
	case <-time.After(time.Second):
		t.Fatal("subscriber for agent-1 timed out")
	}

	// ch2 should NOT receive anything
	select {
	case <-ch2:
		t.Fatal("subscriber for agent-2 should not receive events for agent-1")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event
	}
}

func TestBroadcaster_ExcludeSubIDSkipsOriginator(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	ch1, subID1 := b.Subscribe(ctx, "agent-1")
	ch2, _ := b.Subscribe(ctx, "agent-1")

	event := makeEvent("evt-4", "agent-1")
	b.Publish("agent-1", event, subID1)

	// ch1 (the excluded subscriber) should NOT receive the event
	select {
	case <-ch1:
		t.Fatal("excluded subscriber should not receive the event")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	// ch2 should still receive it
	select {
	case received := <-ch2:
		assert.Equal(t, "evt-4", received.ID)
	case <-time.After(time.Second):
		t.Fatal("non-excluded subscriber timed out")
	}
}

func TestBroadcaster_SlowConsumerDoesNotBlockPublisher(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	// Subscribe but never read from ch1 (slow consumer)
	_, _ = b.Subscribe(ctx, "agent-1")
	ch2, _ := b.Subscribe(ctx, "agent-1")

	// Publish more events than the buffer size to overflow ch1
	for i := range 100 {
		event := makeEvent("evt-overflow-"+string(rune('0'+i%10)), "agent-1")
		b.Publish("agent-1", event, "")
	}

	// ch2 should still receive events (publisher wasn't blocked)
	receivedCount := 0
	for {
		select {
		case <-ch2:
			receivedCount++
		case <-time.After(200 * time.Millisecond):
			goto done
		}
	}
done:
	assert.Greater(t, receivedCount, 0, "fast consumer should receive at least some events")
}

func TestBroadcaster_ContextCancellationCleansUp(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, subID := b.Subscribe(ctx, "agent-1")

	// Verify subscription exists
	b.mu.RLock()
	_, exists := b.subscribers["agent-1"][subID]
	b.mu.RUnlock()
	assert.True(t, exists, "subscription should exist before cancel")

	// Cancel the context
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(50 * time.Millisecond)

	// Subscription should be cleaned up
	b.mu.RLock()
	subs, convExists := b.subscribers["agent-1"]
	if convExists {
		_, subExists := subs[subID]
		assert.False(t, subExists, "subscription should be removed after context cancel")
	}
	b.mu.RUnlock()

	// Channel should be closed
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed after context cancel")
	case <-time.After(time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

func TestBroadcaster_ManualUnsubscribe(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	ch, subID := b.Subscribe(ctx, "agent-1")

	b.Unsubscribe("agent-1", subID)

	// Channel should be closed
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed after unsubscribe")
	case <-time.After(time.Second):
		t.Fatal("channel not closed after unsubscribe")
	}

	// Publishing should not panic
	event := makeEvent("evt-after-unsub", "agent-1")
	b.Publish("agent-1", event, "")
}

func TestBroadcaster_CloseClosesAllSubscriptions(t *testing.T) {
	b := NewEventBroadcaster(nil)

	ctx1 := t.Context()
	ctx2 := t.Context()

	ch1, _ := b.Subscribe(ctx1, "agent-1")
	ch2, _ := b.Subscribe(ctx2, "agent-2")

	b.Close()

	// Both channels should be closed
	for i, ch := range []<-chan *store.LedgerEvent{ch1, ch2} {
		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channel %d should be closed after Close()", i)
		case <-time.After(time.Second):
			t.Fatalf("channel %d not closed after Close()", i)
		}
	}
}

func TestBroadcaster_ConcurrentPublishSubscribe(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	var wg sync.WaitGroup
	ctx := t.Context()

	// Spawn concurrent subscribers
	for range 10 {
		wg.Go(func() {
			ch, _ := b.Subscribe(ctx, "agent-concurrent")
			// Read a few events then exit
			for range 5 {
				select {
				case <-ch:
				case <-time.After(500 * time.Millisecond):
					return
				}
			}
		})
	}

	// Spawn concurrent publishers
	for range 10 {
		wg.Go(func() {
			for range 10 {
				event := makeEvent("concurrent-evt", "agent-concurrent")
				b.Publish("agent-concurrent", event, "")
			}
		})
	}

	wg.Wait()
	// If we get here without deadlock or panic, the test passes
}

func TestBroadcaster_SubscribeReturnsUniqueIDs(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	ctx := t.Context()

	_, id1 := b.Subscribe(ctx, "agent-1")
	_, id2 := b.Subscribe(ctx, "agent-1")
	_, id3 := b.Subscribe(ctx, "agent-2")

	require.NotEqual(t, id1, id2)
	require.NotEqual(t, id1, id3)
	require.NotEqual(t, id2, id3)
}

func TestBroadcaster_PublishToNonexistentConversation(t *testing.T) {
	b := NewEventBroadcaster(nil)
	defer b.Close()

	// Should not panic
	event := makeEvent("evt-nowhere", "nobody-listening")
	b.Publish("nobody-listening", event, "")
}
