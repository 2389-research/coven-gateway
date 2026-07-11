// ABOUTME: Tests for api.go HTTP handler gaps: answer-question, tool-approval,
// ABOUTME: send-to-agent+SSE, thread messages/usage, usage stats, agent routes, and binding errors.

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/config"
	"github.com/2389/coven-gateway/internal/store"
)

// =============================================================================
// validateAnswerQuestionRequest
// =============================================================================

func TestValidateAnswerQuestionRequest_Valid(t *testing.T) {
	req := &AnswerQuestionRequestBody{
		AgentID:    "agent-1",
		QuestionID: "q-1",
		Selected:   []string{"yes"},
	}
	if msg := validateAnswerQuestionRequest(req); msg != "" {
		t.Errorf("expected no error, got %q", msg)
	}
}

func TestValidateAnswerQuestionRequest_WithCustomText(t *testing.T) {
	req := &AnswerQuestionRequestBody{
		AgentID:    "agent-1",
		QuestionID: "q-1",
		CustomText: "my answer",
	}
	if msg := validateAnswerQuestionRequest(req); msg != "" {
		t.Errorf("expected no error for custom_text only, got %q", msg)
	}
}

func TestValidateAnswerQuestionRequest_MissingAgentID(t *testing.T) {
	req := &AnswerQuestionRequestBody{QuestionID: "q-1", Selected: []string{"a"}}
	if msg := validateAnswerQuestionRequest(req); msg != "agent_id is required" {
		t.Errorf("expected 'agent_id is required', got %q", msg)
	}
}

func TestValidateAnswerQuestionRequest_MissingQuestionID(t *testing.T) {
	req := &AnswerQuestionRequestBody{AgentID: "agent-1", Selected: []string{"a"}}
	if msg := validateAnswerQuestionRequest(req); msg != "question_id is required" {
		t.Errorf("expected 'question_id is required', got %q", msg)
	}
}

func TestValidateAnswerQuestionRequest_MissingSelectionAndCustomText(t *testing.T) {
	req := &AnswerQuestionRequestBody{AgentID: "agent-1", QuestionID: "q-1"}
	msg := validateAnswerQuestionRequest(req)
	if !strings.Contains(msg, "selection") && !strings.Contains(msg, "custom_text") {
		t.Errorf("expected selection/custom_text error, got %q", msg)
	}
}

// =============================================================================
// handleAnswerQuestion
// =============================================================================

