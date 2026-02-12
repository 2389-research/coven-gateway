// ABOUTME: SQLite implementation of the Store interface using modernc.org/sqlite
// ABOUTME: Provides thread/message persistence with automatic schema creation

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements the Store interface using SQLite.
type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewSQLiteStore creates a new SQLite store at the given path.
// The schema is automatically created if it doesn't exist.
// Parent directories are created if needed.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	logger := slog.Default().With("component", "store")

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &SQLiteStore{
		db:     db,
		logger: logger,
	}

	if err := s.createSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	if err := s.runMigrations(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	logger.Info("SQLite store initialized", "path", path)
	return s, nil
}

// Schema segments split for maintainability.
var (
	schemaCoreSQL = `
CREATE TABLE IF NOT EXISTS threads (id TEXT PRIMARY KEY, frontend_name TEXT NOT NULL, external_id TEXT NOT NULL, agent_id TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS idx_threads_frontend_external ON threads(frontend_name, external_id);
CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, thread_id TEXT NOT NULL, sender TEXT NOT NULL, content TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'message', tool_name TEXT, tool_id TEXT, created_at DATETIME NOT NULL, FOREIGN KEY (thread_id) REFERENCES threads(id));
CREATE INDEX IF NOT EXISTS idx_messages_thread_id ON messages(thread_id);
CREATE INDEX IF NOT EXISTS idx_messages_thread_created ON messages(thread_id, created_at);
CREATE TABLE IF NOT EXISTS agent_state (agent_id TEXT PRIMARY KEY, state BLOB NOT NULL, updated_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS channel_bindings (frontend TEXT NOT NULL, channel_id TEXT NOT NULL, agent_id TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, PRIMARY KEY (frontend, channel_id));
`
	schemaAuthSQL = `
CREATE TABLE IF NOT EXISTS principals (principal_id TEXT PRIMARY KEY, type TEXT NOT NULL, pubkey_fingerprint TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, last_seen TEXT, metadata_json TEXT, CHECK (type IN ('client', 'agent', 'pack')), CHECK (status IN ('pending', 'approved', 'revoked', 'offline', 'online')));
CREATE INDEX IF NOT EXISTS idx_principals_status ON principals(status);
CREATE INDEX IF NOT EXISTS idx_principals_type ON principals(type);
CREATE INDEX IF NOT EXISTS idx_principals_pubkey ON principals(pubkey_fingerprint);
CREATE TABLE IF NOT EXISTS roles (subject_type TEXT NOT NULL, subject_id TEXT NOT NULL, role TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY (subject_type, subject_id, role), CHECK (subject_type IN ('principal', 'member')), CHECK (role IN ('owner', 'admin', 'member', 'leader')));
CREATE INDEX IF NOT EXISTS idx_roles_subject ON roles(subject_type, subject_id);
CREATE TABLE IF NOT EXISTS audit_log (audit_id TEXT PRIMARY KEY, actor_principal_id TEXT NOT NULL, actor_member_id TEXT, action TEXT NOT NULL, target_type TEXT NOT NULL, target_id TEXT NOT NULL, ts TEXT NOT NULL, detail_json TEXT, CHECK (action IN ('approve_principal', 'revoke_principal', 'grant_capability', 'revoke_capability', 'create_binding', 'update_binding', 'delete_binding', 'create_token', 'create_principal', 'delete_principal')));
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_log(ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_principal_id);
CREATE INDEX IF NOT EXISTS idx_audit_target ON audit_log(target_type, target_id);
`
	schemaLedgerSQL = `
CREATE TABLE IF NOT EXISTS ledger_events (event_id TEXT PRIMARY KEY, conversation_key TEXT NOT NULL, thread_id TEXT, direction TEXT NOT NULL, author TEXT NOT NULL, timestamp TEXT NOT NULL, type TEXT NOT NULL, text TEXT, raw_transport TEXT, raw_payload_ref TEXT, actor_principal_id TEXT, actor_member_id TEXT, CHECK (direction IN ('inbound_to_agent', 'outbound_from_agent')), CHECK (type IN ('message', 'tool_call', 'tool_result', 'system', 'error')));
CREATE INDEX IF NOT EXISTS idx_ledger_conversation ON ledger_events(conversation_key, timestamp);
CREATE INDEX IF NOT EXISTS idx_ledger_actor ON ledger_events(actor_principal_id);
CREATE INDEX IF NOT EXISTS idx_ledger_timestamp ON ledger_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_ledger_thread ON ledger_events(thread_id) WHERE thread_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS bindings (binding_id TEXT PRIMARY KEY, frontend TEXT NOT NULL, channel_id TEXT NOT NULL, agent_id TEXT NOT NULL, working_dir TEXT, created_at TEXT NOT NULL, created_by TEXT, UNIQUE(frontend, channel_id));
CREATE INDEX IF NOT EXISTS idx_bindings_frontend ON bindings(frontend);
CREATE INDEX IF NOT EXISTS idx_bindings_agent ON bindings(agent_id);
`
	schemaAdminSQL = `
CREATE TABLE IF NOT EXISTS admin_users (id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT, display_name TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_admin_users_username ON admin_users(username);
CREATE TABLE IF NOT EXISTS admin_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE, created_at TEXT NOT NULL, expires_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_user ON admin_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON admin_sessions(expires_at);
CREATE TABLE IF NOT EXISTS admin_invites (id TEXT PRIMARY KEY, created_by TEXT REFERENCES admin_users(id), created_at TEXT NOT NULL, expires_at TEXT NOT NULL, used_at TEXT, used_by TEXT REFERENCES admin_users(id));
CREATE INDEX IF NOT EXISTS idx_admin_invites_expires ON admin_invites(expires_at);
CREATE TABLE IF NOT EXISTS link_codes (id TEXT PRIMARY KEY, code TEXT UNIQUE NOT NULL, fingerprint TEXT NOT NULL, device_name TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'expired')), created_at TEXT NOT NULL, expires_at TEXT NOT NULL, approved_by TEXT REFERENCES admin_users(id), approved_at TEXT, principal_id TEXT REFERENCES principals(principal_id), token TEXT);
CREATE INDEX IF NOT EXISTS idx_link_codes_code ON link_codes(code);
CREATE INDEX IF NOT EXISTS idx_link_codes_expires ON link_codes(expires_at);
CREATE INDEX IF NOT EXISTS idx_link_codes_status ON link_codes(status);
CREATE TABLE IF NOT EXISTS webauthn_credentials (id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE, credential_id BLOB UNIQUE NOT NULL, public_key BLOB NOT NULL, attestation_type TEXT, transports TEXT, sign_count INTEGER DEFAULT 0, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_webauthn_user ON webauthn_credentials(user_id);
`
	schemaToolsSQL = `
CREATE TABLE IF NOT EXISTS log_entries (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, message TEXT NOT NULL, tags TEXT, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_log_entries_agent ON log_entries(agent_id);
CREATE INDEX IF NOT EXISTS idx_log_entries_created ON log_entries(created_at);
CREATE TABLE IF NOT EXISTS todos (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, description TEXT NOT NULL, status TEXT DEFAULT 'pending', priority TEXT DEFAULT 'medium', notes TEXT, due_date TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_todos_agent ON todos(agent_id);
CREATE INDEX IF NOT EXISTS idx_todos_status ON todos(status);
CREATE TABLE IF NOT EXISTS bbs_posts (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, thread_id TEXT, subject TEXT, content TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_bbs_posts_thread ON bbs_posts(thread_id);
CREATE INDEX IF NOT EXISTS idx_bbs_posts_created ON bbs_posts(created_at);
CREATE TABLE IF NOT EXISTS agent_mail (id TEXT PRIMARY KEY, from_agent_id TEXT NOT NULL, to_agent_id TEXT NOT NULL, subject TEXT NOT NULL, content TEXT NOT NULL, read_at TEXT, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_agent_mail_to ON agent_mail(to_agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_mail_unread ON agent_mail(to_agent_id, read_at);
CREATE TABLE IF NOT EXISTS agent_notes (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(agent_id, key));
CREATE INDEX IF NOT EXISTS idx_agent_notes_agent ON agent_notes(agent_id);
`
	schemaUsageSQL = `
CREATE TABLE IF NOT EXISTS message_usage (id TEXT PRIMARY KEY, thread_id TEXT NOT NULL, message_id TEXT, request_id TEXT NOT NULL, agent_id TEXT NOT NULL, input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0, thinking_tokens INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, FOREIGN KEY (thread_id) REFERENCES threads(id));
CREATE INDEX IF NOT EXISTS idx_message_usage_thread ON message_usage(thread_id);
CREATE INDEX IF NOT EXISTS idx_message_usage_agent ON message_usage(agent_id);
CREATE INDEX IF NOT EXISTS idx_message_usage_created ON message_usage(created_at);
CREATE INDEX IF NOT EXISTS idx_message_usage_request ON message_usage(request_id);
CREATE TABLE IF NOT EXISTS secrets (id TEXT PRIMARY KEY, key TEXT NOT NULL, value TEXT NOT NULL, agent_id TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, created_by TEXT);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_unique_global ON secrets(key) WHERE agent_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_unique_agent ON secrets(key, agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_secrets_agent ON secrets(agent_id);
`
)

