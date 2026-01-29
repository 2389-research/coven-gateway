// ABOUTME: Tests for admin pack tool handlers.
// ABOUTME: Uses real Manager and SQLite store for integration testing.

package builtins

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/store"
)

func TestAdminListAgents(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s)

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

func TestAdminAgentHistory_RequiresAgentID(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s)

	handler := findHandler(pack, "admin_agent_history")
	if handler == nil {
		t.Fatal("admin_agent_history handler not found")
	}

	_, err := handler(context.Background(), "admin-agent", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error when agent_id is missing")
	}
}

func TestAdminAgentHistory_UnknownAgent(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s)

	handler := findHandler(pack, "admin_agent_history")
	if handler == nil {
		t.Fatal("admin_agent_history handler not found")
	}

	result, err := handler(context.Background(), "admin-agent", json.RawMessage(`{"agent_id": "unknown-agent"}`))
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
}

func TestAdminAgentHistory_WithMessages(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s)

	handler := findHandler(pack, "admin_agent_history")
	if handler == nil {
		t.Fatal("admin_agent_history handler not found")
	}

	ctx := context.Background()

	// Create a thread for an agent
	thread := &store.Thread{
		ID:           "thread-1",
		FrontendName: "test",
		ExternalID:   "ext-1",
		AgentID:      "test-agent",
	}
	if err := s.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Add a message to the thread
	msg := &store.Message{
		ID:       "msg-1",
		ThreadID: "thread-1",
		Sender:   "user",
		Content:  "Hello agent",
		Type:     "message",
	}
	if err := s.SaveMessage(ctx, msg); err != nil {
		t.Fatalf("save message: %v", err)
	}

	// Query history for the agent
	result, err := handler(ctx, "admin-agent", json.RawMessage(`{"agent_id": "test-agent"}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp["count"].(float64) != 1 {
		t.Errorf("expected count 1, got %v", resp["count"])
	}

	messages := resp["messages"].([]any)
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}

func TestAdminSendMessage(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s)

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
	pack := AdminPack(mgr, s)

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
		expectedTools := []string{"admin_list_agents", "admin_agent_history", "admin_send_message"}
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
