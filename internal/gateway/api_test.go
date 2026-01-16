// ABOUTME: Tests for HTTP API handlers that expose agent messaging via SSE.
// ABOUTME: Verifies request handling, streaming responses, and error conditions.

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/config"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestHandleSendMessage_NoAgents(t *testing.T) {
	gw := newTestGateway(t)

	// With agent_id specified but agent not available, should get "agent unavailable"
	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "Hello",
		AgentID: "some-agent",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	// Check error response
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "agent unavailable" {
		t.Errorf("unexpected error message: %s", errResp["error"])
	}
}

func TestHandleSendMessage_MissingAgentContext(t *testing.T) {
	gw := newTestGateway(t)

	// Without agent_id and without frontend+channel_id, should get validation error
	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	// Check error response
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "must specify agent_id or frontend+channel_id" {
		t.Errorf("unexpected error message: %s", errResp["error"])
	}
}

func TestHandleSendMessage_InvalidJSON(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleSendMessage_EmptyContent(t *testing.T) {
	gw := newTestGateway(t)

	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleSendMessage_MethodNotAllowed(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/send", nil)
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleSendMessage_SSEHeaders(t *testing.T) {
	// Test that SSE headers are set correctly when streaming starts
	// This test uses a mock agent manager to verify the streaming behavior
	gw := newTestGatewayWithMockManager(t)

	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "Hello",
		AgentID: "test-agent",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Use a context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	// Verify SSE headers are set
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Connection") != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %s", rec.Header().Get("Connection"))
	}
}

func TestHandleListAgents_Empty(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()

	gw.handleListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var agents []AgentInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestHandleListAgents_WithAgents(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()

	gw.handleListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
	}

	var agents []AgentInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	if agents[0].ID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got %s", agents[0].ID)
	}
	if agents[0].Name != "Test" {
		t.Errorf("expected agent name 'Test', got %s", agents[0].Name)
	}
	if len(agents[0].Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(agents[0].Capabilities))
	}
}

func TestHandleListAgents_MethodNotAllowed(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest(http.MethodPost, "/api/agents", nil)
	rec := httptest.NewRecorder()

	gw.handleListAgents(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleSendMessage_WithAgentID(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "Hello",
		AgentID: "test-agent",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	// Should succeed and set SSE headers
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestHandleSendMessage_AgentNotFound(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	// Requesting a nonexistent agent should return 503 "agent unavailable"
	// since we now check agent availability before routing
	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "Hello",
		AgentID: "nonexistent-agent",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "agent unavailable" {
		t.Errorf("unexpected error message: %s", errResp["error"])
	}
}

func TestHandleSendMessage_BindingLookup(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	// Create a binding for slack/C001 -> test-agent
	createReq := `{"frontend":"slack","channel_id":"C001","agent_id":"test-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create binding: %s", w.Body.String())
	}

	// Send message using frontend+channel_id (should resolve via binding)
	reqBody := SendMessageRequest{
		Sender:    "test-user",
		Content:   "Hello via binding",
		Frontend:  "slack",
		ChannelID: "C001",
	}
	body, _ := json.Marshal(reqBody)

	req = httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	gw.handleSendMessage(rec, req)

	// Should succeed and set SSE headers
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestHandleSendMessage_BindingNotFound(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	// Send message for unbound channel
	reqBody := SendMessageRequest{
		Sender:    "test-user",
		Content:   "Hello",
		Frontend:  "slack",
		ChannelID: "UNBOUND",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "channel not bound to agent" {
		t.Errorf("unexpected error message: %s", errResp["error"])
	}
}

func TestHandleSendMessage_BoundAgentOffline(t *testing.T) {
	gw := newTestGateway(t)

	// Create a binding for a nonexistent agent
	createReq := `{"frontend":"slack","channel_id":"C001","agent_id":"offline-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create binding: %s", w.Body.String())
	}

	// Send message - binding exists but agent is offline
	reqBody := SendMessageRequest{
		Sender:    "test-user",
		Content:   "Hello",
		Frontend:  "slack",
		ChannelID: "C001",
	}
	body, _ := json.Marshal(reqBody)

	req = httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.handleSendMessage(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "agent unavailable" {
		t.Errorf("unexpected error message: %s", errResp["error"])
	}
}

func newTestGateway(t *testing.T) *Gateway {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			GRPCAddr: "localhost:0",
			HTTPAddr: "localhost:0",
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}

	logger := slog.New(slog.NewTextHandler(nil, nil))

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	return gw
}

// mockAgentManager is a test double that provides a controllable response channel.
type mockAgentManager struct {
	respChan chan *agent.Response
}

func (m *mockAgentManager) SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error) {
	// If a specific agent is requested and it doesn't exist, return error
	if req.AgentID != "" && req.AgentID != "test-agent" {
		return nil, agent.ErrAgentNotFound
	}
	return m.respChan, nil
}

func (m *mockAgentManager) ListAgents() []*agent.AgentInfo {
	return []*agent.AgentInfo{{ID: "test-agent", Name: "Test", Capabilities: []string{"chat", "code"}}}
}

// testMockStream implements pb.FoldControl_AgentStreamServer for testing.
type testMockStream struct {
	grpc.ServerStream
}

func (m *testMockStream) Send(msg *pb.ServerMessage) error { return nil }
func (m *testMockStream) Recv() (*pb.AgentMessage, error)  { return nil, nil }

func newTestGatewayWithMockManager(t *testing.T) *Gateway {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			GRPCAddr: "localhost:0",
			HTTPAddr: "localhost:0",
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}

	logger := slog.Default()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	// Register a test agent with the real agent manager so GetAgent succeeds
	conn := agent.NewConnection("test-agent", "Test", []string{"chat", "code"}, &testMockStream{}, logger)
	if err := gw.agentManager.Register(conn); err != nil {
		t.Fatalf("failed to register test agent: %v", err)
	}

	// Replace with mock manager that returns a controllable channel
	respChan := make(chan *agent.Response)
	go func() {
		time.Sleep(10 * time.Millisecond)
		respChan <- &agent.Response{
			Event: agent.EventText,
			Text:  "Hello!",
		}
		respChan <- &agent.Response{
			Event: agent.EventDone,
			Text:  "Hello!",
			Done:  true,
		}
		close(respChan)
	}()

	gw.mockSender = &mockAgentManager{respChan: respChan}

	return gw
}

