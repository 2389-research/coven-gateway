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

// GetEvents tests - TDD tests for the History Store feature

func TestGetEvents_Basic(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:basic"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 5 events
	for i := 0; i < 5; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("basic-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch events with default params
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 5)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.NextCursor)

	// Verify chronological order
	for i := 1; i < len(result.Events); i++ {
		assert.True(t, result.Events[i-1].Timestamp.Before(result.Events[i].Timestamp) ||
			result.Events[i-1].Timestamp.Equal(result.Events[i].Timestamp))
	}
}

func TestGetEvents_Empty(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: "nonexistent:conversation",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 0)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.NextCursor)
}

func TestGetEvents_WithSince(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:since"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 5 events at 1-second intervals
	for i := 0; i < 5; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("since-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch events since the 3rd event (index 2)
	since := baseTime.Add(2 * time.Second)
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Since:           &since,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 3) // Events at t+2s, t+3s, t+4s
	assert.False(t, result.HasMore)
}

func TestGetEvents_WithUntil(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:until"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 5 events at 1-second intervals
	for i := 0; i < 5; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("until-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch events until the 3rd event (index 2)
	until := baseTime.Add(2 * time.Second)
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Until:           &until,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 3) // Events at t+0s, t+1s, t+2s
	assert.False(t, result.HasMore)
}

func TestGetEvents_WithBoth(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:both"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 5 events at 1-second intervals
	for i := 0; i < 5; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("both-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch events between 1s and 3s
	since := baseTime.Add(1 * time.Second)
	until := baseTime.Add(3 * time.Second)
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Since:           &since,
		Until:           &until,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 3) // Events at t+1s, t+2s, t+3s
	assert.False(t, result.HasMore)
}

func TestGetEvents_Pagination_FirstPage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:pagination"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 10 events
	for i := 0; i < 10; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("page-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch first page of 3
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Limit:           3,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 3)
	assert.True(t, result.HasMore)
	assert.NotEmpty(t, result.NextCursor)

	// Verify we got the first 3 events (chronologically)
	assert.Equal(t, "page-event-a", result.Events[0].ID)
	assert.Equal(t, "page-event-b", result.Events[1].ID)
	assert.Equal(t, "page-event-c", result.Events[2].ID)
}

func TestGetEvents_Pagination_SecondPage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:pagination2"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 10 events
	for i := 0; i < 10; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("page2-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch first page
	result1, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Limit:           3,
	})
	require.NoError(t, err)
	require.True(t, result1.HasMore)
	require.NotEmpty(t, result1.NextCursor)

	// Fetch second page using cursor
	result2, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Limit:           3,
		Cursor:          result1.NextCursor,
	})
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Len(t, result2.Events, 3)
	assert.True(t, result2.HasMore)
	assert.NotEmpty(t, result2.NextCursor)

	// Verify we got events 4-6 (indices 3, 4, 5)
	assert.Equal(t, "page2-event-d", result2.Events[0].ID)
	assert.Equal(t, "page2-event-e", result2.Events[1].ID)
	assert.Equal(t, "page2-event-f", result2.Events[2].ID)
}

func TestGetEvents_Pagination_LastPage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:lastpage"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 5 events
	for i := 0; i < 5; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("last-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message " + string(rune('A'+i))),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch first page of 3
	result1, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Limit:           3,
	})
	require.NoError(t, err)
	require.True(t, result1.HasMore)

	// Fetch second (last) page
	result2, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Limit:           3,
		Cursor:          result1.NextCursor,
	})
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Len(t, result2.Events, 2) // Only 2 events remaining
	assert.False(t, result2.HasMore)
	assert.Empty(t, result2.NextCursor)
}

func TestGetEvents_LimitCapped(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:limitcap"

	// Test that limit is capped to 500
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Limit:           1000, // exceeds max
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Even though limit was 1000, it should be capped to 500
	// Since we have no events, we just verify no error
}

func TestGetEvents_InvalidCursor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:badcursor"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create an event so we have something to query
	event := &LedgerEvent{
		ID:              "bad-cursor-event",
		ConversationKey: convKey,
		Direction:       EventDirectionInbound,
		Author:          "user",
		Timestamp:       baseTime,
		Type:            EventTypeMessage,
		Text:            strPtr("Test"),
	}
	require.NoError(t, store.SaveEvent(ctx, event))

	// Test with invalid cursor (not valid base64)
	_, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Cursor:          "not-valid-base64!!!",
	})
	assert.Error(t, err)

	// Test with valid base64 but invalid format
	_, err = store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
		Cursor:          "dGVzdA==", // "test" in base64, missing pipe separator
	})
	assert.Error(t, err)
}

func TestGetEvents_RequiresConversationKey(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetEvents(ctx, GetEventsParams{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversation_key required")
}

func TestGetEvents_DefaultLimit(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	convKey := "test:conversation:defaultlimit"
	baseTime := time.Now().UTC().Truncate(time.Second)

	// Create 60 events (more than default 50)
	for i := 0; i < 60; i++ {
		event := &LedgerEvent{
			ID:              generateTestID("deflim-event", i),
			ConversationKey: convKey,
			Direction:       EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            EventTypeMessage,
			Text:            strPtr("Message"),
		}
		require.NoError(t, store.SaveEvent(ctx, event))
	}

	// Fetch with no limit specified (should default to 50)
	result, err := store.GetEvents(ctx, GetEventsParams{
		ConversationKey: convKey,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Events, 50)
	assert.True(t, result.HasMore)
}
