// ABOUTME: Tests for SQLite store implementation
// ABOUTME: Covers thread CRUD, message persistence, and message ordering/limiting

package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSQLiteStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	// Verify the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNewSQLiteStore_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "nested", "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	// Verify the database file was created in the nested directory
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created in nested directory")
	}
}

func TestCreateAndGetThread(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	thread := &Thread{
		ID:           "thread-123",
		FrontendName: "slack",
		ExternalID:   "C12345678",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := store.CreateThread(ctx, thread); err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}

	got, err := store.GetThread(ctx, "thread-123")
	if err != nil {
		t.Fatalf("GetThread failed: %v", err)
	}

	if got.ID != thread.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, thread.ID)
	}
	if got.FrontendName != thread.FrontendName {
		t.Errorf("FrontendName mismatch: got %q, want %q", got.FrontendName, thread.FrontendName)
	}
	if got.ExternalID != thread.ExternalID {
		t.Errorf("ExternalID mismatch: got %q, want %q", got.ExternalID, thread.ExternalID)
	}
	if got.AgentID != thread.AgentID {
		t.Errorf("AgentID mismatch: got %q, want %q", got.AgentID, thread.AgentID)
	}
	if !got.CreatedAt.Equal(thread.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", got.CreatedAt, thread.CreatedAt)
	}
	if !got.UpdatedAt.Equal(thread.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: got %v, want %v", got.UpdatedAt, thread.UpdatedAt)
	}
}

func TestGetThread_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	_, err := store.GetThread(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateThread(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	thread := &Thread{
		ID:           "thread-456",
		FrontendName: "matrix",
		ExternalID:   "!room:matrix.org",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := store.CreateThread(ctx, thread); err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}

	// Update the thread
	thread.AgentID = "agent-002"
	thread.UpdatedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Second)

	if err := store.UpdateThread(ctx, thread); err != nil {
		t.Fatalf("UpdateThread failed: %v", err)
	}

	got, err := store.GetThread(ctx, "thread-456")
	if err != nil {
		t.Fatalf("GetThread failed: %v", err)
	}

	if got.AgentID != "agent-002" {
		t.Errorf("AgentID not updated: got %q, want %q", got.AgentID, "agent-002")
	}
	if !got.UpdatedAt.Equal(thread.UpdatedAt) {
		t.Errorf("UpdatedAt not updated: got %v, want %v", got.UpdatedAt, thread.UpdatedAt)
	}
}

