// ABOUTME: Role entity and store methods for authorization
// ABOUTME: Roles grant capabilities to subjects (principals or members)

package store

import (
	"context"
	"fmt"
	"time"
)

// RoleSubjectType represents the type of subject that can have roles
type RoleSubjectType string

const (
	RoleSubjectPrincipal RoleSubjectType = "principal"
	RoleSubjectMember    RoleSubjectType = "member"
)

// RoleName represents a role that can be assigned
type RoleName string

const (
	RoleOwner  RoleName = "owner"
	RoleAdmin  RoleName = "admin"
	RoleMember RoleName = "member"
	RoleLeader RoleName = "leader"
)

// ValidRoleNames lists all valid role names
var ValidRoleNames = []RoleName{
	RoleOwner,
	RoleAdmin,
	RoleMember,
	RoleLeader,
}

// Role represents a role assignment to a subject
type Role struct {
	SubjectType RoleSubjectType
	SubjectID   string
	Role        RoleName
	CreatedAt   time.Time
}

// AddRole adds a role to a subject. This operation is idempotent - adding an
// existing role succeeds silently.
func (s *SQLiteStore) AddRole(ctx context.Context, subjectType RoleSubjectType, subjectID string, role RoleName) error {
	query := `
		INSERT OR IGNORE INTO roles (subject_type, subject_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		subjectType,
		subjectID,
		role,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("adding role: %w", err)
	}

	s.logger.Debug("added role", "subject_type", subjectType, "subject_id", subjectID, "role", role)
	return nil
}

// RemoveRole removes a role from a subject. This operation is idempotent -
// removing a non-existent role succeeds silently.
func (s *SQLiteStore) RemoveRole(ctx context.Context, subjectType RoleSubjectType, subjectID string, role RoleName) error {
	query := `DELETE FROM roles WHERE subject_type = ? AND subject_id = ? AND role = ?`

	_, err := s.db.ExecContext(ctx, query, subjectType, subjectID, role)
	if err != nil {
		return fmt.Errorf("removing role: %w", err)
	}

	s.logger.Debug("removed role", "subject_type", subjectType, "subject_id", subjectID, "role", role)
	return nil
}

// HasRole checks if a subject has a specific role. Returns false for
// non-existent subjects (not an error).
func (s *SQLiteStore) HasRole(ctx context.Context, subjectType RoleSubjectType, subjectID string, role RoleName) (bool, error) {
	query := `
		SELECT COUNT(*) FROM roles
		WHERE subject_type = ? AND subject_id = ? AND role = ?
	`

	var count int
	err := s.db.QueryRowContext(ctx, query, subjectType, subjectID, role).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking role: %w", err)
	}

	return count > 0, nil
}

// ListRoles returns all roles assigned to a subject. Returns an empty slice
// if the subject has no roles.
func (s *SQLiteStore) ListRoles(ctx context.Context, subjectType RoleSubjectType, subjectID string) ([]RoleName, error) {
	query := `
		SELECT role FROM roles
		WHERE subject_type = ? AND subject_id = ?
		ORDER BY role
	`

	rows, err := s.db.QueryContext(ctx, query, subjectType, subjectID)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	defer rows.Close()

	var roles []RoleName
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		roles = append(roles, RoleName(role))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating roles: %w", err)
	}

	// Return empty slice (not nil) if no roles
	if roles == nil {
		roles = []RoleName{}
	}

	return roles, nil
}
