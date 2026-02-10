// ABOUTME: Binding entity and store methods for channel-to-agent mapping
// ABOUTME: Bindings map (frontend, channel_id) to agent_id for message routing

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Binding errors.
var (
	ErrBindingNotFound  = errors.New("binding not found")
	ErrDuplicateChannel = errors.New("duplicate frontend+channel_id combination")
	ErrAgentNotFound    = errors.New("agent not found or not of type agent")
)

// Binding represents a channel-to-agent mapping for message routing.
type Binding struct {
	ID         string    // UUID v4
	Frontend   string    // "matrix", "slack", "telegram" (1-50 chars, lowercase alphanumeric + underscore)
	ChannelID  string    // room/channel identifier (1-500 chars)
	AgentID    string    // principal_id of agent
	WorkingDir string    // filesystem path where the agent operates (optional, empty string if not set)
	CreatedAt  time.Time // when the binding was created
	CreatedBy  *string   // principal_id who created it (optional)
}

// BindingFilter specifies filtering options for listing bindings.
type BindingFilter struct {
	Frontend *string // filter by frontend name
	AgentID  *string // filter by agent ID
}

// validateAgent checks that the given ID exists in principals with type='agent'.
func (s *SQLiteStore) validateAgent(ctx context.Context, agentID string) error {
	query := `SELECT type FROM principals WHERE principal_id = ?`

	var principalType string
	err := s.db.QueryRowContext(ctx, query, agentID).Scan(&principalType)
	if err == sql.ErrNoRows {
		return ErrAgentNotFound
	}
	if err != nil {
		return fmt.Errorf("checking agent: %w", err)
	}

	if principalType != string(PrincipalTypeAgent) {
		return ErrAgentNotFound
	}

	return nil
}

