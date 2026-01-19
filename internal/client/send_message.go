// ABOUTME: SendMessage RPC handler for direct client message sending with deduplication.
// ABOUTME: Uses idempotency keys to prevent duplicate message processing.

package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/fold-gateway/internal/dedupe"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// DedupeCache defines the interface for deduplication operations
type DedupeCache interface {
	Check(key string) bool
	Mark(key string)
}

// SendMessage handles a direct client message with idempotency key deduplication.
// It validates the idempotency key and returns "duplicate" status if the key has been seen.
func (s *ClientService) SendMessage(ctx context.Context, req *pb.ClientSendMessageRequest) (*pb.ClientSendMessageResponse, error) {
	// Validate idempotency key
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key required")
	}
	if len(req.IdempotencyKey) > 100 {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key too long")
	}

	// Validate required fields
	if req.ConversationKey == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_key required")
	}
	if req.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "content required")
	}

	// Check dedupe - use "client:" prefix to avoid collisions with bridge keys
	key := fmt.Sprintf("client:%s", req.IdempotencyKey)
	if s.dedupe != nil && s.dedupe.Check(key) {
		slog.Debug("duplicate client message ignored",
			"idempotency_key", req.IdempotencyKey,
		)
		return &pb.ClientSendMessageResponse{
			Status: "duplicate",
		}, nil
	}

	// Process message
	messageID, err := s.processClientMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	// Mark after success
	if s.dedupe != nil {
		s.dedupe.Mark(key)
	}

	return &pb.ClientSendMessageResponse{
		Status:    "accepted",
		MessageId: messageID,
	}, nil
}

// processClientMessage handles the actual message processing logic.
// This is separated from SendMessage to allow the dedupe pattern:
// check -> process -> mark (only mark after successful processing).
func (s *ClientService) processClientMessage(ctx context.Context, req *pb.ClientSendMessageRequest) (string, error) {
	// Generate a unique message ID
	messageID := uuid.New().String()

	// The actual message routing to agents would go here.
	// For now, this returns the generated message ID.
	// The core dedupe logic is what this task implements.

	return messageID, nil
}

// NewClientServiceWithDedupe creates a new ClientService with a dedupe cache.
// This is the constructor to use when deduplication is needed for SendMessage.
func NewClientServiceWithDedupe(eventStore EventStore, principalStore PrincipalStore, dedupeCache *dedupe.Cache) *ClientService {
	return &ClientService{
		store:      eventStore,
		principals: principalStore,
		dedupe:     dedupeCache,
	}
}

// NewClientServiceFull creates a new ClientService with all dependencies.
// This is the constructor to use when all features are needed including agent listing.
func NewClientServiceFull(eventStore EventStore, principalStore PrincipalStore, dedupeCache *dedupe.Cache, agents AgentLister) *ClientService {
	return &ClientService{
		store:      eventStore,
		principals: principalStore,
		dedupe:     dedupeCache,
		agents:     agents,
	}
}
