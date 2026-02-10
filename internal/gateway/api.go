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
	"slices"
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
	Event string `json:"event"`
	Data  any    `json:"data"`
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
}

// containsWorkspace checks if a workspace is in the list of workspaces.
func containsWorkspace(workspaces []string, target string) bool {
	return slices.Contains(workspaces, target)
}

// parseLimitParam parses a limit query parameter with default and max values.
// Returns parsed value clamped to [1, max], or default if not specified.
// Returns 0 and error message if invalid.
func parseLimitParam(r *http.Request, defaultLimit, maxLimit int) (int, string) {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit, ""
	}
	parsed, err := strconv.Atoi(limitStr)
	if err != nil || parsed < 1 {
		return 0, "limit must be a positive integer"
	}
	if parsed > maxLimit {
		return maxLimit, ""
	}
	return parsed, ""
}

// extractPathSegment extracts a segment from a path between prefix and suffix.
// Returns the segment and true if successful, or empty string and false if invalid.
func extractPathSegment(path, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	segment := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if segment == "" || strings.Contains(segment, "/") || strings.Contains(segment, "..") {
		return "", false
	}
	return segment, true
}

// eventToHistoryEvent converts a LedgerEvent to an AgentHistoryEvent.
func eventToHistoryEvent(evt store.LedgerEvent) AgentHistoryEvent {
	e := AgentHistoryEvent{
		ID:        evt.ID,
		Direction: string(evt.Direction),
		Author:    evt.Author,
		Type:      string(evt.Type),
		Timestamp: evt.Timestamp.Format(time.RFC3339),
	}
	if evt.ThreadID != nil {
		e.ThreadID = *evt.ThreadID
	}
	if evt.Text != nil {
		e.Text = *evt.Text
	}
	return e
}

// usageStatsToHistory converts store.UsageStats to AgentHistoryUsage.
func usageStatsToHistory(stats *store.UsageStats) AgentHistoryUsage {
	return AgentHistoryUsage{
		TotalInput:      stats.TotalInput,
		TotalOutput:     stats.TotalOutput,
		TotalCacheRead:  stats.TotalCacheRead,
		TotalCacheWrite: stats.TotalCacheWrite,
		TotalThinking:   stats.TotalThinking,
		TotalTokens:     stats.TotalTokens,
		RequestCount:    stats.RequestCount,
	}
}

// handleSendMessage handles POST /api/send requests.
// It accepts a JSON body with the message content and streams responses via SSE.
// If agent_id is specified, the message is sent to that specific agent.
// Messages are persisted via ConversationService (the source of truth for history).
//
// Responsibilities:
//  1. Parse JSON body - decode SendMessageRequest from request body
//
// resolvedTarget holds the result of agent/thread resolution.
type resolvedTarget struct {
	AgentID      string
	ThreadID     string
	FrontendName string
	ExternalID   string
}

// resolveTarget resolves agent ID and thread ID from the request.
// Returns nil with an error message if resolution fails.
func (g *Gateway) resolveTarget(ctx context.Context, req *SendMessageRequest) (*resolvedTarget, string) {
	if req.AgentID != "" {
		// Direct agent ID specified
		threadID := req.ThreadID
		if threadID == "" {
			threadID = uuid.New().String()
		}
		// Verify agent exists and is online
		if _, ok := g.agentManager.GetAgent(req.AgentID); !ok {
			return nil, "agent unavailable"
		}
		return &resolvedTarget{
			AgentID:      req.AgentID,
			ThreadID:     threadID,
			FrontendName: "direct",
			ExternalID:   threadID,
		}, ""
	}

	// Must have frontend + channel_id for binding lookup
	if req.Frontend == "" || req.ChannelID == "" {
		return nil, "must specify agent_id or frontend+channel_id"
	}

	// Use bindingResolver for binding and thread lookup
	resolver := &bindingResolver{store: g.store}
	result, err := resolver.Resolve(ctx, req.Frontend, req.ChannelID, req.ThreadID)
	if errors.Is(err, ErrChannelNotBound) {
		return nil, "channel not bound to agent"
	}
	if err != nil {
		g.logger.Error("failed to resolve binding", "error", err)
		return nil, "internal server error"
	}

	// Find the online agent matching the binding's principal_id + working_dir
	agentConn := g.agentManager.GetByPrincipalAndWorkDir(result.AgentID, result.WorkingDir)
	if agentConn == nil {
		return nil, "agent unavailable"
	}

	return &resolvedTarget{
		AgentID:      agentConn.ID,
		ThreadID:     result.ThreadID,
		FrontendName: req.Frontend,
		ExternalID:   req.ChannelID,
	}, ""
}

