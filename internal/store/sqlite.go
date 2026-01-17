// ABOUTME: SQLite implementation of the Store interface using modernc.org/sqlite
// ABOUTME: Provides thread/message persistence with automatic schema creation

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements the Store interface using SQLite
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &SQLiteStore{
		db:     db,
		logger: logger,
	}

	if err := s.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	logger.Info("SQLite store initialized", "path", path)
	return s, nil
}

// createSchema creates the database tables if they don't exist
func (s *SQLiteStore) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS threads (
			id TEXT PRIMARY KEY,
			frontend_name TEXT NOT NULL,
			external_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_threads_frontend_external
			ON threads(frontend_name, external_id);

		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			sender TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (thread_id) REFERENCES threads(id)
		);

		CREATE INDEX IF NOT EXISTS idx_messages_thread_id
			ON messages(thread_id);

		CREATE INDEX IF NOT EXISTS idx_messages_thread_created
			ON messages(thread_id, created_at);

		CREATE TABLE IF NOT EXISTS agent_state (
			agent_id TEXT PRIMARY KEY,
			state BLOB NOT NULL,
			updated_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS channel_bindings (
			frontend TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (frontend, channel_id)
		);

		CREATE TABLE IF NOT EXISTS principals (
			principal_id       TEXT PRIMARY KEY,
			type               TEXT NOT NULL,
			pubkey_fingerprint TEXT NOT NULL UNIQUE,
			display_name       TEXT NOT NULL,
			status             TEXT NOT NULL,
			created_at         TEXT NOT NULL,
			last_seen          TEXT,
			metadata_json      TEXT,

			CHECK (type IN ('client', 'agent', 'pack')),
			CHECK (status IN ('pending', 'approved', 'revoked', 'offline', 'online'))
		);

		CREATE INDEX IF NOT EXISTS idx_principals_status ON principals(status);
		CREATE INDEX IF NOT EXISTS idx_principals_type ON principals(type);
		CREATE INDEX IF NOT EXISTS idx_principals_pubkey ON principals(pubkey_fingerprint);

		CREATE TABLE IF NOT EXISTS roles (
			subject_type TEXT NOT NULL,
			subject_id   TEXT NOT NULL,
			role         TEXT NOT NULL,
			created_at   TEXT NOT NULL,

			PRIMARY KEY (subject_type, subject_id, role),
			CHECK (subject_type IN ('principal', 'member')),
			CHECK (role IN ('owner', 'admin', 'member'))
		);

		CREATE INDEX IF NOT EXISTS idx_roles_subject ON roles(subject_type, subject_id);

		CREATE TABLE IF NOT EXISTS audit_log (
			audit_id           TEXT PRIMARY KEY,
			actor_principal_id TEXT NOT NULL,
			actor_member_id    TEXT,
			action             TEXT NOT NULL,
			target_type        TEXT NOT NULL,
			target_id          TEXT NOT NULL,
			ts                 TEXT NOT NULL,
			detail_json        TEXT,

			CHECK (action IN (
				'approve_principal',
				'revoke_principal',
				'grant_capability',
				'revoke_capability',
				'create_binding',
				'update_binding',
				'delete_binding',
				'create_token'
			))
		);

		CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_log(ts DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_principal_id);
		CREATE INDEX IF NOT EXISTS idx_audit_target ON audit_log(target_type, target_id);

		CREATE TABLE IF NOT EXISTS ledger_events (
			event_id           TEXT PRIMARY KEY,
			conversation_key   TEXT NOT NULL,
			direction          TEXT NOT NULL,
			author             TEXT NOT NULL,
			timestamp          TEXT NOT NULL,
			type               TEXT NOT NULL,
			text               TEXT,
			raw_transport      TEXT,
			raw_payload_ref    TEXT,
			actor_principal_id TEXT,
			actor_member_id    TEXT,

			CHECK (direction IN ('inbound_to_agent', 'outbound_from_agent')),
			CHECK (type IN ('message', 'tool_call', 'tool_result', 'system', 'error'))
		);

		CREATE INDEX IF NOT EXISTS idx_ledger_conversation ON ledger_events(conversation_key, timestamp);
		CREATE INDEX IF NOT EXISTS idx_ledger_actor ON ledger_events(actor_principal_id);
		CREATE INDEX IF NOT EXISTS idx_ledger_timestamp ON ledger_events(timestamp);

		CREATE TABLE IF NOT EXISTS bindings (
			binding_id TEXT PRIMARY KEY,
			frontend   TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			agent_id   TEXT NOT NULL,
			created_at TEXT NOT NULL,
			created_by TEXT,

			UNIQUE(frontend, channel_id)
		);

		CREATE INDEX IF NOT EXISTS idx_bindings_frontend ON bindings(frontend);
		CREATE INDEX IF NOT EXISTS idx_bindings_agent ON bindings(agent_id);

		-- Admin users (humans who manage the system via web UI)
		CREATE TABLE IF NOT EXISTS admin_users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			display_name TEXT NOT NULL,
			created_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_admin_users_username ON admin_users(username);

		-- Admin sessions (cookie-based)
		CREATE TABLE IF NOT EXISTS admin_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_admin_sessions_user ON admin_sessions(user_id);
		CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON admin_sessions(expires_at);

		-- Admin invite links (for signup)
		CREATE TABLE IF NOT EXISTS admin_invites (
			id TEXT PRIMARY KEY,
			created_by TEXT REFERENCES admin_users(id),
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			used_at TEXT,
			used_by TEXT REFERENCES admin_users(id)
		);

		CREATE INDEX IF NOT EXISTS idx_admin_invites_expires ON admin_invites(expires_at);

		-- WebAuthn credentials for passkeys
		CREATE TABLE IF NOT EXISTS webauthn_credentials (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
			credential_id BLOB UNIQUE NOT NULL,
			public_key BLOB NOT NULL,
			attestation_type TEXT,
			transports TEXT,
			sign_count INTEGER DEFAULT 0,
			created_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_webauthn_user ON webauthn_credentials(user_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	s.logger.Info("closing SQLite store")
	return s.db.Close()
}

// CreateThread creates a new thread in the database
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
		return fmt.Errorf("inserting thread: %w", err)
	}

	s.logger.Debug("created thread", "id", thread.ID, "frontend", thread.FrontendName)
	return nil
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

	if err == sql.ErrNoRows {
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

	if err == sql.ErrNoRows {
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
	defer rows.Close()

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

// SaveMessage saves a message to the database
func (s *SQLiteStore) SaveMessage(ctx context.Context, msg *Message) error {
	query := `
		INSERT INTO messages (id, thread_id, sender, content, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		msg.ID,
		msg.ThreadID,
		msg.Sender,
		msg.Content,
		msg.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}

	s.logger.Debug("saved message", "id", msg.ID, "thread_id", msg.ThreadID)
	return nil
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
			SELECT id, thread_id, sender, content, created_at
			FROM (
				SELECT id, thread_id, sender, content, created_at
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
			SELECT id, thread_id, sender, content, created_at
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
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var msg Message
		var createdAtStr string

		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.Sender, &msg.Content, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}

		msg.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing message created_at: %w", err)
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

	if err == sql.ErrNoRows {
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
	defer rows.Close()

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

// Ensure SQLiteStore implements Store interface
var _ Store = (*SQLiteStore)(nil)
