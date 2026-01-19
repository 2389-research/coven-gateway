// ABOUTME: Tests for HTTP API handlers that expose agent messaging via SSE.
// ABOUTME: Verifies request handling, streaming responses, and error conditions.

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	// Verify new fields
	if agents[0].InstanceID != "inst-abc123" {
		t.Errorf("expected instance_id 'inst-abc123', got %s", agents[0].InstanceID)
	}
	if len(agents[0].Workspaces) != 2 || agents[0].Workspaces[0] != "Code" || agents[0].Workspaces[1] != "Personal" {
		t.Errorf("expected workspaces ['Code', 'Personal'], got %v", agents[0].Workspaces)
	}
	if agents[0].WorkingDir != "/projects/website" {
		t.Errorf("expected working_dir '/projects/website', got %s", agents[0].WorkingDir)
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

func TestHandleListAgents_WorkspaceFilter_Matches(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	// Filter by workspace that exists in the agent's workspaces
	req := httptest.NewRequest(http.MethodGet, "/api/agents?workspace=Code", nil)
	rec := httptest.NewRecorder()

	gw.handleListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var agents []AgentInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return the test agent since it has "Code" workspace
	if len(agents) != 1 {
		t.Errorf("expected 1 agent with 'Code' workspace, got %d", len(agents))
	}
	if len(agents) > 0 && agents[0].ID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got %s", agents[0].ID)
	}
}

func TestHandleListAgents_WorkspaceFilter_NoMatch(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	// Filter by workspace that doesn't exist
	req := httptest.NewRequest(http.MethodGet, "/api/agents?workspace=Nonexistent", nil)
	rec := httptest.NewRecorder()

	gw.handleListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var agents []AgentInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return no agents since none have "Nonexistent" workspace
	if len(agents) != 0 {
		t.Errorf("expected 0 agents with 'Nonexistent' workspace, got %d", len(agents))
	}
}