func TestUpdateThread_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	thread := &Thread{
		ID:           "nonexistent",
		FrontendName: "slack",
		ExternalID:   "C999",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	err := store.UpdateThread(ctx, thread)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSaveAndGetMessages(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create a thread first
	thread := &Thread{
		ID:           "thread-msg-test",
		FrontendName: "slack",
		ExternalID:   "C12345",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := store.CreateThread(ctx, thread); err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}

	// Save some messages
	msg1 := &Message{
		ID:        "msg-001",
		ThreadID:  "thread-msg-test",
		Sender:    "user",
		Content:   "Hello, agent!",
		CreatedAt: time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Second),
	}
	msg2 := &Message{
		ID:        "msg-002",
		ThreadID:  "thread-msg-test",
		Sender:    "agent",
		Content:   "Hello, user!",
		CreatedAt: time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Second),
	}
	msg3 := &Message{
		ID:        "msg-003",
		ThreadID:  "thread-msg-test",
		Sender:    "user",
		Content:   "How are you?",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	for _, msg := range []*Message{msg1, msg2, msg3} {
		if err := store.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}
	}

	// Get all messages
	messages, err := store.GetThreadMessages(ctx, "thread-msg-test", 100)
	if err != nil {
		t.Fatalf("GetThreadMessages failed: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Verify messages are returned in chronological order (oldest first)
	if messages[0].ID != "msg-001" {
		t.Errorf("expected first message to be msg-001, got %s", messages[0].ID)
	}
	if messages[1].ID != "msg-002" {
		t.Errorf("expected second message to be msg-002, got %s", messages[1].ID)
	}
	if messages[2].ID != "msg-003" {
		t.Errorf("expected third message to be msg-003, got %s", messages[2].ID)
	}
}

func TestGetThreadMessages_Limit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create a thread
	thread := &Thread{
		ID:           "thread-limit-test",
		FrontendName: "slack",
		ExternalID:   "C999",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := store.CreateThread(ctx, thread); err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}

	// Save 5 messages
	baseTime := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		msgID := fmt.Sprintf("msg-%c", 'a'+i)
		msg := &Message{
			ID:        msgID,
			ThreadID:  "thread-limit-test",
			Sender:    "user",
			Content:   fmt.Sprintf("Message %c", 'a'+i),
			CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
		}
		if err := store.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}
	}

	// Get with limit of 2
	messages, err := store.GetThreadMessages(ctx, "thread-limit-test", 2)
	if err != nil {
		t.Fatalf("GetThreadMessages failed: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages with limit, got %d", len(messages))
	}

	// Should get the 2 most recent messages (oldest first within those 2)
	// This means we get the last 2 messages by time, returned in chronological order
	if messages[0].ID != "msg-d" {
		t.Errorf("expected first limited message to be msg-d, got %s", messages[0].ID)
	}
	if messages[1].ID != "msg-e" {
		t.Errorf("expected second limited message to be msg-e, got %s", messages[1].ID)
	}
}

func TestGetThreadMessages_EmptyThread(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create a thread with no messages
	thread := &Thread{
		ID:           "thread-empty",
		FrontendName: "slack",
		ExternalID:   "C000",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := store.CreateThread(ctx, thread); err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}

	messages, err := store.GetThreadMessages(ctx, "thread-empty", 100)
	if err != nil {
		t.Fatalf("GetThreadMessages failed: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}

func TestAgentState_SaveAndGet(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	agentID := "agent-state-test"
	state := []byte(`{"foo": "bar", "count": 42}`)

	if err := store.SaveAgentState(ctx, agentID, state); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	got, err := store.GetAgentState(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgentState failed: %v", err)
	}

	if string(got) != string(state) {
		t.Errorf("state mismatch: got %q, want %q", string(got), string(state))
	}
}

func TestAgentState_Update(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	agentID := "agent-update-test"

	// Save initial state
	state1 := []byte(`{"version": 1}`)
	if err := store.SaveAgentState(ctx, agentID, state1); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	// Update state
	state2 := []byte(`{"version": 2}`)
	if err := store.SaveAgentState(ctx, agentID, state2); err != nil {
		t.Fatalf("SaveAgentState (update) failed: %v", err)
	}

	got, err := store.GetAgentState(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgentState failed: %v", err)
	}

	if string(got) != string(state2) {
		t.Errorf("state not updated: got %q, want %q", string(got), string(state2))
	}
}

func TestAgentState_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	_, err := store.GetAgentState(ctx, "nonexistent-agent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetThreadByFrontendID(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	thread := &Thread{
		ID:           "thread-frontend-test",
		FrontendName: "slack",
		ExternalID:   "C12345678",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := store.CreateThread(ctx, thread); err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}

	got, err := store.GetThreadByFrontendID(ctx, "slack", "C12345678")
	if err != nil {
		t.Fatalf("GetThreadByFrontendID failed: %v", err)
	}

	if got.ID != thread.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, thread.ID)
	}
	if got.FrontendName != thread.FrontendName {
		t.Errorf("FrontendName mismatch: got %q, want %q", got.FrontendName, thread.FrontendName)
	}
	if got.ExternalID != thread.ExternalID {
		t.Errorf("ExternalID mismatch: got %q, want %q", got.ExternalID, thread.ExternalID)
	}
	if got.AgentID != thread.AgentID {
		t.Errorf("AgentID mismatch: got %q, want %q", got.AgentID, thread.AgentID)
	}
	if !got.CreatedAt.Equal(thread.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", got.CreatedAt, thread.CreatedAt)
	}
	if !got.UpdatedAt.Equal(thread.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: got %v, want %v", got.UpdatedAt, thread.UpdatedAt)
	}
}