// 2. Validate required fields - ensure content and sender are present
// 3. Resolve agent ID - look up via binding (frontend+channel_id) or use direct agent_id
// 4. Verify agent online - check agent exists and is available
// 5. Send via ConversationService - handles thread creation and message persistence
// 6. Setup SSE streaming - verify flusher support, set SSE headers
// 7. Stream responses as SSE - responses are already persisted by ConversationService.
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

	// Resolve agent ID and thread ID using helper
	target, errMsg := g.resolveTarget(r.Context(), req)
	if target == nil {
		// Determine appropriate status code based on error message
		var status int
		switch errMsg {
		case "agent unavailable":
			status = http.StatusServiceUnavailable
		case "internal server error":
			status = http.StatusInternalServerError
		default:
			status = http.StatusBadRequest
		}
		g.sendJSONError(w, status, errMsg)
		return
	}

	agentID := target.AgentID
	threadID := target.ThreadID
	frontendName := target.FrontendName
	externalID := target.ExternalID

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
			g.writeSSEEvent(w, "error", map[string]string{"error": "request canceled"})
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

// malformedEvent returns an error SSE event for malformed data.
func malformedEvent(eventType string) SSEEvent {
	return SSEEvent{Event: "error", Data: map[string]string{"error": "malformed " + eventType + " event"}}
}

// toolUseToSSE converts a ToolUse event to SSE format.
func toolUseToSSE(tu *agent.ToolUseEvent) SSEEvent {
	if tu == nil {
		return malformedEvent("tool_use")
	}
	return SSEEvent{Event: "tool_use", Data: map[string]string{"id": tu.ID, "name": tu.Name, "input_json": tu.InputJSON}}
}

// toolResultToSSE converts a ToolResult event to SSE format.
func toolResultToSSE(tr *agent.ToolResultEvent) SSEEvent {
	if tr == nil {
		return malformedEvent("tool_result")
	}
	return SSEEvent{Event: "tool_result", Data: map[string]any{"id": tr.ID, "output": tr.Output, "is_error": tr.IsError}}
}

// fileToSSE converts a File event to SSE format.
func fileToSSE(f *agent.FileEvent) SSEEvent {
	if f == nil {
		return malformedEvent("file")
	}
	return SSEEvent{Event: "file", Data: map[string]string{"filename": f.Filename, "mime_type": f.MimeType}}
}

// usageToSSE converts a Usage event to SSE format.
func usageToSSE(u *agent.UsageEvent) SSEEvent {
	if u == nil {
		return malformedEvent("usage")
	}
	return SSEEvent{Event: "usage", Data: map[string]any{
		"input_tokens": u.InputTokens, "output_tokens": u.OutputTokens,
		"cache_read_tokens": u.CacheReadTokens, "cache_write_tokens": u.CacheWriteTokens,
		"thinking_tokens": u.ThinkingTokens,
	}}
}

// toolStateToSSE converts a ToolState event to SSE format.
func toolStateToSSE(ts *agent.ToolStateEvent) SSEEvent {
	if ts == nil {
		return malformedEvent("tool_state")
	}
	return SSEEvent{Event: "tool_state", Data: map[string]string{"id": ts.ID, "state": ts.State, "detail": ts.Detail}}
}

// toolApprovalToSSE converts a ToolApprovalRequest event to SSE format.
func toolApprovalToSSE(ta *agent.ToolApprovalRequestEvent) SSEEvent {
	if ta == nil {
		return malformedEvent("tool_approval")
	}
	return SSEEvent{Event: "tool_approval", Data: map[string]string{"id": ta.ID, "name": ta.Name, "input_json": ta.InputJSON, "request_id": ta.RequestID}}
}

