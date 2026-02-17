// ABOUTME: Tests for ClientService SendMessage RPC with idempotency key deduplication.
// ABOUTME: Validates idempotency key validation, duplicate detection, and message routing.

package client

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/conversation"
	"github.com/2389/coven-gateway/internal/dedupe"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

func TestSendMessage_MissingIdempotencyKey(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  "", // missing
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "idempotency_key required", st.Message())
}

func TestSendMessage_IdempotencyKeyTooLong(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  strings.Repeat("a", 101), // 101 chars, max is 100
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "idempotency_key too long", st.Message())
}

func TestSendMessage_FirstRequest(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  "unique-key-123",
	}

	resp, err := svc.SendMessage(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.MessageId)
}

func TestSendMessage_DuplicateRequest(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  "duplicate-key-456",
	}

	// First request should succeed
	resp1, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp1.Status)

	// Second request with same idempotency key should return "duplicate"
	resp2, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", resp2.Status)
	assert.Empty(t, resp2.MessageId, "duplicate should not return a message ID")
}

func TestSendMessage_DifferentKeys(t *testing.T) {
	svc := newTestClientService(t)

	req1 := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "First message",
		IdempotencyKey:  "key-1",
	}

	req2 := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Second message",
		IdempotencyKey:  "key-2",
	}

	// Both requests with different keys should succeed
	resp1, err := svc.SendMessage(context.Background(), req1)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp1.Status)

	resp2, err := svc.SendMessage(context.Background(), req2)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp2.Status)

	// IDs should be different
	assert.NotEqual(t, resp1.MessageId, resp2.MessageId)
}

func TestSendMessage_MaxLengthIdempotencyKey(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  strings.Repeat("a", 100), // exactly 100 chars (the max)
	}

	resp, err := svc.SendMessage(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
}

func TestSendMessage_MissingConversationKey(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "", // missing
		Content:         "Hello, world!",
		IdempotencyKey:  "unique-key-789",
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "conversation_key required", st.Message())
}

func TestSendMessage_MissingContent(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "", // missing
		IdempotencyKey:  "unique-key-abc",
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "content required", st.Message())
}

// newTestClientService creates a ClientService with a real dedupe cache for testing.
func newTestClientService(t *testing.T) *ClientService {
	t.Helper()

	// Create a dedupe cache with a 5-minute TTL (same as production)
	dedupeCache := dedupe.New(5*time.Minute, 1000)
	t.Cleanup(func() {
		dedupeCache.Close()
	})

	return NewClientServiceWithDedupe(nil, nil, dedupeCache)
}

// mockEventStore implements EventStore for testing.
type mockEventStore struct {
	mu     sync.Mutex
	events []*store.LedgerEvent
}

func (m *mockEventStore) GetEvents(ctx context.Context, params store.GetEventsParams) (*store.GetEventsResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []store.LedgerEvent
	for _, e := range m.events {
		if e.ConversationKey == params.ConversationKey {
			filtered = append(filtered, *e)
		}
	}
	return &store.GetEventsResult{Events: filtered}, nil
}

func (m *mockEventStore) SaveEvent(ctx context.Context, event *store.LedgerEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventStore) getEvents() []*store.LedgerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*store.LedgerEvent, len(m.events))
	copy(result, m.events)
	return result
}

// mockRouter implements MessageRouter for testing.
type mockRouter struct {
	mu          sync.Mutex
	requests    []*agent.SendRequest
	responses   []*agent.Response
	err         error
	agentOnline bool
}

func (m *mockRouter) SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests = append(m.requests, req)

	if m.err != nil {
		return nil, m.err
	}

	ch := make(chan *agent.Response, len(m.responses)+1)
	for _, r := range m.responses {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *mockRouter) getRequests() []*agent.SendRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*agent.SendRequest, len(m.requests))
	copy(result, m.requests)
	return result
}

// newTestClientServiceWithRouting creates a ClientService with routing support for testing.
func newTestClientServiceWithRouting(t *testing.T, eventStore *mockEventStore, router *mockRouter) *ClientService {
	t.Helper()

	dedupeCache := dedupe.New(5*time.Minute, 1000)
	t.Cleanup(func() {
		dedupeCache.Close()
	})

	return NewClientServiceWithRouter(eventStore, nil, dedupeCache, nil, router)
}

