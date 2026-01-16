// ABOUTME: Tests for ledger event store operations
// ABOUTME: Covers CRUD, actor attribution, and filtering for the ledger_events table

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStore_SaveEvent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	event := &LedgerEvent{
		ID:              "event-123",
		ConversationKey: "matrix:!room123:server.com",
		Direction:       EventDirectionInbound,
		Author:          "user@example.com",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            EventTypeMessage,
		Text:            strPtr("Hello, world!"),
	}

	err := store.SaveEvent(ctx, event)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetEvent(ctx, "event-123")
	require.NoError(t, err)
	assert.Equal(t, "event-123", retrieved.ID)
	assert.Equal(t, "matrix:!room123:server.com", retrieved.ConversationKey)
	assert.Equal(t, EventDirectionInbound, retrieved.Direction)
	assert.Equal(t, "Hello, world!", *retrieved.Text)
}

func TestEventStore_SaveEvent_WithActorPrincipal(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	principalID := "principal-client-123"
	event := &LedgerEvent{
		ID:               "event-actor-test",
		ConversationKey:  "tui:client-1:pane-1",
		Direction:        EventDirectionInbound,
		Author:           "harper",
		Timestamp:        time.Now().UTC().Truncate(time.Second),
		Type:             EventTypeMessage,
		Text:             strPtr("Test message"),
		ActorPrincipalID: &principalID,
	}

	err := store.SaveEvent(ctx, event)
	require.NoError(t, err)

	retrieved, err := store.GetEvent(ctx, "event-actor-test")
	require.NoError(t, err)
	require.NotNil(t, retrieved.ActorPrincipalID)
	assert.Equal(t, principalID, *retrieved.ActorPrincipalID)
	assert.Nil(t, retrieved.ActorMemberID)
}

func TestEventStore_SaveEvent_WithActorPrincipalAndMember(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	principalID := "principal-client-456"
	memberID := "member-harper"
	event := &LedgerEvent{
		ID:               "event-full-actor",
		ConversationKey:  "tui:client-1:pane-1",
		Direction:        EventDirectionInbound,
		Author:           "harper",
		Timestamp:        time.Now().UTC().Truncate(time.Second),
		Type:             EventTypeMessage,
		Text:             strPtr("Test message with member"),
		ActorPrincipalID: &principalID,
		ActorMemberID:    &memberID,
	}

	err := store.SaveEvent(ctx, event)
	require.NoError(t, err)

	retrieved, err := store.GetEvent(ctx, "event-full-actor")
	require.NoError(t, err)
	require.NotNil(t, retrieved.ActorPrincipalID)
	require.NotNil(t, retrieved.ActorMemberID)
	assert.Equal(t, principalID, *retrieved.ActorPrincipalID)
	assert.Equal(t, memberID, *retrieved.ActorMemberID)
}

func TestEventStore_SaveEvent_NoActor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// System events or events before auth might not have actor
	event := &LedgerEvent{
		ID:              "event-no-actor",
		ConversationKey: "system:internal",
		Direction:       EventDirectionOutbound,
		Author:          "system",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            EventTypeSystem,
		Text:            strPtr("System notification"),
	}

	err := store.SaveEvent(ctx, event)
	require.NoError(t, err)

	retrieved, err := store.GetEvent(ctx, "event-no-actor")
	require.NoError(t, err)
	assert.Nil(t, retrieved.ActorPrincipalID)
	assert.Nil(t, retrieved.ActorMemberID)
}

func TestEventStore_GetEvent_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetEvent(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrEventNotFound)
}

func TestEventStore_ListEventsByConversation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "matrix:!testroom:server.com"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create several events for the same conversation
	for i := 0; i < 5; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('1'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Create event for different conversation
	otherEvent := &LedgerEvent{
		ID:              "other-conv-event",
		ConversationKey: "matrix:!other:server.com",
		Direction:       EventDirectionInbound,
		Author:          "user",
		Timestamp:       baseTime,
		Type:            EventTypeMessage,
		Text:            strPtr("Other conv"),
	}
	require.NoError(t, store.SaveEvent(ctx, otherEvent))

	// List events for our conversation
	events, err := store.ListEventsByConversation(ctx, convKey, 100)
	require.NoError(t, err)
	assert.Len(t, events, 5)

	// Should be in chronological order
	for i := 1; i < len(events); i++ {
		assert.True(t, events[i-1].Timestamp.Before(events[i].Timestamp) || events[i-1].Timestamp.Equal(events[i].Timestamp))
	}
}

func TestEventStore_ListEventsByActor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	principalID := "principal-test-actor"
	otherPrincipalID := "principal-other"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create events from our target principal
	for i := 0; i < 3; i++ {
		event := &LedgerEvent{
			ID:               generateTestID("actor-event", i),
			ConversationKey:  "test:conv",
			Direction:        EventDirectionInbound,
			Author:           "user",
			Timestamp:        baseTime.Add(time.Duration(i) * time.Second),
			Type:             EventTypeMessage,
			Text:             strPtr("From actor"),
			ActorPrincipalID: &principalID,
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Create event from different principal
	otherEvent := &LedgerEvent{
		ID:               "other-actor-event",
		ConversationKey:  "test:conv",
		Direction:        EventDirectionInbound,
		Author:           "other-user",
		Timestamp:        baseTime,
		Type:             EventTypeMessage,
		Text:             strPtr("From other actor"),
		ActorPrincipalID: &otherPrincipalID,
	}
	require.NoError(t, store.SaveEvent(ctx, otherEvent))

	// List events by actor
	events, err := store.ListEventsByActor(ctx, principalID, 100)
	require.NoError(t, err)
	assert.Len(t, events, 3)

	for _, e := range events {
		require.NotNil(t, e.ActorPrincipalID)
		assert.Equal(t, principalID, *e.ActorPrincipalID)
	}
}

func strPtr(s string) *string {
	return &s
}