// responseToSSEEvent converts an agent response to an SSE event.
// SSE event builders for simple text-based events.
func textSSE(event, key, value string) SSEEvent {
	return SSEEvent{Event: event, Data: map[string]string{key: value}}
}

// responseConverter is a function that converts an agent.Response to an SSEEvent.
type responseConverter func(*agent.Response) SSEEvent

// sseConverters maps event types to their converter functions.
var sseConverters = map[agent.ResponseEvent]responseConverter{
	agent.EventThinking:            func(r *agent.Response) SSEEvent { return textSSE("thinking", "text", r.Text) },
	agent.EventText:                func(r *agent.Response) SSEEvent { return textSSE("text", "text", r.Text) },
	agent.EventToolUse:             func(r *agent.Response) SSEEvent { return toolUseToSSE(r.ToolUse) },
	agent.EventToolResult:          func(r *agent.Response) SSEEvent { return toolResultToSSE(r.ToolResult) },
	agent.EventFile:                func(r *agent.Response) SSEEvent { return fileToSSE(r.File) },
	agent.EventDone:                func(r *agent.Response) SSEEvent { return textSSE("done", "full_response", r.Text) },
	agent.EventError:               func(r *agent.Response) SSEEvent { return textSSE("error", "error", r.Error) },
	agent.EventSessionInit:         func(r *agent.Response) SSEEvent { return textSSE("session_init", "session_id", r.SessionID) },
	agent.EventSessionOrphaned:     func(r *agent.Response) SSEEvent { return textSSE("session_orphaned", "reason", r.Error) },
	agent.EventUsage:               func(r *agent.Response) SSEEvent { return usageToSSE(r.Usage) },
	agent.EventToolState:           func(r *agent.Response) SSEEvent { return toolStateToSSE(r.ToolState) },
	agent.EventCanceled:            func(r *agent.Response) SSEEvent { return textSSE("canceled", "reason", r.Error) },
	agent.EventToolApprovalRequest: func(r *agent.Response) SSEEvent { return toolApprovalToSSE(r.ToolApprovalRequest) },
}

func (g *Gateway) responseToSSEEvent(resp *agent.Response) SSEEvent {
	if conv, ok := sseConverters[resp.Event]; ok {
		return conv(resp)
	}
	return textSSE("unknown", "text", resp.Text)
}

// formatSSEEvent formats an SSE event as a string with the standard format:
// event: <eventType>\ndata: <data>\n\n
func formatSSEEvent(eventType, data string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
}

// writeSSEEvent writes a single SSE event to the response writer.
func (g *Gateway) writeSSEEvent(w http.ResponseWriter, event string, data any) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		g.logger.Error("failed to marshal SSE data", "error", err)
		return
	}

	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", dataJSON)
}

// sendJSONError writes a JSON error response.
func (g *Gateway) sendJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		g.logger.Debug("failed to encode error response", "error", err)
	}
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
}

// validateCreateBindingRequest validates the binding request fields.
func validateCreateBindingRequest(req *CreateBindingRequest) string {
	if req.Frontend == "" || req.ChannelID == "" || req.InstanceID == "" {
		return "frontend, channel_id, and instance_id are required"
	}
	return ""
}

// bindingMatchesAgent checks if an existing binding matches the target agent and workdir.
func bindingMatchesAgent(binding *store.Binding, agentConn *agent.Connection) bool {
	return binding != nil && binding.AgentID == agentConn.PrincipalID && binding.WorkingDir == agentConn.WorkingDir
}

// handleCreateBinding handles POST /api/bindings.
// Looks up an agent by instance_id and creates a binding to it.
// Handles rebinding if the channel is already bound to a different agent.
// decodeAndValidateBindingRequest decodes and validates the request body.
func (g *Gateway) decodeAndValidateBindingRequest(w http.ResponseWriter, r *http.Request) (*CreateBindingRequest, bool) {
	var req CreateBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return nil, false
	}
	if errMsg := validateCreateBindingRequest(&req); errMsg != "" {
		g.sendJSONError(w, http.StatusBadRequest, errMsg)
		return nil, false
	}
	return &req, true
}

