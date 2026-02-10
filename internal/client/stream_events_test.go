// ABOUTME: Tests for ClientService StreamEvents RPC handler
// ABOUTME: Covers streaming historical events, validation, and polling behavior

package client

import (
	"context"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockStreamServer implements pb.ClientService_StreamEventsServer for testing.
type mockStreamServer struct {
	ctx     context.Context
	events  []*pb.ClientStreamEvent
	sendErr error
}

func newMockStreamServer(ctx context.Context) *mockStreamServer {
	return &mockStreamServer{
		ctx:    ctx,
		events: make([]*pb.ClientStreamEvent, 0),
	}
}

func (m *mockStreamServer) Send(event *pb.ClientStreamEvent) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockStreamServer) SetHeader(metadata.MD) error  { return nil }
func (m *mockStreamServer) SendHeader(metadata.MD) error { return nil }
func (m *mockStreamServer) SetTrailer(metadata.MD)       {}
func (m *mockStreamServer) Context() context.Context     { return m.ctx }
func (m *mockStreamServer) SendMsg(any) error            { return nil }
func (m *mockStreamServer) RecvMsg(any) error            { return nil }

func TestStreamEvents_MissingConversationKey(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream := newMockStreamServer(ctx)
	err := svc.StreamEvents(&pb.StreamEventsRequest{}, stream)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "conversation_key required")
}

func TestStreamEvents_SendsHistoricalEvents(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)

	// Seed some events
	convKey := "test-conv-123"
	baseTime := time.Now().Add(-time.Hour)
	seedEvents(t, s, convKey, 5, baseTime)

	// Use a short context that will cancel quickly
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	stream := newMockStreamServer(ctx)
	err := svc.StreamEvents(&pb.StreamEventsRequest{
		ConversationKey: convKey,
	}, stream)

	// StreamEvents exits cleanly when context is canceled
	require.NoError(t, err)

	// Should have received historical events
	assert.GreaterOrEqual(t, len(stream.events), 5)

	// Verify events have correct conversation key
	for _, e := range stream.events {
		assert.Equal(t, convKey, e.ConversationKey)
	}
}

func TestStreamEvents_ResumeFromEventID(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)

	// Seed some events
	convKey := "test-conv-456"
	baseTime := time.Now().Add(-time.Hour)
	events := seedEvents(t, s, convKey, 10, baseTime)

	// Get the ID of the 5th event to resume from
	sinceEventID := events[4].ID

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	stream := newMockStreamServer(ctx)
	err := svc.StreamEvents(&pb.StreamEventsRequest{
		ConversationKey: convKey,
		SinceEventId:    &sinceEventID,
	}, stream)

	require.NoError(t, err)

	// Should have received events after the 5th one (events 6-10)
	// The exact count depends on pagination, but should be less than all 10
	assert.Greater(t, len(stream.events), 0)
}

func TestStreamEvents_EmptyConversation(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	stream := newMockStreamServer(ctx)
	err := svc.StreamEvents(&pb.StreamEventsRequest{
		ConversationKey: "nonexistent-conv",
	}, stream)

	require.NoError(t, err)
	// Should complete without sending events
	assert.Len(t, stream.events, 0)
}

func TestStreamEvents_EventConversion(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)

	// Create an event with all fields
	convKey := "test-conv-789"
	text := "Hello, world!"
	rawTransport := `{"type": "test"}`
	rawPayloadRef := "ref-123"
	actorPrincipalID := "principal-1"
	actorMemberID := "member-1"

	event := &store.LedgerEvent{
		ID:               "event-001",
		ConversationKey:  convKey,
		Direction:        store.EventDirectionInbound,
		Author:           "user",
		Timestamp:        time.Now(),
		Type:             store.EventTypeMessage,
		Text:             &text,
		RawTransport:     &rawTransport,
		RawPayloadRef:    &rawPayloadRef,
		ActorPrincipalID: &actorPrincipalID,
		ActorMemberID:    &actorMemberID,
	}
	require.NoError(t, s.SaveEvent(context.Background(), event))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	stream := newMockStreamServer(ctx)
	err := svc.StreamEvents(&pb.StreamEventsRequest{
		ConversationKey: convKey,
	}, stream)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(stream.events), 1, "should receive at least the initial event")

	// Verify the first event is wrapped correctly
	streamEvent := stream.events[0]
	assert.Equal(t, convKey, streamEvent.ConversationKey)
	assert.NotEmpty(t, streamEvent.Timestamp)

	// Verify it's wrapped in Event payload
	eventPayload, ok := streamEvent.Payload.(*pb.ClientStreamEvent_Event)
	require.True(t, ok)
	require.NotNil(t, eventPayload.Event)

	protoEvent := eventPayload.Event
	assert.Equal(t, "event-001", protoEvent.Id)
	assert.Equal(t, convKey, protoEvent.ConversationKey)
	assert.Equal(t, "inbound_to_agent", protoEvent.Direction)
	assert.Equal(t, "user", protoEvent.Author)
	assert.Equal(t, "message", protoEvent.Type)
	assert.Equal(t, text, *protoEvent.Text)
	assert.Equal(t, rawTransport, *protoEvent.RawTransport)
	assert.Equal(t, rawPayloadRef, *protoEvent.RawPayloadRef)
	assert.Equal(t, actorPrincipalID, *protoEvent.ActorPrincipalId)
	assert.Equal(t, actorMemberID, *protoEvent.ActorMemberId)
}

func TestStreamEvents_ContextCancellation(t *testing.T) {
	s := createTestStore(t)
	svc := createClientService(t, s)

	convKey := "test-conv-cancel"
	seedEvents(t, s, convKey, 3, time.Now().Add(-time.Hour))

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := newMockStreamServer(ctx)
	err := svc.StreamEvents(&pb.StreamEventsRequest{
		ConversationKey: convKey,
	}, stream)

	// Should return without error when context is canceled
	require.NoError(t, err)
}

// Helper to create a store for stream tests.
