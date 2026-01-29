// ABOUTME: Tests for BuiltinStore methods (log, todo, bbs, mail, notes).
// ABOUTME: Uses real SQLite in-memory database for integration testing.

package store

import (
	"context"
	"testing"
	"time"
)

func TestLogEntries(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Create entry
	entry := &LogEntry{
		AgentID: "agent-1",
		Message: "did something important",
		Tags:    []string{"work", "urgent"},
	}
	if err := s.CreateLogEntry(ctx, entry); err != nil {
		t.Fatalf("CreateLogEntry: %v", err)
	}
	if entry.ID == "" {
		t.Error("expected ID to be set")
	}

	// Search
	entries, err := s.SearchLogEntries(ctx, "important", nil, 10)
	if err != nil {
		t.Fatalf("SearchLogEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != "did something important" {
		t.Errorf("unexpected message: %s", entries[0].Message)
	}
	if len(entries[0].Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(entries[0].Tags))
	}
}

func TestLogEntriesWithSince(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Create an old entry
	oldEntry := &LogEntry{
		AgentID:   "agent-1",
		Message:   "old entry",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}
	if err := s.CreateLogEntry(ctx, oldEntry); err != nil {
		t.Fatalf("CreateLogEntry: %v", err)
	}

	// Create a new entry
	newEntry := &LogEntry{
		AgentID: "agent-1",
		Message: "new entry",
	}
	if err := s.CreateLogEntry(ctx, newEntry); err != nil {
		t.Fatalf("CreateLogEntry: %v", err)
	}

	// Search with since filter (only last hour)
	since := time.Now().Add(-1 * time.Hour)
	entries, err := s.SearchLogEntries(ctx, "", &since, 10)
	if err != nil {
		t.Fatalf("SearchLogEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry with since filter, got %d", len(entries))
	}
	if entries[0].Message != "new entry" {
		t.Errorf("expected new entry, got: %s", entries[0].Message)
	}
}

func TestTodos(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Create
	todo := &Todo{
		AgentID:     "agent-1",
		Description: "write tests",
		Priority:    "high",
	}
	if err := s.CreateTodo(ctx, todo); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	// Get
	got, err := s.GetTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("GetTodo: %v", err)
	}
	if got.Description != "write tests" {
		t.Errorf("unexpected description: %s", got.Description)
	}
	if got.Status != "pending" {
		t.Errorf("unexpected status: %s", got.Status)
	}

	// Update
	got.Status = "completed"
	if err := s.UpdateTodo(ctx, got); err != nil {
		t.Fatalf("UpdateTodo: %v", err)
	}

	// List with filter
	todos, err := s.ListTodos(ctx, "agent-1", "completed", "")
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	if len(todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(todos))
	}

	// Delete
	if err := s.DeleteTodo(ctx, todo.ID); err != nil {
		t.Fatalf("DeleteTodo: %v", err)
	}
	_, err = s.GetTodo(ctx, todo.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTodosWithDueDate(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	dueDate := time.Now().Add(24 * time.Hour)
	todo := &Todo{
		AgentID:     "agent-1",
		Description: "future task",
		DueDate:     &dueDate,
	}
	if err := s.CreateTodo(ctx, todo); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	got, err := s.GetTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("GetTodo: %v", err)
	}
	if got.DueDate == nil {
		t.Fatal("expected due date to be set")
	}
	// Compare truncated to seconds since SQLite stores as RFC3339
	if got.DueDate.Unix() != dueDate.Unix() {
		t.Errorf("due date mismatch: got %v, want %v", got.DueDate, dueDate)
	}
}

func TestTodosNotFound(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	_, err := s.GetTodo(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	err = s.UpdateTodo(ctx, &Todo{ID: "nonexistent"})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for update, got %v", err)
	}

	err = s.DeleteTodo(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for delete, got %v", err)
	}
}

func TestBBS(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Create thread
	thread := &BBSPost{
		AgentID: "agent-1",
		Subject: "Hello world",
		Content: "First post!",
	}
	if err := s.CreateBBSPost(ctx, thread); err != nil {
		t.Fatalf("CreateBBSPost: %v", err)
	}

	// Create reply
	reply := &BBSPost{
		AgentID:  "agent-2",
		ThreadID: thread.ID,
		Content:  "Nice to meet you!",
	}
	if err := s.CreateBBSPost(ctx, reply); err != nil {
		t.Fatalf("CreateBBSPost reply: %v", err)
	}

	// List threads
	threads, err := s.ListBBSThreads(ctx, 10)
	if err != nil {
		t.Fatalf("ListBBSThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}

	// Get thread with replies
	full, err := s.GetBBSThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetBBSThread: %v", err)
	}
	if full.Post.Subject != "Hello world" {
		t.Errorf("unexpected subject: %s", full.Post.Subject)
	}
	if len(full.Replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(full.Replies))
	}
}

