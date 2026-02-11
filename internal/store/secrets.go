// ABOUTME: Secrets store implementation for managing agent environment variables
// ABOUTME: Supports global defaults with per-agent overrides

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Secret represents an environment variable that can be pushed to agents.
// If AgentID is nil, this is a global default. If set, it's an agent-specific override.
type Secret struct {
	ID        string
	Key       string
	Value     string
	AgentID   *string // nil = global default
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy *string
}

// SecretsStore defines methods for managing secrets.
type SecretsStore interface {
	CreateSecret(ctx context.Context, secret *Secret) error
	GetSecret(ctx context.Context, id string) (*Secret, error)
	UpdateSecret(ctx context.Context, secret *Secret) error
	DeleteSecret(ctx context.Context, id string) error
	ListAllSecrets(ctx context.Context) ([]*Secret, error)
	GetEffectiveSecrets(ctx context.Context, agentID string) (map[string]string, error)
}

// CreateSecret creates a new secret in the database.
// Returns an error if a secret with the same key and scope already exists.
func (s *SQLiteStore) CreateSecret(ctx context.Context, secret *Secret) error {
	if secret.ID == "" {
		secret.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if secret.CreatedAt.IsZero() {
		secret.CreatedAt = now
	}
	if secret.UpdatedAt.IsZero() {
		secret.UpdatedAt = now
	}

	query := `
		INSERT INTO secrets (id, key, value, agent_id, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		secret.ID,
		secret.Key,
		secret.Value,
		nullString(ptrToString(secret.AgentID)),
		secret.CreatedAt.Format(time.RFC3339),
		secret.UpdatedAt.Format(time.RFC3339),
		nullString(ptrToString(secret.CreatedBy)),
	)
	if err != nil {
		if isConstraintViolation(err) {
			return fmt.Errorf("secret with key %q already exists for this scope", secret.Key)
		}
		return fmt.Errorf("inserting secret: %w", err)
	}

	s.logger.Debug("created secret", "id", secret.ID, "key", secret.Key, "agent_id", secret.AgentID)
	return nil
}

// GetSecret retrieves a secret by ID.
// Returns ErrNotFound if the secret doesn't exist.
func (s *SQLiteStore) GetSecret(ctx context.Context, id string) (*Secret, error) {
	query := `
		SELECT id, key, value, agent_id, created_at, updated_at, created_by
		FROM secrets
		WHERE id = ?
	`

	var secret Secret
	var agentID, createdBy sql.NullString
	var createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&secret.ID,
		&secret.Key,
		&secret.Value,
		&agentID,
		&createdAt,
		&updatedAt,
		&createdBy,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying secret: %w", err)
	}

	if parsed, err := time.Parse(time.RFC3339, createdAt); err != nil {
		slog.Warn("failed to parse secret created_at", "id", secret.ID, "error", err)
	} else {
		secret.CreatedAt = parsed
	}
	if parsed, err := time.Parse(time.RFC3339, updatedAt); err != nil {
		slog.Warn("failed to parse secret updated_at", "id", secret.ID, "error", err)
	} else {
		secret.UpdatedAt = parsed
	}
	if agentID.Valid {
		secret.AgentID = &agentID.String
	}
	if createdBy.Valid {
		secret.CreatedBy = &createdBy.String
	}

	return &secret, nil
}

// UpdateSecret updates an existing secret's value.
// Returns ErrNotFound if the secret doesn't exist.
func (s *SQLiteStore) UpdateSecret(ctx context.Context, secret *Secret) error {
	secret.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE secrets
		SET value = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		secret.Value,
		secret.UpdatedAt.Format(time.RFC3339),
		secret.ID,
	)
	if err != nil {
		return fmt.Errorf("updating secret: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	s.logger.Debug("updated secret", "id", secret.ID, "key", secret.Key)
	return nil
}

// DeleteSecret removes a secret by ID.
// Returns ErrNotFound if the secret doesn't exist.
func (s *SQLiteStore) DeleteSecret(ctx context.Context, id string) error {
	query := `DELETE FROM secrets WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting secret: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	s.logger.Debug("deleted secret", "id", id)
	return nil
}

// ListAllSecrets returns all secrets (both global and agent-specific).
// Secrets are ordered by key, then by scope (global first, then agent-specific).
func (s *SQLiteStore) ListAllSecrets(ctx context.Context) ([]*Secret, error) {
	query := `
		SELECT id, key, value, agent_id, created_at, updated_at, created_by
		FROM secrets
		ORDER BY key, agent_id NULLS FIRST
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var secrets []*Secret
	for rows.Next() {
		var secret Secret
		var agentID, createdBy sql.NullString
		var createdAt, updatedAt string

		if err := rows.Scan(
			&secret.ID,
			&secret.Key,
			&secret.Value,
			&agentID,
			&createdAt,
			&updatedAt,
			&createdBy,
		); err != nil {
			return nil, fmt.Errorf("scanning secret row: %w", err)
		}

		if parsed, err := time.Parse(time.RFC3339, createdAt); err != nil {
			slog.Warn("failed to parse secret created_at", "id", secret.ID, "error", err)
		} else {
			secret.CreatedAt = parsed
		}
		if parsed, err := time.Parse(time.RFC3339, updatedAt); err != nil {
			slog.Warn("failed to parse secret updated_at", "id", secret.ID, "error", err)
		} else {
			secret.UpdatedAt = parsed
		}
		if agentID.Valid {
			secret.AgentID = &agentID.String
		}
		if createdBy.Valid {
			secret.CreatedBy = &createdBy.String
		}

		secrets = append(secrets, &secret)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating secret rows: %w", err)
	}

	return secrets, nil
}

// GetEffectiveSecrets returns the resolved secrets for a specific agent.
// Agent-specific overrides take precedence over global defaults.
// Returns a map of key -> value suitable for setting as environment variables.
func (s *SQLiteStore) GetEffectiveSecrets(ctx context.Context, agentID string) (map[string]string, error) {
	// Query all secrets relevant to this agent (global + agent-specific)
	// Use a CTE to merge and resolve overrides
	query := `
		WITH merged AS (
			SELECT key, value,
				   CASE WHEN agent_id IS NOT NULL THEN 1 ELSE 0 END as priority
			FROM secrets
			WHERE agent_id IS NULL OR agent_id = ?
		)
		SELECT key, value FROM merged m1
		WHERE priority = (SELECT MAX(priority) FROM merged m2 WHERE m2.key = m1.key)
	`

	rows, err := s.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("querying effective secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	secrets := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scanning effective secret: %w", err)
		}
		secrets[key] = value
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating effective secrets: %w", err)
	}

	return secrets, nil
}

// ptrToString returns the dereferenced string or empty string if nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Ensure SQLiteStore implements SecretsStore.
var _ SecretsStore = (*SQLiteStore)(nil)