// CreateBindingV2 creates a new channel binding in the database.
// The agent_id must exist in principals with type='agent'.
// Named V2 to distinguish from legacy CreateBinding method.
func (s *SQLiteStore) CreateBindingV2(ctx context.Context, b *Binding) error {
	// Validate that the agent exists and is of type agent
	if err := s.validateAgent(ctx, b.AgentID); err != nil {
		return err
	}

	query := `
		INSERT INTO bindings (binding_id, frontend, channel_id, agent_id, working_dir, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	// Convert empty string to NULL for working_dir
	var workingDir any
	if b.WorkingDir != "" {
		workingDir = b.WorkingDir
	}

	_, err := s.db.ExecContext(ctx, query,
		b.ID,
		b.Frontend,
		b.ChannelID,
		b.AgentID,
		workingDir,
		b.CreatedAt.UTC().Format(time.RFC3339),
		b.CreatedBy,
	)
	if err != nil {
		if isDuplicateChannelError(err) {
			return ErrDuplicateChannel
		}
		return fmt.Errorf("inserting binding: %w", err)
	}

	s.logger.Debug("created binding", "id", b.ID, "frontend", b.Frontend, "channel", b.ChannelID)
	return nil
}

// GetBindingByID retrieves a binding by its ID.
func (s *SQLiteStore) GetBindingByID(ctx context.Context, id string) (*Binding, error) {
	query := `
		SELECT binding_id, frontend, channel_id, agent_id, working_dir, created_at, created_by
		FROM bindings
		WHERE binding_id = ?
	`

	return s.scanBinding(s.db.QueryRowContext(ctx, query, id))
}

// GetBindingByChannel retrieves a binding by frontend and channel_id.
func (s *SQLiteStore) GetBindingByChannel(ctx context.Context, frontend, channelID string) (*Binding, error) {
	query := `
		SELECT binding_id, frontend, channel_id, agent_id, working_dir, created_at, created_by
		FROM bindings
		WHERE frontend = ? AND channel_id = ?
	`

	return s.scanBinding(s.db.QueryRowContext(ctx, query, frontend, channelID))
}

// UpdateBinding updates a binding's agent_id.
// The new agent_id must exist in principals with type='agent'.
func (s *SQLiteStore) UpdateBinding(ctx context.Context, id, agentID string) error {
	// Validate that the new agent exists
	if err := s.validateAgent(ctx, agentID); err != nil {
		return err
	}

	query := `UPDATE bindings SET agent_id = ? WHERE binding_id = ?`

	result, err := s.db.ExecContext(ctx, query, agentID, id)
	if err != nil {
		return fmt.Errorf("updating binding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrBindingNotFound
	}

	s.logger.Debug("updated binding", "id", id, "agent_id", agentID)
	return nil
}

// DeleteBindingByID deletes a binding by its ID.
func (s *SQLiteStore) DeleteBindingByID(ctx context.Context, id string) error {
	query := `DELETE FROM bindings WHERE binding_id = ?`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting binding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrBindingNotFound
	}

	s.logger.Debug("deleted binding", "id", id)
	return nil
}

// DeleteBindingByChannel deletes a binding by frontend and channel_id.
func (s *SQLiteStore) DeleteBindingByChannel(ctx context.Context, frontend, channelID string) error {
	query := `DELETE FROM bindings WHERE frontend = ? AND channel_id = ?`

	result, err := s.db.ExecContext(ctx, query, frontend, channelID)
	if err != nil {
		return fmt.Errorf("deleting binding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrBindingNotFound
	}

	s.logger.Debug("deleted binding", "frontend", frontend, "channel_id", channelID)
	return nil
}

// ListBindingsV2 returns bindings matching the filter criteria.
// Named V2 to avoid collision with existing ListBindings method.
func (s *SQLiteStore) ListBindingsV2(ctx context.Context, f BindingFilter) ([]Binding, error) {
	query := `
		SELECT binding_id, frontend, channel_id, agent_id, working_dir, created_at, created_by
		FROM bindings
		WHERE (? IS NULL OR frontend = ?)
		  AND (? IS NULL OR agent_id = ?)
		ORDER BY created_at DESC
	`

	var frontendFilter, agentFilter *string
	if f.Frontend != nil {
		frontendFilter = f.Frontend
	}
	if f.AgentID != nil {
		agentFilter = f.AgentID
	}

	rows, err := s.db.QueryContext(ctx, query,
		frontendFilter, frontendFilter,
		agentFilter, agentFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("querying bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bindings []Binding
	for rows.Next() {
		b, err := s.scanBindingRow(rows)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, *b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating binding rows: %w", err)
	}

	return bindings, nil
}

// scanBinding scans a single binding row from a sql.Row.
func (s *SQLiteStore) scanBinding(row *sql.Row) (*Binding, error) {
	var b Binding
	var createdAtStr string
	var createdBy *string
	var workingDir sql.NullString

	err := row.Scan(
		&b.ID,
		&b.Frontend,
		&b.ChannelID,
		&b.AgentID,
		&workingDir,
		&createdAtStr,
		&createdBy,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning binding: %w", err)
	}

	b.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	b.CreatedBy = createdBy
	if workingDir.Valid {
		b.WorkingDir = workingDir.String
	}

	return &b, nil
}

// scanBindingRow scans a binding from sql.Rows (for list queries).
func (s *SQLiteStore) scanBindingRow(rows *sql.Rows) (*Binding, error) {
	var b Binding
	var createdAtStr string
	var createdBy *string
	var workingDir sql.NullString

	err := rows.Scan(
		&b.ID,
		&b.Frontend,
		&b.ChannelID,
		&b.AgentID,
		&workingDir,
		&createdAtStr,
		&createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning binding row: %w", err)
	}

	b.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	b.CreatedBy = createdBy
	if workingDir.Valid {
		b.WorkingDir = workingDir.String
	}

	return &b, nil
}

// isDuplicateChannelError checks if the error is a unique constraint violation
// for the frontend+channel_id combination.
func isDuplicateChannelError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE constraint failed") &&
		(strings.Contains(errStr, "bindings.frontend") ||
			strings.Contains(errStr, "frontend, channel_id"))
}

// UpdateBindingsByWorkspace updates all bindings whose agent_id ends with "_<workspace>"
// to point to the new agent ID. This is used during agent registration to automatically
// update bindings when an agent reconnects with a different prefix (e.g., when a device
// reconnects with a new prefix like "magic_notes" instead of "m_notes").
// Returns the number of bindings updated.
func (s *SQLiteStore) UpdateBindingsByWorkspace(ctx context.Context, workspace, newAgentID string) (int64, error) {
	// Match agent_ids that end with "_<workspace>"
	// e.g., workspace="notes" matches "m_notes", "magic_notes", etc.
	pattern := "%_" + workspace

	query := `UPDATE bindings SET agent_id = ? WHERE agent_id LIKE ? AND agent_id != ?`

	result, err := s.db.ExecContext(ctx, query, newAgentID, pattern, newAgentID)
	if err != nil {
		return 0, fmt.Errorf("updating bindings by workspace: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected > 0 {
		s.logger.Info("updated bindings for workspace",
			"workspace", workspace,
			"new_agent_id", newAgentID,
			"count", rowsAffected,
		)
	}

	return rowsAffected, nil
}