func TestBBSNotFound(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	_, err := s.GetBBSPost(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	_, err = s.GetBBSThread(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for thread, got %v", err)
	}
}

func TestMail(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Send
	mail := &AgentMail{
		FromAgentID: "agent-1",
		ToAgentID:   "agent-2",
		Subject:     "Hello",
		Content:     "How are you?",
	}
	if err := s.SendMail(ctx, mail); err != nil {
		t.Fatalf("SendMail: %v", err)
	}

	// List inbox
	inbox, err := s.ListInbox(ctx, "agent-2", false, 10)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 message, got %d", len(inbox))
	}
	if inbox[0].ReadAt != nil {
		t.Error("expected unread message")
	}

	// Mark read
	if err := s.MarkMailRead(ctx, mail.ID); err != nil {
		t.Fatalf("MarkMailRead: %v", err)
	}

	// Verify read
	got, err := s.GetMail(ctx, mail.ID)
	if err != nil {
		t.Fatalf("GetMail: %v", err)
	}
	if got.ReadAt == nil {
		t.Error("expected message to be marked read")
	}

	// Unread only filter
	unread, err := s.ListInbox(ctx, "agent-2", true, 10)
	if err != nil {
		t.Fatalf("ListInbox unread: %v", err)
	}
	if len(unread) != 0 {
		t.Errorf("expected 0 unread, got %d", len(unread))
	}
}

func TestMailNotFound(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	_, err := s.GetMail(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	err = s.MarkMailRead(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for mark read, got %v", err)
	}
}

func TestMailMarkReadIdempotent(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	mail := &AgentMail{
		FromAgentID: "agent-1",
		ToAgentID:   "agent-2",
		Subject:     "Test",
		Content:     "Content",
	}
	if err := s.SendMail(ctx, mail); err != nil {
		t.Fatalf("SendMail: %v", err)
	}

	// Mark read twice - should not error
	if err := s.MarkMailRead(ctx, mail.ID); err != nil {
		t.Fatalf("MarkMailRead first: %v", err)
	}
	if err := s.MarkMailRead(ctx, mail.ID); err != nil {
		t.Fatalf("MarkMailRead second (should be idempotent): %v", err)
	}
}

func TestNotes(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Set
	note := &AgentNote{
		AgentID: "agent-1",
		Key:     "favorite_color",
		Value:   "blue",
	}
	if err := s.SetNote(ctx, note); err != nil {
		t.Fatalf("SetNote: %v", err)
	}

	// Get
	got, err := s.GetNote(ctx, "agent-1", "favorite_color")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.Value != "blue" {
		t.Errorf("unexpected value: %s", got.Value)
	}

	// Update (upsert)
	note.Value = "green"
	if err := s.SetNote(ctx, note); err != nil {
		t.Fatalf("SetNote update: %v", err)
	}
	got, _ = s.GetNote(ctx, "agent-1", "favorite_color")
	if got.Value != "green" {
		t.Errorf("expected updated value green, got: %s", got.Value)
	}

	// List
	notes, err := s.ListNotes(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}

	// Delete
	if err := s.DeleteNote(ctx, "agent-1", "favorite_color"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	_, err = s.GetNote(ctx, "agent-1", "favorite_color")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNotesNotFound(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	_, err := s.GetNote(ctx, "agent-1", "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	err = s.DeleteNote(ctx, "agent-1", "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for delete, got %v", err)
	}
}

func TestNotesMultipleKeys(t *testing.T) {
	s := newBuiltinTestStore(t)
	ctx := context.Background()

	// Set multiple notes for same agent
	notes := []struct {
		key   string
		value string
	}{
		{"alpha", "first"},
		{"beta", "second"},
		{"gamma", "third"},
	}

	for _, n := range notes {
		if err := s.SetNote(ctx, &AgentNote{
			AgentID: "agent-1",
			Key:     n.key,
			Value:   n.value,
		}); err != nil {
			t.Fatalf("SetNote %s: %v", n.key, err)
		}
	}

	// List should return all, sorted by key
	list, err := s.ListNotes(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(list))
	}
	// Verify sorted order
	if list[0].Key != "alpha" || list[1].Key != "beta" || list[2].Key != "gamma" {
		t.Errorf("notes not sorted by key: %v, %v, %v", list[0].Key, list[1].Key, list[2].Key)
	}
}

func newBuiltinTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
