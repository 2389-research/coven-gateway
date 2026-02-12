// ABOUTME: Tests for ClientService GetMe RPC handler
// ABOUTME: Covers success cases and role inclusion for self-identity retrieval

package client

import (
	"context"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

// seedPrincipal creates an approved test client principal in the store.
func seedPrincipal(t *testing.T, s *store.SQLiteStore, id, displayName string) *store.Principal {
	t.Helper()
	ctx := context.Background()

	p := &store.Principal{
		ID:          id,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "fp-" + id,
		DisplayName: displayName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, s.CreatePrincipal(ctx, p))
	return p
}

func TestGetMe_Success(t *testing.T) {
	s := createTestStore(t)

	// Seed a principal
	principal := seedPrincipal(t, s, "client-001", "Test Client")

	// Create context with auth
	authCtx := &auth.AuthContext{
		PrincipalID:   principal.ID,
		PrincipalType: string(principal.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	// Create service with principal store
	svc := NewClientService(s, s)

	// Call GetMe
	resp, err := svc.GetMe(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response fields
	assert.Equal(t, "client-001", resp.PrincipalId)
	assert.Equal(t, "client", resp.PrincipalType)
	assert.Equal(t, "Test Client", resp.DisplayName)
	assert.Equal(t, "approved", resp.Status)
	assert.ElementsMatch(t, []string{"member"}, resp.Roles)

	// MemberId and MemberDisplayName should be nil in v1
	assert.Nil(t, resp.MemberId)
	assert.Nil(t, resp.MemberDisplayName)
}

func TestGetMe_IncludesRoles(t *testing.T) {
	s := createTestStore(t)

	// Seed a principal with admin role
	principal := seedPrincipal(t, s, "admin-001", "Admin User")

	// Create context with multiple roles
	authCtx := &auth.AuthContext{
		PrincipalID:   principal.ID,
		PrincipalType: string(principal.Type),
		Roles:         []string{"member", "admin", "owner"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	resp, err := svc.GetMe(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify all roles are included
	assert.ElementsMatch(t, []string{"member", "admin", "owner"}, resp.Roles)
}