func TestGetThreadByFrontendID_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()
	_, err := store.GetThreadByFrontendID(ctx, "slack", "nonexistent-channel")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetThreadByFrontendID_DifferentFrontends(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create two threads with the same external ID but different frontends
	slackThread := &Thread{
		ID:           "thread-slack",
		FrontendName: "slack",
		ExternalID:   "C12345",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	matrixThread := &Thread{
		ID:           "thread-matrix",
		FrontendName: "matrix",
		ExternalID:   "C12345", // Same external ID, different frontend
		AgentID:      "agent-002",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := store.CreateThread(ctx, slackThread); err != nil {
		t.Fatalf("CreateThread (slack) failed: %v", err)
	}
	if err := store.CreateThread(ctx, matrixThread); err != nil {
		t.Fatalf("CreateThread (matrix) failed: %v", err)
	}

	// Lookup slack thread
	gotSlack, err := store.GetThreadByFrontendID(ctx, "slack", "C12345")
	if err != nil {
		t.Fatalf("GetThreadByFrontendID (slack) failed: %v", err)
	}
	if gotSlack.ID != "thread-slack" {
		t.Errorf("expected thread-slack, got %s", gotSlack.ID)
	}

	// Lookup matrix thread
	gotMatrix, err := store.GetThreadByFrontendID(ctx, "matrix", "C12345")
	if err != nil {
		t.Fatalf("GetThreadByFrontendID (matrix) failed: %v", err)
	}
	if gotMatrix.ID != "thread-matrix" {
		t.Errorf("expected thread-matrix, got %s", gotMatrix.ID)
	}
}

func TestCreateBinding(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	binding := &ChannelBinding{
		FrontendName: "slack",
		ChannelID:    "C0123456789",
		AgentID:      "agent-uuid-123",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := store.CreateBinding(context.Background(), binding)
	if err != nil {
		t.Fatalf("CreateBinding failed: %v", err)
	}

	// Verify by reading back
	got, err := store.GetBinding(context.Background(), "slack", "C0123456789")
	if err != nil {
		t.Fatalf("GetBinding failed: %v", err)
	}

	if got.AgentID != "agent-uuid-123" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-uuid-123")
	}
}

func TestListBindings(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create two bindings
	bindings := []*ChannelBinding{
		{FrontendName: "slack", ChannelID: "C001", AgentID: "agent-1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{FrontendName: "matrix", ChannelID: "!room:example.com", AgentID: "agent-2", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, b := range bindings {
		if err := store.CreateBinding(ctx, b); err != nil {
			t.Fatalf("CreateBinding failed: %v", err)
		}
	}

	got, err := store.ListBindings(ctx)
	if err != nil {
		t.Fatalf("ListBindings failed: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("got %d bindings, want 2", len(got))
	}
}

func TestDeleteBinding(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	binding := &ChannelBinding{
		FrontendName: "slack",
		ChannelID:    "C001",
		AgentID:      "agent-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := store.CreateBinding(ctx, binding); err != nil {
		t.Fatalf("CreateBinding failed: %v", err)
	}

	if err := store.DeleteBinding(ctx, "slack", "C001"); err != nil {
		t.Fatalf("DeleteBinding failed: %v", err)
	}

	_, err := store.GetBinding(ctx, "slack", "C001")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Test deleting non-existent binding returns ErrNotFound
	err = store.DeleteBinding(ctx, "nonexistent", "C999")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent binding, got %v", err)
	}
}

// newTestStore creates a new SQLite store in a temporary directory for testing
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}

	return store
}
