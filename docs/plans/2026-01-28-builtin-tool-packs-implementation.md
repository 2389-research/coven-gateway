# Built-in Tool Packs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add embedded tool packs (base, admin, mail, notes) that register at gateway startup and execute in-process.

**Architecture:** Built-in tools register with the existing `packs.Registry` but execute via Go function calls instead of gRPC. A new `BuiltinTool` type wraps tool definitions with handlers. The Router checks for builtins first before routing to external packs.

**Tech Stack:** Go, SQLite (existing store), existing packs.Registry

---

## Task 1: Store Schema - Add Builtin Tables

**Files:**
- Modify: `internal/store/sqlite.go` (add to createSchema)
- Modify: `internal/store/store.go` (add interface methods)
- Create: `internal/store/builtins.go` (implementation)
- Create: `internal/store/builtins_test.go`

**Step 1: Add tables to createSchema in sqlite.go**

Add after the existing `link_codes` table definition (around line 250):

```go
		-- Built-in tool pack tables

		-- Log entries (activity logging)
		CREATE TABLE IF NOT EXISTS log_entries (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			message TEXT NOT NULL,
			tags TEXT,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_log_entries_agent ON log_entries(agent_id);
		CREATE INDEX IF NOT EXISTS idx_log_entries_created ON log_entries(created_at);

		-- Todos (task management)
		CREATE TABLE IF NOT EXISTS todos (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			description TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			priority TEXT DEFAULT 'medium',
			notes TEXT,
			due_date TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_todos_agent ON todos(agent_id);
		CREATE INDEX IF NOT EXISTS idx_todos_status ON todos(status);

		-- BBS posts (bulletin board)
		CREATE TABLE IF NOT EXISTS bbs_posts (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			thread_id TEXT,
			subject TEXT,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_bbs_posts_thread ON bbs_posts(thread_id);
		CREATE INDEX IF NOT EXISTS idx_bbs_posts_created ON bbs_posts(created_at);

		-- Agent mail (inter-agent messaging)
		CREATE TABLE IF NOT EXISTS agent_mail (
			id TEXT PRIMARY KEY,
			from_agent_id TEXT NOT NULL,
			to_agent_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			content TEXT NOT NULL,
			read_at TEXT,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_agent_mail_to ON agent_mail(to_agent_id);
		CREATE INDEX IF NOT EXISTS idx_agent_mail_unread ON agent_mail(to_agent_id, read_at);

		-- Agent notes (key-value storage)
		CREATE TABLE IF NOT EXISTS agent_notes (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(agent_id, key)
		);
		CREATE INDEX IF NOT EXISTS idx_agent_notes_agent ON agent_notes(agent_id);
```

**Step 2: Run gateway to verify schema creation**

Run: `go build -o bin/coven-gateway ./cmd/coven-gateway && rm -f /tmp/test-builtins.db && COVEN_DB_PATH=/tmp/test-builtins.db ./bin/coven-gateway serve &; sleep 2; sqlite3 /tmp/test-builtins.db ".tables" | grep -E "log_entries|todos|bbs_posts|agent_mail|agent_notes"; kill %1`

Expected: All five tables exist

**Step 3: Commit schema changes**

```bash
git add internal/store/sqlite.go
git commit -m "feat(store): add schema for built-in tool pack tables"
```

---

## Task 2: Store Interface - Builtin Methods

**Files:**
- Modify: `internal/store/store.go` (add BuiltinStore interface)
- Create: `internal/store/builtins.go` (types and SQLite implementation)
- Create: `internal/store/builtins_test.go`

**Step 1: Add types to store.go**

Add after existing types (around line 55):

