// ABOUTME: Tests for notes pack tool handlers.
// ABOUTME: Uses real SQLite store for integration testing.

package builtins

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNoteSet(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	handler := findHandler(pack, "note_set")
	if handler == nil {
		t.Fatal("note_set handler not found")
	}

	input := `{"key": "mykey", "value": "myvalue"}`
	result, err := handler(context.Background(), "agent-1", json.RawMessage(input))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if resp["status"] != "saved" {
		t.Errorf("unexpected status: %s", resp["status"])
	}
	if resp["key"] != "mykey" {
		t.Errorf("unexpected key: %s", resp["key"])
	}
}

func TestNoteGet(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	// First set a note
	setHandler := findHandler(pack, "note_set")
	_, err := setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "testkey", "value": "testvalue"}`))
	if err != nil {
		t.Fatalf("note_set: %v", err)
	}

	// Then get it
	getHandler := findHandler(pack, "note_get")
	result, err := getHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "testkey"}`))
	if err != nil {
		t.Fatalf("note_get: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if resp["key"] != "testkey" {
		t.Errorf("unexpected key: %s", resp["key"])
	}
	if resp["value"] != "testvalue" {
		t.Errorf("unexpected value: %s", resp["value"])
	}
}

func TestNoteGetNotFound(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	getHandler := findHandler(pack, "note_get")
	_, err := getHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "nonexistent"}`))
	if err == nil {
		t.Error("expected error for nonexistent note")
	}
}

func TestNoteList(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	setHandler := findHandler(pack, "note_set")
	listHandler := findHandler(pack, "note_list")

	// List with no notes
	result, err := listHandler(context.Background(), "agent-1", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("note_list (empty): %v", err)
	}
	var resp map[string]any
	json.Unmarshal(result, &resp)
	if resp["count"].(float64) != 0 {
		t.Errorf("expected 0 notes, got %v", resp["count"])
	}

	// Add some notes
	_, err = setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "key1", "value": "val1"}`))
	if err != nil {
		t.Fatalf("note_set key1: %v", err)
	}
	_, err = setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "key2", "value": "val2"}`))
	if err != nil {
		t.Fatalf("note_set key2: %v", err)
	}

	// List again
	result, err = listHandler(context.Background(), "agent-1", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("note_list: %v", err)
	}
	json.Unmarshal(result, &resp)
	if resp["count"].(float64) != 2 {
		t.Errorf("expected 2 notes, got %v", resp["count"])
	}

	keys := resp["keys"].([]any)
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestNoteDelete(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	setHandler := findHandler(pack, "note_set")
	getHandler := findHandler(pack, "note_get")
	deleteHandler := findHandler(pack, "note_delete")

	// Set a note
	_, err := setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "todelete", "value": "value"}`))
	if err != nil {
		t.Fatalf("note_set: %v", err)
	}

	// Delete it
	result, err := deleteHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "todelete"}`))
	if err != nil {
		t.Fatalf("note_delete: %v", err)
	}

	var resp map[string]string
	json.Unmarshal(result, &resp)
	if resp["status"] != "deleted" {
		t.Errorf("unexpected status: %s", resp["status"])
	}
	if resp["key"] != "todelete" {
		t.Errorf("unexpected key: %s", resp["key"])
	}

	// Verify it's gone
	_, err = getHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "todelete"}`))
	if err == nil {
		t.Error("expected error after delete, note still exists")
	}
}

func TestNoteSetUpsert(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	setHandler := findHandler(pack, "note_set")
	getHandler := findHandler(pack, "note_get")

	// Set initial value
	_, err := setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "upsertkey", "value": "initial"}`))
	if err != nil {
		t.Fatalf("note_set initial: %v", err)
	}

	// Update the value (upsert)
	_, err = setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "upsertkey", "value": "updated"}`))
	if err != nil {
		t.Fatalf("note_set upsert: %v", err)
	}

	// Verify updated value
	result, err := getHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "upsertkey"}`))
	if err != nil {
		t.Fatalf("note_get: %v", err)
	}

	var resp map[string]string
	json.Unmarshal(result, &resp)
	if resp["value"] != "updated" {
		t.Errorf("expected updated value, got: %s", resp["value"])
	}
}

func TestNoteIsolationBetweenAgents(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	setHandler := findHandler(pack, "note_set")
	getHandler := findHandler(pack, "note_get")
	listHandler := findHandler(pack, "note_list")

	// Agent 1 sets a note
	_, err := setHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "secret", "value": "agent1secret"}`))
	if err != nil {
		t.Fatalf("agent-1 note_set: %v", err)
	}

	// Agent 2 sets a note with the same key
	_, err = setHandler(context.Background(), "agent-2", json.RawMessage(`{"key": "secret", "value": "agent2secret"}`))
	if err != nil {
		t.Fatalf("agent-2 note_set: %v", err)
	}

	// Agent 1 should only see their own note
	result, err := getHandler(context.Background(), "agent-1", json.RawMessage(`{"key": "secret"}`))
	if err != nil {
		t.Fatalf("agent-1 note_get: %v", err)
	}
	var resp map[string]string
	json.Unmarshal(result, &resp)
	if resp["value"] != "agent1secret" {
		t.Errorf("agent-1 got wrong value: %s", resp["value"])
	}

	// Agent 2 should only see their own note
	result, err = getHandler(context.Background(), "agent-2", json.RawMessage(`{"key": "secret"}`))
	if err != nil {
		t.Fatalf("agent-2 note_get: %v", err)
	}
	json.Unmarshal(result, &resp)
	if resp["value"] != "agent2secret" {
		t.Errorf("agent-2 got wrong value: %s", resp["value"])
	}

	// Each agent's list should only show their own keys
	result, err = listHandler(context.Background(), "agent-1", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("agent-1 note_list: %v", err)
	}
	var listResp map[string]any
	json.Unmarshal(result, &listResp)
	if listResp["count"].(float64) != 1 {
		t.Errorf("agent-1 should have 1 note, got %v", listResp["count"])
	}

	result, err = listHandler(context.Background(), "agent-2", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("agent-2 note_list: %v", err)
	}
	json.Unmarshal(result, &listResp)
	if listResp["count"].(float64) != 1 {
		t.Errorf("agent-2 should have 1 note, got %v", listResp["count"])
	}
}

func TestNotesPackDefinitions(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	if pack.ID != "builtin:notes" {
		t.Errorf("unexpected pack ID: %s", pack.ID)
	}

	expectedTools := []string{"note_set", "note_get", "note_list", "note_delete"}
	if len(pack.Tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(pack.Tools))
	}

	for _, expected := range expectedTools {
		handler := findHandler(pack, expected)
		if handler == nil {
			t.Errorf("missing tool: %s", expected)
		}
	}

	// Verify all tools require the "notes" capability
	for _, tool := range pack.Tools {
		caps := tool.Definition.GetRequiredCapabilities()
		if len(caps) != 1 || caps[0] != "notes" {
			t.Errorf("tool %s should require [notes] capability, got %v", tool.Definition.GetName(), caps)
		}
	}
}

func TestNoteSetInvalidInput(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	handler := findHandler(pack, "note_set")

	_, err := handler(context.Background(), "agent-1", json.RawMessage(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNoteGetInvalidInput(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	handler := findHandler(pack, "note_get")

	_, err := handler(context.Background(), "agent-1", json.RawMessage(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNoteDeleteInvalidInput(t *testing.T) {
	s := newTestStore(t)
	pack := NotesPack(s)

	handler := findHandler(pack, "note_delete")

	_, err := handler(context.Background(), "agent-1", json.RawMessage(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
