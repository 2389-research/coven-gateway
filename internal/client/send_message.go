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

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/dedupe"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// DedupeCache defines the interface for deduplication operations.
type DedupeCache interface {
	Check(key string) bool
	Mark(key string)
}

// MessageRouter defines the interface for routing messages to agents.
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
	key := "client:" + req.IdempotencyKey
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

	// Use agentID as threadID so all frontends (TUI, web) share one
	// conversation per agent. The agent sees a single continuous thread.
	threadID := conversationKey

	// Store inbound message event
	if s.store != nil {
		inboundEvent := &store.LedgerEvent{
			ID:              messageID,
			ConversationKey: conversationKey,
			ThreadID:        &threadID,
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

	// Use a detached context that preserves request-scoped values (auth, tracing)
	// but won't cancel when the RPC returns. Agent response handling must continue
	// after the client gets the acceptance response.
	routingCtx := context.WithoutCancel(ctx)

	respChan, err := s.router.SendMessage(routingCtx, &agent.SendRequest{
		AgentID:  conversationKey,
		ThreadID: threadID,
		Content:  req.Content,
		Sender:   "client",
	})
	if err != nil {
		// Store error event so the ledger reflects the failed delivery attempt
		if s.store != nil {
			errText := err.Error()
			errEvent := &store.LedgerEvent{
				ID:              uuid.New().String(),
				ConversationKey: conversationKey,
				Direction:       store.EventDirectionOutbound,
				Author:          "system",
				Timestamp:       time.Now(),
				Type:            store.EventTypeError,
				Text:            &errText,
			}
			if storeErr := s.store.SaveEvent(ctx, errEvent); storeErr != nil {
				slog.Error("failed to store routing error event",
					"error", storeErr,
					"original_error", err,
				)
			}
		}
		if errors.Is(err, agent.ErrAgentNotFound) {
			return "", status.Error(codes.NotFound, "agent not found")
		}
		return "", status.Error(codes.Unavailable, "agent unavailable")
	}

	// Spawn goroutine to consume responses with a bounded timeout to prevent leaks
	consumeCtx, cancel := context.WithTimeout(routingCtx, 10*time.Minute)
	go func() {
		defer cancel()
		s.consumeAgentResponses(consumeCtx, conversationKey, threadID, respChan)
	}()

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
	if respChan == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			slog.Warn("agent response consumption canceled",
				"conversation_key", conversationKey,
				"reason", ctx.Err(),
			)
			return
		case resp, ok := <-respChan:
			if !ok {
				return
			}

			// Broadcast text chunks directly without saving to ledger.
			// EventDone carries the final accumulated text for persistence.
			if resp.Event == agent.EventText && s.broadcaster != nil && resp.Text != "" {
				chunk := &store.LedgerEvent{
					ID:              uuid.New().String(),
					ConversationKey: conversationKey,
					ThreadID:        &threadID,
					Direction:       store.EventDirectionOutbound,
					Author:          "agent",
					Type:            store.EventTypeTextChunk,
					Text:            &resp.Text,
					Timestamp:       time.Now(),
				}
				s.broadcaster.Publish(conversationKey, chunk, "")
				continue
			}

			event := s.responseToLedgerEvent(conversationKey, threadID, resp)
			if event != nil && s.store != nil {
				if err := s.store.SaveEvent(ctx, event); err != nil {
					slog.Error("failed to store agent response",
						"error", err,
						"conversation_key", conversationKey,
						"event_type", resp.Event,
					)
				} else if s.broadcaster != nil {
					s.broadcaster.Publish(conversationKey, event, "")
				}
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
		ThreadID:        &threadID,
		Direction:       store.EventDirectionOutbound,
		Author:          "agent",
		Timestamp:       time.Now(),
	}

	switch resp.Event {
	case agent.EventText:
		// Skip streaming text chunks for ledger storage. EventDone carries
		// the final accumulated text, so storing chunks would create duplicates.
		return nil
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
