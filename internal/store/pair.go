// ABOUTME: Pair token entity and store operations for QR-code device pairing.
// ABOUTME: Tokens are single-use, short-lived, and stored only as SHA-256 hex hashes.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrPairTokenUsed indicates the pair token was already consumed.
var ErrPairTokenUsed = errors.New("pair token already used")

// PairToken is a single-use QR pairing token. Only the SHA-256 hex digest of
// the token is stored; the plaintext never touches the database.
type PairToken struct {
	ID          string // UUID v4
	TokenHash   string // hex SHA-256 of the token
	CreatedBy   string // admin user ID that minted it
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time // nil until consumed
	PrincipalID *string    // set after enrollment
}

// PairTokenStore defines pair token persistence operations.
type PairTokenStore interface {
	// CreatePairToken stores a new token hash; the store generates the row ID.
	CreatePairToken(ctx context.Context, tokenHash, createdBy string, expiresAt time.Time) (*PairToken, error)
	// GetPairTokenByHash returns the token row for a hash, or ErrNotFound.
	GetPairTokenByHash(ctx context.Context, tokenHash string) (*PairToken, error)
	// ConsumePairToken claims the token; ErrPairTokenUsed if already claimed.
	ConsumePairToken(ctx context.Context, id string) error
	// SetPairTokenPrincipal records the enrolled principal on a token row.
	SetPairTokenPrincipal(ctx context.Context, id, principalID string) error
	// DeleteExpiredPairTokens removes expired tokens that were never used.
	DeleteExpiredPairTokens(ctx context.Context) error
}

// Ensure SQLiteStore implements PairTokenStore interface.
var _ PairTokenStore = (*SQLiteStore)(nil)

// CreatePairToken stores a new pair token hash and returns the created row.
func (s *SQLiteStore) CreatePairToken(ctx context.Context, tokenHash, createdBy string, expiresAt time.Time) (*PairToken, error) {
	pt := &PairToken{
		ID:        uuid.New().String(),
		TokenHash: tokenHash,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt.UTC(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pair_tokens (id, token_hash, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, pt.ID, pt.TokenHash, pt.CreatedBy,
		pt.CreatedAt.Format(time.RFC3339), pt.ExpiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("creating pair token: %w", err)
	}
	return pt, nil
}

// GetPairTokenByHash returns the token row matching a hex SHA-256 digest.
func (s *SQLiteStore) GetPairTokenByHash(ctx context.Context, tokenHash string) (*PairToken, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, token_hash, created_by, created_at, expires_at, used_at, principal_id
		FROM pair_tokens WHERE token_hash = ?
	`, tokenHash)
	return scanPairToken(row)
}

func scanPairToken(row *sql.Row) (*PairToken, error) {
	var pt PairToken
	var createdAt, expiresAt string
	var usedAt, principalID sql.NullString
	err := row.Scan(&pt.ID, &pt.TokenHash, &pt.CreatedBy, &createdAt, &expiresAt, &usedAt, &principalID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning pair token: %w", err)
	}
	pt.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parsing pair token created_at: %w", err)
	}
	pt.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parsing pair token expires_at: %w", err)
	}
	if usedAt.Valid {
		t, err := time.Parse(time.RFC3339, usedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing pair token used_at: %w", err)
		}
		pt.UsedAt = &t
	}
	if principalID.Valid {
		pt.PrincipalID = &principalID.String
	}
	return &pt, nil
}

// ConsumePairToken claims the token: it sets used_at if and only if the token
// is still unused. Exactly one concurrent caller succeeds; the rest get
// ErrPairTokenUsed. (An unknown id also reports ErrPairTokenUsed — callers
// look the row up by hash first.)
func (s *SQLiteStore) ConsumePairToken(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		UPDATE pair_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL
	`, now, id)
	if err != nil {
		return fmt.Errorf("consuming pair token: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("consuming pair token: %w", err)
	}
	if rowsAffected == 0 {
		return ErrPairTokenUsed
	}
	return nil
}

// SetPairTokenPrincipal records the enrolled principal after enrollment runs.
func (s *SQLiteStore) SetPairTokenPrincipal(ctx context.Context, id, principalID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE pair_tokens SET principal_id = ? WHERE id = ?
	`, principalID, id)
	if err != nil {
		return fmt.Errorf("setting pair token principal: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("setting pair token principal: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExpiredPairTokens removes expired tokens that were never used.
// Used tokens are kept: their principal_id linkage documents the enrollment.
func (s *SQLiteStore) DeleteExpiredPairTokens(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM pair_tokens WHERE expires_at <= ? AND used_at IS NULL
	`, now)
	if err != nil {
		return fmt.Errorf("deleting expired pair tokens: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		s.logger.Debug("deleted expired pair tokens", "count", rowsAffected)
	}
	return nil
}
