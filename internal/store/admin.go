// ABOUTME: Admin user, session, and invite types and store methods
// ABOUTME: Supports username/password auth and WebAuthn passkeys for admin UI

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAdminUserNotFound is returned when an admin user doesn't exist.
var ErrAdminUserNotFound = errors.New("admin user not found")

// ErrAdminSessionNotFound is returned when a session doesn't exist or is expired.
var ErrAdminSessionNotFound = errors.New("admin session not found")

// ErrAdminInviteNotFound is returned when an invite doesn't exist.
var ErrAdminInviteNotFound = errors.New("admin invite not found")

// ErrAdminInviteUsed is returned when trying to use an already-used invite.
var ErrAdminInviteUsed = errors.New("admin invite already used")

// ErrAdminInviteExpired is returned when an invite has expired.
var ErrAdminInviteExpired = errors.New("admin invite expired")

// ErrUsernameExists is returned when trying to create a user with an existing username.
var ErrUsernameExists = errors.New("username already exists")

// AdminUser represents an admin who can access the web UI.
type AdminUser struct {
	ID           string
	Username     string
	PasswordHash string // bcrypt hash, empty if passkey-only
	DisplayName  string
	CreatedAt    time.Time
}

// AdminSession represents an authenticated admin session.
type AdminSession struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// AdminInvite represents a signup invitation link.
type AdminInvite struct {
	ID        string
	CreatedBy string // user ID, empty for bootstrap invite
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	UsedBy    string // user ID who used the invite
}

// WebAuthnCredential represents a passkey credential.
type WebAuthnCredential struct {
	ID              string
	UserID          string
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	Transports      string // JSON array
	SignCount       uint32
	CreatedAt       time.Time
}

// AdminStore defines the interface for admin-related persistence.
type AdminStore interface {
	// Admin Users
	CreateAdminUser(ctx context.Context, user *AdminUser) error
	GetAdminUser(ctx context.Context, id string) (*AdminUser, error)
	GetAdminUserByUsername(ctx context.Context, username string) (*AdminUser, error)
	UpdateAdminUserPassword(ctx context.Context, id, passwordHash string) error
	ListAdminUsers(ctx context.Context) ([]*AdminUser, error)
	CountAdminUsers(ctx context.Context) (int, error)

	// Sessions
	CreateAdminSession(ctx context.Context, session *AdminSession) error
	GetAdminSession(ctx context.Context, id string) (*AdminSession, error)
	DeleteAdminSession(ctx context.Context, id string) error
	DeleteExpiredAdminSessions(ctx context.Context) error

	// Invites
	CreateAdminInvite(ctx context.Context, invite *AdminInvite) error
	GetAdminInvite(ctx context.Context, id string) (*AdminInvite, error)
	UseAdminInvite(ctx context.Context, inviteID, userID string) error

	// WebAuthn Credentials
	CreateWebAuthnCredential(ctx context.Context, cred *WebAuthnCredential) error
	GetWebAuthnCredentialsByUser(ctx context.Context, userID string) ([]*WebAuthnCredential, error)
	GetWebAuthnCredentialByCredentialID(ctx context.Context, credentialID []byte) (*WebAuthnCredential, error)
	UpdateWebAuthnCredentialSignCount(ctx context.Context, id string, signCount uint32) error
	DeleteWebAuthnCredential(ctx context.Context, id string) error
}

// Ensure SQLiteStore implements AdminStore.
var _ AdminStore = (*SQLiteStore)(nil)

// CreateAdminUser creates a new admin user.
func (s *SQLiteStore) CreateAdminUser(ctx context.Context, user *AdminUser) error {
	query := `
		INSERT INTO admin_users (id, username, password_hash, display_name, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		user.ID,
		user.Username,
		user.PasswordHash,
		user.DisplayName,
		user.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		// Check for unique constraint violation
		if isUniqueConstraintError(err) {
			return ErrUsernameExists
		}
		return fmt.Errorf("inserting admin user: %w", err)
	}

	s.logger.Info("created admin user", "id", user.ID, "username", user.Username)
	return nil
}

// GetAdminUser retrieves an admin user by ID.
func (s *SQLiteStore) GetAdminUser(ctx context.Context, id string) (*AdminUser, error) {
	query := `
		SELECT id, username, password_hash, display_name, created_at
		FROM admin_users
		WHERE id = ?
	`

	var user AdminUser
	var passwordHash sql.NullString
	var createdAtStr string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&passwordHash,
		&user.DisplayName,
		&createdAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAdminUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying admin user: %w", err)
	}

	user.PasswordHash = passwordHash.String
	user.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	return &user, nil
}

// GetAdminUserByUsername retrieves an admin user by username.
func (s *SQLiteStore) GetAdminUserByUsername(ctx context.Context, username string) (*AdminUser, error) {
	query := `
		SELECT id, username, password_hash, display_name, created_at
		FROM admin_users
		WHERE username = ?
	`

	var user AdminUser
	var passwordHash sql.NullString
	var createdAtStr string

	err := s.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&passwordHash,
		&user.DisplayName,
		&createdAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAdminUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying admin user by username: %w", err)
	}

	user.PasswordHash = passwordHash.String
	user.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	return &user, nil
}

// UpdateAdminUserPassword updates an admin user's password hash.
func (s *SQLiteStore) UpdateAdminUserPassword(ctx context.Context, id, passwordHash string) error {
	query := `UPDATE admin_users SET password_hash = ? WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, passwordHash, id)
	if err != nil {
		return fmt.Errorf("updating admin user password: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrAdminUserNotFound
	}

	s.logger.Info("updated admin user password", "id", id)
	return nil
}