// Tests for message routing functionality

func TestProcessClientMessage_StoresInboundEvent(t *testing.T) {
	eventStore := &mockEventStore{}
	router := &mockRouter{agentOnline: true}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "agent-123",
		Content:         "Hello, agent!",
		IdempotencyKey:  "test-key-1",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.MessageId)

	// Give async processing time to complete
	time.Sleep(50 * time.Millisecond)

	// Verify inbound event was stored
	events := eventStore.getEvents()
	require.NotEmpty(t, events, "expected at least one event to be stored")

	// Find the inbound event
	var inboundEvent *store.LedgerEvent
	for _, e := range events {
		if e.Direction == store.EventDirectionInbound {
			inboundEvent = e
			break
		}
	}
	require.NotNil(t, inboundEvent, "expected inbound event to be stored")
	assert.Equal(t, "agent-123", inboundEvent.ConversationKey)
	assert.Equal(t, store.EventTypeMessage, inboundEvent.Type)
	assert.Equal(t, "Hello, agent!", *inboundEvent.Text)
}

func TestProcessClientMessage_RoutesToAgent(t *testing.T) {
	eventStore := &mockEventStore{}
	router := &mockRouter{agentOnline: true}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "agent-456",
		Content:         "Route this message",
		IdempotencyKey:  "test-key-2",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)

	// Verify message was routed to agent
	requests := router.getRequests()
	require.Len(t, requests, 1, "expected one request to be sent to router")
	assert.Equal(t, "agent-456", requests[0].AgentID)
	assert.Equal(t, "Route this message", requests[0].Content)
}

func TestProcessClientMessage_StableThreadID(t *testing.T) {
	eventStore := &mockEventStore{}
	router := &mockRouter{agentOnline: true}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	// Send two messages to the same agent
	for i, content := range []string{"First message", "Second message"} {
		req := &pb.ClientSendMessageRequest{
			ConversationKey: "agent-stable",
			Content:         content,
			IdempotencyKey:  fmt.Sprintf("stable-key-%d", i),
		}
		resp, err := svc.SendMessage(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "accepted", resp.Status)
	}

	// Both messages should have been routed with the same stable ThreadID
	requests := router.getRequests()
	require.Len(t, requests, 2, "expected two requests to be sent to router")
	assert.Equal(t, requests[0].ThreadID, requests[1].ThreadID,
		"ThreadID should be stable across messages to the same agent")
	assert.Equal(t, "agent-stable", requests[0].ThreadID,
		"ThreadID should equal the conversation key (agent ID)")

	// Verify inbound events also have the stable ThreadID
	time.Sleep(50 * time.Millisecond)
	events := eventStore.getEvents()
	for _, e := range events {
		if e.Direction == store.EventDirectionInbound {
			require.NotNil(t, e.ThreadID, "inbound event should have ThreadID")
			assert.Equal(t, "agent-stable", *e.ThreadID)
		}
	}
}

func TestProcessClientMessage_AgentNotFound(t *testing.T) {
	eventStore := &mockEventStore{}
	router := &mockRouter{err: agent.ErrAgentNotFound}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "nonexistent-agent",
		Content:         "Hello?",
		IdempotencyKey:  "test-key-3",
	}

	_, err := svc.SendMessage(context.Background(), req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "agent not found")
}

func TestProcessClientMessage_StoresAgentResponses(t *testing.T) {
	eventStore := &mockEventStore{}
	finalContent := "Hello from the agent!"
	router := &mockRouter{
		agentOnline: true,
		responses: []*agent.Response{
			// EventText chunks are skipped for ledger storage to avoid
			// duplicating with EventDone which carries the final text.
			{Event: agent.EventText, Text: "Hello from"},
			{Event: agent.EventText, Text: " the agent!"},
			{Event: agent.EventDone, Text: finalContent, Done: true},
		},
	}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "agent-789",
		Content:         "Test message",
		IdempotencyKey:  "test-key-4",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)

	// Give async processing time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify both inbound and outbound events were stored
	events := eventStore.getEvents()
	require.GreaterOrEqual(t, len(events), 2, "expected at least 2 events (inbound + done)")

	// Find outbound final message event (from EventDone)
	var textEvent *store.LedgerEvent
	for _, e := range events {
		if e.Direction == store.EventDirectionOutbound && e.Type == store.EventTypeMessage {
			textEvent = e
			break
		}
	}
	require.NotNil(t, textEvent, "expected outbound message event from EventDone")
	assert.Equal(t, finalContent, *textEvent.Text)
}