// handleCreateBindingError handles errors from binding creation.
func (g *Gateway) handleCreateBindingError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrDuplicateChannel) {
		g.sendJSONError(w, http.StatusConflict, "binding already exists")
		return
	}
	g.logger.Error("failed to create binding", "error", err)
	g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
}

func (g *Gateway) handleCreateBinding(w http.ResponseWriter, r *http.Request) {
	req, ok := g.decodeAndValidateBindingRequest(w, r)
	if !ok {
		return
	}

	agentConn := g.agentManager.GetByInstanceID(req.InstanceID)
	if agentConn == nil {
		g.sendJSONError(w, http.StatusNotFound, fmt.Sprintf("no agent online with instance_id '%s'", req.InstanceID))
		return
	}

	ctx := r.Context()
	existingBinding, err := g.store.GetBindingByChannel(ctx, req.Frontend, req.ChannelID)
	if err != nil && !errors.Is(err, store.ErrBindingNotFound) {
		g.logger.Error("failed to check existing binding", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if bindingMatchesAgent(existingBinding, agentConn) {
		g.sendBindingResponse(w, existingBinding.ID, agentConn.Name, existingBinding.WorkingDir, nil, http.StatusOK)
		return
	}

	reboundFrom := g.deleteExistingBinding(ctx, existingBinding)
	if reboundFrom == nil && existingBinding != nil {
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	bindingID, err := g.createBinding(ctx, req, agentConn)
	if err != nil {
		g.handleCreateBindingError(w, err)
		return
	}

	g.sendBindingResponse(w, bindingID, agentConn.Name, agentConn.WorkingDir, reboundFrom, http.StatusCreated)
}

// deleteExistingBinding deletes an existing binding and returns the old agent name.
// Returns nil if no binding exists or error occurred (error is logged).
func (g *Gateway) deleteExistingBinding(ctx context.Context, existing *store.Binding) *string {
	if existing == nil {
		return nil
	}
	oldAgentName := existing.AgentID
	if oldAgent := g.agentManager.GetByPrincipalAndWorkDir(existing.AgentID, existing.WorkingDir); oldAgent != nil {
		oldAgentName = oldAgent.Name
	}
	if err := g.store.DeleteBindingByID(ctx, existing.ID); err != nil {
		g.logger.Error("failed to delete existing binding", "error", err)
		return nil
	}
	return &oldAgentName
}

// createBinding creates a new binding in the store.
func (g *Gateway) createBinding(ctx context.Context, req *CreateBindingRequest, agentConn *agent.Connection) (string, error) {
	bindingID := uuid.New().String()
	binding := &store.Binding{
		ID:         bindingID,
		Frontend:   req.Frontend,
		ChannelID:  req.ChannelID,
		AgentID:    agentConn.PrincipalID,
		WorkingDir: agentConn.WorkingDir,
		CreatedAt:  time.Now(),
		CreatedBy:  nil,
	}
	return bindingID, g.store.CreateBindingV2(ctx, binding)
}

// sendBindingResponse writes a CreateBindingResponse as JSON.
func (g *Gateway) sendBindingResponse(w http.ResponseWriter, bindingID, agentName, workDir string, reboundFrom *string, status int) {
	response := CreateBindingResponse{
		BindingID:   bindingID,
		AgentName:   agentName,
		WorkingDir:  workDir,
		ReboundFrom: reboundFrom,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
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

	threadID, ok := extractPathSegment(r.URL.Path, "/api/threads/", "/messages")
	if !ok {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if _, err := uuid.Parse(threadID); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid thread_id format")
		return
	}

	limit, errMsg := parseLimitParam(r, 50, 1000)
	if errMsg != "" {
		g.sendJSONError(w, http.StatusBadRequest, errMsg)
		return
	}

	if _, err := g.store.GetThread(r.Context(), threadID); errors.Is(err, store.ErrNotFound) {
		g.sendJSONError(w, http.StatusNotFound, "thread not found")
		return
	} else if err != nil {
		g.logger.Error("failed to get thread", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	events, err := g.store.GetEventsByThreadID(r.Context(), threadID, limit)
	if err != nil {
		g.logger.Error("failed to get events", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response := ThreadMessagesResponse{ThreadID: threadID, Messages: make([]MessageResponse, len(events))}
	for i, evt := range events {
		response.Messages[i] = g.eventToMessageResponse(threadID, evt)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
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
// - POST /api/agents/{id}/send -> handleSendToAgent.
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
//
// Deprecated: use handleAgentRoutes which dispatches to handleAgentHistoryImpl.
func (g *Gateway) handleAgentHistory(w http.ResponseWriter, r *http.Request) {
	g.handleAgentRoutes(w, r)
}

// handleAgentHistoryImpl handles GET /api/agents/{id}/history requests.
// Returns conversation events for a specific agent with pagination and usage stats.
// Query params: limit (default 50, max 500), cursor (for pagination).
func (g *Gateway) handleAgentHistoryImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	agentID, ok := extractPathSegment(r.URL.Path, "/api/agents/", "/history")
	if !ok {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path or agent_id")
		return
	}

	limit, errMsg := parseLimitParam(r, 50, 500)
	if errMsg != "" {
		g.sendJSONError(w, http.StatusBadRequest, errMsg)
		return
	}

	result, err := g.store.GetEvents(r.Context(), store.GetEventsParams{
		ConversationKey: agentID,
		Limit:           limit,
		Cursor:          r.URL.Query().Get("cursor"),
	})
	if err != nil {
		g.logger.Error("failed to get events", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	events := make([]AgentHistoryEvent, len(result.Events))
	for i, evt := range result.Events {
		events[i] = eventToHistoryEvent(evt)
	}

	usage := g.fetchUsageStats(r.Context(), agentID)
	response := AgentHistoryResponse{
		AgentID:    agentID,
		Events:     events,
		Count:      len(events),
		HasMore:    result.HasMore,
		Usage:      usage,
		NextCursor: result.NextCursor,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Error("failed to encode agent history response", "error", err)
	}
}

// fetchUsageStats retrieves usage stats for an agent, returning empty stats on error.
func (g *Gateway) fetchUsageStats(ctx context.Context, agentID string) AgentHistoryUsage {
	usageStore, ok := g.store.(store.UsageStore)
	if !ok {
		return AgentHistoryUsage{}
	}
	stats, err := usageStore.GetUsageStats(ctx, store.UsageFilter{AgentID: &agentID})
	if err != nil {
		g.logger.Warn("failed to get usage stats", "error", err)
		return AgentHistoryUsage{}
	}
	return usageStatsToHistory(stats)
}

// handleSendToAgent handles POST /api/agents/{id}/send requests.
// Sends a message to a specific agent and streams the response via SSE.
func (g *Gateway) handleSendToAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	agentID, ok := extractPathSegment(r.URL.Path, "/api/agents/", "/send")
	if !ok {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path or agent_id")
		return
	}

	var req SendToAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Message == "" {
		g.sendJSONError(w, http.StatusBadRequest, "message is required")
		return
	}

	if _, ok := g.agentManager.GetAgent(agentID); !ok {
		g.sendJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		g.logger.Error("streaming not supported")
		g.sendJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	convResp, err := g.sendAgentMessage(r.Context(), agentID, req.Message)
	if err != nil {
		g.handleSendError(w, err)
		return
	}

	g.startSSEStream(r.Context(), w, flusher, convResp)
}

// sendAgentMessage creates and sends a message to an agent via ConversationService.
func (g *Gateway) sendAgentMessage(ctx context.Context, agentID, message string) (*conversation.SendResponse, error) {
	convReq := &conversation.SendRequest{
		ThreadID:     uuid.New().String(),
		FrontendName: "leader",
		ExternalID:   "leader:" + uuid.New().String(),
		AgentID:      agentID,
		Sender:       "coven-leader",
		Content:      message,
	}
	return g.conversation.SendMessage(ctx, convReq)
}

// handleSendError sends the appropriate error response for message send failures.
func (g *Gateway) handleSendError(w http.ResponseWriter, err error) {
	if errors.Is(err, agent.ErrAgentNotFound) {
		g.sendJSONError(w, http.StatusNotFound, "agent not found")
		return
	}
	g.logger.Error("failed to send message", "error", err)
	g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
}

// startSSEStream sets SSE headers and begins streaming responses.
func (g *Gateway) startSSEStream(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, convResp *conversation.SendResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	g.writeSSEEvent(w, "started", map[string]string{"thread_id": convResp.ThreadID})
	flusher.Flush()
	g.streamResponses(ctx, w, flusher, convResp.Stream)
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
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
}

// handleThreadUsage handles GET /api/threads/{id}/usage requests.
// Returns token usage records for a specific thread.
func (g *Gateway) handleThreadUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	threadID, ok := extractPathSegment(r.URL.Path, "/api/threads/", "/usage")
	if !ok {
		g.sendJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if _, err := uuid.Parse(threadID); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid thread_id format")
		return
	}

	usageStore, ok := g.store.(store.UsageStore)
	if !ok {
		g.logger.Error("store does not implement UsageStore")
		g.sendJSONError(w, http.StatusInternalServerError, "usage tracking not available")
		return
	}

	if errMsg := g.verifyThreadExists(r.Context(), threadID); errMsg != "" {
		g.sendJSONError(w, http.StatusNotFound, errMsg)
		return
	}

	usages, err := usageStore.GetThreadUsage(r.Context(), threadID)
	if err != nil {
		g.logger.Error("failed to get thread usage", "error", err)
		g.sendJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response := ThreadUsageResponse{ThreadID: threadID, Usage: usagesToResponse(usages)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
}

// verifyThreadExists checks if a thread exists and returns an error message if not.
func (g *Gateway) verifyThreadExists(ctx context.Context, threadID string) string {
	_, err := g.store.GetThread(ctx, threadID)
	if errors.Is(err, store.ErrNotFound) {
		return "thread not found"
	}
	if err != nil {
		g.logger.Error("failed to get thread", "error", err)
		return "internal server error"
	}
	return ""
}

// usagesToResponse converts usage records to response format.
func usagesToResponse(usages []*store.TokenUsage) []UsageResponse {
	result := make([]UsageResponse, len(usages))
	for i, u := range usages {
		result[i] = UsageResponse{
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
	return result
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
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"approved": req.Approved,
	}); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
}

// AnswerQuestionRequestBody is the JSON request for POST /api/questions/answer.
type AnswerQuestionRequestBody struct {
	AgentID    string   `json:"agent_id"`
	QuestionID string   `json:"question_id"`
	Selected   []string `json:"selected"`
	CustomText string   `json:"custom_text,omitempty"`
}

// validateAnswerQuestionRequest validates the answer question request.
func validateAnswerQuestionRequest(req *AnswerQuestionRequestBody) string {
	if req.AgentID == "" {
		return "agent_id is required"
	}
	if req.QuestionID == "" {
		return "question_id is required"
	}
	if len(req.Selected) == 0 && req.CustomText == "" {
		return "at least one selection or custom_text is required"
	}
	return ""
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
	if errMsg := validateAnswerQuestionRequest(&req); errMsg != "" {
		g.sendJSONError(w, http.StatusBadRequest, errMsg)
		return
	}
	if g.questionRouter == nil {
		g.sendJSONError(w, http.StatusServiceUnavailable, "question router not configured")
		return
	}

	answer := &pb.AnswerQuestionRequest{
		AgentId:    req.AgentID,
		QuestionId: req.QuestionID,
		Selected:   req.Selected,
	}
	if req.CustomText != "" {
		answer.CustomText = &req.CustomText
	}

	if err := g.questionRouter.DeliverAnswer(req.AgentID, req.QuestionID, answer); err != nil {
		g.logger.Error("failed to deliver question answer", "error", err)
		g.sendJSONError(w, http.StatusNotFound, "question not found or already answered")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
		g.logger.Debug("failed to encode response", "error", err)
	}
}