// ListAdminUsers returns all admin users.
func (s *SQLiteStore) ListAdminUsers(ctx context.Context) ([]*AdminUser, error) {
	query := `
		SELECT id, username, password_hash, display_name, created_at
		FROM admin_users
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying admin users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*AdminUser
	for rows.Next() {
		var user AdminUser
		var passwordHash sql.NullString
		var createdAtStr string

		if err := rows.Scan(&user.ID, &user.Username, &passwordHash, &user.DisplayName, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scanning admin user: %w", err)
		}

		user.PasswordHash = passwordHash.String
		user.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating admin users: %w", err)
	}

	return users, nil
}

// CountAdminUsers returns the number of admin users.
func (s *SQLiteStore) CountAdminUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_users").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting admin users: %w", err)
	}
	return count, nil
}

// CreateAdminSession creates a new admin session.
func (s *SQLiteStore) CreateAdminSession(ctx context.Context, session *AdminSession) error {
	query := `
		INSERT INTO admin_sessions (id, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		session.ID,
		session.UserID,
		session.CreatedAt.UTC().Format(time.RFC3339),
		session.ExpiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting admin session: %w", err)
	}

	s.logger.Debug("created admin session", "id", session.ID, "user_id", session.UserID)
	return nil
}

// GetAdminSession retrieves a valid (non-expired) admin session.
func (s *SQLiteStore) GetAdminSession(ctx context.Context, id string) (*AdminSession, error) {
	query := `
		SELECT id, user_id, created_at, expires_at
		FROM admin_sessions
		WHERE id = ? AND expires_at > ?
	`

	var session AdminSession
	var createdAtStr, expiresAtStr string
	now := time.Now().UTC().Format(time.RFC3339)

	err := s.db.QueryRowContext(ctx, query, id, now).Scan(
		&session.ID,
		&session.UserID,
		&createdAtStr,
		&expiresAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAdminSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying admin session: %w", err)
	}

	session.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	session.ExpiresAt, err = time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing expires_at: %w", err)
	}

	return &session, nil
}

// DeleteAdminSession deletes an admin session.
func (s *SQLiteStore) DeleteAdminSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting admin session: %w", err)
	}
	return nil
}

// DeleteExpiredAdminSessions removes all expired sessions.
func (s *SQLiteStore) DeleteExpiredAdminSessions(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE expires_at <= ?", now)
	if err != nil {
		return fmt.Errorf("deleting expired sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		s.logger.Debug("deleted expired admin sessions", "count", rowsAffected)
	}
	return nil
}

// CreateAdminInvite creates a new admin invite.
func (s *SQLiteStore) CreateAdminInvite(ctx context.Context, invite *AdminInvite) error {
	query := `
		INSERT INTO admin_invites (id, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?)
	`

	var createdBy sql.NullString
	if invite.CreatedBy != "" {
		createdBy = sql.NullString{String: invite.CreatedBy, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, query,
		invite.ID,
		createdBy,
		invite.CreatedAt.UTC().Format(time.RFC3339),
		invite.ExpiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting admin invite: %w", err)
	}

	s.logger.Info("created admin invite", "id", invite.ID, "expires_at", invite.ExpiresAt)
	return nil
}

// GetAdminInvite retrieves an admin invite by ID.
func (s *SQLiteStore) GetAdminInvite(ctx context.Context, id string) (*AdminInvite, error) {
	query := `
		SELECT id, created_by, created_at, expires_at, used_at, used_by
		FROM admin_invites
		WHERE id = ?
	`

	var invite AdminInvite
	var createdBy, usedBy sql.NullString
	var createdAtStr, expiresAtStr string
	var usedAtStr sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&invite.ID,
		&createdBy,
		&createdAtStr,
		&expiresAtStr,
		&usedAtStr,
		&usedBy,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAdminInviteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying admin invite: %w", err)
	}

	invite.CreatedBy = createdBy.String
	invite.UsedBy = usedBy.String

	invite.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	invite.ExpiresAt, err = time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing expires_at: %w", err)
	}

	if usedAtStr.Valid {
		usedAt, err := time.Parse(time.RFC3339, usedAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("parsing used_at: %w", err)
		}
		invite.UsedAt = &usedAt
	}

	return &invite, nil
}

