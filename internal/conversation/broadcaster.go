// ABOUTME: In-memory fan-out event broadcaster for cross-client awareness
// ABOUTME: Publishes persisted LedgerEvents to all subscribers of a conversation key

package conversation

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/store"
)

const (
	// subscriberBufferSize is the channel buffer for each subscriber.
	// Matches the existing chatHub pattern (64 events).
	subscriberBufferSize = 64
)

// EventBroadcaster provides in-memory pub/sub for persisted LedgerEvents.
// Subscribers register for a conversation key (agent_id) and receive events
// as they are persisted. This enables cross-client awareness without polling.
type EventBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]chan *store.LedgerEvent // conversationKey -> subID -> ch
	logger      *slog.Logger
}

// NewEventBroadcaster creates a broadcaster. Pass nil logger for default.
func NewEventBroadcaster(logger *slog.Logger) *EventBroadcaster {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventBroadcaster{
		subscribers: make(map[string]map[string]chan *store.LedgerEvent),
		logger:      logger.With("component", "broadcaster"),
	}
}

// Subscribe registers a subscriber for events on the given conversation key.
// Returns a channel that receives events and a subscription ID for later
// unsubscription. The subscription is automatically cleaned up when ctx is
// cancelled.
func (b *EventBroadcaster) Subscribe(ctx context.Context, conversationKey string) (<-chan *store.LedgerEvent, string) {
	subID := uuid.New().String()
	ch := make(chan *store.LedgerEvent, subscriberBufferSize)

	b.mu.Lock()
	if _, ok := b.subscribers[conversationKey]; !ok {
		b.subscribers[conversationKey] = make(map[string]chan *store.LedgerEvent)
	}
	b.subscribers[conversationKey][subID] = ch
	b.mu.Unlock()

	b.logger.Debug("subscriber added",
		"conversation_key", conversationKey,
		"sub_id", subID)

	// Auto-cleanup on context cancellation
	go func() {
		<-ctx.Done()
		b.Unsubscribe(conversationKey, subID)
	}()

	return ch, subID
}

// Publish sends an event to all subscribers of the given conversation key.
// If excludeSubID is non-empty, that subscriber is skipped (used to avoid
// sending events back to the originating client).
// Non-blocking: events are dropped for subscribers whose channels are full.
func (b *EventBroadcaster) Publish(conversationKey string, event *store.LedgerEvent, excludeSubID string) {
	b.mu.RLock()
	subs, ok := b.subscribers[conversationKey]
	if !ok || len(subs) == 0 {
		b.mu.RUnlock()
		return
	}

	// Copy subscriber channels under read lock to avoid holding lock during sends
	targets := make([]chan *store.LedgerEvent, 0, len(subs))
	for id, ch := range subs {
		if excludeSubID != "" && id == excludeSubID {
			continue
		}
		targets = append(targets, ch)
	}
	b.mu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- event:
			// Sent
		default:
			// Subscriber channel full — drop event for this subscriber
			b.logger.Debug("dropped event for slow subscriber",
				"conversation_key", conversationKey,
				"event_id", event.ID)
		}
	}
}

// Unsubscribe removes a subscription and closes its channel.
func (b *EventBroadcaster) Unsubscribe(conversationKey, subID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, ok := b.subscribers[conversationKey]
	if !ok {
		return
	}

	ch, exists := subs[subID]
	if !exists {
		return
	}

	delete(subs, subID)
	close(ch)

	// Clean up empty conversation key entries
	if len(subs) == 0 {
		delete(b.subscribers, conversationKey)
	}

	b.logger.Debug("subscriber removed",
		"conversation_key", conversationKey,
		"sub_id", subID)
}

// Close shuts down the broadcaster and closes all subscriber channels.
func (b *EventBroadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for convKey, subs := range b.subscribers {
		for subID, ch := range subs {
			close(ch)
			delete(subs, subID)
		}
		delete(b.subscribers, convKey)
	}

	b.logger.Debug("broadcaster closed")
}