// createSchema creates the database tables if they don't exist.
func (s *SQLiteStore) createSchema() error {
	schemas := []string{schemaCoreSQL, schemaAuthSQL, schemaLedgerSQL, schemaAdminSQL, schemaToolsSQL, schemaUsageSQL}
	for _, sql := range schemas {
		if _, err := s.db.Exec(sql); err != nil {
			return err
		}
	}
	return nil
}

// columnMigration defines a column migration with check and apply queries.
type columnMigration struct {
	check  string
	apply  string
	column string
	table  string
}

// applyColumnMigration applies a single column migration if needed.
func (s *SQLiteStore) applyColumnMigration(m columnMigration) error {
	var exists int
	if err := s.db.QueryRow(m.check).Scan(&exists); err == nil {
		return nil // Column already exists
	}
	if _, err := s.db.Exec(m.apply); err != nil {
		return fmt.Errorf("adding %s column to %s: %w", m.column, m.table, err)
	}
	s.logger.Info("applied migration", "column", m.column, "table", m.table)
	return nil
}

// migrateThreadIDColumn adds thread_id to ledger_events with its index.
func (s *SQLiteStore) migrateThreadIDColumn() error {
	migration := columnMigration{
		check:  `SELECT 1 FROM pragma_table_info('ledger_events') WHERE name = 'thread_id'`,
		apply:  `ALTER TABLE ledger_events ADD COLUMN thread_id TEXT`,
		column: "thread_id",
		table:  "ledger_events",
	}
	var exists int
	if err := s.db.QueryRow(migration.check).Scan(&exists); err == nil {
		return nil // Column already exists
	}
	if err := s.applyColumnMigration(migration); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_ledger_thread ON ledger_events(thread_id) WHERE thread_id IS NOT NULL`); err != nil {
		return fmt.Errorf("creating idx_ledger_thread index: %w", err)
	}
	s.logger.Info("applied migration", "index", "idx_ledger_thread", "table", "ledger_events")
	return nil
}

// runMigrations applies schema migrations for existing databases.
// These are idempotent - safe to run multiple times.
func (s *SQLiteStore) runMigrations() error {
	// Migration: Add tool-related columns to messages table
	messageMigrations := []columnMigration{
		{`SELECT 1 FROM pragma_table_info('messages') WHERE name = 'type'`, `ALTER TABLE messages ADD COLUMN type TEXT NOT NULL DEFAULT 'message'`, "type", "messages"},
		{`SELECT 1 FROM pragma_table_info('messages') WHERE name = 'tool_name'`, `ALTER TABLE messages ADD COLUMN tool_name TEXT`, "tool_name", "messages"},
		{`SELECT 1 FROM pragma_table_info('messages') WHERE name = 'tool_id'`, `ALTER TABLE messages ADD COLUMN tool_id TEXT`, "tool_id", "messages"},
		{`SELECT 1 FROM pragma_table_info('bindings') WHERE name = 'working_dir'`, `ALTER TABLE bindings ADD COLUMN working_dir TEXT`, "working_dir", "bindings"},
	}

	for _, m := range messageMigrations {
		if err := s.applyColumnMigration(m); err != nil {
			return err
		}
	}

	if err := s.migrateThreadIDColumn(); err != nil {
		return err
	}

	if err := s.migrateMessagesToEvents(); err != nil {
		return fmt.Errorf("migrating messages to events: %w", err)
	}

	if err := s.migrateConversationKeysToAgentID(); err != nil {
		return fmt.Errorf("migrating conversation keys to agent_id: %w", err)
	}

	return nil
}

// migrateMessagesToEvents copies existing messages from the messages table to ledger_events.
// This is idempotent - it only copies messages that don't already exist in ledger_events.
func (s *SQLiteStore) migrateMessagesToEvents() error {
	// Check how many messages need migration
	var messageCount, eventCount int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&messageCount)
	if err != nil {
		return fmt.Errorf("counting messages: %w", err)
	}

	// If no messages, skip migration
	if messageCount == 0 {
		return nil
	}

	// Count messages not yet in ledger_events
	err = s.db.QueryRow(`
		SELECT COUNT(*)
		FROM messages m
		WHERE NOT EXISTS (SELECT 1 FROM ledger_events WHERE event_id = m.id)
	`).Scan(&eventCount)
	if err != nil {
		return fmt.Errorf("counting pending migrations: %w", err)
	}

	if eventCount == 0 {
		return nil
	}

	s.logger.Info("migrating messages to ledger_events", "pending", eventCount, "total_messages", messageCount)

	// Migrate messages to events
	// Direction: agent:* senders -> outbound_from_agent, others -> inbound_to_agent
	// Type mapping: tool_use -> tool_call, tool_result -> tool_result, message -> message
	result, err := s.db.Exec(`
		INSERT INTO ledger_events (event_id, conversation_key, thread_id, direction, author, timestamp, type, text)
		SELECT
			m.id,
			'thread:' || m.thread_id,
			m.thread_id,
			CASE WHEN m.sender LIKE 'agent:%' THEN 'outbound_from_agent' ELSE 'inbound_to_agent' END,
			m.sender,
			m.created_at,
			CASE m.type
				WHEN 'tool_use' THEN 'tool_call'
				WHEN 'tool_result' THEN 'tool_result'
				ELSE 'message'
			END,
			m.content
		FROM messages m
		WHERE NOT EXISTS (SELECT 1 FROM ledger_events WHERE event_id = m.id)
	`)
	if err != nil {
		return fmt.Errorf("inserting events: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	s.logger.Info("migrated messages to ledger_events", "count", rowsAffected)

	return nil
}

// migrateConversationKeysToAgentID updates existing ledger_events that use
// "thread:{thread_id}" format to use agent_id directly. This enables cross-client
// history sync since TUI, web, and mobile all query by agent_id.
func (s *SQLiteStore) migrateConversationKeysToAgentID() error {
	// Count events that need migration (have thread: prefix and a linked thread)
	var needsMigration int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM ledger_events e
		JOIN threads t ON e.thread_id = t.id
		WHERE e.conversation_key LIKE 'thread:%'
	`).Scan(&needsMigration)
	if err != nil {
		return fmt.Errorf("counting events needing migration: %w", err)
	}

	if needsMigration == 0 {
		return nil
	}

	s.logger.Info("migrating conversation keys to agent_id", "count", needsMigration)

	// Update conversation_key to the agent_id from the linked thread
	result, err := s.db.Exec(`
		UPDATE ledger_events
		SET conversation_key = (
			SELECT t.agent_id
			FROM threads t
			WHERE t.id = ledger_events.thread_id
		)
		WHERE conversation_key LIKE 'thread:%'
		AND thread_id IS NOT NULL
		AND EXISTS (SELECT 1 FROM threads t WHERE t.id = ledger_events.thread_id)
	`)
	if err != nil {
		return fmt.Errorf("updating conversation keys: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	s.logger.Info("migrated conversation keys to agent_id", "count", rowsAffected)

	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	s.logger.Info("closing SQLite store")
	return s.db.Close()
}

// CreateThread creates a new thread in the database.
// If a thread with the same frontend_name and external_id already exists,
// it returns ErrDuplicateThread.
func (s *SQLiteStore) CreateThread(ctx context.Context, thread *Thread) error {
	query := `
		INSERT INTO threads (id, frontend_name, external_id, agent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		thread.ID,
		thread.FrontendName,
		thread.ExternalID,
		thread.AgentID,
		thread.CreatedAt.UTC().Format(time.RFC3339),
		thread.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		// Check for UNIQUE constraint violation
		if isConstraintViolation(err) {
			return ErrDuplicateThread
		}
		return fmt.Errorf("inserting thread: %w", err)
	}

	s.logger.Debug("created thread", "id", thread.ID, "frontend", thread.FrontendName)
	return nil
}

// isConstraintViolation checks if the error is a SQLite UNIQUE constraint violation.
func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "constraint failed")
}

// GetThread retrieves a thread by ID.
// Returns ErrNotFound if the thread doesn't exist.
func (s *SQLiteStore) GetThread(ctx context.Context, id string) (*Thread, error) {
	query := `
		SELECT id, frontend_name, external_id, agent_id, created_at, updated_at
		FROM threads
		WHERE id = ?
	`

	var thread Thread
	var createdAtStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&thread.ID,
		&thread.FrontendName,
		&thread.ExternalID,
		&thread.AgentID,
		&createdAtStr,
		&updatedAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying thread: %w", err)
	}

	thread.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	thread.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing updated_at: %w", err)
	}

	return &thread, nil
}

// GetThreadByFrontendID retrieves a thread by frontend name and external ID.
// This uses the idx_threads_frontend_external index for efficient lookups.
// Returns ErrNotFound if no thread exists for the given frontend/external ID combination.
func (s *SQLiteStore) GetThreadByFrontendID(ctx context.Context, frontendName, externalID string) (*Thread, error) {
	query := `
		SELECT id, frontend_name, external_id, agent_id, created_at, updated_at
		FROM threads
		WHERE frontend_name = ? AND external_id = ?
	`

	var thread Thread
	var createdAtStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx, query, frontendName, externalID).Scan(
		&thread.ID,
		&thread.FrontendName,
		&thread.ExternalID,
		&thread.AgentID,
		&createdAtStr,
		&updatedAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying thread by frontend ID: %w", err)
	}

	thread.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	thread.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing updated_at: %w", err)
	}

	return &thread, nil
}

// UpdateThread updates an existing thread.
// Returns ErrNotFound if the thread doesn't exist.
func (s *SQLiteStore) UpdateThread(ctx context.Context, thread *Thread) error {
	query := `
		UPDATE threads
		SET frontend_name = ?, external_id = ?, agent_id = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		thread.FrontendName,
		thread.ExternalID,
		thread.AgentID,
		thread.UpdatedAt.UTC().Format(time.RFC3339),
		thread.ID,
	)
	if err != nil {
		return fmt.Errorf("updating thread: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	s.logger.Debug("updated thread", "id", thread.ID)
	return nil
}

// ListThreads retrieves threads ordered by most recent activity.
// If limit is 0 or negative, a default limit of 100 is used.
func (s *SQLiteStore) ListThreads(ctx context.Context, limit int) ([]*Thread, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT id, frontend_name, external_id, agent_id, created_at, updated_at
		FROM threads
		ORDER BY updated_at DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying threads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var threads []*Thread
	for rows.Next() {
		var thread Thread
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(
			&thread.ID,
			&thread.FrontendName,
			&thread.ExternalID,
			&thread.AgentID,
			&createdAtStr,
			&updatedAtStr,
		); err != nil {
			return nil, fmt.Errorf("scanning thread row: %w", err)
		}

		thread.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}

		thread.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing updated_at: %w", err)
		}

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating thread rows: %w", err)
	}

	return threads, nil
}

