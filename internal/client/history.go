// ABOUTME: ClientService gRPC handlers for conversation history retrieval
// ABOUTME: Implements GetEvents RPC for fetching ledger events with pagination

package client

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/conversation"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// EventStore defines the store operations needed for event retrieval and storage.
type EventStore interface {
	GetEvents(ctx context.Context, params store.GetEventsParams) (*store.GetEventsResult, error)
	SaveEvent(ctx context.Context, event *store.LedgerEvent) error
}

// PrincipalStore defines the store operations needed for principal retrieval.
type PrincipalStore interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
}

// ClientService implements the ClientService gRPC service.
type ClientService struct {
	pb.UnimplementedClientServiceServer
	store       EventStore
	principals  PrincipalStore
	dedupe      DedupeCache
	agents      AgentLister
	router      MessageRouter
	approver    ToolApprover
	answerer    QuestionAnswerer
	broadcaster *conversation.EventBroadcaster
}

// NewClientService creates a new ClientService with the given stores.
func NewClientService(eventStore EventStore, principalStore PrincipalStore) *ClientService {
	return &ClientService{
		store:      eventStore,
		principals: principalStore,
	}
}

// SetBroadcaster configures the event broadcaster for push-based streaming.
// When set, StreamEvents uses the broadcaster instead of polling.
func (s *ClientService) SetBroadcaster(b *conversation.EventBroadcaster) {
	s.broadcaster = b
}

// GetEvents retrieves events for a conversation with optional filtering and pagination.
func (s *ClientService) GetEvents(ctx context.Context, req *pb.GetEventsRequest) (*pb.GetEventsResponse, error) {
	if req.ConversationKey == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_key required")
	}

	params := store.GetEventsParams{
		ConversationKey: req.ConversationKey,
		Cursor:          req.GetCursor(),
	}

	if req.Since != nil {
		t, err := time.Parse(time.RFC3339, *req.Since)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid since timestamp")
		}
		params.Since = &t
	}

	if req.Until != nil {
		t, err := time.Parse(time.RFC3339, *req.Until)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid until timestamp")
		}
		params.Until = &t
	}

	if req.Limit != nil {
		params.Limit = int(*req.Limit)
	}

	result, err := s.store.GetEvents(ctx, params)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch events")
	}

	resp := &pb.GetEventsResponse{
		Events:  toProtoEvents(result.Events),
		HasMore: result.HasMore,
	}

	if result.NextCursor != "" {
		resp.NextCursor = &result.NextCursor
	}

	return resp, nil
}

// toProtoEvents converts store.LedgerEvent slice to protobuf Event slice.
func toProtoEvents(events []store.LedgerEvent) []*pb.Event {
	pbEvents := make([]*pb.Event, len(events))
	for i, e := range events {
		pbEvents[i] = toProtoEvent(&e)
	}
	return pbEvents
}

// toProtoEvent converts a single store.LedgerEvent to a protobuf Event.
func toProtoEvent(e *store.LedgerEvent) *pb.Event {
	event := &pb.Event{
		Id:              e.ID,
		ConversationKey: e.ConversationKey,
		Direction:       string(e.Direction),
		Author:          e.Author,
		Timestamp:       e.Timestamp.Format(time.RFC3339),
		Type:            string(e.Type),
	}

	if e.Text != nil {
		event.Text = e.Text
	}
	if e.RawTransport != nil {
		event.RawTransport = e.RawTransport
	}
	if e.RawPayloadRef != nil {
		event.RawPayloadRef = e.RawPayloadRef
	}
	if e.ActorPrincipalID != nil {
		event.ActorPrincipalId = e.ActorPrincipalID
	}
	if e.ActorMemberID != nil {
		event.ActorMemberId = e.ActorMemberID
	}

	return event
}
