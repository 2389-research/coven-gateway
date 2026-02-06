// ABOUTME: HTTP API handlers for exposing agent messaging via SSE.
// ABOUTME: Provides POST /api/send endpoint for external clients like TUI.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/conversation"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
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
	InstanceID   string   `json:"instance_id,omitempty"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Workspaces   []string `json:"workspaces,omitempty"`
	WorkingDir   string   `json:"working_dir,omitempty"`
	Backend      string   `json:"backend,omitempty"`
}

// CreateBindingRequest is the JSON request body for POST /api/bindings.
// Uses instance_id to look up the agent by its short instance identifier.
type CreateBindingRequest struct {
	Frontend   string `json:"frontend"`
	ChannelID  string `json:"channel_id"`
	InstanceID string `json:"instance_id"`
}

// CreateBindingResponse is the JSON response for POST /api/bindings.
type CreateBindingResponse struct {
	BindingID   string  `json:"binding_id"`
	AgentName   string  `json:"agent_name"`
	WorkingDir  string  `json:"working_dir"`
	ReboundFrom *string `json:"rebound_from"`
}

// BindingResponse is the JSON response for binding operations.
type BindingResponse struct {
	Frontend    string `json:"frontend"`
	ChannelID   string `json:"channel_id"`
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name,omitempty"`
	AgentOnline bool   `json:"agent_online"`
	WorkingDir  string `json:"working_dir"`
	CreatedAt   string `json:"created_at"`
}

// ListBindingsResponse is the JSON response for GET /api/bindings.
type ListBindingsResponse struct {
	Bindings []BindingResponse `json:"bindings"`
}

// SingleBindingResponse is the JSON response for GET /api/bindings?frontend=X&channel_id=Y.
type SingleBindingResponse struct {
	BindingID  string `json:"binding_id"`
	AgentName  string `json:"agent_name"`
	WorkingDir string `json:"working_dir"`
	Online     bool   `json:"online"`
}

// SendToAgentRequest is the JSON request body for POST /api/agents/{id}/send.
type SendToAgentRequest struct {
	Message string `json:"message"`
}

// AgentHistoryEvent is the JSON response for events in GET /api/agents/{id}/history.
type AgentHistoryEvent struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Author    string `json:"author"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	ThreadID  string `json:"thread_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

// AgentHistoryUsage contains aggregated usage stats for an agent.
type AgentHistoryUsage struct {
	TotalInput      int64 `json:"total_input"`
	TotalOutput     int64 `json:"total_output"`
	TotalCacheRead  int64 `json:"total_cache_read"`
	TotalCacheWrite int64 `json:"total_cache_write"`
	TotalThinking   int64 `json:"total_thinking"`
	TotalTokens     int64 `json:"total_tokens"`
	RequestCount    int64 `json:"request_count"`
}

// AgentHistoryResponse is the JSON response for GET /api/agents/{id}/history.
type AgentHistoryResponse struct {
	AgentID    string              `json:"agent_id"`
	Events     []AgentHistoryEvent `json:"events"`
	Count      int                 `json:"count"`
	HasMore    bool                `json:"has_more"`
	NextCursor string              `json:"next_cursor,omitempty"`
	Usage      AgentHistoryUsage   `json:"usage"`
}