// SaveMessage saves a message to the database.
func (s *SQLiteStore) SaveMessage(ctx context.Context, msg *Message) error {
	// Default to "message" type if not specified
	msgType := msg.Type
	if msgType == "" {
		msgType = MessageTypeMessage
	}

	query := `
		INSERT INTO messages (id, thread_id, sender, content, type, tool_name, tool_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		msg.ID,
		msg.ThreadID,
		msg.Sender,
		msg.Content,
		msgType,
		nullString(msg.ToolName),
		nullString(msg.ToolID),
		msg.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}

	s.logger.Debug("saved message", "id", msg.ID, "thread_id", msg.ThreadID, "type", msgType)
	return nil
}

// nullString returns nil for empty strings, otherwise the string pointer.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// GetThreadMessages retrieves messages for a thread, limited to the most recent `limit` messages.
// Messages are returned in chronological order (oldest first).
// If limit is 0 or negative, all messages are returned.
func (s *SQLiteStore) GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*Message, error) {
	var query string
	var args []any

	if limit > 0 {
		// Get the N most recent messages, but return them in chronological order
		// We use a subquery to get the most recent N, then order ascending
		query = `
			SELECT id, thread_id, sender, content, type, tool_name, tool_id, created_at
			FROM (
				SELECT id, thread_id, sender, content, type, tool_name, tool_id, created_at
				FROM messages
				WHERE thread_id = ?
				ORDER BY created_at DESC
				LIMIT ?
			)
			ORDER BY created_at ASC
		`
		args = []any{threadID, limit}
	} else {
		query = `
			SELECT id, thread_id, sender, content, type, tool_name, tool_id, created_at
			FROM messages
			WHERE thread_id = ?
			ORDER BY created_at ASC
		`
		args = []any{threadID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []*Message
	for rows.Next() {
		var msg Message
		var createdAtStr string
		var toolName, toolID *string

		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.Sender, &msg.Content, &msg.Type, &toolName, &toolID, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}

		msg.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing message created_at: %w", err)
		}

		// Handle nullable fields
		if toolName != nil {
			msg.ToolName = *toolName
		}
		if toolID != nil {
			msg.ToolID = *toolID
		}

		messages = append(messages, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}

	return messages, nil
}

// SaveAgentState saves or updates agent state.
// Uses INSERT OR REPLACE to handle both insert and update cases.
func (s *SQLiteStore) SaveAgentState(ctx context.Context, agentID string, state []byte) error {
	query := `
		INSERT OR REPLACE INTO agent_state (agent_id, state, updated_at)
		VALUES (?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		agentID,
		state,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("saving agent state: %w", err)
	}

	s.logger.Debug("saved agent state", "agent_id", agentID, "size", len(state))
	return nil
}