func TestProcessClientMessage_AgentError(t *testing.T) {
	eventStore := &mockEventStore{}
	router := &mockRouter{
		agentOnline: true,
		responses: []*agent.Response{
			{Event: agent.EventError, Error: "something went wrong", Done: true},
		},
	}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "agent-error",
		Content:         "Trigger error",
		IdempotencyKey:  "test-key-5",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)

	// Give async processing time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify error event was stored
	events := eventStore.getEvents()
	var errorEvent *store.LedgerEvent
	for _, e := range events {
		if e.Type == store.EventTypeError {
			errorEvent = e
			break
		}
	}
	require.NotNil(t, errorEvent, "expected error event to be stored")
	assert.Equal(t, "something went wrong", *errorEvent.Text)
}

func TestConsumeAgentResponses_BroadcastsEvents(t *testing.T) {
	eventStore := &mockEventStore{}
	finalContent := "Broadcasted response!"
	router := &mockRouter{
		agentOnline: true,
		responses: []*agent.Response{
			{Event: agent.EventText, Text: "Broadcasted"},            // skipped (not persisted)
			{Event: agent.EventThinking, Text: "thinking..."},        // skipped (not persisted)
			{Event: agent.EventDone, Text: finalContent, Done: true}, // persisted + broadcast
		},
	}
	svc := newTestClientServiceWithRouting(t, eventStore, router)

	// Set up broadcaster and subscribe before sending
	broadcaster := conversation.NewEventBroadcaster(nil)
	t.Cleanup(func() { broadcaster.Close() })
	svc.SetBroadcaster(broadcaster)

	eventCh, _ := broadcaster.Subscribe(t.Context(), "agent-broadcast")

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "agent-broadcast",
		Content:         "Trigger broadcast",
		IdempotencyKey:  "broadcast-key-1",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)

	// Wait for broadcast event with a reasonable timeout
	select {
	case event := <-eventCh:
		require.NotNil(t, event, "expected a non-nil broadcast event")
		assert.Equal(t, store.EventTypeMessage, event.Type)
		assert.Equal(t, finalContent, *event.Text)
		assert.Equal(t, "agent-broadcast", event.ConversationKey)
		assert.Equal(t, store.EventDirectionOutbound, event.Direction)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}

func TestConsumeAgentResponses_NoBroadcasterNoPanic(t *testing.T) {
	// Verify that consumeAgentResponses works without a broadcaster (nil-safe)
	eventStore := &mockEventStore{}
	router := &mockRouter{
		agentOnline: true,
		responses: []*agent.Response{
			{Event: agent.EventDone, Text: "No broadcaster set", Done: true},
		},
	}
	svc := newTestClientServiceWithRouting(t, eventStore, router)
	// Intentionally do NOT set a broadcaster

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "agent-no-broadcaster",
		Content:         "Should not panic",
		IdempotencyKey:  "no-broadcaster-key",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)

	// Give async processing time
	time.Sleep(100 * time.Millisecond)

	// Events should still be stored even without broadcaster
	events := eventStore.getEvents()
	require.GreaterOrEqual(t, len(events), 2, "expected inbound + outbound events stored")
}

func TestProcessClientMessage_NoRouterConfigured(t *testing.T) {
	// Test that message is accepted even without a router (just stores inbound event)
	eventStore := &mockEventStore{}
	dedupeCache := dedupe.New(5*time.Minute, 1000)
	t.Cleanup(func() {
		dedupeCache.Close()
	})

	// Create service without router
	svc := NewClientServiceWithRouter(eventStore, nil, dedupeCache, nil, nil)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "some-agent",
		Content:         "Hello",
		IdempotencyKey:  "test-key-6",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.MessageId)

	// Verify inbound event was still stored
	events := eventStore.getEvents()
	require.Len(t, events, 1)
	assert.Equal(t, store.EventDirectionInbound, events[0].Direction)
}