// MessageResponse is the JSON response for message history.
type MessageResponse struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Type      string `json:"type"`                // "message", "tool_use", "tool_result"
	ToolName  string `json:"tool_name,omitempty"` // For tool_use: name of the tool
	ToolID    string `json:"tool_id,omitempty"`   // Links tool_use to tool_result
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
// Supports optional ?workspace=X query parameter to filter by workspace membership.
func (g *Gateway) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	sender := g.getSender()
	agents := sender.ListAgents()

	// Check for workspace filter
	workspaceFilter := r.URL.Query().Get("workspace")

	response := make([]AgentInfoResponse, 0, len(agents))
	for _, a := range agents {
		// Apply workspace filter if specified
		if workspaceFilter != "" {
			if !containsWorkspace(a.Workspaces, workspaceFilter) {
				continue
			}
		}

		response = append(response, AgentInfoResponse{
			ID:           a.ID,
			InstanceID:   a.InstanceID,
			Name:         a.Name,
			Capabilities: a.Capabilities,
			Workspaces:   a.Workspaces,
			WorkingDir:   a.WorkingDir,
			Backend:      a.Backend,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// containsWorkspace checks if a workspace is in the list of workspaces.
func containsWorkspace(workspaces []string, target string) bool {
	for _, ws := range workspaces {
		if ws == target {
			return true
		}
	}
	return false
}

// handleSendMessage handles POST /api/send requests.
// It accepts a JSON body with the message content and streams responses via SSE.
// If agent_id is specified, the message is sent to that specific agent.
// Messages are persisted via ConversationService (the source of truth for history).
//
// Responsibilities:
//  1. Parse JSON body - decode SendMessageRequest from request body
//  2. Validate required fields - ensure content and sender are present
//  3. Resolve agent ID - look up via binding (frontend+channel_id) or use direct agent_id
//  4. Verify agent online - check agent exists and is available
//  5. Send via ConversationService - handles thread creation and message persistence
//  6. Setup SSE streaming - verify flusher support, set SSE headers
//  7. Stream responses as SSE - responses are already persisted by ConversationService
func (g *Gateway) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req, err := parseSendRequest(r.Body)
	if err != nil {
		g.sendJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Resolve agent ID and thread ID
	var agentID, threadID string
	var frontendName, externalID string
	if req.AgentID != "" {
		// Direct agent ID specified
		agentID = req.AgentID
		threadID = req.ThreadID
		if threadID == "" {
			// Generate a new thread ID if none provided - required for unique constraint
			threadID = uuid.New().String()
		}
		frontendName = "direct"
		externalID = threadID
	} else {
		// Must have frontend + channel_id for binding lookup
		if req.Frontend == "" || req.ChannelID == "" {
			g.sendJSONError(w, http.StatusBadRequest, "must specify agent_id or frontend+channel_id")
			return
		}

		// Use bindingResolver for binding and thread lookup
		resolver := &bindingResolver{store: g.store}
		result, err := resolver.Resolve(r.Context(), req.Frontend, req.ChannelID, req.ThreadID)
		if errors.Is(err, ErrChannelNotBound) {
			g.sendJSONError(w, http.StatusBadRequest, "channel not bound to agent")
			return
		}
		if err != nil {
			g.logger.Error("failed to resolve binding", "error", err)
			g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Find the online agent matching the binding's principal_id + working_dir
		// Bindings store (principal_id, working_dir) but agent manager is keyed by Connection.ID
		agentConn := g.agentManager.GetByPrincipalAndWorkDir(result.AgentID, result.WorkingDir)
		if agentConn == nil {
			g.sendJSONError(w, http.StatusServiceUnavailable, "agent unavailable")
			return
		}
		agentID = agentConn.ID // Use the connection ID, not principal_id
		threadID = result.ThreadID
		frontendName = req.Frontend
		externalID = req.ChannelID
	}

	// Verify agent exists and is online (for direct agent_id path)
	if req.AgentID != "" {
		if _, ok := g.agentManager.GetAgent(agentID); !ok {
			g.sendJSONError(w, http.StatusServiceUnavailable, "agent unavailable")
			return
		}
	}

	// Check streaming support before sending (fail fast)
	flusher, ok := w.(http.Flusher)
	if !ok {
		g.logger.Error("streaming not supported")
		g.sendJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Send message via ConversationService
	// This handles: thread creation, user message persistence, and response persistence
	convReq := &conversation.SendRequest{
		ThreadID:     threadID,
		FrontendName: frontendName,
		ExternalID:   externalID,
		AgentID:      agentID,
		Sender:       req.Sender,
		Content:      req.Content,
	}

	convResp, err := g.conversation.SendMessage(r.Context(), convReq)
	if err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			g.sendJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		g.logger.Error("failed to send message", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial "started" event with thread_id so client can track the conversation
	g.writeSSEEvent(w, "started", map[string]string{"thread_id": convResp.ThreadID})
	flusher.Flush()

	// Stream responses (persistence is handled by ConversationService)
	g.streamResponses(r.Context(), w, flusher, convResp.Stream)
}

// streamResponses reads from the response channel and writes SSE events.
// Message persistence is handled by ConversationService which wraps the channel.
func (g *Gateway) streamResponses(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, respChan <-chan *agent.Response) {
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

			if resp.Event == agent.EventDone {
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
	case agent.EventSessionInit:
		return SSEEvent{
			Event: "session_init",
			Data:  map[string]string{"session_id": resp.SessionID},
		}
	case agent.EventSessionOrphaned:
		return SSEEvent{
			Event: "session_orphaned",
			Data:  map[string]string{"reason": resp.Error},
		}
	case agent.EventUsage:
		if resp.Usage == nil {
			return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed usage event"}}
		}
		return SSEEvent{
			Event: "usage",
			Data: map[string]interface{}{
				"input_tokens":       resp.Usage.InputTokens,
				"output_tokens":      resp.Usage.OutputTokens,
				"cache_read_tokens":  resp.Usage.CacheReadTokens,
				"cache_write_tokens": resp.Usage.CacheWriteTokens,
				"thinking_tokens":    resp.Usage.ThinkingTokens,
			},
		}
	case agent.EventToolState:
		if resp.ToolState == nil {
			return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed tool_state event"}}
		}
		return SSEEvent{
			Event: "tool_state",
			Data: map[string]string{
				"id":     resp.ToolState.ID,
				"state":  resp.ToolState.State,
				"detail": resp.ToolState.Detail,
			},
		}
	case agent.EventCancelled:
		return SSEEvent{
			Event: "cancelled",
			Data:  map[string]string{"reason": resp.Error},
		}
	case agent.EventToolApprovalRequest:
		if resp.ToolApprovalRequest == nil {
			return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed tool_approval event"}}
		}
		return SSEEvent{
			Event: "tool_approval",
			Data: map[string]string{
				"id":         resp.ToolApprovalRequest.ID,
				"name":       resp.ToolApprovalRequest.Name,
				"input_json": resp.ToolApprovalRequest.InputJSON,
				"request_id": resp.ToolApprovalRequest.RequestID,
			},
		}
	default:
		return SSEEvent{
			Event: "unknown",
			Data:  map[string]string{"text": resp.Text},
		}
	}
}

// formatSSEEvent formats an SSE event as a string with the standard format:
// event: <eventType>\ndata: <data>\n\n
func formatSSEEvent(eventType, data string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
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

// parseSendRequest parses and validates a SendMessageRequest from the given reader.
// Returns an error if the JSON is invalid or required fields (content, sender) are missing.
func parseSendRequest(r io.Reader) (*SendMessageRequest, error) {
	var req SendMessageRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return nil, errors.New("invalid JSON body")
	}

	if req.Content == "" {
		return nil, errors.New("content is required")
	}

	if req.Sender == "" {
		return nil, errors.New("sender is required")
	}

	return &req, nil
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

// ErrChannelNotBound is returned when a channel has no binding to an agent.
var ErrChannelNotBound = errors.New("channel not bound to agent")

// BindingResult contains the resolved thread and agent information.
type BindingResult struct {
	ThreadID   string
	AgentID    string // principal_id from the binding
	WorkingDir string // working_dir from the binding (needed to find exact agent)
}

// bindingResolver handles looking up and creating bindings and threads.
type bindingResolver struct {
	store store.Store
}

// Resolve looks up a binding for the given frontend and channel.
// If a threadID is provided, it uses that; otherwise it looks up an existing thread
// by frontend/channel or generates a new thread ID.
// Returns ErrChannelNotBound if no binding exists for the channel.
func (r *bindingResolver) Resolve(ctx context.Context, frontend, channelID, threadID string) (*BindingResult, error) {
	// Look up the binding from V2 bindings table (created by admin service)
	binding, err := r.store.GetBindingByChannel(ctx, frontend, channelID)
	if errors.Is(err, store.ErrBindingNotFound) {
		return nil, ErrChannelNotBound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get binding: %w", err)
	}

	result := &BindingResult{
		AgentID:    binding.AgentID,
		WorkingDir: binding.WorkingDir,
	}

	// If thread ID was provided, use it
	if threadID != "" {
		result.ThreadID = threadID
		return result, nil
	}

	// Try to find existing thread by frontend/channel
	thread, err := r.store.GetThreadByFrontendID(ctx, frontend, channelID)
	if err == nil {
		result.ThreadID = thread.ID
		return result, nil
	}

	// No existing thread, generate a new ID
	if errors.Is(err, store.ErrNotFound) {
		result.ThreadID = uuid.New().String()
		return result, nil
	}

	return nil, fmt.Errorf("failed to get thread: %w", err)
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
// When frontend+channel_id query params are provided, returns a single binding status.
// Otherwise, lists all bindings.
func (g *Gateway) handleListBindings(w http.ResponseWriter, r *http.Request) {
	frontend := r.URL.Query().Get("frontend")
	channelID := r.URL.Query().Get("channel_id")

	// If frontend + channel_id provided, return single binding status
	if frontend != "" && channelID != "" {
		g.handleGetSingleBinding(w, r, frontend, channelID)
		return
	}

	// List all bindings
	bindings, err := g.store.ListBindingsV2(r.Context(), store.BindingFilter{})
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
		// Use GetByPrincipalAndWorkDir since b.AgentID is principal_id, not connection ID
		if agent := g.agentManager.GetByPrincipalAndWorkDir(b.AgentID, b.WorkingDir); agent != nil {
			agentOnline = true
			agentName = agent.Name
		}

		response.Bindings[i] = BindingResponse{
			Frontend:    b.Frontend,
			ChannelID:   b.ChannelID,
			AgentID:     b.AgentID,
			AgentName:   agentName,
			AgentOnline: agentOnline,
			WorkingDir:  b.WorkingDir,
			CreatedAt:   b.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetSingleBinding handles GET /api/bindings?frontend=X&channel_id=Y.
// Returns status for a single binding including working_dir and online status.
func (g *Gateway) handleGetSingleBinding(w http.ResponseWriter, r *http.Request, frontend, channelID string) {
	binding, err := g.store.GetBindingByChannel(r.Context(), frontend, channelID)
	if errors.Is(err, store.ErrBindingNotFound) {
		g.sendJSONError(w, http.StatusNotFound, "no binding for this channel")
		return
	}
	if err != nil {
		g.logger.Error("failed to get binding", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Check if agent is online - use GetByPrincipalAndWorkDir since binding.AgentID is principal_id
	online := false
	agentName := ""
	if agent := g.agentManager.GetByPrincipalAndWorkDir(binding.AgentID, binding.WorkingDir); agent != nil {
		online = true
		agentName = agent.Name
	}

	response := SingleBindingResponse{
		BindingID:  binding.ID,
		AgentName:  agentName,
		WorkingDir: binding.WorkingDir,
		Online:     online,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateBinding handles POST /api/bindings.
// Looks up an agent by instance_id and creates a binding to it.
// Handles rebinding if the channel is already bound to a different agent.
func (g *Gateway) handleCreateBinding(w http.ResponseWriter, r *http.Request) {
	var req CreateBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Frontend == "" || req.ChannelID == "" || req.InstanceID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "frontend, channel_id, and instance_id are required")
		return
	}

	// Look up agent by instance_id
	agentConn := g.agentManager.GetByInstanceID(req.InstanceID)
	if agentConn == nil {
		g.sendJSONError(w, http.StatusNotFound, fmt.Sprintf("no agent online with instance_id '%s'", req.InstanceID))
		return
	}

	ctx := r.Context()
	var reboundFrom *string

	// Check if binding already exists for this (frontend, channel_id)
	existingBinding, err := g.store.GetBindingByChannel(ctx, req.Frontend, req.ChannelID)
	if err != nil && !errors.Is(err, store.ErrBindingNotFound) {
		g.logger.Error("failed to check existing binding", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if existingBinding != nil {
		// Binding already exists
		if existingBinding.AgentID == agentConn.PrincipalID && existingBinding.WorkingDir == agentConn.WorkingDir {
			// Same agent and workdir - return existing binding (idempotent)
			response := CreateBindingResponse{
				BindingID:   existingBinding.ID,
				AgentName:   agentConn.Name,
				WorkingDir:  existingBinding.WorkingDir,
				ReboundFrom: nil,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Different agent - delete old binding and record rebound
		oldAgentName := ""
		// Use GetByPrincipalAndWorkDir since existingBinding.AgentID is principal_id
		if oldAgent := g.agentManager.GetByPrincipalAndWorkDir(existingBinding.AgentID, existingBinding.WorkingDir); oldAgent != nil {
			oldAgentName = oldAgent.Name
		} else {
			oldAgentName = existingBinding.AgentID // fallback to ID if agent offline
		}
		reboundFrom = &oldAgentName

		if err := g.store.DeleteBindingByID(ctx, existingBinding.ID); err != nil {
			g.logger.Error("failed to delete existing binding", "error", err)
			g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	// Create new binding
	bindingID := uuid.New().String()
	binding := &store.Binding{
		ID:         bindingID,
		Frontend:   req.Frontend,
		ChannelID:  req.ChannelID,
		AgentID:    agentConn.PrincipalID,
		WorkingDir: agentConn.WorkingDir,
		CreatedAt:  time.Now(),
		CreatedBy:  nil, // TODO: get from auth context when available
	}

	if err := g.store.CreateBindingV2(ctx, binding); err != nil {
		if errors.Is(err, store.ErrDuplicateChannel) {
			g.sendJSONError(w, http.StatusConflict, "binding already exists")
			return
		}
		g.logger.Error("failed to create binding", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response := CreateBindingResponse{
		BindingID:   bindingID,
		AgentName:   agentConn.Name,
		WorkingDir:  agentConn.WorkingDir,
		ReboundFrom: reboundFrom,
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

	err := g.store.DeleteBindingByChannel(r.Context(), frontend, channelID)
	if errors.Is(err, store.ErrBindingNotFound) {
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

// handleThreadRoutes routes /api/threads/{id}/... requests to the appropriate handler.
func (g *Gateway) handleThreadRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/messages") {
		g.handleThreadMessages(w, r)
		return
	}
	if strings.HasSuffix(path, "/usage") {
		g.handleThreadUsage(w, r)
		return
	}
	g.sendJSONError(w, http.StatusNotFound, "unknown endpoint")
}

// handleThreadMessages handles GET /api/threads/{id}/messages requests.
// Returns the message history for a thread, optionally limited by ?limit=N.
// Uses ledger_events as the source of truth for unified message storage.
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

	// Get messages from unified ledger_events storage
	events, err := g.store.GetEventsByThreadID(r.Context(), threadID, limit)
	if err != nil {
		g.logger.Error("failed to get events", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Build response - convert events to message format for backward compatibility
	response := ThreadMessagesResponse{
		ThreadID: threadID,
		Messages: make([]MessageResponse, len(events)),
	}

	for i, evt := range events {
		response.Messages[i] = g.eventToMessageResponse(threadID, evt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// eventToMessageResponse converts a ledger event to MessageResponse for API backward compatibility.
// Uses the shared store.EventToMessage helper for core conversion logic, then formats for API.
func (g *Gateway) eventToMessageResponse(threadID string, evt *store.LedgerEvent) MessageResponse {
	// Use shared conversion helper for the core logic
	storeMsg := store.EventToMessage(evt)

	// Build API response from store.Message
	return MessageResponse{
		ID:        storeMsg.ID,
		ThreadID:  threadID,
		Sender:    storeMsg.Sender,
		Content:   storeMsg.Content,
		Type:      storeMsg.Type,
		ToolName:  storeMsg.ToolName,
		ToolID:    storeMsg.ToolID,
		CreatedAt: storeMsg.CreatedAt.Format(time.RFC3339),
	}
}

// handleAgentRoutes routes /api/agents/{id}/* requests to the appropriate handler.
// Routes:
// - GET /api/agents/{id}/history -> handleAgentHistoryImpl
// - POST /api/agents/{id}/send -> handleSendToAgent
func (g *Gateway) handleAgentRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := "/api/agents/"

	if !strings.HasPrefix(path, prefix) {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	// Route based on suffix
	switch {
	case strings.HasSuffix(path, "/history"):
		g.handleAgentHistoryImpl(w, r)
	case strings.HasSuffix(path, "/send"):
		g.handleSendToAgent(w, r)
	default:
		g.sendJSONError(w, http.StatusBadRequest, "invalid path: must end with /history or /send")
	}
}

// handleAgentHistory handles GET /api/agents/{id}/history requests.
// Returns recent conversation events for a specific agent, ordered by timestamp DESC.
// Deprecated: use handleAgentRoutes which dispatches to handleAgentHistoryImpl.
func (g *Gateway) handleAgentHistory(w http.ResponseWriter, r *http.Request) {
	g.handleAgentRoutes(w, r)
}

// handleAgentHistoryImpl handles GET /api/agents/{id}/history requests.
// Returns conversation events for a specific agent with pagination and usage stats.
// Query params: limit (default 50, max 500), cursor (for pagination)
func (g *Gateway) handleAgentHistoryImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path: /api/agents/{id}/history
	path := r.URL.Path
	prefix := "/api/agents/"
	suffix := "/history"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	agentID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if agentID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	// Validate agent ID doesn't contain path traversal characters
	if strings.Contains(agentID, "/") || strings.Contains(agentID, "..") {
		g.sendJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Note: We intentionally don't require the agent to be currently connected.
	// History should be queryable even for offline agents.

	// Parse optional limit parameter (default 50, max 500)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			g.sendJSONError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
		if limit > 500 {
			limit = 500
		}
	}

	// Parse optional cursor parameter for pagination
	cursor := r.URL.Query().Get("cursor")

	// Query events by conversation key (agent ID), ordered chronologically
	result, err := g.store.GetEvents(r.Context(), store.GetEventsParams{
		ConversationKey: agentID,
		Limit:           limit,
		Cursor:          cursor,
	})
	if err != nil {
		g.logger.Error("failed to get events", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Build events response
	events := make([]AgentHistoryEvent, len(result.Events))
	for i, evt := range result.Events {
		events[i] = AgentHistoryEvent{
			ID:        evt.ID,
			Direction: string(evt.Direction),
			Author:    evt.Author,
			Type:      string(evt.Type),
			Timestamp: evt.Timestamp.Format(time.RFC3339),
		}
		if evt.ThreadID != nil {
			events[i].ThreadID = *evt.ThreadID
		}
		if evt.Text != nil {
			events[i].Text = *evt.Text
		}
	}

	// Get aggregated usage stats
	var usage AgentHistoryUsage
	usageStore, ok := g.store.(store.UsageStore)
	if ok {
		stats, err := usageStore.GetUsageStats(r.Context(), store.UsageFilter{
			AgentID: &agentID,
		})
		if err != nil {
			g.logger.Warn("failed to get usage stats", "error", err)
			// Continue without usage stats rather than failing the request
		} else {
			usage = AgentHistoryUsage{
				TotalInput:      stats.TotalInput,
				TotalOutput:     stats.TotalOutput,
				TotalCacheRead:  stats.TotalCacheRead,
				TotalCacheWrite: stats.TotalCacheWrite,
				TotalThinking:   stats.TotalThinking,
				TotalTokens:     stats.TotalTokens,
				RequestCount:    stats.RequestCount,
			}
		}
	}

	response := AgentHistoryResponse{
		AgentID: agentID,
		Events:  events,
		Count:   len(events),
		HasMore: result.HasMore,
		Usage:   usage,
	}
	if result.NextCursor != "" {
		response.NextCursor = result.NextCursor
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Error("failed to encode agent history response", "error", err)
	}
}

// handleSendToAgent handles POST /api/agents/{id}/send requests.
// Sends a message to a specific agent and streams the response via SSE.
func (g *Gateway) handleSendToAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path: /api/agents/{id}/send
	path := r.URL.Path
	prefix := "/api/agents/"
	suffix := "/send"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	agentID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if agentID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	// Parse JSON body
	var req SendToAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate message is not empty
	if req.Message == "" {
		g.sendJSONError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Look up agent by ID to verify it exists
	_, ok := g.agentManager.GetAgent(agentID)
	if !ok {
		g.sendJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Check streaming support before sending (fail fast)
	flusher, ok := w.(http.Flusher)
	if !ok {
		g.logger.Error("streaming not supported")
		g.sendJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Generate unique conversation key for this request
	conversationKey := "leader:" + uuid.New().String()

	// Send message via ConversationService
	convReq := &conversation.SendRequest{
		ThreadID:     uuid.New().String(), // New thread for each request
		FrontendName: "leader",
		ExternalID:   conversationKey,
		AgentID:      agentID,
		Sender:       "coven-leader",
		Content:      req.Message,
	}

	convResp, err := g.conversation.SendMessage(r.Context(), convReq)
	if err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			g.sendJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		g.logger.Error("failed to send message", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial "started" event with thread_id so client can track the conversation
	g.writeSSEEvent(w, "started", map[string]string{"thread_id": convResp.ThreadID})
	flusher.Flush()

	// Stream responses (persistence is handled by ConversationService)
	g.streamResponses(r.Context(), w, flusher, convResp.Stream)
}

// UsageStatsResponse is the JSON response for GET /api/stats/usage.
type UsageStatsResponse struct {
	TotalInput      int64 `json:"total_input"`
	TotalOutput     int64 `json:"total_output"`
	TotalCacheRead  int64 `json:"total_cache_read"`
	TotalCacheWrite int64 `json:"total_cache_write"`
	TotalThinking   int64 `json:"total_thinking"`
	TotalTokens     int64 `json:"total_tokens"`
	RequestCount    int64 `json:"request_count"`
}

// ThreadUsageResponse is the JSON response for GET /api/threads/{id}/usage.
type ThreadUsageResponse struct {
	ThreadID string          `json:"thread_id"`
	Usage    []UsageResponse `json:"usage"`
}

// UsageResponse represents a single usage record.
type UsageResponse struct {
	ID               string `json:"id"`
	MessageID        string `json:"message_id,omitempty"`
	RequestID        string `json:"request_id"`
	AgentID          string `json:"agent_id"`
	InputTokens      int32  `json:"input_tokens"`
	OutputTokens     int32  `json:"output_tokens"`
	CacheReadTokens  int32  `json:"cache_read_tokens"`
	CacheWriteTokens int32  `json:"cache_write_tokens"`
	ThinkingTokens   int32  `json:"thinking_tokens"`
	CreatedAt        string `json:"created_at"`
}

// handleUsageStats handles GET /api/stats/usage requests.
// Returns aggregate token usage statistics with optional filters.
// Query parameters:
//   - agent_id: filter by agent
//   - since: start time (RFC3339)
//   - until: end time (RFC3339)
func (g *Gateway) handleUsageStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Type assert to get UsageStore methods
	usageStore, ok := g.store.(store.UsageStore)
	if !ok {
		g.logger.Error("store does not implement UsageStore")
		g.sendJSONError(w, http.StatusInternalServerError, "usage tracking not available")
		return
	}

	// Parse filter parameters
	filter := store.UsageFilter{}

	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		filter.AgentID = &agentID
	}

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			g.sendJSONError(w, http.StatusBadRequest, "invalid 'since' format (use RFC3339)")
			return
		}
		filter.Since = &since
	}

	if untilStr := r.URL.Query().Get("until"); untilStr != "" {
		until, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			g.sendJSONError(w, http.StatusBadRequest, "invalid 'until' format (use RFC3339)")
			return
		}
		filter.Until = &until
	}

	stats, err := usageStore.GetUsageStats(r.Context(), filter)
	if err != nil {
		g.logger.Error("failed to get usage stats", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response := UsageStatsResponse{
		TotalInput:      stats.TotalInput,
		TotalOutput:     stats.TotalOutput,
		TotalCacheRead:  stats.TotalCacheRead,
		TotalCacheWrite: stats.TotalCacheWrite,
		TotalThinking:   stats.TotalThinking,
		TotalTokens:     stats.TotalTokens,
		RequestCount:    stats.RequestCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleThreadUsage handles GET /api/threads/{id}/usage requests.
// Returns token usage records for a specific thread.
func (g *Gateway) handleThreadUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract thread ID from path: /api/threads/{id}/usage
	path := r.URL.Path
	prefix := "/api/threads/"
	suffix := "/usage"

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

	// Type assert to get UsageStore methods
	usageStore, ok := g.store.(store.UsageStore)
	if !ok {
		g.logger.Error("store does not implement UsageStore")
		g.sendJSONError(w, http.StatusInternalServerError, "usage tracking not available")
		return
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

	// Get usage records
	usages, err := usageStore.GetThreadUsage(r.Context(), threadID)
	if err != nil {
		g.logger.Error("failed to get thread usage", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Build response
	response := ThreadUsageResponse{
		ThreadID: threadID,
		Usage:    make([]UsageResponse, len(usages)),
	}

	for i, u := range usages {
		response.Usage[i] = UsageResponse{
			ID:               u.ID,
			MessageID:        u.MessageID,
			RequestID:        u.RequestID,
			AgentID:          u.AgentID,
			InputTokens:      u.InputTokens,
			OutputTokens:     u.OutputTokens,
			CacheReadTokens:  u.CacheReadTokens,
			CacheWriteTokens: u.CacheWriteTokens,
			ThinkingTokens:   u.ThinkingTokens,
			CreatedAt:        u.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ToolApprovalRequestBody is the JSON request for POST /api/tools/approve.
type ToolApprovalRequestBody struct {
	AgentID    string `json:"agent_id"`
	ToolID     string `json:"tool_id"`
	Approved   bool   `json:"approved"`
	ApproveAll bool   `json:"approve_all,omitempty"`
}

// handleToolApproval handles POST /api/tools/approve requests.
// Sends a tool approval response to the agent.
func (g *Gateway) handleToolApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req ToolApprovalRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.AgentID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	if req.ToolID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "tool_id is required")
		return
	}

	err := g.agentManager.SendToolApproval(req.AgentID, req.ToolID, req.Approved, req.ApproveAll)
	if errors.Is(err, agent.ErrAgentNotFound) {
		g.sendJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		g.logger.Error("failed to send tool approval", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "failed to send approval")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"approved": req.Approved,
	})
}

// AnswerQuestionRequestBody is the JSON request for POST /api/questions/answer.
type AnswerQuestionRequestBody struct {
	AgentID    string   `json:"agent_id"`
	QuestionID string   `json:"question_id"`
	Selected   []string `json:"selected"`
	CustomText string   `json:"custom_text,omitempty"`
}

// handleAnswerQuestion handles POST /api/questions/answer requests.
// Sends a user's answer to a pending ask_user question.
func (g *Gateway) handleAnswerQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req AnswerQuestionRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.AgentID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	if req.QuestionID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "question_id is required")
		return
	}

	if len(req.Selected) == 0 && req.CustomText == "" {
		g.sendJSONError(w, http.StatusBadRequest, "at least one selection or custom_text is required")
		return
	}

	if g.questionRouter == nil {
		g.sendJSONError(w, http.StatusServiceUnavailable, "question router not configured")
		return
	}

	// Build the proto answer
	answer := &pb.AnswerQuestionRequest{
		AgentId:    req.AgentID,
		QuestionId: req.QuestionID,
		Selected:   req.Selected,
	}
	if req.CustomText != "" {
		answer.CustomText = &req.CustomText
	}

	err := g.questionRouter.DeliverAnswer(req.AgentID, req.QuestionID, answer)
	if err != nil {
		g.logger.Error("failed to deliver question answer", "error", err)
		// Return generic error to avoid leaking internal agent IDs
		g.sendJSONError(w, http.StatusNotFound, "question not found or already answered")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
