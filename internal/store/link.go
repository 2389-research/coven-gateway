// ABOUTME: Link code types and store interface for device linking
// ABOUTME: Handles temporary codes for pairing devices with the gateway

package store

import (
	"context"
	"time"
)

// LinkCodeStatus represents the state of a link code
type LinkCodeStatus string

const (
	LinkCodeStatusPending  LinkCodeStatus = "pending"
	LinkCodeStatusApproved LinkCodeStatus = "approved"
	LinkCodeStatusExpired  LinkCodeStatus = "expired"
)

// LinkCode represents a temporary code for device linking
type LinkCode struct {
	ID          string
	Code        string // 6-character alphanumeric code
	Fingerprint string // SSH key fingerprint of requesting device
	DeviceName  string // User-provided device name
	Status      LinkCodeStatus
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ApprovedBy  *string // Admin user ID who approved
	ApprovedAt  *time.Time
	PrincipalID *string // Created principal ID (set on approval)
	Token       *string // JWT token (set on approval)
}

// LinkCodeStore defines operations for link code management
type LinkCodeStore interface {
	// CreateLinkCode creates a new pending link code
	CreateLinkCode(ctx context.Context, code *LinkCode) error

	// GetLinkCode retrieves a link code by ID
	GetLinkCode(ctx context.Context, id string) (*LinkCode, error)

	// GetLinkCodeByCode retrieves a link code by its short code
	GetLinkCodeByCode(ctx context.Context, code string) (*LinkCode, error)

	// ApproveLinkCode marks a code as approved and stores the principal/token
	ApproveLinkCode(ctx context.Context, id string, approvedBy string, principalID string, token string) error

	// ListPendingLinkCodes returns all pending (non-expired) link codes
	ListPendingLinkCodes(ctx context.Context) ([]*LinkCode, error)

	// DeleteExpiredLinkCodes removes expired link codes
	DeleteExpiredLinkCodes(ctx context.Context) error
}