// UseAdminInvite atomically marks an invite as used by a user.
// This prevents race conditions where the same invite could be used twice.
// Returns ErrAdminInviteUsed if already used, ErrAdminInviteExpired if expired,
// or ErrAdminInviteNotFound if the invite doesn't exist.
func (s *SQLiteStore) UseAdminInvite(ctx context.Context, inviteID, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Atomic update: only succeeds if invite exists, is not used, and not expired
	// This prevents TOCTOU race conditions
	query := `
		UPDATE admin_invites
		SET used_at = ?, used_by = ?
		WHERE id = ?
		  AND used_at IS NULL
		  AND expires_at > ?
	`

	result, err := s.db.ExecContext(ctx, query, now, userID, inviteID, now)
	if err != nil {
		return fmt.Errorf("marking invite as used: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected > 0 {
		s.logger.Info("admin invite used", "invite_id", inviteID, "user_id", userID)
		return nil
	}

	// rowsAffected == 0 - need to determine why - check the invite
	invite, err := s.GetAdminInvite(ctx, inviteID)
	if errors.Is(err, ErrAdminInviteNotFound) {
		return ErrAdminInviteNotFound
	}
	if err != nil {
		return err
	}
	if invite.UsedAt != nil {
		return ErrAdminInviteUsed
	}
	if time.Now().After(invite.ExpiresAt) {
		return ErrAdminInviteExpired
	}

	// Shouldn't reach here, but just in case
	return ErrAdminInviteNotFound
}

// CreateWebAuthnCredential stores a new WebAuthn credential.
func (s *SQLiteStore) CreateWebAuthnCredential(ctx context.Context, cred *WebAuthnCredential) error {
	query := `
		INSERT INTO webauthn_credentials (id, user_id, credential_id, public_key, attestation_type, transports, sign_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		cred.ID,
		cred.UserID,
		cred.CredentialID,
		cred.PublicKey,
		cred.AttestationType,
		cred.Transports,
		cred.SignCount,
		cred.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting webauthn credential: %w", err)
	}

	s.logger.Info("created webauthn credential", "id", cred.ID, "user_id", cred.UserID)
	return nil
}

// GetWebAuthnCredentialsByUser retrieves all WebAuthn credentials for a user.
func (s *SQLiteStore) GetWebAuthnCredentialsByUser(ctx context.Context, userID string) ([]*WebAuthnCredential, error) {
	query := `
		SELECT id, user_id, credential_id, public_key, attestation_type, transports, sign_count, created_at
		FROM webauthn_credentials
		WHERE user_id = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("querying webauthn credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var creds []*WebAuthnCredential
	for rows.Next() {
		var cred WebAuthnCredential
		var createdAtStr string
		var transports sql.NullString

		if err := rows.Scan(
			&cred.ID,
			&cred.UserID,
			&cred.CredentialID,
			&cred.PublicKey,
			&cred.AttestationType,
			&transports,
			&cred.SignCount,
			&createdAtStr,
		); err != nil {
			return nil, fmt.Errorf("scanning webauthn credential: %w", err)
		}

		cred.Transports = transports.String
		cred.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}
		creds = append(creds, &cred)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating webauthn credentials: %w", err)
	}

	return creds, nil
}

// GetWebAuthnCredentialByCredentialID retrieves a WebAuthn credential by its credential ID.
func (s *SQLiteStore) GetWebAuthnCredentialByCredentialID(ctx context.Context, credentialID []byte) (*WebAuthnCredential, error) {
	query := `
		SELECT id, user_id, credential_id, public_key, attestation_type, transports, sign_count, created_at
		FROM webauthn_credentials
		WHERE credential_id = ?
	`

	var cred WebAuthnCredential
	var createdAtStr string
	var transports sql.NullString

	err := s.db.QueryRowContext(ctx, query, credentialID).Scan(
		&cred.ID,
		&cred.UserID,
		&cred.CredentialID,
		&cred.PublicKey,
		&cred.AttestationType,
		&transports,
		&cred.SignCount,
		&createdAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying webauthn credential: %w", err)
	}

	cred.Transports = transports.String
	cred.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	return &cred, nil
}

// UpdateWebAuthnCredentialSignCount updates the sign count for a credential.
func (s *SQLiteStore) UpdateWebAuthnCredentialSignCount(ctx context.Context, id string, signCount uint32) error {
	query := `UPDATE webauthn_credentials SET sign_count = ? WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, signCount, id)
	if err != nil {
		return fmt.Errorf("updating webauthn sign count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteWebAuthnCredential deletes a WebAuthn credential.
func (s *SQLiteStore) DeleteWebAuthnCredential(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM webauthn_credentials WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting webauthn credential: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	s.logger.Info("deleted webauthn credential", "id", id)
	return nil
}

// isUniqueConstraintError checks if an error is a unique constraint violation.
func isUniqueConstraintError(err error) bool {
	// SQLite returns "UNIQUE constraint failed" in the error message
	return err != nil && (strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "unique constraint"))
}
