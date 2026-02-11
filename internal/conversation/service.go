// ABOUTME: ConversationService is the central layer for message persistence
// ABOUTME: All messages flow through here - history is the source of truth, not a side effect

package conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/store"
)

// ConversationStore defines what the service needs from storage.
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

// MessageSender defines what the service needs from the agent layer.
type MessageSender interface {
	SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}

// Service is the central conversation layer that ensures all messages
// are persisted before being sent to agents and as responses stream back.
type Service struct {
	store       ConversationStore
	sender      MessageSender
	broadcaster *EventBroadcaster
	logger      *slog.Logger
}

// New creates a new ConversationService.
// The broadcaster parameter is optional — pass nil to disable cross-client
// event broadcasting (events will still be persisted normally).
func New(store ConversationStore, sender MessageSender, logger *slog.Logger, broadcaster *EventBroadcaster) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:       store,
		sender:      sender,
		broadcaster: broadcaster,
		logger:      logger.With("component", "conversation"),
	}
}

// SendRequest contains everything needed to send a message through the conversation layer.
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

// SendResponse contains the result of sending a message.
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
		return nil, errors.New("agent_id is required")
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

	// Broadcast user message to other clients watching this conversation
	if s.broadcaster != nil {
		s.broadcaster.Publish(req.AgentID, userEvent, "")
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

// Subscribe registers a subscriber for broadcast events on a conversation key.
// Returns nil channel if the broadcaster is not configured.
func (s *Service) Subscribe(ctx context.Context, conversationKey string) (<-chan *store.LedgerEvent, string) {
	if s.broadcaster == nil {
		return nil, ""
	}
	return s.broadcaster.Subscribe(ctx, conversationKey)
}

// GetHistory returns messages for a thread (converted from events).
func (s *Service) GetHistory(ctx context.Context, threadID string, limit int) ([]*store.Message, error) {
	events, err := s.store.GetEventsByThreadID(ctx, threadID, limit)
	if err != nil {
		return nil, err
	}
	return store.EventsToMessages(events), nil
}

// GetThread returns thread metadata.
func (s *Service) GetThread(ctx context.Context, threadID string) (*store.Thread, error) {
	return s.store.GetThread(ctx, threadID)
}

// newThreadRecord creates a Thread struct from a request with optional custom ID.
func newThreadRecord(req *SendRequest, threadID string) *store.Thread {
	if threadID == "" {
		threadID = uuid.New().String()
	}
	now := time.Now()
	return &store.Thread{
		ID:           threadID,
		FrontendName: req.FrontendName,
		ExternalID:   req.ExternalID,
		AgentID:      req.AgentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// tryRecoverDuplicateThread handles the race condition when thread already exists.
func (s *Service) tryRecoverDuplicateThread(ctx context.Context, req *SendRequest, threadID string) (*store.Thread, error) {
	// First try by the provided ThreadID
	if threadID != "" {
		thread, err := s.store.GetThread(ctx, threadID)
		if err == nil {
			s.logger.Debug("found existing thread after race", "thread_id", thread.ID)
			return thread, nil
		}
	}

	// Try by frontend+external ID since that's what the UNIQUE constraint is on
	if req.FrontendName == "" || req.ExternalID == "" {
		return nil, store.ErrDuplicateThread
	}

	thread, err := s.store.GetThreadByFrontendID(ctx, req.FrontendName, req.ExternalID)
	if err == nil {
		s.logger.Debug("found existing thread by frontend ID after race", "thread_id", thread.ID)
		return thread, nil
	}

	s.logger.Error("retry lookup failed after duplicate error", "lookup_error", err)
	return nil, store.ErrDuplicateThread
}

// ensureThreadByID finds or creates a thread with the given ID.
func (s *Service) ensureThreadByID(ctx context.Context, req *SendRequest) (*store.Thread, error) {
	thread, err := s.store.GetThread(ctx, req.ThreadID)
	if err == nil {
		return thread, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	thread = newThreadRecord(req, req.ThreadID)
	if err := s.store.CreateThread(ctx, thread); err != nil {
		if errors.Is(err, store.ErrDuplicateThread) {
			return s.tryRecoverDuplicateThread(ctx, req, req.ThreadID)
		}
		return nil, err
	}
	s.logger.Debug("thread created", "thread_id", thread.ID)
	return thread, nil
}

// ensureThreadByFrontendID finds or creates a thread by frontend/external ID.
func (s *Service) ensureThreadByFrontendID(ctx context.Context, req *SendRequest) (*store.Thread, error) {
	if req.FrontendName != "" && req.ExternalID != "" {
		s.logger.Debug("looking up thread by frontend ID", "frontend", req.FrontendName, "external_id", req.ExternalID)
		thread, err := s.store.GetThreadByFrontendID(ctx, req.FrontendName, req.ExternalID)
		if err == nil {
			s.logger.Debug("found existing thread", "thread_id", thread.ID)
			return thread, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		s.logger.Debug("thread not found, will create new one")
	}

	thread := newThreadRecord(req, "")
	if err := s.store.CreateThread(ctx, thread); err != nil {
		if errors.Is(err, store.ErrDuplicateThread) {
			return s.tryRecoverDuplicateThread(ctx, req, "")
		}
		return nil, err
	}
	s.logger.Debug("thread created", "thread_id", thread.ID)
	return thread, nil
}

// ensureThread resolves an existing thread or creates a new one.
func (s *Service) ensureThread(ctx context.Context, req *SendRequest) (*store.Thread, error) {
	if req.ThreadID != "" {
		return s.ensureThreadByID(ctx, req)
	}
	return s.ensureThreadByFrontendID(ctx, req)
}

// responsePersister holds state for persisting agent responses.
type responsePersister struct {
	service            *Service
	ctx                context.Context
	threadID           string
	agentID            string
	sender             string
	requestID          string
	textBuffer         string
	receivedStreamText bool
	savedUsage         bool
}

// handleToolUse persists a tool use event.
func (p *responsePersister) handleToolUse(tu *agent.ToolUseEvent) {
	if tu == nil {
		return
	}
	toolText := fmt.Sprintf(`{"name":%q,"id":%q,"input":%s}`, tu.Name, tu.ID, tu.InputJSON)
	p.service.saveEvent(p.ctx, &store.LedgerEvent{
		ID:              uuid.New().String(),
		ConversationKey: p.agentID,
		ThreadID:        &p.threadID,
		Direction:       store.EventDirectionOutbound,
		Author:          p.sender,
		Timestamp:       time.Now(),
		Type:            store.EventTypeToolCall,
		Text:            &toolText,
	})
}

// handleToolResult persists a tool result event.
func (p *responsePersister) handleToolResult(tr *agent.ToolResultEvent) {
	if tr == nil {
		return
	}
	isErrorStr := "false"
	if tr.IsError {
		isErrorStr = "true"
	}
	toolResultText := fmt.Sprintf(`{"id":%q,"output":%q,"is_error":%s}`, tr.ID, tr.Output, isErrorStr)
	p.service.saveEvent(p.ctx, &store.LedgerEvent{
		ID:              uuid.New().String(),
		ConversationKey: p.agentID,
		ThreadID:        &p.threadID,
		Direction:       store.EventDirectionOutbound,
		Author:          p.sender,
		Timestamp:       time.Now(),
		Type:            store.EventTypeToolResult,
		Text:            &toolResultText,
	})
}

// handleUsage persists a usage event.
func (p *responsePersister) handleUsage(usage *agent.UsageEvent) {
	if usage == nil || p.savedUsage {
		return
	}
	p.service.saveUsage(p.ctx, &store.TokenUsage{
		ID:               uuid.New().String(),
		ThreadID:         p.threadID,
		RequestID:        p.requestID,
		AgentID:          p.agentID,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		ThinkingTokens:   usage.ThinkingTokens,
		CreatedAt:        time.Now(),
	})
	p.savedUsage = true
}

// handleDone persists the final message and links usage.
func (p *responsePersister) handleDone(resp *agent.Response) {
	content := p.textBuffer
	if !p.receivedStreamText && resp.Text != "" {
		content = resp.Text
	}
	if content == "" {
		return
	}
	messageID := uuid.New().String()
	p.service.saveEvent(p.ctx, &store.LedgerEvent{
		ID:              messageID,
		ConversationKey: p.agentID,
		ThreadID:        &p.threadID,
		Direction:       store.EventDirectionOutbound,
		Author:          p.sender,
		Timestamp:       time.Now(),
		Type:            store.EventTypeMessage,
		Text:            &content,
	})
	if p.savedUsage {
		p.service.linkUsageToMessage(p.ctx, p.requestID, messageID)
	}
}

// handleResponse dispatches a response to the appropriate handler.
func (p *responsePersister) handleResponse(resp *agent.Response) {
	switch resp.Event {
	case agent.EventText:
		p.textBuffer += resp.Text
		p.receivedStreamText = true
	case agent.EventToolUse:
		p.handleToolUse(resp.ToolUse)
	case agent.EventToolResult:
		p.handleToolResult(resp.ToolResult)
	case agent.EventUsage:
		p.handleUsage(resp.Usage)
	case agent.EventDone:
		p.handleDone(resp)
	}
}

// persistResponses wraps the agent response channel to save messages as they stream.
// Events are keyed by agentID for cross-client history sync (TUI, web, mobile all query by agent).
func (s *Service) persistResponses(ctx context.Context, threadID, agentID string, in <-chan *agent.Response) <-chan *agent.Response {
	out := make(chan *agent.Response, 16)

	go func() {
		defer close(out)

		p := &responsePersister{
			service:   s,
			ctx:       ctx,
			threadID:  threadID,
			agentID:   agentID,
			sender:    "agent:" + agentID,
			requestID: uuid.New().String(),
		}

		// Use a reusable timer to avoid memory leaks from time.After in loops
		sendTimer := time.NewTimer(5 * time.Second)
		defer sendTimer.Stop()

		for resp := range in {
			p.handleResponse(resp)

			// Reset timer for each send attempt
			if !sendTimer.Stop() {
				select {
				case <-sendTimer.C:
				default:
				}
			}
			sendTimer.Reset(5 * time.Second)

			select {
			case out <- resp:
			case <-sendTimer.C:
				s.logger.Warn("response channel full, dropping message", "thread_id", threadID, "event", resp.Event)
			case <-ctx.Done():
				s.logger.Debug("context canceled during response streaming", "thread_id", threadID)
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

// saveUsage saves a token usage record with a separate timeout context.
// Uses WithoutCancel to ensure saves complete even if parent context is canceled.
func (s *Service) saveUsage(ctx context.Context, usage *store.TokenUsage) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
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

// linkUsageToMessage links a usage record to its final message.
// Uses WithoutCancel to ensure saves complete even if parent context is canceled.
func (s *Service) linkUsageToMessage(ctx context.Context, requestID, messageID string) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.store.LinkUsageToMessage(saveCtx, requestID, messageID); err != nil {
		s.logger.Error("failed to link usage to message",
			"error", err,
			"request_id", requestID,
			"message_id", messageID)
	}
}

// saveEvent saves a ledger event with a separate timeout context.
// Uses WithoutCancel to ensure persistence continues even if the request context is canceled.
func (s *Service) saveEvent(ctx context.Context, event *store.LedgerEvent) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.store.SaveEvent(saveCtx, event); err != nil {
		s.logger.Error("failed to save event",
			"error", err,
			"event_id", event.ID,
			"thread_id", event.ThreadID,
			"type", event.Type)
	} else {
		// Broadcast persisted event to subscribers
		if s.broadcaster != nil {
			s.broadcaster.Publish(event.ConversationKey, event, "")
		}

		s.logger.Debug("event saved",
			"event_id", event.ID,
			"thread_id", event.ThreadID,
			"type", event.Type)
	}
}