// GetAgentState retrieves agent state.
// Returns ErrNotFound if the agent has no saved state.
func (s *SQLiteStore) GetAgentState(ctx context.Context, agentID string) ([]byte, error) {
	query := `SELECT state FROM agent_state WHERE agent_id = ?`

	var state []byte
	err := s.db.QueryRowContext(ctx, query, agentID).Scan(&state)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying agent state: %w", err)
	}

	return state, nil
}

// CreateBinding creates a new channel binding.
// Returns an error if a binding already exists for this frontend/channel combination.
func (s *SQLiteStore) CreateBinding(ctx context.Context, binding *ChannelBinding) error {
	query := `
		INSERT INTO channel_bindings (frontend, channel_id, agent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		binding.FrontendName,
		binding.ChannelID,
		binding.AgentID,
		binding.CreatedAt.UTC().Format(time.RFC3339),
		binding.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting binding: %w", err)
	}

	s.logger.Debug("created binding", "frontend", binding.FrontendName, "channel", binding.ChannelID, "agent", binding.AgentID)
	return nil
}

// GetBinding retrieves a binding by frontend and channel ID.
// Returns ErrNotFound if no binding exists.
func (s *SQLiteStore) GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error) {
	query := `
		SELECT frontend, channel_id, agent_id, created_at, updated_at
		FROM channel_bindings
		WHERE frontend = ? AND channel_id = ?
	`

	var binding ChannelBinding
	var createdAtStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx, query, frontend, channelID).Scan(
		&binding.FrontendName,
		&binding.ChannelID,
		&binding.AgentID,
		&createdAtStr,
		&updatedAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying binding: %w", err)
	}

	binding.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	binding.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing updated_at: %w", err)
	}

	return &binding, nil
}

// ListBindings returns all channel bindings.
func (s *SQLiteStore) ListBindings(ctx context.Context) ([]*ChannelBinding, error) {
	query := `
		SELECT frontend, channel_id, agent_id, created_at, updated_at
		FROM channel_bindings
		ORDER BY frontend, channel_id
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bindings []*ChannelBinding
	for rows.Next() {
		var b ChannelBinding
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&b.FrontendName, &b.ChannelID, &b.AgentID, &createdAtStr, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("scanning binding: %w", err)
		}

		b.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}
		b.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing updated_at: %w", err)
		}
		bindings = append(bindings, &b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating binding rows: %w", err)
	}
	return bindings, nil
}

