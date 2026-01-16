// ABOUTME: Tests for ClientService GetEvents RPC handler
// ABOUTME: Covers success cases, validation errors, and timestamp parsing for history retrieval

package client

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/2389/fold-gateway/internal/auth"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// createTestStore creates a real SQLite store in a temp directory
func createTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLite store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	return s
}

// createClientService creates a ClientService with the given store
func createClientService(t *testing.T, s *store.SQLiteStore) *ClientService {
	t.Helper()
	return NewClientService(s)
}

// createMemberContext creates a context with member auth for testing
func createMemberContext(principalID string) context.Context {
	authCtx := &auth.AuthContext{
		PrincipalID:   principalID,
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	return auth.WithAuth(context.Background(), authCtx)
}

// seedEvents creates test events in the store
func seedEvents(t *testing.T, s *store.SQLiteStore, convKey string, count int, baseTime time.Time) []store.LedgerEvent {
	t.Helper()
	ctx := context.Background()

	events := make([]store.LedgerEvent, count)
	for i := 0; i < count; i++ {
		text := "Message " + string(rune('A'+i))
		event := &store.LedgerEvent{
			ID:              generateTestID("event", i),
			ConversationKey: convKey,
			Direction:       store.EventDirectionInbound,
			Author:          "user",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			Type:            store.EventTypeMessage,
			Text:            &text,
		}
		require.NoError(t, s.SaveEvent(ctx, event))
		events[i] = *event
	}

	return events
}

// generateTestID generates deterministic test IDs
func generateTestID(prefix string, index int) string {
	suffix := string(rune('a' + index))
	return prefix + "-" + suffix
}

func TestGetEventsRPC_Success(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	convKey := "test:conversation:success"
	baseTime := time.Now().UTC().Truncate(time.Second)
	seedEvents(t, s, convKey, 5, baseTime)

	req := &pb.GetEventsRequest{
		ConversationKey: convKey,
	}

	resp, err := svc.GetEvents(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Len(t, resp.Events, 5)
	assert.False(t, resp.HasMore)

	// Verify events are in chronological order
	for i := 1; i < len(resp.Events); i++ {
		assert.True(t, resp.Events[i-1].Timestamp <= resp.Events[i].Timestamp)
	}

	// Verify event fields are properly mapped
	assert.Equal(t, "event-a", resp.Events[0].Id)
	assert.Equal(t, convKey, resp.Events[0].ConversationKey)
	assert.Equal(t, "inbound_to_agent", resp.Events[0].Direction)
	assert.Equal(t, "user", resp.Events[0].Author)
	assert.Equal(t, "message", resp.Events[0].Type)
	assert.NotNil(t, resp.Events[0].Text)
	assert.Equal(t, "Message A", *resp.Events[0].Text)
}

func TestGetEventsRPC_MissingKey(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	req := &pb.GetEventsRequest{
		ConversationKey: "",
	}

	_, err := svc.GetEvents(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "conversation_key required")
}

func TestGetEventsRPC_InvalidSince(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	invalidSince := "not-a-valid-timestamp"
	req := &pb.GetEventsRequest{
		ConversationKey: "test:conversation",
		Since:           &invalidSince,
	}

	_, err := svc.GetEvents(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid since timestamp")
}

func TestGetEventsRPC_InvalidUntil(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	invalidUntil := "also-not-valid"
	req := &pb.GetEventsRequest{
		ConversationKey: "test:conversation",
		Until:           &invalidUntil,
	}

	_, err := svc.GetEvents(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid until timestamp")
}

func TestGetEventsRPC_WithTimeFilters(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	convKey := "test:conversation:timefilters"
	baseTime := time.Now().UTC().Truncate(time.Second)
	seedEvents(t, s, convKey, 5, baseTime)

	// Fetch events between second and fourth
	since := baseTime.Add(1 * time.Second).Format(time.RFC3339)
	until := baseTime.Add(3 * time.Second).Format(time.RFC3339)

	req := &pb.GetEventsRequest{
		ConversationKey: convKey,
		Since:           &since,
		Until:           &until,
	}

	resp, err := svc.GetEvents(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Len(t, resp.Events, 3) // Events at t+1s, t+2s, t+3s
}

func TestGetEventsRPC_Pagination(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	convKey := "test:conversation:pagination"
	baseTime := time.Now().UTC().Truncate(time.Second)
	seedEvents(t, s, convKey, 10, baseTime)

	// Fetch first page
	limit := int32(3)
	req := &pb.GetEventsRequest{
		ConversationKey: convKey,
		Limit:           &limit,
	}

	resp1, err := svc.GetEvents(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp1)

	assert.Len(t, resp1.Events, 3)
	assert.True(t, resp1.HasMore)
	require.NotNil(t, resp1.NextCursor)
	assert.NotEmpty(t, *resp1.NextCursor)

	// Fetch second page
	req2 := &pb.GetEventsRequest{
		ConversationKey: convKey,
		Limit:           &limit,
		Cursor:          resp1.NextCursor,
	}

	resp2, err := svc.GetEvents(ctx, req2)
	require.NoError(t, err)
	require.NotNil(t, resp2)

	assert.Len(t, resp2.Events, 3)
	assert.True(t, resp2.HasMore)

	// Ensure we got different events
	assert.NotEqual(t, resp1.Events[0].Id, resp2.Events[0].Id)
}

func TestGetEventsRPC_EmptyResult(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	req := &pb.GetEventsRequest{
		ConversationKey: "nonexistent:conversation",
	}

	resp, err := svc.GetEvents(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Len(t, resp.Events, 0)
	assert.False(t, resp.HasMore)
	assert.Nil(t, resp.NextCursor)
}

func TestGetEventsRPC_OptionalFields(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)
	ctx := createMemberContext("client-001")

	// Create event with optional fields populated
	convKey := "test:conversation:optional"
	principalID := "principal-123"
	memberID := "member-456"
	transport := "matrix"
	payloadRef := "ref-789"
	text := "Hello"

	event := &store.LedgerEvent{
		ID:               "event-optional",
		ConversationKey:  convKey,
		Direction:        store.EventDirectionOutbound,
		Author:           "agent",
		Timestamp:        time.Now().UTC().Truncate(time.Second),
		Type:             store.EventTypeMessage,
		Text:             &text,
		RawTransport:     &transport,
		RawPayloadRef:    &payloadRef,
		ActorPrincipalID: &principalID,
		ActorMemberID:    &memberID,
	}
	require.NoError(t, s.SaveEvent(context.Background(), event))

	req := &pb.GetEventsRequest{
		ConversationKey: convKey,
	}

	resp, err := svc.GetEvents(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.Events, 1)

	e := resp.Events[0]
	assert.Equal(t, "outbound_from_agent", e.Direction)
	assert.NotNil(t, e.Text)
	assert.Equal(t, "Hello", *e.Text)
	assert.NotNil(t, e.RawTransport)
	assert.Equal(t, "matrix", *e.RawTransport)
	assert.NotNil(t, e.RawPayloadRef)
	assert.Equal(t, "ref-789", *e.RawPayloadRef)
	assert.NotNil(t, e.ActorPrincipalId)
	assert.Equal(t, "principal-123", *e.ActorPrincipalId)
	assert.NotNil(t, e.ActorMemberId)
	assert.Equal(t, "member-456", *e.ActorMemberId)
}
