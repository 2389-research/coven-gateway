// ABOUTME: Chat session management for admin UI agent conversations
// ABOUTME: Bridges POST /send and GET /stream endpoints via shared channels

package webadmin

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/store"
)

// chatMessage represents a message in the chat stream
type chatMessage struct {
	Type      string    `json:"type"` // "user", "text", "thinking", "tool_use", "tool_result", "usage", "tool_state", "tool_approval", "user_question", "cancelled", "error", "done"
	Content   string    `json:"content,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
	ToolID    string    `json:"tool_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`

	// Usage fields (for type="usage")
	InputTokens      int32 `json:"input_tokens,omitempty"`
	OutputTokens     int32 `json:"output_tokens,omitempty"`
	CacheReadTokens  int32 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int32 `json:"cache_write_tokens,omitempty"`
	ThinkingTokens   int32 `json:"thinking_tokens,omitempty"`

	// ToolState fields (for type="tool_state")
	State  string `json:"state,omitempty"`
	Detail string `json:"detail,omitempty"`

	// Cancelled fields (for type="cancelled")
	Reason string `json:"reason,omitempty"`

	// ToolApproval fields (for type="tool_approval")
	InputJSON string `json:"input_json,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	// UserQuestion fields (for type="user_question")
	QuestionID     string           `json:"question_id,omitempty"`
	Question       string           `json:"question,omitempty"`
	Options        []questionOption `json:"options,omitempty"`
	MultiSelect    bool             `json:"multi_select,omitempty"`
	Header         string           `json:"header,omitempty"`
	TimeoutSeconds int32            `json:"timeout_seconds,omitempty"`
}

// questionOption represents a choice in a user question
type questionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// chatSession represents an active chat between a user and an agent
type chatSession struct {
	mu             sync.RWMutex
	agentID        string
	userID         string
	messages       chan *chatMessage
	closed         bool
	cancel         context.CancelFunc
	ctx            context.Context
	createdAt      time.Time
	lastUsed       time.Time
	broadcastSubID string // subscription ID for the event broadcaster (set by handleChatStream)
	activeRequest  bool   // true while pipeAgentResponses is running (suppresses broadcast dedup)
}

// send safely sends a message to the session channel
// Returns false if the session is closed or channel is full
func (s *chatSession) send(msg *chatMessage) bool {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return false
	}
	// Hold the read lock while sending to prevent close during send
	select {
	case s.messages <- msg:
		s.mu.RUnlock()
		return true
	default:
		// Channel full
		s.mu.RUnlock()
		return false
	}
}

// close safely closes the session
func (s *chatSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.cancel()
	close(s.messages)
}

// isClosed checks if the session is closed
func (s *chatSession) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// chatHub manages active chat sessions between users and agents
type chatHub struct {
	mu       sync.RWMutex
	sessions map[string]*chatSession // keyed by "agentID|userID"
	cancel   context.CancelFunc
}

func newChatHub() *chatHub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &chatHub{
		sessions: make(map[string]*chatSession),
		cancel:   cancel,
	}
	// Start cleanup goroutine
	go hub.cleanupLoop(ctx)
	return hub
}

// sessionKey generates a key for the session map
// Uses | as delimiter since it's not valid in UUIDs
func sessionKey(agentID, userID string) string {
	return agentID + "|" + userID
}

// cleanupLoop periodically removes stale sessions
func (h *chatHub) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupStaleSessions()
		}
	}
}

// cleanupStaleSessions removes sessions idle for more than 30 minutes
func (h *chatHub) cleanupStaleSessions() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	staleThreshold := 30 * time.Minute

	for key, session := range h.sessions {
		session.mu.RLock()
		idle := now.Sub(session.lastUsed)
		session.mu.RUnlock()

		if idle > staleThreshold {
			session.close()
			delete(h.sessions, key)
		}
	}
}

// getOrCreateSession gets an existing session or creates a new one
func (h *chatHub) getOrCreateSession(agentID, userID string) *chatSession {
	key := sessionKey(agentID, userID)

	h.mu.Lock()
	defer h.mu.Unlock()

	if session, ok := h.sessions[key]; ok {
		session.mu.Lock()
		session.lastUsed = time.Now()
		session.mu.Unlock()
		return session
	}

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()

	session := &chatSession{
		agentID:   agentID,
		userID:    userID,
		messages:  make(chan *chatMessage, 64),
		closed:    false,
		cancel:    cancel,
		ctx:       ctx,
		createdAt: now,
		lastUsed:  now,
	}
	h.sessions[key] = session
	return session
}

// getSession gets an existing session if it exists
func (h *chatHub) getSession(agentID, userID string) (*chatSession, bool) {
	key := sessionKey(agentID, userID)

	h.mu.RLock()
	defer h.mu.RUnlock()

	session, ok := h.sessions[key]
	if ok {
		session.mu.Lock()
		session.lastUsed = time.Now()
		session.mu.Unlock()
	}
	return session, ok
}

// removeSession removes and closes a session
func (h *chatHub) removeSession(agentID, userID string) {
	key := sessionKey(agentID, userID)

	h.mu.Lock()
	defer h.mu.Unlock()

	if session, ok := h.sessions[key]; ok {
		session.close()
		delete(h.sessions, key)
	}
}

// pipeAgentResponses reads from an agent response channel and sends to the chat session
// This function is designed to exit gracefully in all cases
func (h *chatHub) pipeAgentResponses(ctx context.Context, agentID, userID string, respChan <-chan *agent.Response) {
	session, ok := h.getSession(agentID, userID)
	if !ok {
		// Session doesn't exist, drain the response channel to prevent agent blocking
		for range respChan {
		}
		return
	}

	for {
		select {
		case <-ctx.Done():
			// Request context cancelled - send error and exit
			session.send(&chatMessage{
				Type:      "error",
				Content:   "Request cancelled",
				Timestamp: time.Now(),
			})
			// Drain remaining messages to unblock agent
			go drainChannel(respChan)
			return

		case <-session.ctx.Done():
			// Session closed - drain and exit
			go drainChannel(respChan)
			return

		case resp, ok := <-respChan:
			if !ok {
				// Agent response channel closed - normal completion
				return
			}

			msg := convertAgentResponse(resp)

			// Try to send with timeout to prevent blocking forever
			sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			sent := sendWithContext(sendCtx, session, msg)
			cancel()

			if !sent {
				// Could not send (channel full or closed) - log but continue
				// to drain agent responses and prevent blocking
				if session.isClosed() {
					go drainChannel(respChan)
					return
				}
				// Channel full - message dropped, continue processing
			}

			if resp.Done {
				return
			}
		}
	}
}

// sendWithContext attempts to send a message with context-aware cancellation
func sendWithContext(ctx context.Context, session *chatSession, msg *chatMessage) bool {
	// First try non-blocking send
	if session.send(msg) {
		return true
	}

	// Channel was full, wait with timeout
	select {
	case <-ctx.Done():
		return false
	case <-session.ctx.Done():
		return false
	case <-time.After(100 * time.Millisecond):
		// Retry once after brief wait
		return session.send(msg)
	}
}

// drainChannel consumes all remaining messages from a channel
func drainChannel(ch <-chan *agent.Response) {
	for range ch {
	}
}

// convertAgentResponse converts an agent.Response to a chatMessage
func convertAgentResponse(resp *agent.Response) *chatMessage {
	msg := &chatMessage{
		Timestamp: time.Now(),
	}

	switch resp.Event {
	case agent.EventThinking:
		msg.Type = "thinking"
		msg.Content = resp.Text

	case agent.EventText:
		msg.Type = "text"
		msg.Content = resp.Text

	case agent.EventToolUse:
		msg.Type = "tool_use"
		if resp.ToolUse != nil {
			msg.ToolName = resp.ToolUse.Name
			msg.ToolID = resp.ToolUse.ID
			msg.Content = resp.ToolUse.InputJSON
		}

	case agent.EventToolResult:
		msg.Type = "tool_result"
		if resp.ToolResult != nil {
			msg.ToolID = resp.ToolResult.ID
			msg.Content = resp.ToolResult.Output
		}

	case agent.EventDone:
		msg.Type = "done"
		msg.Content = resp.Text // Full response if available

	case agent.EventError:
		msg.Type = "error"
		msg.Content = resp.Error

	case agent.EventUsage:
		msg.Type = "usage"
		if resp.Usage != nil {
			msg.InputTokens = resp.Usage.InputTokens
			msg.OutputTokens = resp.Usage.OutputTokens
			msg.CacheReadTokens = resp.Usage.CacheReadTokens
			msg.CacheWriteTokens = resp.Usage.CacheWriteTokens
			msg.ThinkingTokens = resp.Usage.ThinkingTokens
		}

	case agent.EventToolState:
		msg.Type = "tool_state"
		if resp.ToolState != nil {
			msg.ToolID = resp.ToolState.ID
			msg.State = resp.ToolState.State
			msg.Detail = resp.ToolState.Detail
		}

	case agent.EventCancelled:
		msg.Type = "cancelled"
		msg.Reason = resp.Text

	case agent.EventToolApprovalRequest:
		msg.Type = "tool_approval"
		if resp.ToolApprovalRequest != nil {
			msg.ToolID = resp.ToolApprovalRequest.ID
			msg.ToolName = resp.ToolApprovalRequest.Name
			msg.InputJSON = resp.ToolApprovalRequest.InputJSON
			msg.RequestID = resp.ToolApprovalRequest.RequestID
		}

	default:
		msg.Type = "text"
		msg.Content = resp.Text
	}

	return msg
}

// sendToAgent sends a message to all sessions for a given agent
func (h *chatHub) sendToAgent(agentID string, msg *chatMessage) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sent := 0
	for key, session := range h.sessions {
		// Key format is "agentID|userID"
		if len(key) > len(agentID) && key[:len(agentID)+1] == agentID+"|" {
			if session.send(msg) {
				sent++
			}
		}
	}
	return sent
}

// ledgerEventToChatMessage converts a persisted LedgerEvent into a chatMessage
// for the web SSE stream. This enables cross-client awareness: when client B
// sends a message, client A sees it via the broadcaster.
func ledgerEventToChatMessage(event *store.LedgerEvent) *chatMessage {
	msg := &chatMessage{
		Timestamp: event.Timestamp,
	}

	switch event.Type {
	case store.EventTypeMessage:
		if event.Direction == store.EventDirectionInbound {
			msg.Type = "user"
			if event.Text != nil {
				msg.Content = *event.Text
			}
		} else {
			msg.Type = "text"
			if event.Text != nil {
				msg.Content = *event.Text
			}
		}
	case store.EventTypeToolCall:
		msg.Type = "tool_use"
		if event.Text != nil {
			// Parse tool metadata from JSON
			var toolData struct {
				Name  string `json:"name"`
				ID    string `json:"id"`
				Input string `json:"input"`
			}
			if err := json.Unmarshal([]byte(*event.Text), &toolData); err == nil {
				msg.ToolName = toolData.Name
				msg.ToolID = toolData.ID
				msg.Content = toolData.Input
			} else {
				msg.Content = *event.Text
			}
		}
	case store.EventTypeToolResult:
		msg.Type = "tool_result"
		if event.Text != nil {
			var resultData struct {
				ID     string `json:"id"`
				Output string `json:"output"`
			}
			if err := json.Unmarshal([]byte(*event.Text), &resultData); err == nil {
				msg.ToolID = resultData.ID
				msg.Content = resultData.Output
			} else {
				msg.Content = *event.Text
			}
		}
	case store.EventTypeError:
		msg.Type = "error"
		if event.Text != nil {
			msg.Content = *event.Text
		}
	default:
		msg.Type = "text"
		if event.Text != nil {
			msg.Content = *event.Text
		}
	}

	return msg
}

// Close closes all sessions and stops the cleanup goroutine
func (h *chatHub) Close() {
	h.cancel()

	h.mu.Lock()
	defer h.mu.Unlock()

	for key, session := range h.sessions {
		session.close()
		delete(h.sessions, key)
	}
}
