// ABOUTME: ConversationService is the central layer for message persistence
// ABOUTME: All messages flow through here - history is the source of truth, not a side effect

package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/store"
)

// ConversationStore defines what the service needs from storage
type ConversationStore interface {
	CreateThread(ctx context.Context, thread *store.Thread) error
	GetThread(ctx context.Context, id string) (*store.Thread, error)
	GetThreadByFrontendID(ctx context.Context, frontendName, externalID string) (*store.Thread, error)

	// Ledger events (unified message storage)
	SaveEvent(ctx context.Context, event *store.LedgerEvent) error
	GetEventsByThreadID(ctx context.Context, threadID string, limit int) ([]*store.LedgerEvent, error)

	// Token usage tracking
	SaveUsage(ctx context.Context, usage *store.TokenUsage) error
	LinkUsageToMessage(ctx context.Context, requestID, messageID string) error
}

// MessageSender defines what the service needs from the agent layer
type MessageSender interface {
	SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}

// Service is the central conversation layer that ensures all messages
// are persisted before being sent to agents and as responses stream back.
type Service struct {
	store  ConversationStore
	sender MessageSender
	logger *slog.Logger
}

// New creates a new ConversationService
func New(store ConversationStore, sender MessageSender, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		sender: sender,
		logger: logger.With("component", "conversation"),
	}
}

// SendRequest contains everything needed to send a message through the conversation layer
type SendRequest struct {
	// Thread identification (provide ThreadID directly, or FrontendName+ExternalID for lookup)
	ThreadID     string
	FrontendName string
	ExternalID   string

	// Agent routing (required)
	AgentID string

	// Message content
	Sender      string
	Content     string
	Attachments []agent.Attachment
}

// SendResponse contains the result of sending a message
type SendResponse struct {
	ThreadID  string                 // The thread this message belongs to
	MessageID string                 // ID of the saved user message
	Stream    <-chan *agent.Response // Responses flow through here (and get persisted)
}

// SendMessage records the user message, sends to the agent, and returns a channel
// that streams responses while also persisting them.
//
// Key principle: Record first, then act. The user message is saved to the store
// BEFORE being sent to the agent. This ensures we have a record even if the agent fails.
func (s *Service) SendMessage(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// 1. Resolve or create thread
	thread, err := s.ensureThread(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("thread resolution failed: %w", err)
	}

	// 2. Record user message FIRST (source of truth in ledger_events)
	now := time.Now()
	messageID := uuid.New().String()
	userEvent := &store.LedgerEvent{
		ID:              messageID,
		ConversationKey: req.AgentID,
		ThreadID:        &thread.ID,
		Direction:       store.EventDirectionInbound,
		Author:          req.Sender,
		Timestamp:       now,
		Type:            store.EventTypeMessage,
		Text:            &req.Content,
	}
	if err := s.store.SaveEvent(ctx, userEvent); err != nil {
		return nil, fmt.Errorf("failed to record message: %w", err)
	}

	s.logger.Debug("user message recorded",
		"thread_id", thread.ID,
		"message_id", messageID,
		"sender", req.Sender)

	// 3. Send to agent
	agentReq := &agent.SendRequest{
		ThreadID:    thread.ID,
		Sender:      req.Sender,
		Content:     req.Content,
		Attachments: req.Attachments,
		AgentID:     req.AgentID,
	}
	respChan, err := s.sender.SendMessage(ctx, agentReq)
	if err != nil {
		// Message is recorded, but agent failed
		// Future: could mark message as "pending" or "failed"
		return nil, fmt.Errorf("agent send failed: %w", err)
	}

	// 4. Wrap channel to persist responses as they stream
	persistedChan := s.persistResponses(ctx, thread.ID, req.AgentID, respChan)

	return &SendResponse{
		ThreadID:  thread.ID,
		MessageID: messageID,
		Stream:    persistedChan,
	}, nil
}

// GetHistory returns messages for a thread (converted from events)
func (s *Service) GetHistory(ctx context.Context, threadID string, limit int) ([]*store.Message, error) {
	events, err := s.store.GetEventsByThreadID(ctx, threadID, limit)
	if err != nil {
		return nil, err
	}
	return store.EventsToMessages(events), nil
}

// GetThread returns thread metadata
func (s *Service) GetThread(ctx context.Context, threadID string) (*store.Thread, error) {
	return s.store.GetThread(ctx, threadID)
}