func TestHandleAnswerQuestion_MethodNotAllowed(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/questions/answer", nil)
	rec := httptest.NewRecorder()
	gw.handleAnswerQuestion(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAnswerQuestion_InvalidJSON(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/questions/answer", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	gw.handleAnswerQuestion(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAnswerQuestion_ValidationFails(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"question_id":"q-1","selected":["a"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/questions/answer", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleAnswerQuestion(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] != "agent_id is required" {
		t.Errorf("error = %q, want 'agent_id is required'", errResp["error"])
	}
}

func TestHandleAnswerQuestion_NoQuestionRouter(t *testing.T) {
	gw := newTestGateway(t)
	// Force questionRouter to nil to exercise the nil-check branch
	gw.questionRouter = nil
	body := `{"agent_id":"a","question_id":"q","selected":["x"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/questions/answer", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleAnswerQuestion(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// =============================================================================
// handleToolApproval
// =============================================================================

func TestHandleToolApproval_MethodNotAllowed(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tools/approve", nil)
	rec := httptest.NewRecorder()
	gw.handleToolApproval(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleToolApproval_InvalidJSON(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/tools/approve", strings.NewReader("bad"))
	rec := httptest.NewRecorder()
	gw.handleToolApproval(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleToolApproval_MissingAgentID(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"tool_id":"t-1","approved":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/approve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleToolApproval(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] != "agent_id is required" {
		t.Errorf("error = %q, want 'agent_id is required'", errResp["error"])
	}
}

func TestHandleToolApproval_MissingToolID(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"agent_id":"agent-1","approved":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/approve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleToolApproval(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleToolApproval_AgentNotFound(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"agent_id":"nonexistent","tool_id":"t-1","approved":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/approve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleToolApproval(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] != "agent not found" {
		t.Errorf("error = %q, want 'agent not found'", errResp["error"])
	}
}

// =============================================================================
// handleSendError
// =============================================================================

func TestHandleSendError_AgentNotFound(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()
	gw.handleSendError(rec, agent.ErrAgentNotFound)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSendError_GenericError(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()
	gw.handleSendError(rec, errors.New("something broke"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// handleSendToAgent
// =============================================================================

func TestHandleSendToAgent_MethodNotAllowed(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/send", nil)
	rec := httptest.NewRecorder()
	gw.handleSendToAgent(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleSendToAgent_InvalidPath(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents//send", nil)
	rec := httptest.NewRecorder()
	gw.handleSendToAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSendToAgent_InvalidJSON(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/send", strings.NewReader("bad"))
	rec := httptest.NewRecorder()
	gw.handleSendToAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSendToAgent_MissingMessage(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"message":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/send", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleSendToAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSendToAgent_AgentNotFound(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/nonexistent/send", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleSendToAgent(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] != "agent not found" {
		t.Errorf("error = %q, want 'agent not found'", errResp["error"])
	}
}

// TestHandleSendToAgent_SSEStream exercises the SSE streaming path for sendToAgent.
// It registers a real agent, posts a message, and uses a context timeout to end streaming.
func TestHandleSendToAgent_SSEStream(t *testing.T) {
	gw := newTestGatewayWithMockManagerAndStore(t)

	body := `{"message":"hello from test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/send", strings.NewReader(body))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	gw.handleSendToAgent(rec, req)

	// Should have set SSE headers
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

// =============================================================================
// streamResponses + startSSEStream (via mockAgentManager)
// =============================================================================

// TestStreamResponses_TextAndDone exercises the text+done response path.
func TestStreamResponses_TextAndDone(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()

	respChan := make(chan *agent.Response, 3)
	respChan <- &agent.Response{Event: agent.EventThinking, Text: "thinking..."}
	respChan <- &agent.Response{Event: agent.EventText, Text: "hello"}
	respChan <- &agent.Response{Event: agent.EventDone, Text: "hello", Done: true}
	close(respChan)

	ctx := context.Background()
	gw.streamResponses(ctx, rec, rec, respChan)

	body := rec.Body.String()
	if !strings.Contains(body, "event: thinking") {
		t.Errorf("expected 'event: thinking' in body, got: %q", body)
	}
	if !strings.Contains(body, "event: text") {
		t.Errorf("expected 'event: text' in body, got: %q", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected 'event: done' in body, got: %q", body)
	}
}

// TestStreamResponses_ContextCanceled exercises the context-cancel error branch.
func TestStreamResponses_ContextCanceled(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()

	// Provide a pre-canceled context so the cancel branch fires immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	respChan := make(chan *agent.Response) // never sends; cancel fires first
	gw.streamResponses(ctx, rec, rec, respChan)

	body := rec.Body.String()
	if !strings.Contains(body, "error") {
		t.Errorf("expected error event on cancel, got: %q", body)
	}
}

// =============================================================================
// handleAgentRoutes
// =============================================================================

func TestHandleAgentRoutes_InvalidPrefix(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/other/path", nil)
	rec := httptest.NewRecorder()
	gw.handleAgentRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAgentRoutes_InvalidSuffix(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/unknown", nil)
	rec := httptest.NewRecorder()
	gw.handleAgentRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAgentRoutes_HistoryDispatch(t *testing.T) {
	gw := newTestGatewayWithMockManagerAndStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/history", nil)
	rec := httptest.NewRecorder()
	gw.handleAgentRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleAgentRoutes_SendDispatch_AgentNotFound(t *testing.T) {
	gw := newTestGateway(t)
	body := `{"message":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/ghost/send", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleAgentRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// =============================================================================
// handleThreadMessages
// =============================================================================

func TestHandleThreadMessages_MethodNotAllowed(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/threads/abc/messages", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadMessages(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleThreadMessages_InvalidPath(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/threads//messages", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadMessages(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleThreadMessages_InvalidUUID(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/threads/not-a-uuid/messages", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadMessages(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleThreadMessages_InvalidLimit(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/threads/550e8400-e29b-41d4-a716-446655440000/messages?limit=bad", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadMessages(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleThreadMessages_ThreadNotFound(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/threads/550e8400-e29b-41d4-a716-446655440000/messages", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadMessages(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleThreadMessages_WithEvents(t *testing.T) {
	gw := newTestGateway(t)
	ctx := context.Background()

	// Seed a thread and events
	sqlStore := gw.store.(*store.SQLiteStore)
	threadID := "550e8400-e29b-41d4-a716-446655440001"
	thread := &store.Thread{
		ID:           threadID,
		FrontendName: "test",
		ExternalID:   "ext-1",
		AgentID:      "agent-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := sqlStore.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	text := "hello from test"
	event := &store.LedgerEvent{
		ID:              "evt-1",
		ConversationKey: "agent-1",
		ThreadID:        &threadID,
		Direction:       store.EventDirectionInbound,
		Author:          "user@test.com",
		Timestamp:       time.Now(),
		Type:            store.EventTypeMessage,
		Text:            &text,
	}
	if err := sqlStore.SaveEvent(ctx, event); err != nil {
		t.Fatalf("save event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/threads/"+threadID+"/messages", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp ThreadMessagesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ThreadID != threadID {
		t.Errorf("ThreadID = %q, want %q", resp.ThreadID, threadID)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(resp.Messages))
	}
	// eventToMessageResponse check: verify at least one field is set
	if resp.Messages[0].ID == "" {
		t.Error("expected non-empty message ID")
	}
}

// =============================================================================
// eventToMessageResponse
// =============================================================================

func TestEventToMessageResponse_Basic(t *testing.T) {
	gw := newTestGateway(t)

	content := "hello"
	threadID := "test-thread"
	evt := &store.LedgerEvent{
		ID:        "event-123",
		ThreadID:  &threadID,
		Direction: store.EventDirectionInbound,
		Author:    "user@test.com",
		Timestamp: time.Now(),
		Type:      store.EventTypeMessage,
		Text:      &content,
	}

	msg := gw.eventToMessageResponse(threadID, evt)

	if msg.ThreadID != threadID {
		t.Errorf("ThreadID = %q, want %q", msg.ThreadID, threadID)
	}
	if msg.Content != content {
		t.Errorf("Content = %q, want %q", msg.Content, content)
	}
	if msg.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
}

// =============================================================================
// handleThreadUsage
// =============================================================================

func TestHandleThreadUsage_MethodNotAllowedPost(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/threads/abc/usage", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadUsage(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleThreadUsage_EmptySegment(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/threads//usage", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadUsage(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleThreadUsage_WithUsageRecords(t *testing.T) {
	gw := newTestGateway(t)
	ctx := context.Background()

	sqlStore := gw.store.(*store.SQLiteStore)
	threadID := "550e8400-e29b-41d4-a716-446655440002"
	thread := &store.Thread{
		ID:           threadID,
		FrontendName: "test",
		ExternalID:   "ext-usage",
		AgentID:      "agent-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := sqlStore.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Add a usage record using the correct method name
	usage := &store.TokenUsage{
		ID:          "usage-1",
		ThreadID:    threadID,
		RequestID:   "req-1",
		AgentID:     "agent-1",
		InputTokens: 10,
		CreatedAt:   time.Now(),
	}
	if err := sqlStore.SaveUsage(ctx, usage); err != nil {
		t.Fatalf("save usage: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/threads/"+threadID+"/usage", nil)
	rec := httptest.NewRecorder()
	gw.handleThreadUsage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp ThreadUsageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ThreadID != threadID {
		t.Errorf("ThreadID = %q, want %q", resp.ThreadID, threadID)
	}
	if len(resp.Usage) != 1 {
		t.Errorf("usage count = %d, want 1", len(resp.Usage))
	}
}

// =============================================================================
// handleUsageStats filter branches
// =============================================================================

func TestHandleUsageStats_MethodNotAllowedPost(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/stats/usage", nil)
	rec := httptest.NewRecorder()
	gw.handleUsageStats(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleUsageStats_InvalidSince(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/stats/usage?since=not-a-date", nil)
	rec := httptest.NewRecorder()
	gw.handleUsageStats(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleUsageStats_InvalidUntil(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/stats/usage?until=bad", nil)
	rec := httptest.NewRecorder()
	gw.handleUsageStats(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleUsageStats_WithAgentFilter(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/stats/usage?agent_id=agent-x", nil)
	rec := httptest.NewRecorder()
	gw.handleUsageStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleUsageStats_WithSinceAndUntil(t *testing.T) {
	gw := newTestGateway(t)
	since := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	until := time.Now().Format(time.RFC3339)
	url := fmt.Sprintf("/api/stats/usage?since=%s&until=%s", since, until)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	gw.handleUsageStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// =============================================================================
// fetchUsageStats
// =============================================================================

func TestFetchUsageStats_HappyPath(t *testing.T) {
	gw := newTestGateway(t)
	// With a real SQLiteStore that implements UsageStore, should return empty stats
	usage := gw.fetchUsageStats(context.Background(), "agent-xyz")
	// Should not panic; stats should be zero-valued
	if usage.TotalTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", usage.TotalTokens)
	}
}

// =============================================================================
// verifyThreadExists
// =============================================================================

func TestVerifyThreadExists_NotFound(t *testing.T) {
	gw := newTestGateway(t)
	msg := gw.verifyThreadExists(context.Background(), "nonexistent-id")
	if msg != "thread not found" {
		t.Errorf("msg = %q, want 'thread not found'", msg)
	}
}

func TestVerifyThreadExists_Found(t *testing.T) {
	gw := newTestGateway(t)
	ctx := context.Background()

	threadID := "550e8400-e29b-41d4-a716-446655440003"
	sqlStore := gw.store.(*store.SQLiteStore)
	thread := &store.Thread{
		ID:           threadID,
		FrontendName: "test",
		ExternalID:   "ext-verify",
		AgentID:      "agent-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := sqlStore.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	msg := gw.verifyThreadExists(ctx, threadID)
	if msg != "" {
		t.Errorf("expected empty msg for existing thread, got %q", msg)
	}
}

// =============================================================================
// handleCreateBindingError
// =============================================================================

func TestHandleCreateBindingError_DuplicateChannel(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()
	gw.handleCreateBindingError(rec, store.ErrDuplicateChannel)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] != "binding already exists" {
		t.Errorf("error = %q, want 'binding already exists'", errResp["error"])
	}
}

func TestHandleCreateBindingError_GenericError(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()
	gw.handleCreateBindingError(rec, errors.New("database error"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// =============================================================================
// newTestGatewayWithBroadcastableAgent: gateway with an agent that can respond to sends
// =============================================================================

// newTestGatewayWithRespondingAgent creates a gateway with an agent whose stream
// sends a text+done response sequence. Used for SSE streaming path coverage.
func newTestGatewayWithRespondingAgent(t *testing.T) *Gateway {
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

	// Use the mockSender so handleSendMessage exercises the SSE path
	respChan := make(chan *agent.Response, 2)
	respChan <- &agent.Response{Event: agent.EventText, Text: "hi"}
	respChan <- &agent.Response{Event: agent.EventDone, Text: "hi", Done: true}
	close(respChan)

	gw.mockSender = &mockAgentManager{respChan: respChan}

	return gw
}

// TestStreamResponses_ClosedChannelWithoutDone ensures the loop exits cleanly
// when the channel closes without a Done event.
func TestStreamResponses_ClosedChannelWithoutDone(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()

	respChan := make(chan *agent.Response, 1)
	respChan <- &agent.Response{Event: agent.EventText, Text: "partial"}
	close(respChan)

	ctx := context.Background()
	gw.streamResponses(ctx, rec, rec, respChan)

	body := rec.Body.String()
	if !strings.Contains(body, "event: text") {
		t.Errorf("expected 'event: text' in body, got: %q", body)
	}
}

// TestHandleAgentHistoryImpl_BadPath exercises the path extraction failure branch.
func TestHandleAgentHistoryImpl_BadPath(t *testing.T) {
	gw := newTestGateway(t)
	// Embedded slash in agent ID makes extractPathSegment fail
	req := httptest.NewRequest(http.MethodGet, "/api/agents/bad/path/history", nil)
	rec := httptest.NewRecorder()
	gw.handleAgentHistoryImpl(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleAgentHistoryImpl_BadLimit exercises the limit parse failure branch.
func TestHandleAgentHistoryImpl_BadLimit(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/history?limit=bad", nil)
	rec := httptest.NewRecorder()
	gw.handleAgentHistoryImpl(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleSendMessage_StreamResponses exercises streamResponses via a real mock-manager flow.
// Uses newTestGatewayWithMockManager which registers "test-agent" in the agentManager
// and sets a mockSender that serves a pre-filled response channel.
func TestHandleSendMessage_StreamResponses(t *testing.T) {
	// newTestGatewayWithMockManager registers "test-agent" in the real agentManager so
	// resolveTarget's GetAgent check passes, and installs a mockSender with a pre-filled
	// channel so the conversation service bypasses the gRPC stream.
	gw := newTestGatewayWithMockManager(t)

	body, _ := json.Marshal(SendMessageRequest{
		AgentID: "test-agent",
		Sender:  "user@test.com",
		Content: "Hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	gw.handleSendMessage(rec, req)

	output := rec.Body.String()
	// At minimum SSE headers should be set (streaming started)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	// Body should contain at least a started event
	if !strings.Contains(output, "event:") {
		t.Errorf("expected SSE events in output, got: %q", output)
	}
}

// =============================================================================
// handleToolApproval — success and other-error arms
// =============================================================================

func TestHandleToolApproval_Success(t *testing.T) {
	// Need a real registered agent with a stream so Send works
	gw := newTestGatewayWithAgentForBinding(t, "inst-tapr", "/path", "agent-tapr")

	body := `{"agent_id":"agent-tapr","tool_id":"tool-xyz","approved":true,"approve_all":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/approve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleToolApproval(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
	if resp["approved"] != true {
		t.Errorf("approved = %v, want true", resp["approved"])
	}
}

// =============================================================================
// handleAnswerQuestion — DeliverAnswer error arm
// =============================================================================

func TestHandleAnswerQuestion_QuestionNotFound(t *testing.T) {
	gw := newTestGateway(t)
	// questionRouter is set by New(); deliver to an unknown question ID
	body := `{"agent_id":"a-1","question_id":"q-unknown","selected":["yes"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/questions/answer", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.handleAnswerQuestion(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] != "question not found or already answered" {
		t.Errorf("error = %q, want 'question not found or already answered'", errResp["error"])
	}
}
