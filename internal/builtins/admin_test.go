// ABOUTME: Tests for admin pack tool handlers.
// ABOUTME: Uses real Manager and SQLite store for integration testing.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/store"
)

func TestAdminListAgents(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_list_agents")
	if handler == nil {
		t.Fatal("admin_list_agents handler not found")
	}

	t.Run("returns empty list when no agents", func(t *testing.T) {
		result, err := handler(context.Background(), "admin-agent", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}

		var resp map[string]any
		if err := json.Unmarshal(result, &resp); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}

		if resp["count"].(float64) != 0 {
			t.Errorf("expected count 0, got %v", resp["count"])
		}

		agents := resp["agents"].([]any)
		if len(agents) != 0 {
			t.Errorf("expected empty agents list, got %d", len(agents))
		}
	})
}

func TestAdminAgentMessages_RequiresAgentID(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	_, err := handler(context.Background(), "admin-agent", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error when agent_id is missing")
	}
}

func TestAdminAgentMessages_EmptyHistory(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	result, err := handler(context.Background(), "admin-agent", json.RawMessage(`{"agent_id": "no-events-agent"}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp["count"].(float64) != 0 {
		t.Errorf("expected count 0, got %v", resp["count"])
	}
	if resp["has_more"].(bool) != false {
		t.Errorf("expected has_more false, got %v", resp["has_more"])
	}

	usage := resp["usage"].(map[string]any)
	if usage["total_tokens"].(float64) != 0 {
		t.Errorf("expected total_tokens 0, got %v", usage["total_tokens"])
	}
	if usage["request_count"].(float64) != 0 {
		t.Errorf("expected request_count 0, got %v", usage["request_count"])
	}
}

func TestAdminAgentMessages_WithEvents(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	ctx := context.Background()
	agentID := "test-agent"
	threadID := "thread-1"

	// Create a thread so the FK constraint on message_usage is satisfied
	thread := &store.Thread{
		ID:           threadID,
		FrontendName: "test",
		ExternalID:   "ext-1",
		AgentID:      agentID,
	}
	if err := s.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Create two events: inbound then outbound
	inboundText := "Hello agent"
	evt1 := &store.LedgerEvent{
		ID:              "evt-1",
		ConversationKey: agentID,
		ThreadID:        &threadID,
		Direction:       store.EventDirectionInbound,
		Author:          "user",
		Timestamp:       time.Now().Add(-2 * time.Second),
		Type:            store.EventTypeMessage,
		Text:            &inboundText,
	}
	if err := s.SaveEvent(ctx, evt1); err != nil {
		t.Fatalf("save event 1: %v", err)
	}

	outboundText := "Hello user"
	evt2 := &store.LedgerEvent{
		ID:              "evt-2",
		ConversationKey: agentID,
		ThreadID:        &threadID,
		Direction:       store.EventDirectionOutbound,
		Author:          agentID,
		Timestamp:       time.Now().Add(-1 * time.Second),
		Type:            store.EventTypeMessage,
		Text:            &outboundText,
	}
	if err := s.SaveEvent(ctx, evt2); err != nil {
		t.Fatalf("save event 2: %v", err)
	}

	// Save usage data for this agent
	usage := &store.TokenUsage{
		ID:           "usage-1",
		ThreadID:     threadID,
		RequestID:    "req-1",
		AgentID:      agentID,
		InputTokens:  100,
		OutputTokens: 50,
	}
	if err := s.SaveUsage(ctx, usage); err != nil {
		t.Fatalf("save usage: %v", err)
	}

	// Query messages for the agent
	result, err := handler(ctx, "admin-agent", json.RawMessage(`{"agent_id": "test-agent"}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Verify count
	if resp["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}

	// Verify chronological order (evt-1 before evt-2)
	events := resp["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	first := events[0].(map[string]any)
	second := events[1].(map[string]any)

	if first["id"] != "evt-1" {
		t.Errorf("expected first event id 'evt-1', got %v", first["id"])
	}
	if second["id"] != "evt-2" {
		t.Errorf("expected second event id 'evt-2', got %v", second["id"])
	}

	// Verify event fields
	if first["direction"] != string(store.EventDirectionInbound) {
		t.Errorf("expected inbound direction, got %v", first["direction"])
	}
	if first["author"] != "user" {
		t.Errorf("expected author 'user', got %v", first["author"])
	}
	if first["text"] != inboundText {
		t.Errorf("expected text %q, got %v", inboundText, first["text"])
	}
	if first["thread_id"] != threadID {
		t.Errorf("expected thread_id %q, got %v", threadID, first["thread_id"])
	}

	// Verify usage stats are present
	usageStats := resp["usage"].(map[string]any)
	if usageStats["total_input"].(float64) != 100 {
		t.Errorf("expected total_input 100, got %v", usageStats["total_input"])
	}
	if usageStats["total_output"].(float64) != 50 {
		t.Errorf("expected total_output 50, got %v", usageStats["total_output"])
	}
	if usageStats["request_count"].(float64) != 1 {
		t.Errorf("expected request_count 1, got %v", usageStats["request_count"])
	}
}

func TestAdminAgentMessages_Pagination(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	ctx := context.Background()
	agentID := "paginated-agent"

	// Create 3 events
	for i := range 3 {
		text := fmt.Sprintf("message %d", i)
		evt := &store.LedgerEvent{
			ID:              fmt.Sprintf("evt-%d", i),
			ConversationKey: agentID,
			Direction:       store.EventDirectionInbound,
			Author:          "user",
			Timestamp:       time.Now().Add(time.Duration(i) * time.Second),
			Type:            store.EventTypeMessage,
			Text:            &text,
		}
		if err := s.SaveEvent(ctx, evt); err != nil {
			t.Fatalf("save event %d: %v", i, err)
		}
	}

	// First page: limit=2
	result, err := handler(ctx, "admin-agent", json.RawMessage(`{"agent_id": "paginated-agent", "limit": 2}`))
	if err != nil {
		t.Fatalf("handler error (page 1): %v", err)
	}

	var page1 map[string]any
	if err := json.Unmarshal(result, &page1); err != nil {
		t.Fatalf("unmarshal page 1: %v", err)
	}

	if page1["count"].(float64) != 2 {
		t.Errorf("page 1: expected count 2, got %v", page1["count"])
	}
	if page1["has_more"].(bool) != true {
		t.Errorf("page 1: expected has_more true, got %v", page1["has_more"])
	}
	cursor, ok := page1["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatal("page 1: expected non-empty next_cursor")
	}

	// Second page: use cursor
	result, err = handler(ctx, "admin-agent", json.RawMessage(fmt.Sprintf(`{"agent_id": "paginated-agent", "limit": 2, "cursor": %q}`, cursor)))
	if err != nil {
		t.Fatalf("handler error (page 2): %v", err)
	}

	var page2 map[string]any
	if err := json.Unmarshal(result, &page2); err != nil {
		t.Fatalf("unmarshal page 2: %v", err)
	}

	if page2["count"].(float64) != 1 {
		t.Errorf("page 2: expected count 1, got %v", page2["count"])
	}
	if page2["has_more"].(bool) != false {
		t.Errorf("page 2: expected has_more false, got %v", page2["has_more"])
	}
}

func TestAdminSendMessage(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_send_message")
	if handler == nil {
		t.Fatal("admin_send_message handler not found")
	}

	t.Run("requires agent_id", func(t *testing.T) {
		_, err := handler(context.Background(), "admin-agent", json.RawMessage(`{"content": "hello"}`))
		if err == nil {
			t.Error("expected error when agent_id is missing")
		}
	})

	t.Run("requires content", func(t *testing.T) {
		_, err := handler(context.Background(), "admin-agent", json.RawMessage(`{"agent_id": "test-agent"}`))
		if err == nil {
			t.Error("expected error when content is missing")
		}
	})

	t.Run("returns error for unknown agent", func(t *testing.T) {
		_, err := handler(context.Background(), "admin-agent", json.RawMessage(`{"agent_id": "unknown-agent", "content": "hello"}`))
		if err == nil {
			t.Error("expected error when agent is not found")
		}
	})
}

func TestAdminPackToolDefinitions(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	t.Run("pack has correct ID", func(t *testing.T) {
		if pack.ID != "builtin:admin" {
			t.Errorf("expected pack ID 'builtin:admin', got %q", pack.ID)
		}
	})

	t.Run("all tools require admin capability", func(t *testing.T) {
		for _, tool := range pack.Tools {
			caps := tool.Definition.GetRequiredCapabilities()
			if len(caps) != 1 || caps[0] != "admin" {
				t.Errorf("tool %s should require only 'admin' capability, got %v", tool.Definition.GetName(), caps)
			}
		}
	})

	t.Run("has expected tools", func(t *testing.T) {
		expectedTools := []string{"admin_list_agents", "admin_agent_messages", "admin_send_message"}
		toolNames := make(map[string]bool)
		for _, tool := range pack.Tools {
			toolNames[tool.Definition.GetName()] = true
		}

		for _, name := range expectedTools {
			if !toolNames[name] {
				t.Errorf("missing expected tool: %s", name)
			}
		}
	})
}