// DeleteBinding removes a channel binding.
// Returns ErrNotFound if the binding doesn't exist.
func (s *SQLiteStore) DeleteBinding(ctx context.Context, frontend, channelID string) error {
	query := `DELETE FROM channel_bindings WHERE frontend = ? AND channel_id = ?`

	result, err := s.db.ExecContext(ctx, query, frontend, channelID)
	if err != nil {
		return fmt.Errorf("deleting binding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	s.logger.Debug("deleted binding", "frontend", frontend, "channel", channelID)
	return nil
}

// Ensure SQLiteStore implements Store interface.
var _ Store = (*SQLiteStore)(nil)

// Ensure SQLiteStore implements LinkCodeStore interface.
var _ LinkCodeStore = (*SQLiteStore)(nil)

// CreateLinkCode creates a new pending link code.
func (s *SQLiteStore) CreateLinkCode(ctx context.Context, code *LinkCode) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO link_codes (id, code, fingerprint, device_name, status, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, code.ID, code.Code, code.Fingerprint, code.DeviceName, code.Status,
		code.CreatedAt.UTC().Format(time.RFC3339),
		code.ExpiresAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("creating link code: %w", err)
	}
	s.logger.Debug("created link code", "id", code.ID, "code", code.Code)
	return nil
}

// GetLinkCode retrieves a link code by ID.
func (s *SQLiteStore) GetLinkCode(ctx context.Context, id string) (*LinkCode, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, code, fingerprint, device_name, status, created_at, expires_at,
		       approved_by, approved_at, principal_id, token
		FROM link_codes WHERE id = ?
	`, id)
	return s.scanLinkCode(row)
}

// GetLinkCodeByCode retrieves a link code by its short code.
func (s *SQLiteStore) GetLinkCodeByCode(ctx context.Context, code string) (*LinkCode, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, code, fingerprint, device_name, status, created_at, expires_at,
		       approved_by, approved_at, principal_id, token
		FROM link_codes WHERE code = ?
	`, code)
	return s.scanLinkCode(row)
}

