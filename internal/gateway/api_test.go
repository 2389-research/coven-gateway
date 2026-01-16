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
)

func TestHandleSendMessage_NoAgents(t *testing.T) {
	gw := newTestGateway(t)

	reqBody := SendMessageRequest{
		Sender:  "test-user",
		Content: "Hello",
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
	if errResp["error"] != "no agents available" {
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
	return m.respChan, nil
}

func (m *mockAgentManager) ListAgents() []*agent.AgentInfo {
	return []*agent.AgentInfo{{ID: "test-agent", Name: "Test", Capabilities: []string{}}}
}

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

	logger := slog.New(slog.NewTextHandler(nil, nil))

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
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
