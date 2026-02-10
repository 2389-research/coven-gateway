// ABOUTME: SQLite implementation of BuiltinStore for built-in tool pack data.
// ABOUTME: Handles log entries, todos, BBS posts, mail, and notes persistence.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Ensure SQLiteStore implements BuiltinStore.
var _ BuiltinStore = (*SQLiteStore)(nil)

// CreateLogEntry creates a new log entry.
func (s *SQLiteStore) CreateLogEntry(ctx context.Context, entry *LogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	var tagsJSON *string
	if len(entry.Tags) > 0 {
		b, err := json.Marshal(entry.Tags)
		if err != nil {
			return fmt.Errorf("marshaling tags: %w", err)
		}
		str := string(b)
		tagsJSON = &str
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO log_entries (id, agent_id, message, tags, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, entry.ID, entry.AgentID, entry.Message, tagsJSON, entry.CreatedAt.Format(time.RFC3339))

	return err
}

// SearchLogEntries searches log entries by message content.
// If agentID is non-empty, results are scoped to that agent.
func (s *SQLiteStore) SearchLogEntries(ctx context.Context, agentID string, query string, since *time.Time, limit int) ([]*LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var args []any
	sqlQuery := `SELECT id, agent_id, message, tags, created_at FROM log_entries WHERE 1=1`

	if agentID != "" {
		sqlQuery += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	if query != "" {
		sqlQuery += ` AND message LIKE ?`
		args = append(args, "%"+query+"%")
	}
	if since != nil {
		sqlQuery += ` AND created_at >= ?`
		args = append(args, since.Format(time.RFC3339))
	}

	sqlQuery += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
			_ = json.Unmarshal([]byte(tagsJSON.String), &e.Tags) // Best effort: invalid JSON leaves tags empty
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// CreateTodo creates a new todo.
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

// GetTodo retrieves a todo by ID.
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

// ListTodos lists todos for an agent with optional filters.
func (s *SQLiteStore) ListTodos(ctx context.Context, agentID string, status, priority string) ([]*Todo, error) {
	var args []any
	sqlQuery := `SELECT id, agent_id, description, status, priority, notes, due_date, created_at, updated_at FROM todos WHERE agent_id = ?`
	args = append(args, agentID)

	if status != "" {
		sqlQuery += ` AND status = ?`
		args = append(args, status)
	}
	if priority != "" {
		sqlQuery += ` AND priority = ?`
		args = append(args, priority)
	}
	sqlQuery += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// ListAllTodos lists all todos across all agents.
func (s *SQLiteStore) ListAllTodos(ctx context.Context, limit int) ([]*Todo, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, description, status, priority, notes, due_date, created_at, updated_at
		FROM todos ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// UpdateTodo updates an existing todo.
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

// DeleteTodo deletes a todo by ID.
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

// CreateBBSPost creates a new BBS post or reply.
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

// GetBBSPost retrieves a single post by ID.
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

// ListBBSThreads lists top-level threads (posts with no thread_id).
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
	defer func() { _ = rows.Close() }()

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

// GetBBSThread retrieves a thread with all its replies.
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
	defer func() { _ = rows.Close() }()

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

// SendMail creates a new mail message.
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

// GetMail retrieves a mail message by ID.
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

// ListInbox lists mail for an agent.
func (s *SQLiteStore) ListInbox(ctx context.Context, agentID string, unreadOnly bool, limit int) ([]*AgentMail, error) {
	if limit <= 0 {
		limit = 50
	}

	args := make([]any, 0, 2)
	sqlQuery := `SELECT id, from_agent_id, to_agent_id, subject, content, read_at, created_at FROM agent_mail WHERE to_agent_id = ?`
	args = append(args, agentID)

	if unreadOnly {
		sqlQuery += ` AND read_at IS NULL`
	}
	sqlQuery += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// MarkMailRead marks a mail message as read.
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
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM agent_mail WHERE id = ?`, id).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("checking mail existence: %w", err)
		}
		// Already read, that's fine
	}
	return nil
}

// SetNote creates or updates a note.
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

// GetNote retrieves a note by agent and key.
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

// ListNotes lists all notes for an agent.
func (s *SQLiteStore) ListNotes(ctx context.Context, agentID string) ([]*AgentNote, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, key, value, created_at, updated_at
		FROM agent_notes WHERE agent_id = ?
		ORDER BY key ASC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// DeleteNote deletes a note by agent and key.
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
