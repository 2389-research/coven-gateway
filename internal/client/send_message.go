// ABOUTME: SendMessage RPC handler for direct client message sending with deduplication.
// ABOUTME: Routes messages to agents and stores events for async response handling.

package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/dedupe"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// DedupeCache defines the interface for deduplication operations
type DedupeCache interface {
	Check(key string) bool
	Mark(key string)
}

// MessageRouter defines the interface for routing messages to agents
type MessageRouter interface {
	SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
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
// It stores the inbound message event, routes to the agent, and spawns
// a goroutine to consume and store agent responses.
func (s *ClientService) processClientMessage(ctx context.Context, req *pb.ClientSendMessageRequest) (string, error) {
	messageID := uuid.New().String()
	conversationKey := req.ConversationKey

	// Store inbound message event
	if s.store != nil {
		inboundEvent := &store.LedgerEvent{
			ID:              messageID,
			ConversationKey: conversationKey,
			Direction:       store.EventDirectionInbound,
			Author:          "client",
			Timestamp:       time.Now(),
			Type:            store.EventTypeMessage,
			Text:            &req.Content,
		}

		if err := s.store.SaveEvent(ctx, inboundEvent); err != nil {
			slog.Error("failed to store inbound message",
				"error", err,
				"message_id", messageID,
			)
			return "", status.Error(codes.Internal, "failed to store message")
		}
	}

	// Route to agent (conversation_key is the agent ID for direct messages)
	if s.router == nil {
		return messageID, nil
	}

	// Use a detached context for routing - the response handling should continue
	// even after the RPC returns to the client. The RPC context would cancel
	// when the client gets the response, but we need to keep processing
	// agent responses asynchronously.
	respChan, err := s.router.SendMessage(context.Background(), &agent.SendRequest{
		AgentID:  conversationKey,
		ThreadID: messageID,
		Content:  req.Content,
		Sender:   "client",
	})
	if err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			return "", status.Error(codes.NotFound, "agent not found")
		}
		return "", status.Error(codes.Unavailable, "agent unavailable")
	}

	// Spawn goroutine to consume responses asynchronously
	go s.consumeAgentResponses(context.Background(), conversationKey, messageID, respChan)

	return messageID, nil
}

// consumeAgentResponses reads responses from the agent channel and stores them as events.
// This runs asynchronously after the RPC returns to the client.
func (s *ClientService) consumeAgentResponses(
	ctx context.Context,
	conversationKey string,
	threadID string,
	respChan <-chan *agent.Response,
) {
	for resp := range respChan {
		event := s.responseToLedgerEvent(conversationKey, threadID, resp)
		if event != nil {
			if err := s.store.SaveEvent(ctx, event); err != nil {
				slog.Error("failed to store agent response",
					"error", err,
					"conversation_key", conversationKey,
					"event_type", resp.Event,
				)
			}
		}
	}
}

// responseToLedgerEvent converts an agent Response to a LedgerEvent for storage.
// Returns nil for event types that should not be stored.
func (s *ClientService) responseToLedgerEvent(convKey, threadID string, resp *agent.Response) *store.LedgerEvent {
	event := &store.LedgerEvent{
		ID:              uuid.New().String(),
		ConversationKey: convKey,
		Direction:       store.EventDirectionOutbound,
		Author:          "agent",
		Timestamp:       time.Now(),
	}

	switch resp.Event {
	case agent.EventText:
		event.Type = store.EventTypeMessage
		event.Text = &resp.Text
	case agent.EventThinking:
		// Skip thinking events
		return nil
	case agent.EventToolUse:
		event.Type = store.EventTypeToolCall
		if resp.ToolUse != nil {
			toolInfo := fmt.Sprintf("tool:%s input:%s", resp.ToolUse.Name, resp.ToolUse.InputJSON)
			event.Text = &toolInfo
		}
	case agent.EventToolResult:
		event.Type = store.EventTypeToolResult
		if resp.ToolResult != nil {
			event.Text = &resp.ToolResult.Output
		}
	case agent.EventError:
		event.Type = store.EventTypeError
		event.Text = &resp.Error
	case agent.EventDone:
		// Store final response only if it has content
		if resp.Text != "" {
			event.Type = store.EventTypeMessage
			event.Text = &resp.Text
		} else {
			return nil
		}
	default:
		// Skip other event types
		return nil
	}

	return event
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

// NewClientServiceWithRouter creates a new ClientService with message routing support.
// This is the constructor to use when messages need to be routed to agents.
func NewClientServiceWithRouter(
	eventStore EventStore,
	principalStore PrincipalStore,
	dedupeCache *dedupe.Cache,
	agents AgentLister,
	router MessageRouter,
) *ClientService {
	return &ClientService{
		store:      eventStore,
		principals: principalStore,
		dedupe:     dedupeCache,
		agents:     agents,
		router:     router,
	}
}
