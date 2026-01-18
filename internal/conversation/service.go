// ABOUTME: ConversationService is the central layer for message persistence
// ABOUTME: All messages flow through here - history is the source of truth, not a side effect

package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/store"
)

// ConversationStore defines what the service needs from storage
type ConversationStore interface {
	CreateThread(ctx context.Context, thread *store.Thread) error
	GetThread(ctx context.Context, id string) (*store.Thread, error)
	GetThreadByFrontendID(ctx context.Context, frontendName, externalID string) (*store.Thread, error)
	SaveMessage(ctx context.Context, msg *store.Message) error
	GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*store.Message, error)
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

	// 2. Record user message FIRST (source of truth)
	userMsg := &store.Message{
		ID:        uuid.New().String(),
		ThreadID:  thread.ID,
		Sender:    req.Sender,
		Content:   req.Content,
		Type:      store.MessageTypeMessage,
		CreatedAt: time.Now(),
	}
	if err := s.store.SaveMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("failed to record message: %w", err)
	}

	s.logger.Debug("user message recorded",
		"thread_id", thread.ID,
		"message_id", userMsg.ID,
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
		MessageID: userMsg.ID,
		Stream:    persistedChan,
	}, nil
}

// GetHistory returns messages for a thread
func (s *Service) GetHistory(ctx context.Context, threadID string, limit int) ([]*store.Message, error) {
	return s.store.GetThreadMessages(ctx, threadID, limit)
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
			return nil, err
		}
		s.logger.Debug("thread created", "thread_id", thread.ID)
		return thread, nil
	}

	// Lookup by frontend + external ID
	if req.FrontendName != "" && req.ExternalID != "" {
		thread, err := s.store.GetThreadByFrontendID(ctx, req.FrontendName, req.ExternalID)
		if err == nil {
			return thread, nil
		}
		if err != store.ErrNotFound {
			return nil, err
		}
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
		return nil, err
	}
	s.logger.Debug("thread created", "thread_id", thread.ID)
	return thread, nil
}

// persistResponses wraps the agent response channel to save messages as they stream
func (s *Service) persistResponses(ctx context.Context, threadID, agentID string, in <-chan *agent.Response) <-chan *agent.Response {
	out := make(chan *agent.Response, 16) // Same buffer size as Manager

	go func() {
		defer close(out)

		var textBuffer string          // Accumulate streaming text chunks
		var receivedStreamingText bool // Track if we got streaming text events
		sender := "agent:" + agentID

		for resp := range in {
			// Persist based on event type
			switch resp.Event {
			case agent.EventText:
				// Accumulate text for final save (don't save each chunk)
				textBuffer += resp.Text
				receivedStreamingText = true

			case agent.EventToolUse:
				if resp.ToolUse != nil {
					s.saveMessage(&store.Message{
						ID:        uuid.New().String(),
						ThreadID:  threadID,
						Sender:    sender,
						Content:   resp.ToolUse.InputJSON,
						Type:      store.MessageTypeToolUse,
						ToolName:  resp.ToolUse.Name,
						ToolID:    resp.ToolUse.ID,
						CreatedAt: time.Now(),
					})
				}

			case agent.EventToolResult:
				if resp.ToolResult != nil {
					s.saveMessage(&store.Message{
						ID:        uuid.New().String(),
						ThreadID:  threadID,
						Sender:    sender,
						Content:   resp.ToolResult.Output,
						Type:      store.MessageTypeToolResult,
						ToolID:    resp.ToolResult.ID,
						CreatedAt: time.Now(),
					})
				}

			case agent.EventDone:
				// Save agent message: use accumulated text if we got streaming events,
				// otherwise use the Done event's text (for non-streaming agents)
				content := textBuffer
				if !receivedStreamingText && resp.Text != "" {
					content = resp.Text
				}
				if content != "" {
					s.saveMessage(&store.Message{
						ID:        uuid.New().String(),
						ThreadID:  threadID,
						Sender:    sender,
						Content:   content,
						Type:      store.MessageTypeMessage,
						CreatedAt: time.Now(),
					})
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

// saveMessage saves a message with a separate timeout context
// This ensures persistence continues even if the request context is cancelled
func (s *Service) saveMessage(msg *store.Message) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.store.SaveMessage(saveCtx, msg); err != nil {
		s.logger.Error("failed to save message",
			"error", err,
			"thread_id", msg.ThreadID,
			"type", msg.Type,
			"message_id", msg.ID)
	} else {
		s.logger.Debug("message saved",
			"thread_id", msg.ThreadID,
			"type", msg.Type,
			"message_id", msg.ID)
	}
}