```go
// LogEntry represents an activity log entry
type LogEntry struct {
	ID        string
	AgentID   string
	Message   string
	Tags      []string
	CreatedAt time.Time
}

// Todo represents a task
type Todo struct {
	ID          string
	AgentID     string
	Description string
	Status      string // pending, in_progress, completed
	Priority    string // low, medium, high
	Notes       string
	DueDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// BBSPost represents a bulletin board post or reply
type BBSPost struct {
	ID        string
	AgentID   string
	ThreadID  string // empty for top-level threads
	Subject   string // required for threads, empty for replies
	Content   string
	CreatedAt time.Time
}

// BBSThread is a post with its replies
type BBSThread struct {
	Post    *BBSPost
	Replies []*BBSPost
}

// AgentMail represents a message between agents
type AgentMail struct {
	ID          string
	FromAgentID string
	ToAgentID   string
	Subject     string
	Content     string
	ReadAt      *time.Time
	CreatedAt   time.Time
}

// AgentNote represents a key-value note for an agent
type AgentNote struct {
	ID        string
	AgentID   string
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

**Step 2: Add BuiltinStore interface to store.go**

Add after the Store interface:

```go
// BuiltinStore defines methods for built-in tool pack data
type BuiltinStore interface {
	// Log entries
	CreateLogEntry(ctx context.Context, entry *LogEntry) error
	SearchLogEntries(ctx context.Context, query string, since *time.Time, limit int) ([]*LogEntry, error)

	// Todos
	CreateTodo(ctx context.Context, todo *Todo) error
	GetTodo(ctx context.Context, id string) (*Todo, error)
	ListTodos(ctx context.Context, agentID string, status, priority string) ([]*Todo, error)
	UpdateTodo(ctx context.Context, todo *Todo) error
	DeleteTodo(ctx context.Context, id string) error

	// BBS
	CreateBBSPost(ctx context.Context, post *BBSPost) error
	GetBBSPost(ctx context.Context, id string) (*BBSPost, error)
	ListBBSThreads(ctx context.Context, limit int) ([]*BBSPost, error)
	GetBBSThread(ctx context.Context, threadID string) (*BBSThread, error)

	// Mail
	SendMail(ctx context.Context, mail *AgentMail) error
	GetMail(ctx context.Context, id string) (*AgentMail, error)
	ListInbox(ctx context.Context, agentID string, unreadOnly bool, limit int) ([]*AgentMail, error)
	MarkMailRead(ctx context.Context, id string) error

	// Notes
	SetNote(ctx context.Context, note *AgentNote) error
	GetNote(ctx context.Context, agentID, key string) (*AgentNote, error)
	ListNotes(ctx context.Context, agentID string) ([]*AgentNote, error)
	DeleteNote(ctx context.Context, agentID, key string) error
}
```

**Step 3: Create builtins.go with SQLite implementation**

Create `internal/store/builtins.go`:

```go
// ABOUTME: SQLite implementation of BuiltinStore for built-in tool pack data.
// ABOUTME: Handles log entries, todos, BBS posts, mail, and notes persistence.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Ensure SQLiteStore implements BuiltinStore
var _ BuiltinStore = (*SQLiteStore)(nil)