// ensureThread resolves an existing thread or creates a new one
func (s *Service) ensureThread(ctx context.Context, req *SendRequest) (*store.Thread, error) {
	// If direct thread ID provided, look it up or create it
	if req.ThreadID != "" {
		thread, err := s.store.GetThread(ctx, req.ThreadID)
		if err == nil {
			return thread, nil
		}
		if err != store.ErrNotFound {
			return nil, err
		}
		// Thread ID provided but doesn't exist - create it
		thread = &store.Thread{
			ID:           req.ThreadID,
			FrontendName: req.FrontendName,
			ExternalID:   req.ExternalID,
			AgentID:      req.AgentID,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := s.store.CreateThread(ctx, thread); err != nil {
			// Handle race condition: another request may have created the thread
			// between our lookup and insert attempt
			if err == store.ErrDuplicateThread {
				// First try by the provided ThreadID
				thread, lookupErr := s.store.GetThread(ctx, req.ThreadID)
				if lookupErr == nil {
					s.logger.Debug("found existing thread after race", "thread_id", thread.ID)
					return thread, nil
				}
				// Also try by frontend+external ID since that's what the UNIQUE constraint is on
				// The existing thread may have a different ID but same frontend/external pair
				if req.FrontendName != "" && req.ExternalID != "" {
					thread, lookupErr = s.store.GetThreadByFrontendID(ctx, req.FrontendName, req.ExternalID)
					if lookupErr == nil {
						s.logger.Debug("found existing thread by frontend ID after race", "thread_id", thread.ID)
						return thread, nil
					}
				}
				s.logger.Error("retry lookup failed after duplicate error", "lookup_error", lookupErr)
			}
			return nil, err
		}
		s.logger.Debug("thread created", "thread_id", thread.ID)
		return thread, nil
	}

	// Lookup by frontend + external ID
	if req.FrontendName != "" && req.ExternalID != "" {
		s.logger.Debug("looking up thread by frontend ID",
			"frontend", req.FrontendName,
			"external_id", req.ExternalID)
		thread, err := s.store.GetThreadByFrontendID(ctx, req.FrontendName, req.ExternalID)
		if err == nil {
			s.logger.Debug("found existing thread", "thread_id", thread.ID)
			return thread, nil
		}
		if err != store.ErrNotFound {
			return nil, err
		}
		s.logger.Debug("thread not found, will create new one")
	}

	// Create new thread with generated ID
	thread := &store.Thread{
		ID:           uuid.New().String(),
		FrontendName: req.FrontendName,
		ExternalID:   req.ExternalID,
		AgentID:      req.AgentID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.store.CreateThread(ctx, thread); err != nil {
		// Handle race condition: another request may have created the thread
		// between our lookup and insert attempt
		if err == store.ErrDuplicateThread && req.FrontendName != "" && req.ExternalID != "" {
			s.logger.Debug("thread creation hit duplicate, retrying lookup",
				"frontend", req.FrontendName,
				"external_id", req.ExternalID)
			existingThread, lookupErr := s.store.GetThreadByFrontendID(ctx, req.FrontendName, req.ExternalID)
			if lookupErr == nil {
				s.logger.Debug("found existing thread after duplicate error", "thread_id", existingThread.ID)
				return existingThread, nil
			}
			s.logger.Error("retry lookup failed after duplicate error",
				"lookup_error", lookupErr)
		}
		return nil, err
	}
	s.logger.Debug("thread created", "thread_id", thread.ID)
	return thread, nil
}

// persistResponses wraps the agent response channel to save messages as they stream.
// Events are keyed by agentID for cross-client history sync (TUI, web, mobile all query by agent).
func (s *Service) persistResponses(ctx context.Context, threadID, agentID string, in <-chan *agent.Response) <-chan *agent.Response {
	out := make(chan *agent.Response, 16) // Same buffer size as Manager

	go func() {
		defer close(out)

		var textBuffer string          // Accumulate streaming text chunks
		var receivedStreamingText bool // Track if we got streaming text events
		sender := "agent:" + agentID

		// Generate a request ID for usage tracking (correlates usage events to this request)
		requestID := uuid.New().String()
		var savedUsage bool // Track if we've saved usage for this request

		for resp := range in {
			// Persist based on event type
			switch resp.Event {
			case agent.EventText:
				// Accumulate text for final save (don't save each chunk)
				textBuffer += resp.Text
				receivedStreamingText = true

			case agent.EventToolUse:
				if resp.ToolUse != nil {
					now := time.Now()
					msgID := uuid.New().String()
					// Store tool metadata as JSON in text field
					toolText := fmt.Sprintf(`{"name":%q,"id":%q,"input":%s}`, resp.ToolUse.Name, resp.ToolUse.ID, resp.ToolUse.InputJSON)
					s.saveEvent(&store.LedgerEvent{
						ID:              msgID,
						ConversationKey: agentID,
						ThreadID:        &threadID,
						Direction:       store.EventDirectionOutbound,
						Author:          sender,
						Timestamp:       now,
						Type:            store.EventTypeToolCall,
						Text:            &toolText,
					})
				}

			case agent.EventToolResult:
				if resp.ToolResult != nil {
					now := time.Now()
					msgID := uuid.New().String()
					// Store tool result with id and output as JSON
					isErrorStr := "false"
					if resp.ToolResult.IsError {
						isErrorStr = "true"
					}
					toolResultText := fmt.Sprintf(`{"id":%q,"output":%q,"is_error":%s}`, resp.ToolResult.ID, resp.ToolResult.Output, isErrorStr)
					s.saveEvent(&store.LedgerEvent{
						ID:              msgID,
						ConversationKey: agentID,
						ThreadID:        &threadID,
						Direction:       store.EventDirectionOutbound,
						Author:          sender,
						Timestamp:       now,
						Type:            store.EventTypeToolResult,
						Text:            &toolResultText,
					})
				}

			case agent.EventUsage:
				// Save token usage to store
				if resp.Usage != nil && !savedUsage {
					s.saveUsage(&store.TokenUsage{
						ID:               uuid.New().String(),
						ThreadID:         threadID,
						RequestID:        requestID,
						AgentID:          agentID,
						InputTokens:      resp.Usage.InputTokens,
						OutputTokens:     resp.Usage.OutputTokens,
						CacheReadTokens:  resp.Usage.CacheReadTokens,
						CacheWriteTokens: resp.Usage.CacheWriteTokens,
						ThinkingTokens:   resp.Usage.ThinkingTokens,
						CreatedAt:        time.Now(),
					})
					savedUsage = true
				}

			case agent.EventDone:
				// Save agent message: use accumulated text if we got streaming events,
				// otherwise use the Done event's text (for non-streaming agents)
				content := textBuffer
				if !receivedStreamingText && resp.Text != "" {
					content = resp.Text
				}
				if content != "" {
					now := time.Now()
					messageID := uuid.New().String()
					s.saveEvent(&store.LedgerEvent{
						ID:              messageID,
						ConversationKey: agentID,
						ThreadID:        &threadID,
						Direction:       store.EventDirectionOutbound,
						Author:          sender,
						Timestamp:       now,
						Type:            store.EventTypeMessage,
						Text:            &content,
					})
					// Link usage record to this message
					if savedUsage {
						s.linkUsageToMessage(requestID, messageID)
					}
				}
			}

			// Forward to output channel
			// Use select with timeout to prevent blocking indefinitely
			select {
			case out <- resp:
				// Sent successfully
			case <-time.After(5 * time.Second):
				s.logger.Warn("response channel full, dropping message",
					"thread_id", threadID,
					"event", resp.Event)
			case <-ctx.Done():
				s.logger.Debug("context cancelled during response streaming",
					"thread_id", threadID)
				// Drain remaining messages to prevent blocking the sender
				go func() {
					for range in {
					}
				}()
				return
			}
		}
	}()

	return out
}

// saveUsage saves a token usage record with a separate timeout context
func (s *Service) saveUsage(usage *store.TokenUsage) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.store.SaveUsage(saveCtx, usage); err != nil {
		s.logger.Error("failed to save usage",
			"error", err,
			"thread_id", usage.ThreadID,
			"request_id", usage.RequestID,
			"usage_id", usage.ID)
	} else {
		s.logger.Debug("usage saved",
			"thread_id", usage.ThreadID,
			"request_id", usage.RequestID,
			"input_tokens", usage.InputTokens,
			"output_tokens", usage.OutputTokens)
	}
}

// linkUsageToMessage links a usage record to its final message
func (s *Service) linkUsageToMessage(requestID, messageID string) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.store.LinkUsageToMessage(saveCtx, requestID, messageID); err != nil {
		s.logger.Error("failed to link usage to message",
			"error", err,
			"request_id", requestID,
			"message_id", messageID)
	}
}

// saveEvent saves a ledger event with a separate timeout context.
// This ensures persistence continues even if the request context is cancelled.
func (s *Service) saveEvent(event *store.LedgerEvent) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.store.SaveEvent(saveCtx, event); err != nil {
		s.logger.Error("failed to save event",
			"error", err,
			"event_id", event.ID,
			"thread_id", event.ThreadID,
			"type", event.Type)
	} else {
		s.logger.Debug("event saved",
			"event_id", event.ID,
			"thread_id", event.ThreadID,
			"type", event.Type)
	}
}
