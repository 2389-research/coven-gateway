// ABOUTME: HTTP API handlers for exposing agent messaging via SSE.
// ABOUTME: Provides POST /api/send endpoint for external clients like TUI.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/store"
)

// SendMessageRequest is the JSON request body for POST /api/send.
type SendMessageRequest struct {
	ThreadID  string `json:"thread_id,omitempty"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	AgentID   string `json:"agent_id,omitempty"`
	Frontend  string `json:"frontend,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
}

// AgentInfoResponse is the JSON response for GET /api/agents.
type AgentInfoResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

// CreateBindingRequest is the JSON request body for POST /api/bindings.
type CreateBindingRequest struct {
	Frontend  string `json:"frontend"`
	ChannelID string `json:"channel_id"`
	AgentID   string `json:"agent_id"`
}

// BindingResponse is the JSON response for binding operations.
type BindingResponse struct {
	Frontend    string `json:"frontend"`
	ChannelID   string `json:"channel_id"`
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name,omitempty"`
	AgentOnline bool   `json:"agent_online"`
	CreatedAt   string `json:"created_at"`
}

// ListBindingsResponse is the JSON response for GET /api/bindings.
type ListBindingsResponse struct {
	Bindings []BindingResponse `json:"bindings"`
}

// MessageResponse is the JSON response for message history.
type MessageResponse struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// ThreadMessagesResponse is the JSON response for GET /api/threads/{id}/messages.
type ThreadMessagesResponse struct {
	ThreadID string            `json:"thread_id"`
	Messages []MessageResponse `json:"messages"`
}

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// handleListAgents handles GET /api/agents requests.
// It returns a JSON array of all connected agents.
func (g *Gateway) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	sender := g.getSender()
	agents := sender.ListAgents()

	response := make([]AgentInfoResponse, len(agents))
	for i, a := range agents {
		response[i] = AgentInfoResponse{
			ID:           a.ID,
			Name:         a.Name,
			Capabilities: a.Capabilities,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSendMessage handles POST /api/send requests.
// It accepts a JSON body with the message content and streams responses via SSE.
// If agent_id is specified, the message is sent to that specific agent.
// Messages are persisted to the store for history retrieval.
func (g *Gateway) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Content == "" {
		g.sendJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	if req.Sender == "" {
		g.sendJSONError(w, http.StatusBadRequest, "sender is required")
		return
	}

	// Resolve agent ID - either from direct specification or binding lookup
	agentID := req.AgentID
	if agentID == "" {
		// Must have frontend + channel_id for binding lookup
		if req.Frontend == "" || req.ChannelID == "" {
			g.sendJSONError(w, http.StatusBadRequest, "must specify agent_id or frontend+channel_id")
			return
		}

		binding, err := g.store.GetBinding(r.Context(), req.Frontend, req.ChannelID)
		if errors.Is(err, store.ErrNotFound) {
			g.sendJSONError(w, http.StatusBadRequest, "channel not bound to agent")
			return
		}
		if err != nil {
			g.logger.Error("failed to get binding", "error", err)
			g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		agentID = binding.AgentID
	}

	// Verify agent exists and is online
	if _, ok := g.agentManager.GetAgent(agentID); !ok {
		g.sendJSONError(w, http.StatusServiceUnavailable, "agent unavailable")
		return
	}

	// Get or create thread for message persistence
	threadID := req.ThreadID
	if threadID == "" {
		// Generate a new thread ID if not provided
		threadID = uuid.New().String()
	}

	// Ensure thread exists in store
	_, err := g.store.GetThread(r.Context(), threadID)
	if errors.Is(err, store.ErrNotFound) {
		// Create new thread
		now := time.Now()
		frontendName := req.Frontend
		externalID := req.ChannelID
		// Use sensible defaults for direct agent_id usage
		if frontendName == "" {
			frontendName = "direct"
		}
		if externalID == "" {
			externalID = threadID
		}
		thread := &store.Thread{
			ID:           threadID,
			FrontendName: frontendName,
			ExternalID:   externalID,
			AgentID:      agentID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if createErr := g.store.CreateThread(r.Context(), thread); createErr != nil {
			g.logger.Error("failed to create thread", "error", createErr)
			// Continue anyway - message persistence is best-effort
		}
	} else if err != nil {
		g.logger.Error("failed to get thread", "error", err)
		// Continue anyway - message persistence is best-effort
	}

	// Save the user's message
	userMsg := &store.Message{
		ID:        uuid.New().String(),
		ThreadID:  threadID,
		Sender:    req.Sender,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}
	if err := g.store.SaveMessage(r.Context(), userMsg); err != nil {
		g.logger.Error("failed to save user message", "error", err)
		// Continue anyway - message persistence is best-effort
	}

	// Use mock sender if set (for testing), otherwise use agent manager
	sender := g.getSender()

	// Create the send request
	sendReq := &agent.SendRequest{
		ThreadID: threadID,
		Sender:   req.Sender,
		Content:  req.Content,
		AgentID:  agentID,
	}

	// Send message to the specified agent
	respChan, err := sender.SendMessage(r.Context(), sendReq)
	if err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			g.sendJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		g.logger.Error("failed to send message", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Check streaming support before setting headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		g.logger.Error("streaming not supported")
		g.sendJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Stream responses and persist agent response
	g.streamResponses(r.Context(), w, flusher, respChan, threadID, agentID)
}

// streamResponses reads from the response channel and writes SSE events.
// It also persists the agent's final response to the store.
func (g *Gateway) streamResponses(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, respChan <-chan *agent.Response, threadID, agentID string) {
	for {
		select {
		case <-ctx.Done():
			g.writeSSEEvent(w, "error", map[string]string{"error": "request cancelled"})
			flusher.Flush()
			return

		case resp, ok := <-respChan:
			if !ok {
				return
			}

			event := g.responseToSSEEvent(resp)
			g.writeSSEEvent(w, event.Event, event.Data)
			flusher.Flush()

			if resp.Done {
				// Save the agent's complete response (Done event contains full_response)
				agentMsg := &store.Message{
					ID:        uuid.New().String(),
					ThreadID:  threadID,
					Sender:    "agent:" + agentID,
					Content:   resp.Text,
					CreatedAt: time.Now(),
				}
				// Use background context since request context may be cancelled
				if err := g.store.SaveMessage(context.Background(), agentMsg); err != nil {
					g.logger.Error("failed to save agent message", "error", err)
				}
				return
			}
		}
	}
}

// responseToSSEEvent converts an agent response to an SSE event.
func (g *Gateway) responseToSSEEvent(resp *agent.Response) SSEEvent {
	switch resp.Event {
	case agent.EventThinking:
		return SSEEvent{
			Event: "thinking",
			Data:  map[string]string{"text": resp.Text},
		}
	case agent.EventText:
		return SSEEvent{
			Event: "text",
			Data:  map[string]string{"text": resp.Text},
		}
	case agent.EventToolUse:
		if resp.ToolUse == nil {
			return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed tool_use event"}}
		}
		return SSEEvent{
			Event: "tool_use",
			Data: map[string]string{
				"id":         resp.ToolUse.ID,
				"name":       resp.ToolUse.Name,
				"input_json": resp.ToolUse.InputJSON,
			},
		}
	case agent.EventToolResult:
		if resp.ToolResult == nil {
			return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed tool_result event"}}
		}
		return SSEEvent{
			Event: "tool_result",
			Data: map[string]interface{}{
				"id":       resp.ToolResult.ID,
				"output":   resp.ToolResult.Output,
				"is_error": resp.ToolResult.IsError,
			},
		}
	case agent.EventFile:
		if resp.File == nil {
			return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed file event"}}
		}
		return SSEEvent{
			Event: "file",
			Data: map[string]string{
				"filename":  resp.File.Filename,
				"mime_type": resp.File.MimeType,
			},
		}
	case agent.EventDone:
		return SSEEvent{
			Event: "done",
			Data:  map[string]string{"full_response": resp.Text},
		}
	case agent.EventError:
		return SSEEvent{
			Event: "error",
			Data:  map[string]string{"error": resp.Error},
		}
	default:
		return SSEEvent{
			Event: "unknown",
			Data:  map[string]string{"text": resp.Text},
		}
	}
}

// writeSSEEvent writes a single SSE event to the response writer.
func (g *Gateway) writeSSEEvent(w http.ResponseWriter, event string, data interface{}) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		g.logger.Error("failed to marshal SSE data", "error", err)
		return
	}

	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", dataJSON)
}

// sendJSONError writes a JSON error response.
func (g *Gateway) sendJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// messageSender is an interface for sending messages to agents.
// This allows injecting mock implementations for testing.
type messageSender interface {
	SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
	ListAgents() []*agent.AgentInfo
}

// getSender returns the message sender (mock or real agent manager).
func (g *Gateway) getSender() messageSender {
	if g.mockSender != nil {
		return g.mockSender
	}
	return g.agentManager
}

// handleBindings routes binding requests by HTTP method.
func (g *Gateway) handleBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.handleListBindings(w, r)
	case http.MethodPost:
		g.handleCreateBinding(w, r)
	case http.MethodDelete:
		g.handleDeleteBinding(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleListBindings handles GET /api/bindings.
func (g *Gateway) handleListBindings(w http.ResponseWriter, r *http.Request) {
	bindings, err := g.store.ListBindings(r.Context())
	if err != nil {
		g.logger.Error("failed to list bindings", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response := ListBindingsResponse{
		Bindings: make([]BindingResponse, len(bindings)),
	}

	for i, b := range bindings {
		agentOnline := false
		agentName := ""
		if agent, ok := g.agentManager.GetAgent(b.AgentID); ok {
			agentOnline = true
			agentName = agent.Name
		}

		response.Bindings[i] = BindingResponse{
			Frontend:    b.FrontendName,
			ChannelID:   b.ChannelID,
			AgentID:     b.AgentID,
			AgentName:   agentName,
			AgentOnline: agentOnline,
			CreatedAt:   b.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateBinding handles POST /api/bindings.
func (g *Gateway) handleCreateBinding(w http.ResponseWriter, r *http.Request) {
	var req CreateBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Frontend == "" || req.ChannelID == "" || req.AgentID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "frontend, channel_id, and agent_id are required")
		return
	}

	now := time.Now()
	binding := &store.ChannelBinding{
		FrontendName: req.Frontend,
		ChannelID:    req.ChannelID,
		AgentID:      req.AgentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := g.store.CreateBinding(r.Context(), binding); err != nil {
		// Check for duplicate
		if existing, _ := g.store.GetBinding(r.Context(), req.Frontend, req.ChannelID); existing != nil {
			g.sendJSONError(w, http.StatusConflict, "binding already exists")
			return
		}
		g.logger.Error("failed to create binding", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	agentOnline := false
	agentName := ""
	if agent, ok := g.agentManager.GetAgent(req.AgentID); ok {
		agentOnline = true
		agentName = agent.Name
	}

	response := BindingResponse{
		Frontend:    binding.FrontendName,
		ChannelID:   binding.ChannelID,
		AgentID:     binding.AgentID,
		AgentName:   agentName,
		AgentOnline: agentOnline,
		CreatedAt:   binding.CreatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleDeleteBinding handles DELETE /api/bindings?frontend=X&channel_id=Y.
func (g *Gateway) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	frontend := r.URL.Query().Get("frontend")
	channelID := r.URL.Query().Get("channel_id")

	if frontend == "" || channelID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "frontend and channel_id query params required")
		return
	}

	err := g.store.DeleteBinding(r.Context(), frontend, channelID)
	if errors.Is(err, store.ErrNotFound) {
		g.sendJSONError(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		g.logger.Error("failed to delete binding", "error", err, "frontend", frontend, "channel_id", channelID)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleThreadMessages handles GET /api/threads/{id}/messages requests.
// Returns the message history for a thread, optionally limited by ?limit=N.
func (g *Gateway) handleThreadMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract thread ID from path: /api/threads/{id}/messages
	path := r.URL.Path
	prefix := "/api/threads/"
	suffix := "/messages"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	threadID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if threadID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "thread_id is required")
		return
	}

	// Validate thread ID is a valid UUID
	if _, err := uuid.Parse(threadID); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid thread_id format")
		return
	}

	// Parse optional limit parameter (default 50, max 1000)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			g.sendJSONError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
		if limit > 1000 {
			limit = 1000
		}
	}

	// Verify thread exists
	_, err := g.store.GetThread(r.Context(), threadID)
	if errors.Is(err, store.ErrNotFound) {
		g.sendJSONError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err != nil {
		g.logger.Error("failed to get thread", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Get messages
	messages, err := g.store.GetThreadMessages(r.Context(), threadID, limit)
	if err != nil {
		g.logger.Error("failed to get messages", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Build response
	response := ThreadMessagesResponse{
		ThreadID: threadID,
		Messages: make([]MessageResponse, len(messages)),
	}

	for i, msg := range messages {
		response.Messages[i] = MessageResponse{
			ID:        msg.ID,
			ThreadID:  msg.ThreadID,
			Sender:    msg.Sender,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