func (s *SQLiteStore) scanLinkCode(row *sql.Row) (*LinkCode, error) {
	var lc LinkCode
	var createdAt, expiresAt string
	var approvedBy, approvedAt, principalID, token sql.NullString

	err := row.Scan(&lc.ID, &lc.Code, &lc.Fingerprint, &lc.DeviceName, &lc.Status,
		&createdAt, &expiresAt, &approvedBy, &approvedAt, &principalID, &token)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning link code: %w", err)
	}

	var parseErr error
	lc.CreatedAt, parseErr = time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parsing created_at: %w", parseErr)
	}
	lc.ExpiresAt, parseErr = time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parsing expires_at: %w", parseErr)
	}

	if approvedBy.Valid {
		lc.ApprovedBy = &approvedBy.String
	}
	if approvedAt.Valid {
		t, parseErr := time.Parse(time.RFC3339, approvedAt.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing approved_at: %w", parseErr)
		}
		lc.ApprovedAt = &t
	}
	if principalID.Valid {
		lc.PrincipalID = &principalID.String
	}
	if token.Valid {
		lc.Token = &token.String
	}

	return &lc, nil
}

// ApproveLinkCode marks a code as approved and stores the principal/token.
func (s *SQLiteStore) ApproveLinkCode(ctx context.Context, id string, approvedBy string, principalID string, token string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		UPDATE link_codes
		SET status = ?, approved_by = ?, approved_at = ?, principal_id = ?, token = ?
		WHERE id = ? AND status = ?
	`, LinkCodeStatusApproved, approvedBy, now, principalID, token, id, LinkCodeStatusPending)
	if err != nil {
		return fmt.Errorf("approving link code: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	s.logger.Info("approved link code", "id", id, "approved_by", approvedBy)
	return nil
}

// ListPendingLinkCodes returns all pending (non-expired) link codes.
func (s *SQLiteStore) ListPendingLinkCodes(ctx context.Context) ([]*LinkCode, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, code, fingerprint, device_name, status, created_at, expires_at,
		       approved_by, approved_at, principal_id, token
		FROM link_codes
		WHERE status = ? AND expires_at > ?
		ORDER BY created_at DESC
	`, LinkCodeStatusPending, now)
	if err != nil {
		return nil, fmt.Errorf("listing pending link codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var codes []*LinkCode
	for rows.Next() {
		var lc LinkCode
		var createdAt, expiresAt string
		var approvedBy, approvedAt, principalID, token sql.NullString

		err := rows.Scan(&lc.ID, &lc.Code, &lc.Fingerprint, &lc.DeviceName, &lc.Status,
			&createdAt, &expiresAt, &approvedBy, &approvedAt, &principalID, &token)
		if err != nil {
			return nil, fmt.Errorf("scanning link code row: %w", err)
		}

		lc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		lc.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
		codes = append(codes, &lc)
	}
	return codes, rows.Err()
}

// DeleteExpiredLinkCodes removes expired link codes.
func (s *SQLiteStore) DeleteExpiredLinkCodes(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM link_codes WHERE expires_at <= ? AND status = ?
	`, now, LinkCodeStatusPending)
	if err != nil {
		return fmt.Errorf("deleting expired link codes: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		s.logger.Debug("deleted expired link codes", "count", rowsAffected)
	}
	return nil
}