func TestHandleListAgents_WorkspaceFilter_Personal(t *testing.T) {
	gw := newTestGatewayWithMockManager(t)

	// Filter by "Personal" workspace (the second workspace in the mock)
	req := httptest.NewRequest(http.MethodGet, "/api/agents?workspace=Personal", nil)
	rec := httptest.NewRecorder()

	gw.handleListAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var agents []AgentInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return the test agent since it has "Personal" workspace
	if len(agents) != 1 {
		t.Errorf("expected 1 agent with 'Personal' workspace, got %d", len(agents))
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

	// Create a V2 binding for slack/C001 -> test-agent
	createTestBindingV2(t, gw, "slack", "C001", "test-agent")

	// Send message using frontend+channel_id (should resolve via binding)
	reqBody := SendMessageRequest{
		Sender:    "test-user",
		Content:   "Hello via binding",
		Frontend:  "slack",
		ChannelID: "C001",
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

	// Create a V2 binding for an agent that isn't connected
	createTestBindingV2(t, gw, "slack", "C001", "offline-agent")

	// Send message - binding exists but agent is offline
	reqBody := SendMessageRequest{
		Sender:    "test-user",
		Content:   "Hello",
		Frontend:  "slack",
		ChannelID: "C001",
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	return gw
}

// createTestBindingV2 creates a V2 binding in the store for testing.
// It first creates a principal for the agent (required by V2 bindings).
func createTestBindingV2(t *testing.T, gw *Gateway, frontend, channelID, agentID string) {
	t.Helper()

	ctx := context.Background()

	// Type assert to SQLiteStore to access V2 methods
	sqlStore, ok := gw.store.(*store.SQLiteStore)
	if !ok {
		t.Fatalf("store is not *SQLiteStore")
	}

	// Create a principal for the agent first (required by V2 bindings validation)
	principal := &store.Principal{
		ID:          agentID,
		Type:        store.PrincipalTypeAgent,
		DisplayName: agentID,
		Status:      store.PrincipalStatusOnline,
		CreatedAt:   time.Now(),
	}
	if err := sqlStore.CreatePrincipal(ctx, principal); err != nil {
		// Ignore duplicate error - principal might already exist
		if !strings.Contains(err.Error(), "UNIQUE constraint") {
			t.Fatalf("failed to create principal: %v", err)
		}
	}

	// Now create the V2 binding
	binding := &store.Binding{
		ID:        "test-binding-" + frontend + "-" + channelID,
		Frontend:  frontend,
		ChannelID: channelID,
		AgentID:   agentID,
		CreatedAt: time.Now(),
	}
	if err := sqlStore.CreateBindingV2(ctx, binding); err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}
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
	return []*agent.AgentInfo{{
		ID:           "test-agent",
		InstanceID:   "inst-abc123",
		Name:         "Test",
		Capabilities: []string{"chat", "code"},
		Workspaces:   []string{"Code", "Personal"},
		WorkingDir:   "/projects/website",
	}}
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

	// Register a test agent with the real agent manager so GetAgent and GetByPrincipalAndWorkDir succeed
	// PrincipalID must match what the binding stores (the binding's AgentID)
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:           "test-agent",
		Name:         "Test",
		PrincipalID:  "test-agent", // Must match binding's AgentID for GetByPrincipalAndWorkDir
		WorkingDir:   "",           // Empty to match bindings with no working_dir
		Capabilities: []string{"chat", "code"},
		Stream:       &testMockStream{},
		Logger:       logger,
	})
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
	gw := newTestGatewayWithAgentForBinding(t, "inst-crud", "/test/path", "agent-crud")

	// Create a binding using instance_id
	createReq := `{"frontend":"slack","channel_id":"C001","instance_id":"inst-crud"}`
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
			body:    `{"channel_id":"C001","instance_id":"test-instance"}`,
			wantErr: "frontend, channel_id, and instance_id are required",
		},
		{
			name:    "missing channel_id",
			body:    `{"frontend":"slack","instance_id":"test-instance"}`,
			wantErr: "frontend, channel_id, and instance_id are required",
		},
		{
			name:    "missing instance_id",
			body:    `{"frontend":"slack","channel_id":"C001"}`,
			wantErr: "frontend, channel_id, and instance_id are required",
		},
		{
			name:    "empty body",
			body:    `{}`,
			wantErr: "frontend, channel_id, and instance_id are required",
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
	gw := newTestGatewayWithAgentForBinding(t, "inst-dup", "/test/path", "agent-dup")

	// Create first binding using instance_id
	createReq := `{"frontend":"slack","channel_id":"C001","instance_id":"inst-dup"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Try to create duplicate binding - should return 200 (same agent noop)
	req = httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	// Since it's the same agent, it returns 200 (idempotent)
	if w.Code != http.StatusOK {
		t.Errorf("duplicate create with same agent: got status %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
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

	// Setup existing V2 binding (the resolver now uses GetBindingByChannel which reads V2 bindings)
	s.AddBindingV2(ctx, &store.Binding{
		ID:        "binding-1",
		Frontend:  "test",
		ChannelID: "channel-1",
		AgentID:   "agent-1",
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

	// Setup existing V2 binding but no thread yet
	s.AddBindingV2(ctx, &store.Binding{
		ID:        "binding-1",
		Frontend:  "test",
		ChannelID: "channel-1",
		AgentID:   "agent-1",
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

	// Setup existing V2 binding but no thread
	s.AddBindingV2(ctx, &store.Binding{
		ID:        "binding-1",
		Frontend:  "test",
		ChannelID: "channel-1",
		AgentID:   "agent-1",
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

// newTestGatewayWithAgentForBinding creates a gateway with a registered agent
// that has the specified instance_id, working_dir, and principal_id for binding tests.
func newTestGatewayWithAgentForBinding(t *testing.T, instanceID, workingDir, principalID string) *Gateway {
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	// Register an agent with the specified parameters
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:           principalID,
		Name:         "test-agent",
		Capabilities: []string{"chat", "code"},
		PrincipalID:  principalID,
		Workspaces:   []string{"Work"},
		WorkingDir:   workingDir,
		InstanceID:   instanceID,
		Stream:       &testMockStream{},
		Logger:       logger,
	})
	if err := gw.agentManager.Register(conn); err != nil {
		t.Fatalf("failed to register test agent: %v", err)
	}

	// Create the principal in the store so the binding can be created
	sqlStore, ok := gw.store.(*store.SQLiteStore)
	if !ok {
		t.Fatalf("store is not *SQLiteStore")
	}
	// Generate a unique pubkey fingerprint based on principal ID
	pubkeyFP := fmt.Sprintf("%064s", principalID)[:64]
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    pubkeyFP,
		DisplayName: "test-agent",
		Status:      store.PrincipalStatusOnline,
		CreatedAt:   time.Now(),
	}
	if err := sqlStore.CreatePrincipal(context.Background(), principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	return gw
}

// Tests for POST /api/bindings with instance_id

func TestCreateBindingByInstanceID_Success(t *testing.T) {
	gw := newTestGatewayWithAgentForBinding(t, "0fb8187d-c06", "/projects/website", "agent-uuid-123")

	reqBody := `{"frontend":"matrix","channel_id":"!roomid:server","instance_id":"0fb8187d-c06"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp CreateBindingResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.BindingID == "" {
		t.Error("expected non-empty binding_id")
	}
	if resp.AgentName != "test-agent" {
		t.Errorf("expected agent_name 'test-agent', got %q", resp.AgentName)
	}
	if resp.WorkingDir != "/projects/website" {
		t.Errorf("expected working_dir '/projects/website', got %q", resp.WorkingDir)
	}
	if resp.ReboundFrom != nil {
		t.Errorf("expected rebound_from to be nil, got %v", resp.ReboundFrom)
	}
}

func TestCreateBindingByInstanceID_NotFound(t *testing.T) {
	gw := newTestGateway(t) // No agents registered

	reqBody := `{"frontend":"matrix","channel_id":"!roomid:server","instance_id":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	gw.handleBindings(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusNotFound, w.Code, w.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	expectedErr := "no agent online with instance_id 'nonexistent'"
	if errResp["error"] != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, errResp["error"])
	}
}

func TestCreateBindingByInstanceID_MissingFields(t *testing.T) {
	gw := newTestGateway(t)

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing frontend",
			body:    `{"channel_id":"C001","instance_id":"abc"}`,
			wantErr: "frontend, channel_id, and instance_id are required",
		},
		{
			name:    "missing channel_id",
			body:    `{"frontend":"slack","instance_id":"abc"}`,
			wantErr: "frontend, channel_id, and instance_id are required",
		},
		{
			name:    "missing instance_id",
			body:    `{"frontend":"slack","channel_id":"C001"}`,
			wantErr: "frontend, channel_id, and instance_id are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			gw.handleBindings(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d. Body: %s", w.Code, http.StatusBadRequest, w.Body.String())
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

func TestCreateBindingByInstanceID_Rebind(t *testing.T) {
	// Create gateway with two agents
	cfg := &config.Config{
		Server: config.ServerConfig{
			GRPCAddr: "localhost:0",
			HTTPAddr: "localhost:0",
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	// Create first agent
	conn1 := agent.NewConnection(agent.ConnectionParams{
		ID:          "agent-1",
		Name:        "first-agent",
		PrincipalID: "agent-1",
		WorkingDir:  "/old/path",
		InstanceID:  "inst-001",
		Stream:      &testMockStream{},
		Logger:      logger,
	})
	if err := gw.agentManager.Register(conn1); err != nil {
		t.Fatalf("failed to register first agent: %v", err)
	}

	// Create second agent
	conn2 := agent.NewConnection(agent.ConnectionParams{
		ID:          "agent-2",
		Name:        "second-agent",
		PrincipalID: "agent-2",
		WorkingDir:  "/new/path",
		InstanceID:  "inst-002",
		Stream:      &testMockStream{},
		Logger:      logger,
	})
	if err := gw.agentManager.Register(conn2); err != nil {
		t.Fatalf("failed to register second agent: %v", err)
	}

	// Create principals in store (each needs unique PubkeyFP)
	sqlStore := gw.store.(*store.SQLiteStore)
	for _, p := range []struct{ id, name, fp string }{
		{"agent-1", "first-agent", "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"},
		{"agent-2", "second-agent", "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"},
	} {
		principal := &store.Principal{
			ID:          p.id,
			Type:        store.PrincipalTypeAgent,
			PubkeyFP:    p.fp,
			DisplayName: p.name,
			Status:      store.PrincipalStatusOnline,
			CreatedAt:   time.Now(),
		}
		if err := sqlStore.CreatePrincipal(context.Background(), principal); err != nil {
			t.Fatalf("failed to create principal: %v", err)
		}
	}

	// Create initial binding with first agent
	reqBody := `{"frontend":"matrix","channel_id":"!room:server","instance_id":"inst-001"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first binding: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Rebind to second agent
	reqBody = `{"frontend":"matrix","channel_id":"!room:server","instance_id":"inst-002"}`
	req = httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("rebind: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp CreateBindingResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.AgentName != "second-agent" {
		t.Errorf("expected agent_name 'second-agent', got %q", resp.AgentName)
	}
	if resp.WorkingDir != "/new/path" {
		t.Errorf("expected working_dir '/new/path', got %q", resp.WorkingDir)
	}
	if resp.ReboundFrom == nil {
		t.Fatal("expected rebound_from to be set")
	}
	if *resp.ReboundFrom != "first-agent" {
		t.Errorf("expected rebound_from 'first-agent', got %q", *resp.ReboundFrom)
	}
}

func TestCreateBindingByInstanceID_SameAgentNoop(t *testing.T) {
	gw := newTestGatewayWithAgentForBinding(t, "inst-abc", "/projects/same", "agent-uuid")

	// Create initial binding
	reqBody := `{"frontend":"matrix","channel_id":"!room:server","instance_id":"inst-abc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first binding: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Try to bind same channel to same agent again
	req = httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	// Should return 200 (idempotent) with existing binding info
	if w.Code != http.StatusOK {
		t.Fatalf("same binding: got status %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp CreateBindingResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ReboundFrom != nil {
		t.Errorf("expected rebound_from to be nil for same agent, got %v", resp.ReboundFrom)
	}
}

// TestGetBindingStatus tests GET /api/bindings?frontend=X&channel_id=Y for a single binding.
func TestGetBindingStatus(t *testing.T) {
	gw := newTestGatewayWithAgentForBinding(t, "inst-status", "/projects/website", "agent-status")

	// Create a binding first
	reqBody := `{"frontend":"matrix","channel_id":"!room:server","instance_id":"inst-status"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create binding: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Get binding status by frontend + channel_id
	req = httptest.NewRequest(http.MethodGet, "/api/bindings?frontend=matrix&channel_id=!room:server", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get binding status: got status %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify expected fields
	if resp["binding_id"] == nil || resp["binding_id"] == "" {
		t.Error("expected non-empty binding_id")
	}
	if resp["agent_name"] != "test-agent" {
		t.Errorf("expected agent_name 'test-agent', got %v", resp["agent_name"])
	}
	if resp["working_dir"] != "/projects/website" {
		t.Errorf("expected working_dir '/projects/website', got %v", resp["working_dir"])
	}
	if resp["online"] != true {
		t.Errorf("expected online=true, got %v", resp["online"])
	}
}

// TestGetBindingStatus_NotFound tests GET with frontend+channel_id for non-existent binding.
func TestGetBindingStatus_NotFound(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/bindings?frontend=matrix&channel_id=!nonexistent:server", nil)
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusNotFound, w.Code, w.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp["error"] != "no binding for this channel" {
		t.Errorf("expected error 'no binding for this channel', got %q", errResp["error"])
	}
}

// TestGetBindingStatus_AgentOffline tests GET binding when agent is offline.
func TestGetBindingStatus_AgentOffline(t *testing.T) {
	gw := newTestGateway(t)

	// Create a binding directly in store without a connected agent
	createTestBindingV2(t, gw, "matrix", "!offline:server", "offline-agent")

	req := httptest.NewRequest(http.MethodGet, "/api/bindings?frontend=matrix&channel_id=!offline:server", nil)
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["online"] != false {
		t.Errorf("expected online=false for offline agent, got %v", resp["online"])
	}
}

// TestListBindings_IncludesWorkingDir tests that list response includes working_dir.
func TestListBindings_IncludesWorkingDir(t *testing.T) {
	gw := newTestGatewayWithAgentForBinding(t, "inst-list", "/projects/listtest", "agent-list")

	// Create a binding
	reqBody := `{"frontend":"slack","channel_id":"C123","instance_id":"inst-list"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create binding: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// List all bindings (no query params)
	req = httptest.NewRequest(http.MethodGet, "/api/bindings", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list bindings: got status %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ListBindingsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(resp.Bindings))
	}

	if resp.Bindings[0].WorkingDir != "/projects/listtest" {
		t.Errorf("expected working_dir '/projects/listtest', got %q", resp.Bindings[0].WorkingDir)
	}
}