func TestBindingsCRUD(t *testing.T) {
	gw := newTestGateway(t)

	// Create a binding
	createReq := `{"frontend":"slack","channel_id":"C001","agent_id":"test-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("create binding: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// List bindings
	req = httptest.NewRequest(http.MethodGet, "/api/bindings", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("list bindings: got status %d, want %d", w.Code, http.StatusOK)
	}

	var listResp ListBindingsResponse
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(listResp.Bindings) != 1 {
		t.Errorf("got %d bindings, want 1", len(listResp.Bindings))
	}

	// Delete binding
	req = httptest.NewRequest(http.MethodDelete, "/api/bindings?frontend=slack&channel_id=C001", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("delete binding: got status %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/bindings", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(listResp.Bindings) != 0 {
		t.Errorf("got %d bindings after delete, want 0", len(listResp.Bindings))
	}
}

func TestBindingsValidation(t *testing.T) {
	gw := newTestGateway(t)

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing frontend",
			body:    `{"channel_id":"C001","agent_id":"test-agent"}`,
			wantErr: "frontend, channel_id, and agent_id are required",
		},
		{
			name:    "missing channel_id",
			body:    `{"frontend":"slack","agent_id":"test-agent"}`,
			wantErr: "frontend, channel_id, and agent_id are required",
		},
		{
			name:    "missing agent_id",
			body:    `{"frontend":"slack","channel_id":"C001"}`,
			wantErr: "frontend, channel_id, and agent_id are required",
		},
		{
			name:    "empty body",
			body:    `{}`,
			wantErr: "frontend, channel_id, and agent_id are required",
		},
		{
			name:    "invalid json",
			body:    `not json`,
			wantErr: "invalid JSON body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			gw.handleBindings(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
			}

			var errResp map[string]string
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("decode error response: %v", err)
			}

			if errResp["error"] != tt.wantErr {
				t.Errorf("got error %q, want %q", errResp["error"], tt.wantErr)
			}
		})
	}
}

func TestBindingsDuplicate(t *testing.T) {
	gw := newTestGateway(t)

	// Create first binding
	createReq := `{"frontend":"slack","channel_id":"C001","agent_id":"test-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Try to create duplicate binding
	req = httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("duplicate create: got status %d, want %d. Body: %s", w.Code, http.StatusConflict, w.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if errResp["error"] != "binding already exists" {
		t.Errorf("got error %q, want %q", errResp["error"], "binding already exists")
	}
}

func TestBindingsNotFound(t *testing.T) {
	gw := newTestGateway(t)

	// Try to delete non-existent binding
	req := httptest.NewRequest(http.MethodDelete, "/api/bindings?frontend=slack&channel_id=NONEXISTENT", nil)
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("delete non-existent: got status %d, want %d. Body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if errResp["error"] != "binding not found" {
		t.Errorf("got error %q, want %q", errResp["error"], "binding not found")
	}
}

// Tests for parseSendRequest

func TestParseSendRequest_Valid(t *testing.T) {
	body := `{"content": "hello", "sender": "user@test.com"}`
	req, err := parseSendRequest(strings.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", req.Content)
	}
	if req.Sender != "user@test.com" {
		t.Errorf("expected sender 'user@test.com', got %q", req.Sender)
	}
}

func TestParseSendRequest_AllFields(t *testing.T) {
	body := `{"content": "hello", "sender": "user@test.com", "thread_id": "abc123", "agent_id": "agent1", "frontend": "slack", "channel_id": "C001"}`
	req, err := parseSendRequest(strings.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", req.Content)
	}
	if req.Sender != "user@test.com" {
		t.Errorf("expected sender 'user@test.com', got %q", req.Sender)
	}
	if req.ThreadID != "abc123" {
		t.Errorf("expected thread_id 'abc123', got %q", req.ThreadID)
	}
	if req.AgentID != "agent1" {
		t.Errorf("expected agent_id 'agent1', got %q", req.AgentID)
	}
	if req.Frontend != "slack" {
		t.Errorf("expected frontend 'slack', got %q", req.Frontend)
	}
	if req.ChannelID != "C001" {
		t.Errorf("expected channel_id 'C001', got %q", req.ChannelID)
	}
}

func TestParseSendRequest_MissingContent(t *testing.T) {
	body := `{"sender": "user@test.com"}`
	_, err := parseSendRequest(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected error for missing content")
	}
	if err.Error() != "content is required" {
		t.Errorf("expected error 'content is required', got %q", err.Error())
	}
}

func TestParseSendRequest_EmptyContent(t *testing.T) {
	body := `{"content": "", "sender": "user@test.com"}`
	_, err := parseSendRequest(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if err.Error() != "content is required" {
		t.Errorf("expected error 'content is required', got %q", err.Error())
	}
}

func TestParseSendRequest_MissingSender(t *testing.T) {
	body := `{"content": "hello"}`
	_, err := parseSendRequest(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected error for missing sender")
	}
	if err.Error() != "sender is required" {
		t.Errorf("expected error 'sender is required', got %q", err.Error())
	}
}

func TestParseSendRequest_EmptySender(t *testing.T) {
	body := `{"content": "hello", "sender": ""}`
	_, err := parseSendRequest(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected error for empty sender")
	}
	if err.Error() != "sender is required" {
		t.Errorf("expected error 'sender is required', got %q", err.Error())
	}
}

func TestParseSendRequest_InvalidJSON(t *testing.T) {
	body := `not valid json`
	_, err := parseSendRequest(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if err.Error() != "invalid JSON body" {
		t.Errorf("expected error 'invalid JSON body', got %q", err.Error())
	}
}

// Tests for bindingResolver

func TestResolveBinding_ExistingBinding(t *testing.T) {
	s := store.NewMockStore()
	ctx := context.Background()

	// Setup existing thread
	s.CreateThread(ctx, &store.Thread{ID: "existing-thread", FrontendName: "test", ExternalID: "channel-1", AgentID: "agent-1"})

	// Setup existing binding
	s.CreateBinding(ctx, &store.ChannelBinding{
		FrontendName: "test",
		ChannelID:    "channel-1",
		AgentID:      "agent-1",
	})

	resolver := &bindingResolver{store: s}
	result, err := resolver.Resolve(ctx, "test", "channel-1", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ThreadID != "existing-thread" {
		t.Errorf("expected thread ID 'existing-thread', got %q", result.ThreadID)
	}
	if result.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got %q", result.AgentID)
	}
}

func TestResolveBinding_NoBinding(t *testing.T) {
	s := store.NewMockStore()
	ctx := context.Background()

	resolver := &bindingResolver{store: s}
	_, err := resolver.Resolve(ctx, "test", "unbound-channel", "")

	if err == nil {
		t.Fatal("expected error for unbound channel")
	}
	if err != ErrChannelNotBound {
		t.Errorf("expected ErrChannelNotBound, got %v", err)
	}
}

func TestResolveBinding_WithProvidedThreadID(t *testing.T) {
	s := store.NewMockStore()
	ctx := context.Background()

	// Setup existing binding but no thread yet
	s.CreateBinding(ctx, &store.ChannelBinding{
		FrontendName: "test",
		ChannelID:    "channel-1",
		AgentID:      "agent-1",
	})

	resolver := &bindingResolver{store: s}
	result, err := resolver.Resolve(ctx, "test", "channel-1", "provided-thread-id")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use the provided thread ID
	if result.ThreadID != "provided-thread-id" {
		t.Errorf("expected thread ID 'provided-thread-id', got %q", result.ThreadID)
	}
	if result.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got %q", result.AgentID)
	}
}

// Tests for formatSSEEvent

func TestFormatSSEEvent(t *testing.T) {
	event := formatSSEEvent("text", `{"content": "hello"}`)
	assert.Equal(t, "event: text\ndata: {\"content\": \"hello\"}\n\n", event)
}

func TestResolveBinding_ExistingBindingNoThread(t *testing.T) {
	s := store.NewMockStore()
	ctx := context.Background()

	// Setup existing binding but no thread
	s.CreateBinding(ctx, &store.ChannelBinding{
		FrontendName: "test",
		ChannelID:    "channel-1",
		AgentID:      "agent-1",
	})

	resolver := &bindingResolver{store: s}
	result, err := resolver.Resolve(ctx, "test", "channel-1", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should generate a new thread ID (UUID format)
	if result.ThreadID == "" {
		t.Error("expected non-empty thread ID")
	}
	if result.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got %q", result.AgentID)
	}
}