// CreateLogEntry creates a new log entry
func (s *SQLiteStore) CreateLogEntry(ctx context.Context, entry *LogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	var tagsJSON *string
	if len(entry.Tags) > 0 {
		b, _ := json.Marshal(entry.Tags)
		str := string(b)
		tagsJSON = &str
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO log_entries (id, agent_id, message, tags, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, entry.ID, entry.AgentID, entry.Message, tagsJSON, entry.CreatedAt.Format(time.RFC3339))

	return err
}

// SearchLogEntries searches log entries by message content
func (s *SQLiteStore) SearchLogEntries(ctx context.Context, query string, since *time.Time, limit int) ([]*LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var args []any
	sql := `SELECT id, agent_id, message, tags, created_at FROM log_entries WHERE 1=1`

	if query != "" {
		sql += ` AND message LIKE ?`
		args = append(args, "%"+query+"%")
	}
	if since != nil {
		sql += ` AND created_at >= ?`
		args = append(args, since.Format(time.RFC3339))
	}

	sql += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*LogEntry
	for rows.Next() {
		var e LogEntry
		var tagsJSON sql.NullString
		var createdAt string
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Message, &tagsJSON, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if tagsJSON.Valid {
			json.Unmarshal([]byte(tagsJSON.String), &e.Tags)
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// CreateTodo creates a new todo
func (s *SQLiteStore) CreateTodo(ctx context.Context, todo *Todo) error {
	if todo.ID == "" {
		todo.ID = uuid.New().String()
	}
	now := time.Now()
	if todo.CreatedAt.IsZero() {
		todo.CreatedAt = now
	}
	todo.UpdatedAt = now
	if todo.Status == "" {
		todo.Status = "pending"
	}
	if todo.Priority == "" {
		todo.Priority = "medium"
	}

	var dueDate *string
	if todo.DueDate != nil {
		d := todo.DueDate.Format(time.RFC3339)
		dueDate = &d
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO todos (id, agent_id, description, status, priority, notes, due_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, todo.ID, todo.AgentID, todo.Description, todo.Status, todo.Priority, todo.Notes, dueDate,
		todo.CreatedAt.Format(time.RFC3339), todo.UpdatedAt.Format(time.RFC3339))

	return err
}

// GetTodo retrieves a todo by ID
func (s *SQLiteStore) GetTodo(ctx context.Context, id string) (*Todo, error) {
	var t Todo
	var notes, dueDate sql.NullString
	var createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, description, status, priority, notes, due_date, created_at, updated_at
		FROM todos WHERE id = ?
	`, id).Scan(&t.ID, &t.AgentID, &t.Description, &t.Status, &t.Priority, &notes, &dueDate, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	t.Notes = notes.String
	if dueDate.Valid {
		d, _ := time.Parse(time.RFC3339, dueDate.String)
		t.DueDate = &d
	}

	return &t, nil
}

// ListTodos lists todos for an agent with optional filters
func (s *SQLiteStore) ListTodos(ctx context.Context, agentID string, status, priority string) ([]*Todo, error) {
	var args []any
	sql := `SELECT id, agent_id, description, status, priority, notes, due_date, created_at, updated_at FROM todos WHERE agent_id = ?`
	args = append(args, agentID)

	if status != "" {
		sql += ` AND status = ?`
		args = append(args, status)
	}
	if priority != "" {
		sql += ` AND priority = ?`
		args = append(args, priority)
	}
	sql += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []*Todo
	for rows.Next() {
		var t Todo
		var notes, dueDate sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.AgentID, &t.Description, &t.Status, &t.Priority, &notes, &dueDate, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		t.Notes = notes.String
		if dueDate.Valid {
			d, _ := time.Parse(time.RFC3339, dueDate.String)
			t.DueDate = &d
		}
		todos = append(todos, &t)
	}
	return todos, rows.Err()
}

// UpdateTodo updates an existing todo
func (s *SQLiteStore) UpdateTodo(ctx context.Context, todo *Todo) error {
	todo.UpdatedAt = time.Now()

	var dueDate *string
	if todo.DueDate != nil {
		d := todo.DueDate.Format(time.RFC3339)
		dueDate = &d
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE todos SET description = ?, status = ?, priority = ?, notes = ?, due_date = ?, updated_at = ?
		WHERE id = ?
	`, todo.Description, todo.Status, todo.Priority, todo.Notes, dueDate, todo.UpdatedAt.Format(time.RFC3339), todo.ID)

	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTodo deletes a todo by ID
func (s *SQLiteStore) DeleteTodo(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateBBSPost creates a new BBS post or reply
func (s *SQLiteStore) CreateBBSPost(ctx context.Context, post *BBSPost) error {
	if post.ID == "" {
		post.ID = uuid.New().String()
	}
	if post.CreatedAt.IsZero() {
		post.CreatedAt = time.Now()
	}

	// For top-level posts (threads), ThreadID is empty
	var threadID, subject *string
	if post.ThreadID != "" {
		threadID = &post.ThreadID
	}
	if post.Subject != "" {
		subject = &post.Subject
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO bbs_posts (id, agent_id, thread_id, subject, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, post.ID, post.AgentID, threadID, subject, post.Content, post.CreatedAt.Format(time.RFC3339))

	return err
}

// GetBBSPost retrieves a single post by ID
func (s *SQLiteStore) GetBBSPost(ctx context.Context, id string) (*BBSPost, error) {
	var p BBSPost
	var threadID, subject sql.NullString
	var createdAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, thread_id, subject, content, created_at
		FROM bbs_posts WHERE id = ?
	`, id).Scan(&p.ID, &p.AgentID, &threadID, &subject, &p.Content, &createdAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	p.ThreadID = threadID.String
	p.Subject = subject.String
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &p, nil
}

// ListBBSThreads lists top-level threads (posts with no thread_id)
func (s *SQLiteStore) ListBBSThreads(ctx context.Context, limit int) ([]*BBSPost, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, thread_id, subject, content, created_at
		FROM bbs_posts WHERE thread_id IS NULL
		ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*BBSPost
	for rows.Next() {
		var p BBSPost
		var threadID, subject sql.NullString
		var createdAt string
		if err := rows.Scan(&p.ID, &p.AgentID, &threadID, &subject, &p.Content, &createdAt); err != nil {
			return nil, err
		}
		p.ThreadID = threadID.String
		p.Subject = subject.String
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		posts = append(posts, &p)
	}
	return posts, rows.Err()
}

// GetBBSThread retrieves a thread with all its replies
func (s *SQLiteStore) GetBBSThread(ctx context.Context, threadID string) (*BBSThread, error) {
	// Get the thread (top-level post)
	post, err := s.GetBBSPost(ctx, threadID)
	if err != nil {
		return nil, err
	}

	// Get replies
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, thread_id, subject, content, created_at
		FROM bbs_posts WHERE thread_id = ?
		ORDER BY created_at ASC
	`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var replies []*BBSPost
	for rows.Next() {
		var p BBSPost
		var tid, subject sql.NullString
		var createdAt string
		if err := rows.Scan(&p.ID, &p.AgentID, &tid, &subject, &p.Content, &createdAt); err != nil {
			return nil, err
		}
		p.ThreadID = tid.String
		p.Subject = subject.String
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		replies = append(replies, &p)
	}

	return &BBSThread{Post: post, Replies: replies}, rows.Err()
}

// SendMail creates a new mail message
func (s *SQLiteStore) SendMail(ctx context.Context, mail *AgentMail) error {
	if mail.ID == "" {
		mail.ID = uuid.New().String()
	}
	if mail.CreatedAt.IsZero() {
		mail.CreatedAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_mail (id, from_agent_id, to_agent_id, subject, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, mail.ID, mail.FromAgentID, mail.ToAgentID, mail.Subject, mail.Content, mail.CreatedAt.Format(time.RFC3339))

	return err
}

// GetMail retrieves a mail message by ID
func (s *SQLiteStore) GetMail(ctx context.Context, id string) (*AgentMail, error) {
	var m AgentMail
	var readAt sql.NullString
	var createdAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, from_agent_id, to_agent_id, subject, content, read_at, created_at
		FROM agent_mail WHERE id = ?
	`, id).Scan(&m.ID, &m.FromAgentID, &m.ToAgentID, &m.Subject, &m.Content, &readAt, &createdAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if readAt.Valid {
		t, _ := time.Parse(time.RFC3339, readAt.String)
		m.ReadAt = &t
	}

	return &m, nil
}

// ListInbox lists mail for an agent
func (s *SQLiteStore) ListInbox(ctx context.Context, agentID string, unreadOnly bool, limit int) ([]*AgentMail, error) {
	if limit <= 0 {
		limit = 50
	}

	var args []any
	sql := `SELECT id, from_agent_id, to_agent_id, subject, content, read_at, created_at FROM agent_mail WHERE to_agent_id = ?`
	args = append(args, agentID)

	if unreadOnly {
		sql += ` AND read_at IS NULL`
	}
	sql += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*AgentMail
	for rows.Next() {
		var m AgentMail
		var readAt sql.NullString
		var createdAt string
		if err := rows.Scan(&m.ID, &m.FromAgentID, &m.ToAgentID, &m.Subject, &m.Content, &readAt, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if readAt.Valid {
			t, _ := time.Parse(time.RFC3339, readAt.String)
			m.ReadAt = &t
		}
		messages = append(messages, &m)
	}
	return messages, rows.Err()
}

// MarkMailRead marks a mail message as read
func (s *SQLiteStore) MarkMailRead(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_mail SET read_at = ? WHERE id = ? AND read_at IS NULL
	`, time.Now().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		// Check if it exists at all
		var exists bool
		s.db.QueryRowContext(ctx, `SELECT 1 FROM agent_mail WHERE id = ?`, id).Scan(&exists)
		if !exists {
			return ErrNotFound
		}
		// Already read, that's fine
	}
	return nil
}

// SetNote creates or updates a note
func (s *SQLiteStore) SetNote(ctx context.Context, note *AgentNote) error {
	if note.ID == "" {
		note.ID = uuid.New().String()
	}
	now := time.Now()
	if note.CreatedAt.IsZero() {
		note.CreatedAt = now
	}
	note.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_notes (id, agent_id, key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, note.ID, note.AgentID, note.Key, note.Value, note.CreatedAt.Format(time.RFC3339), note.UpdatedAt.Format(time.RFC3339))

	return err
}

// GetNote retrieves a note by agent and key
func (s *SQLiteStore) GetNote(ctx context.Context, agentID, key string) (*AgentNote, error) {
	var n AgentNote
	var createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, key, value, created_at, updated_at
		FROM agent_notes WHERE agent_id = ? AND key = ?
	`, agentID, key).Scan(&n.ID, &n.AgentID, &n.Key, &n.Value, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &n, nil
}

// ListNotes lists all notes for an agent
func (s *SQLiteStore) ListNotes(ctx context.Context, agentID string) ([]*AgentNote, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, key, value, created_at, updated_at
		FROM agent_notes WHERE agent_id = ?
		ORDER BY key ASC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []*AgentNote
	for rows.Next() {
		var n AgentNote
		var createdAt, updatedAt string
		if err := rows.Scan(&n.ID, &n.AgentID, &n.Key, &n.Value, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		notes = append(notes, &n)
	}
	return notes, rows.Err()
}

// DeleteNote deletes a note by agent and key
func (s *SQLiteStore) DeleteNote(ctx context.Context, agentID, key string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM agent_notes WHERE agent_id = ? AND key = ?`, agentID, key)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

**Step 4: Create builtins_test.go**

Create `internal/store/builtins_test.go`:

```go
// ABOUTME: Tests for BuiltinStore methods (log, todo, bbs, mail, notes).
// ABOUTME: Uses real SQLite in-memory database for integration testing.

package store

import (
	"context"
	"testing"
	"time"
)

func TestLogEntries(t *testing.T) {
	s := newTestStore(t)
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

func TestTodos(t *testing.T) {
	s := newTestStore(t)
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

func TestBBS(t *testing.T) {
	s := newTestStore(t)
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

func TestMail(t *testing.T) {
	s := newTestStore(t)
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

func TestNotes(t *testing.T) {
	s := newTestStore(t)
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

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
```

**Step 5: Run tests**

Run: `go test -v ./internal/store/... -run "TestLog|TestTodo|TestBBS|TestMail|TestNotes"`

Expected: All tests PASS

**Step 6: Commit**

```bash
git add internal/store/store.go internal/store/builtins.go internal/store/builtins_test.go
git commit -m "feat(store): add BuiltinStore interface and SQLite implementation

Adds store methods for built-in tool pack data:
- Log entries (activity logging)
- Todos (task management)
- BBS posts (bulletin board)
- Agent mail (inter-agent messaging)
- Agent notes (key-value storage)"
```

---

## Task 3: Registry Builtin Support

**Files:**
- Modify: `internal/packs/registry.go`
- Create: `internal/packs/builtin.go`
- Create: `internal/packs/builtin_test.go`

**Step 1: Create builtin.go with types**

Create `internal/packs/builtin.go`:

```go
// ABOUTME: Built-in tool support for tools that execute in-process.
// ABOUTME: Allows gateway-embedded tools to coexist with external packs.

package packs

import (
	"context"
	"encoding/json"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// ToolHandler is a function that executes a built-in tool.
// It receives the calling agent's ID and the tool input as JSON.
// Returns the result as JSON or an error.
type ToolHandler func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error)

// BuiltinTool represents a tool that executes in the gateway process.
type BuiltinTool struct {
	Definition *pb.ToolDefinition
	Handler    ToolHandler
}

// BuiltinPack is a collection of built-in tools with a pack ID.
type BuiltinPack struct {
	ID    string
	Tools []*BuiltinTool
}

// builtinEntry stores a builtin tool with its pack ID for registry lookup.
type builtinEntry struct {
	Tool   *BuiltinTool
	PackID string
}
```

**Step 2: Modify registry.go to support builtins**

Add a new field and methods to Registry. After the existing fields (around line 80):

```go
type Registry struct {
	mu       sync.RWMutex
	packs    map[string]*Pack
	tools    map[string]*Tool // global tool name -> tool (for collision detection)
	builtins map[string]*builtinEntry // builtin tool name -> builtin entry
	logger   *slog.Logger
}
```

Update NewRegistry:

```go
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		packs:    make(map[string]*Pack),
		tools:    make(map[string]*Tool),
		builtins: make(map[string]*builtinEntry),
		logger:   logger,
	}
}
```

Add RegisterBuiltinPack method:

```go
// RegisterBuiltinPack registers a pack of built-in tools that execute in-process.
// Returns error if any tool name collides with existing tools.
func (r *Registry) RegisterBuiltinPack(pack *BuiltinPack) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for collisions
	for _, tool := range pack.Tools {
		name := tool.Definition.GetName()
		if _, exists := r.tools[name]; exists {
			return fmt.Errorf("%w: tool '%s' already registered by external pack", ErrToolCollision, name)
		}
		if _, exists := r.builtins[name]; exists {
			return fmt.Errorf("%w: tool '%s' already registered as builtin", ErrToolCollision, name)
		}
	}

	// Register all tools
	for _, tool := range pack.Tools {
		r.builtins[tool.Definition.GetName()] = &builtinEntry{
			Tool:   tool,
			PackID: pack.ID,
		}
	}

	r.logger.Info("=== BUILTIN PACK REGISTERED ===",
		"pack_id", pack.ID,
		"tool_count", len(pack.Tools),
	)

	return nil
}

// GetBuiltinTool returns a builtin tool by name, or nil if not found.
func (r *Registry) GetBuiltinTool(name string) *BuiltinTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, ok := r.builtins[name]; ok {
		return entry.Tool
	}
	return nil
}

// IsBuiltin returns true if the tool name is a builtin tool.
func (r *Registry) IsBuiltin(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.builtins[name]
	return ok
}
```

Update GetToolsForCapabilities to include builtins:

```go
// GetToolsForCapabilities returns tools where the agent has ALL required capabilities.
// Includes both external pack tools and builtin tools.
func (r *Registry) GetToolsForCapabilities(caps []string) []*pb.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build a set of agent capabilities for fast lookup
	capSet := make(map[string]struct{}, len(caps))
	for _, cap := range caps {
		capSet[cap] = struct{}{}
	}

	var result []*pb.ToolDefinition

	// External pack tools
	for _, tool := range r.tools {
		if r.hasAllCapabilities(tool.Definition.GetRequiredCapabilities(), capSet) {
			result = append(result, tool.Definition)
		}
	}

	// Builtin tools
	for _, entry := range r.builtins {
		if r.hasAllCapabilities(entry.Tool.Definition.GetRequiredCapabilities(), capSet) {
			result = append(result, entry.Tool.Definition)
		}
	}

	return result
}
```

**Step 3: Create builtin_test.go**

Create `internal/packs/builtin_test.go`:

```go
// ABOUTME: Tests for built-in tool registration and lookup.
// ABOUTME: Verifies builtin tools work alongside external pack tools.

package packs

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	pb "github.com/2389/coven-gateway/proto/coven"
)

func TestRegisterBuiltinPack(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	pack := &BuiltinPack{
		ID: "builtin:base",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:                 "log_entry",
					Description:          "Log an activity",
					RequiredCapabilities: []string{"base"},
				},
				Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
					return []byte(`{"ok": true}`), nil
				},
			},
		},
	}

	if err := reg.RegisterBuiltinPack(pack); err != nil {
		t.Fatalf("RegisterBuiltinPack: %v", err)
	}

	// Check IsBuiltin
	if !reg.IsBuiltin("log_entry") {
		t.Error("expected log_entry to be builtin")
	}

	// Check GetBuiltinTool
	tool := reg.GetBuiltinTool("log_entry")
	if tool == nil {
		t.Fatal("expected to find log_entry tool")
	}
	if tool.Definition.GetName() != "log_entry" {
		t.Errorf("unexpected name: %s", tool.Definition.GetName())
	}
}

func TestBuiltinCapabilityFiltering(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	pack := &BuiltinPack{
		ID: "builtin:mixed",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:                 "public_tool",
					RequiredCapabilities: []string{"base"},
				},
				Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
					return nil, nil
				},
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "admin_tool",
					RequiredCapabilities: []string{"admin"},
				},
				Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
					return nil, nil
				},
			},
		},
	}

	if err := reg.RegisterBuiltinPack(pack); err != nil {
		t.Fatalf("RegisterBuiltinPack: %v", err)
	}

	// Agent with only base capability
	tools := reg.GetToolsForCapabilities([]string{"base"})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool for base, got %d", len(tools))
	}
	if tools[0].GetName() != "public_tool" {
		t.Errorf("expected public_tool, got %s", tools[0].GetName())
	}

	// Agent with admin capability
	tools = reg.GetToolsForCapabilities([]string{"admin"})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool for admin, got %d", len(tools))
	}

	// Agent with both
	tools = reg.GetToolsForCapabilities([]string{"base", "admin"})
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
}

func TestBuiltinCollision(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	pack1 := &BuiltinPack{
		ID: "builtin:pack1",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{Name: "my_tool"},
				Handler:    func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) { return nil, nil },
			},
		},
	}

	pack2 := &BuiltinPack{
		ID: "builtin:pack2",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{Name: "my_tool"}, // collision!
				Handler:    func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) { return nil, nil },
			},
		},
	}

	if err := reg.RegisterBuiltinPack(pack1); err != nil {
		t.Fatalf("RegisterBuiltinPack pack1: %v", err)
	}

	err := reg.RegisterBuiltinPack(pack2)
	if err == nil {
		t.Fatal("expected collision error")
	}
}
```

**Step 4: Run tests**

Run: `go test -v ./internal/packs/... -run "TestRegisterBuiltin|TestBuiltinCapability|TestBuiltinCollision"`

Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/packs/builtin.go internal/packs/builtin_test.go internal/packs/registry.go
git commit -m "feat(packs): add built-in tool support to registry

Built-in tools execute in-process via ToolHandler functions.
They coexist with external pack tools and use the same
capability-based filtering."
```

---

## Task 4: Router Builtin Dispatch

**Files:**
- Modify: `internal/packs/router.go`
- Modify: `internal/packs/router_test.go`

**Step 1: Update Router to dispatch builtins**

Add to RouteToolCall at the beginning (before looking up external pack):

```go
// RouteToolCall routes a tool call to the appropriate pack or builtin handler.
func (r *Router) RouteToolCall(ctx context.Context, toolName, inputJSON, requestID string, agentID string) (*pb.ExecuteToolResponse, error) {
	// Check if it's a builtin tool first
	if builtin := r.registry.GetBuiltinTool(toolName); builtin != nil {
		r.logger.Info("→ dispatching to builtin",
			"tool_name", toolName,
			"request_id", requestID,
			"agent_id", agentID,
		)

		result, err := builtin.Handler(ctx, agentID, json.RawMessage(inputJSON))
		if err != nil {
			r.logger.Warn("builtin tool error",
				"tool_name", toolName,
				"request_id", requestID,
				"error", err,
			)
			return &pb.ExecuteToolResponse{
				RequestId: requestID,
				Result:    &pb.ExecuteToolResponse_Error{Error: err.Error()},
			}, nil
		}

		r.logger.Info("← builtin responded",
			"tool_name", toolName,
			"request_id", requestID,
		)
		return &pb.ExecuteToolResponse{
			RequestId: requestID,
			Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: string(result)},
		}, nil
	}

	// ... existing external pack routing code ...
```

Note: The signature changes to include `agentID` - you'll need to update call sites.

**Step 2: Add import for encoding/json**

Add `"encoding/json"` to imports in router.go.

**Step 3: Update router_test.go with builtin test**

Add test case:

```go
func TestRouteBuiltinTool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	// Register a builtin
	pack := &BuiltinPack{
		ID: "builtin:test",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{Name: "echo"},
				Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
					return input, nil // echo back the input
				},
			},
		},
	}
	if err := reg.RegisterBuiltinPack(pack); err != nil {
		t.Fatalf("RegisterBuiltinPack: %v", err)
	}

	router := NewRouter(RouterConfig{
		Registry: reg,
		Logger:   logger,
	})

	resp, err := router.RouteToolCall(context.Background(), "echo", `{"hello": "world"}`, "req-1", "agent-1")
	if err != nil {
		t.Fatalf("RouteToolCall: %v", err)
	}

	if resp.GetError() != "" {
		t.Errorf("unexpected error: %s", resp.GetError())
	}
	if resp.GetOutputJson() != `{"hello": "world"}` {
		t.Errorf("unexpected output: %s", resp.GetOutputJson())
	}
}
```

**Step 4: Run tests**

Run: `go test -v ./internal/packs/... -run "TestRoute"`

Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/packs/router.go internal/packs/router_test.go
git commit -m "feat(packs): route builtin tools in-process

Builtin tools are dispatched directly via handler function
before falling back to external pack routing."
```

---

## Task 5: Base Pack Implementation

**Files:**
- Create: `internal/builtins/base.go`
- Create: `internal/builtins/base_test.go`

**Step 1: Create base.go**

Create `internal/builtins/base.go`:

```go
// ABOUTME: Base pack provides default tools for all agents: log, todo, bbs.
// ABOUTME: Requires the "base" capability.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// BasePack creates the base pack with log, todo, and bbs tools.
func BasePack(s store.BuiltinStore) *packs.BuiltinPack {
	b := &baseHandlers{store: s}
	return &packs.BuiltinPack{
		ID: "builtin:base",
		Tools: []*packs.BuiltinTool{
			// Log tools
			{
				Definition: &pb.ToolDefinition{
					Name:                 "log_entry",
					Description:          "Log an activity or event",
					InputSchemaJson:      `{"type":"object","properties":{"message":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["message"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.LogEntry,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "log_search",
					Description:          "Search past log entries",
					InputSchemaJson:      `{"type":"object","properties":{"query":{"type":"string"},"since":{"type":"string","format":"date-time"},"limit":{"type":"integer"}}}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.LogSearch,
			},
			// Todo tools
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_add",
					Description:          "Create a todo",
					InputSchemaJson:      `{"type":"object","properties":{"description":{"type":"string"},"priority":{"type":"string","enum":["low","medium","high"]},"due_date":{"type":"string","format":"date-time"},"notes":{"type":"string"}},"required":["description"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoAdd,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_list",
					Description:          "List todos",
					InputSchemaJson:      `{"type":"object","properties":{"status":{"type":"string","enum":["pending","in_progress","completed"]},"priority":{"type":"string","enum":["low","medium","high"]}}}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoList,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_update",
					Description:          "Update a todo",
					InputSchemaJson:      `{"type":"object","properties":{"id":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]},"priority":{"type":"string","enum":["low","medium","high"]},"notes":{"type":"string"},"due_date":{"type":"string","format":"date-time"}},"required":["id"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoUpdate,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_delete",
					Description:          "Delete a todo",
					InputSchemaJson:      `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoDelete,
			},
			// BBS tools
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_create_thread",
					Description:          "Create a new discussion thread",
					InputSchemaJson:      `{"type":"object","properties":{"subject":{"type":"string"},"content":{"type":"string"}},"required":["subject","content"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSCreateThread,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_reply",
					Description:          "Reply to a thread",
					InputSchemaJson:      `{"type":"object","properties":{"thread_id":{"type":"string"},"content":{"type":"string"}},"required":["thread_id","content"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSReply,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_list_threads",
					Description:          "List discussion threads",
					InputSchemaJson:      `{"type":"object","properties":{"limit":{"type":"integer"}}}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSListThreads,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_read_thread",
					Description:          "Read a thread with replies",
					InputSchemaJson:      `{"type":"object","properties":{"thread_id":{"type":"string"}},"required":["thread_id"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSReadThread,
			},
		},
	}
}

type baseHandlers struct {
	store store.BuiltinStore
}

// Log handlers

type logEntryInput struct {
	Message string   `json:"message"`
	Tags    []string `json:"tags"`
}

func (b *baseHandlers) LogEntry(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in logEntryInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	entry := &store.LogEntry{
		AgentID: agentID,
		Message: in.Message,
		Tags:    in.Tags,
	}
	if err := b.store.CreateLogEntry(ctx, entry); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"id": entry.ID, "status": "logged"})
}

type logSearchInput struct {
	Query string `json:"query"`
	Since string `json:"since"`
	Limit int    `json:"limit"`
}

func (b *baseHandlers) LogSearch(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in logSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	var since *time.Time
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since date: %w", err)
		}
		since = &t
	}

	entries, err := b.store.SearchLogEntries(ctx, in.Query, since, in.Limit)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"entries": entries, "count": len(entries)})
}

// Todo handlers

type todoAddInput struct {
	Description string `json:"description"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date"`
	Notes       string `json:"notes"`
}

func (b *baseHandlers) TodoAdd(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoAddInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	todo := &store.Todo{
		AgentID:     agentID,
		Description: in.Description,
		Priority:    in.Priority,
		Notes:       in.Notes,
	}
	if in.DueDate != "" {
		t, err := time.Parse(time.RFC3339, in.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		todo.DueDate = &t
	}

	if err := b.store.CreateTodo(ctx, todo); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"id": todo.ID, "status": "created"})
}

type todoListInput struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

func (b *baseHandlers) TodoList(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoListInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	todos, err := b.store.ListTodos(ctx, agentID, in.Status, in.Priority)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"todos": todos, "count": len(todos)})
}

type todoUpdateInput struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Notes    string `json:"notes"`
	DueDate  string `json:"due_date"`
}

func (b *baseHandlers) TodoUpdate(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoUpdateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	todo, err := b.store.GetTodo(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	// Only update fields that were provided
	if in.Status != "" {
		todo.Status = in.Status
	}
	if in.Priority != "" {
		todo.Priority = in.Priority
	}
	if in.Notes != "" {
		todo.Notes = in.Notes
	}
	if in.DueDate != "" {
		t, err := time.Parse(time.RFC3339, in.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		todo.DueDate = &t
	}

	if err := b.store.UpdateTodo(ctx, todo); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"status": "updated"})
}

type todoDeleteInput struct {
	ID string `json:"id"`
}

func (b *baseHandlers) TodoDelete(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoDeleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if err := b.store.DeleteTodo(ctx, in.ID); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"status": "deleted"})
}

// BBS handlers

type bbsCreateThreadInput struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
}

func (b *baseHandlers) BBSCreateThread(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsCreateThreadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	post := &store.BBSPost{
		AgentID: agentID,
		Subject: in.Subject,
		Content: in.Content,
	}
	if err := b.store.CreateBBSPost(ctx, post); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"thread_id": post.ID, "status": "created"})
}

