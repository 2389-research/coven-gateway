// ABOUTME: Tests for base pack tool handlers.
// ABOUTME: Uses real SQLite store for integration testing.

package builtins

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
)

func TestLogEntry(t *testing.T) {
	s := newTestStore(t)
	pack := BasePack(s)

	handler := findHandler(pack, "log_entry")
	if handler == nil {
		t.Fatal("log_entry handler not found")
	}

	input := `{"message": "test log", "tags": ["test"]}`
	result, err := handler(context.Background(), "agent-1", json.RawMessage(input))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if resp["status"] != "logged" {
		t.Errorf("unexpected status: %s", resp["status"])
	}
	if resp["id"] == "" {
		t.Error("expected id in response")
	}
}

func TestTodoCRUD(t *testing.T) {
	s := newTestStore(t)
	pack := BasePack(s)

	// Add
	addHandler := findHandler(pack, "todo_add")
	result, err := addHandler(context.Background(), "agent-1", json.RawMessage(`{"description": "test todo"}`))
	if err != nil {
		t.Fatalf("todo_add: %v", err)
	}
	var addResp map[string]string
	json.Unmarshal(result, &addResp)
	todoID := addResp["id"]

	// List
	listHandler := findHandler(pack, "todo_list")
	result, err = listHandler(context.Background(), "agent-1", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("todo_list: %v", err)
	}
	var listResp map[string]any
	json.Unmarshal(result, &listResp)
	if listResp["count"].(float64) != 1 {
		t.Errorf("expected 1 todo, got %v", listResp["count"])
	}

	// Update
	updateHandler := findHandler(pack, "todo_update")
	_, err = updateHandler(context.Background(), "agent-1", json.RawMessage(`{"id": "`+todoID+`", "status": "completed"}`))
	if err != nil {
		t.Fatalf("todo_update: %v", err)
	}

	// Delete
	deleteHandler := findHandler(pack, "todo_delete")
	_, err = deleteHandler(context.Background(), "agent-1", json.RawMessage(`{"id": "`+todoID+`"}`))
	if err != nil {
		t.Fatalf("todo_delete: %v", err)
	}
}

func TestTodoOwnershipVerification(t *testing.T) {
	s := newTestStore(t)
	pack := BasePack(s)

	// Agent 1 creates a todo
	addHandler := findHandler(pack, "todo_add")
	result, err := addHandler(context.Background(), "agent-1", json.RawMessage(`{"description": "agent-1's todo"}`))
	if err != nil {
		t.Fatalf("todo_add: %v", err)
	}
	var addResp map[string]string
	json.Unmarshal(result, &addResp)
	todoID := addResp["id"]

	// Agent 2 should not be able to update agent-1's todo
	updateHandler := findHandler(pack, "todo_update")
	_, err = updateHandler(context.Background(), "agent-2", json.RawMessage(`{"id": "`+todoID+`", "status": "completed"}`))
	if err == nil {
		t.Error("expected error when agent-2 tries to update agent-1's todo")
	}

	// Agent 2 should not be able to delete agent-1's todo
	deleteHandler := findHandler(pack, "todo_delete")
	_, err = deleteHandler(context.Background(), "agent-2", json.RawMessage(`{"id": "`+todoID+`"}`))
	if err == nil {
		t.Error("expected error when agent-2 tries to delete agent-1's todo")
	}

	// Agent 1 should still be able to update their own todo
	_, err = updateHandler(context.Background(), "agent-1", json.RawMessage(`{"id": "`+todoID+`", "status": "completed"}`))
	if err != nil {
		t.Fatalf("agent-1 should be able to update own todo: %v", err)
	}

	// Agent 1 should still be able to delete their own todo
	_, err = deleteHandler(context.Background(), "agent-1", json.RawMessage(`{"id": "`+todoID+`"}`))
	if err != nil {
		t.Fatalf("agent-1 should be able to delete own todo: %v", err)
	}
}

func TestBBS(t *testing.T) {
	s := newTestStore(t)
	pack := BasePack(s)

	// Create thread
	createHandler := findHandler(pack, "bbs_create_thread")
	result, err := createHandler(context.Background(), "agent-1", json.RawMessage(`{"subject": "Hello", "content": "World"}`))
	if err != nil {
		t.Fatalf("bbs_create_thread: %v", err)
	}
	var createResp map[string]string
	json.Unmarshal(result, &createResp)
	threadID := createResp["thread_id"]

	// Reply
	replyHandler := findHandler(pack, "bbs_reply")
	_, err = replyHandler(context.Background(), "agent-2", json.RawMessage(`{"thread_id": "`+threadID+`", "content": "Nice!"}`))
	if err != nil {
		t.Fatalf("bbs_reply: %v", err)
	}

	// Read thread
	readHandler := findHandler(pack, "bbs_read_thread")
	result, err = readHandler(context.Background(), "agent-1", json.RawMessage(`{"thread_id": "`+threadID+`"}`))
	if err != nil {
		t.Fatalf("bbs_read_thread: %v", err)
	}
	var thread store.BBSThread
	if err := json.Unmarshal(result, &thread); err != nil {
		t.Fatalf("unmarshal thread: %v", err)
	}
	if len(thread.Replies) != 1 {
		t.Errorf("expected 1 reply, got %d", len(thread.Replies))
	}
}

func findHandler(pack *packs.BuiltinPack, name string) packs.ToolHandler {
	for _, tool := range pack.Tools {
		if tool.Definition.GetName() == name {
			return tool.Handler
		}
	}
	return nil
}

func newTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
