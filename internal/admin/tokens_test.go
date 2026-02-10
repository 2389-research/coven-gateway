// ABOUTME: Tests for AdminService token management endpoints
// ABOUTME: Covers token creation, validation, and audit logging

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// testSecret is a 32-byte secret that meets MinSecretLength requirement.
var testSecret = []byte("admin-token-test-secret-32bytes!")

func TestCreateToken_Success(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a principal
	principal := &store.Principal{
		ID:          "test-principal-1",
		Type:        store.PrincipalTypeClient,
		DisplayName: "Test Principal",
		Status:      store.PrincipalStatusApproved,
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), principal))

	// Create JWT verifier
	verifier, err := auth.NewJWTVerifier(testSecret)
	require.NoError(t, err)

	// Create token service
	svc := NewTokenService(s, verifier, s)

	// Create token
	resp, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "test-principal-1",
		TtlSeconds:  3600, // 1 hour
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.NotEmpty(t, resp.ExpiresAt)

	// Verify the token is valid
	principalID, err := verifier.Verify(resp.Token)
	require.NoError(t, err)
	assert.Equal(t, "test-principal-1", principalID)
}

func TestCreateToken_DefaultTTL(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a principal
	principal := &store.Principal{
		ID:          "test-principal-2",
		Type:        store.PrincipalTypeClient,
		DisplayName: "Test Principal",
		Status:      store.PrincipalStatusApproved,
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), principal))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewTokenService(s, verifier, s)

	// Create token with no TTL (should use default 30 days)
	resp, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "test-principal-2",
	})
	require.NoError(t, err)

	// Parse expiration time
	expiresAt, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	require.NoError(t, err)

	// Should be approximately 30 days from now
	expectedExpiry := time.Now().Add(30 * 24 * time.Hour)
	diff := expiresAt.Sub(expectedExpiry)
	assert.Less(t, diff.Abs(), time.Minute) // within 1 minute
}

func TestCreateToken_PrincipalNotFound(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewTokenService(s, verifier, s)

	_, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "nonexistent",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCreateToken_PrincipalNotApproved(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a pending principal
	principal := &store.Principal{
		ID:          "pending-principal",
		Type:        store.PrincipalTypeClient,
		DisplayName: "Pending Principal",
		Status:      store.PrincipalStatusPending,
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), principal))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewTokenService(s, verifier, s)

	_, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "pending-principal",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be approved")
}

func TestCreateToken_NoTokenGenerator(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create service without token generator (nil)
	svc := NewTokenService(s, nil, s)

	_, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "test-principal",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestCreateToken_TTLTooLong(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a principal
	principal := &store.Principal{
		ID:          "test-principal",
		Type:        store.PrincipalTypeClient,
		DisplayName: "Test Principal",
		Status:      store.PrincipalStatusApproved,
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), principal))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewTokenService(s, verifier, s)

	// Try to create token with TTL > 365 days
	_, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "test-principal",
		TtlSeconds:  400 * 24 * 60 * 60, // 400 days
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestCreateToken_AuditLog(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a principal
	principal := &store.Principal{
		ID:          "test-principal",
		Type:        store.PrincipalTypeClient,
		DisplayName: "Test Principal",
		Status:      store.PrincipalStatusApproved,
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), principal))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewTokenService(s, verifier, s)

	// Create token
	_, err := svc.CreateToken(ctx, &pb.CreateTokenRequest{
		PrincipalId: "test-principal",
		TtlSeconds:  3600,
	})
	require.NoError(t, err)

	// Check audit log - list all entries first to debug
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	require.NoError(t, err)

	// Find the create_token entry
	var found bool
	for _, entry := range entries {
		if entry.Action == store.AuditCreateToken {
			found = true
			assert.Equal(t, "admin-1", entry.ActorPrincipalID)
			assert.Equal(t, "test-principal", entry.TargetID)
			break
		}
	}
	assert.True(t, found, "expected to find create_token audit entry, got %d entries total", len(entries))
}