type bbsReplyInput struct {
	ThreadID string `json:"thread_id"`
	Content  string `json:"content"`
}

func (b *baseHandlers) BBSReply(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsReplyInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	post := &store.BBSPost{
		AgentID:  agentID,
		ThreadID: in.ThreadID,
		Content:  in.Content,
	}
	if err := b.store.CreateBBSPost(ctx, post); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"post_id": post.ID, "status": "posted"})
}

type bbsListThreadsInput struct {
	Limit int `json:"limit"`
}

func (b *baseHandlers) BBSListThreads(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsListThreadsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	threads, err := b.store.ListBBSThreads(ctx, in.Limit)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"threads": threads, "count": len(threads)})
}

type bbsReadThreadInput struct {
	ThreadID string `json:"thread_id"`
}

func (b *baseHandlers) BBSReadThread(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsReadThreadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	thread, err := b.store.GetBBSThread(ctx, in.ThreadID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(thread)
}
```

**Step 2: Create base_test.go**

Create `internal/builtins/base_test.go`:

```go
// ABOUTME: Tests for base pack tool handlers.
// ABOUTME: Uses real SQLite store for integration testing.

package builtins

import (
	"context"
	"encoding/json"
	"testing"

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
```

**Step 3: Run tests**

Run: `go test -v ./internal/builtins/...`

Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/builtins/base.go internal/builtins/base_test.go
git commit -m "feat(builtins): implement base pack with log, todo, bbs tools"
```

---

## Task 6: Admin Pack Implementation

Similar structure to Task 5, creates `internal/builtins/admin.go` with admin_list_agents, admin_agent_history, admin_send_message tools. Requires `admin` capability.

---

## Task 7: Mail Pack Implementation

Creates `internal/builtins/mail.go` with mail_send, mail_inbox, mail_read tools. Requires `mail` capability.

---

## Task 8: Notes Pack Implementation

Creates `internal/builtins/notes.go` with note_set, note_get, note_list, note_delete tools. Requires `notes` capability.

---

## Task 9: Gateway Integration

**Files:**
- Modify: `internal/gateway/gateway.go`

**Step 1: Add builtin registration at startup**

In `New()` function, after creating the pack registry, register builtin packs:

```go
// Register built-in packs
builtinStore := s.(*store.SQLiteStore) // Cast to get BuiltinStore methods
if err := packRegistry.RegisterBuiltinPack(builtins.BasePack(builtinStore)); err != nil {
	return nil, fmt.Errorf("registering base pack: %w", err)
}
if err := packRegistry.RegisterBuiltinPack(builtins.AdminPack(agentMgr, s)); err != nil {
	return nil, fmt.Errorf("registering admin pack: %w", err)
}
if err := packRegistry.RegisterBuiltinPack(builtins.MailPack(builtinStore)); err != nil {
	return nil, fmt.Errorf("registering mail pack: %w", err)
}
if err := packRegistry.RegisterBuiltinPack(builtins.NotesPack(builtinStore)); err != nil {
	return nil, fmt.Errorf("registering notes pack: %w", err)
}
```

**Step 2: Add import**

```go
import "github.com/2389/coven-gateway/internal/builtins"
```

---

## Task 10: Webadmin UI - Logs Tab

**Files:**
- Create: `internal/webadmin/logs.go`
- Create: `internal/webadmin/templates/logs.html`
- Create: `internal/webadmin/templates/partials/logs_list.html`
- Modify: `internal/webadmin/webadmin.go` (add routes)

Adds a Logs tab showing all log entries, searchable, with agent filtering.

---

## Task 11: Webadmin UI - Todos Tab

Similar to Task 10, adds Todos tab showing todos grouped by agent.

---

## Task 12: Webadmin UI - Board Tab

Similar to Task 10, adds Board tab showing BBS threads.

---

## Summary

| Task | Description | Estimated Commits |
|------|-------------|-------------------|
| 1 | Store schema | 1 |
| 2 | Store interface & implementation | 1 |
| 3 | Registry builtin support | 1 |
| 4 | Router builtin dispatch | 1 |
| 5 | Base pack | 1 |
| 6 | Admin pack | 1 |
| 7 | Mail pack | 1 |
| 8 | Notes pack | 1 |
| 9 | Gateway integration | 1 |
| 10 | Webadmin Logs | 1 |
| 11 | Webadmin Todos | 1 |
| 12 | Webadmin Board | 1 |

**Total: 12 commits**
