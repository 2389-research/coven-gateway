// ABOUTME: Scenario tests for AdminService principal management using real SQLite
// ABOUTME: Covers create, list, delete operations and validation logic

package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// ============================================================================
// ListPrincipals Tests
// ============================================================================

func TestListPrincipals_Empty(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Principals)
}

func TestListPrincipals_WithPrincipals(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create several principals (all need unique fingerprints due to UNIQUE constraint)
	principals := []*store.Principal{
		{
			ID:          "client-1",
			Type:        store.PrincipalTypeClient,
			PubkeyFP:    strings.Repeat("c1", 32),
			DisplayName: "Client One",
			Status:      store.PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC(),
		},
		{
			ID:          "agent-1",
			Type:        store.PrincipalTypeAgent,
			PubkeyFP:    strings.Repeat("a1", 32),
			DisplayName: "Agent One",
			Status:      store.PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC(),
		},
		{
			ID:          "client-2",
			Type:        store.PrincipalTypeClient,
			PubkeyFP:    strings.Repeat("c2", 32),
			DisplayName: "Client Two",
			Status:      store.PrincipalStatusPending,
			CreatedAt:   time.Now().UTC(),
		},
	}
	for _, p := range principals {
		require.NoError(t, s.CreatePrincipal(context.Background(), p))
	}

	// Add role to one principal
	require.NoError(t, s.AddRole(context.Background(), store.RoleSubjectPrincipal, "client-1", store.RoleAdmin))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 3)

	// Find client-1 and verify roles
	var client1 *pb.Principal
	for _, p := range resp.Principals {
		if p.Id == "client-1" {
			client1 = p
			break
		}
	}
	require.NotNil(t, client1, "client-1 not found in response")
	assert.Contains(t, client1.Roles, "admin")
}

func TestListPrincipals_FilterByType(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create mixed principals
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "client-filter-1",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    strings.Repeat("cf1", 22)[:64],
		DisplayName: "Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "agent-filter-1",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    strings.Repeat("b", 64),
		DisplayName: "Agent",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	// Filter by client type
	clientType := "client"
	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{
		Type: &clientType,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 1)
	assert.Equal(t, "client-filter-1", resp.Principals[0].Id)

	// Filter by agent type
	agentType := "agent"
	resp, err = svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{
		Type: &agentType,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 1)
	assert.Equal(t, "agent-filter-1", resp.Principals[0].Id)
}

func TestListPrincipals_FilterByStatus(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create principals with different statuses (each needs unique fingerprint)
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "approved-1",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    strings.Repeat("ap1", 22)[:64],
		DisplayName: "Approved",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "pending-1",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    strings.Repeat("pe1", 22)[:64],
		DisplayName: "Pending",
		Status:      store.PrincipalStatusPending,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	// Filter by pending status
	pendingStatus := "pending"
	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{
		Status: &pendingStatus,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 1)
	assert.Equal(t, "pending-1", resp.Principals[0].Id)
}

func TestListPrincipals_IncludesPubkeyFP(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	fingerprint := strings.Repeat("c", 64)
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "agent-fp-test",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fingerprint,
		DisplayName: "Agent with FP",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Principals, 1)
	require.NotNil(t, resp.Principals[0].PubkeyFp)
	assert.Equal(t, fingerprint, *resp.Principals[0].PubkeyFp)
}

// ============================================================================
// CreatePrincipal Tests
// ============================================================================

func TestCreatePrincipal_ClientSuccess(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "New Client",
		Roles:       []string{"member"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.True(t, strings.HasPrefix(resp.Id, "client-"), "ID should start with type prefix")
	assert.Equal(t, "client", resp.Type)
	assert.Equal(t, "New Client", resp.DisplayName)
	assert.Equal(t, "approved", resp.Status)
	assert.Contains(t, resp.Roles, "member")
	assert.Nil(t, resp.PubkeyFp) // Client has no fingerprint
}

func TestCreatePrincipal_AgentWithPubkey(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	// Use a valid SSH public key format
	pubkey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "New Agent",
		Pubkey:      &pubkey,
		Roles:       []string{"member"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.True(t, strings.HasPrefix(resp.Id, "agent-"), "ID should start with type prefix")
	assert.Equal(t, "agent", resp.Type)
	assert.NotNil(t, resp.PubkeyFp)
	assert.Len(t, *resp.PubkeyFp, 64) // SHA256 hex is 64 chars
}

func TestCreatePrincipal_AgentWithFingerprint(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	fingerprint := strings.Repeat("d", 64)

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "Agent with FP",
		PubkeyFp:    &fingerprint,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	require.NotNil(t, resp.PubkeyFp)
	assert.Equal(t, fingerprint, *resp.PubkeyFp)
}

func TestCreatePrincipal_AgentMissingPubkey(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "Agent without key",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, err.Error(), "require pubkey or pubkey_fp")
}

func TestCreatePrincipal_MissingType(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		DisplayName: "No Type",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, err.Error(), "type required")
}

func TestCreatePrincipal_MissingDisplayName(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type: "client",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, err.Error(), "display_name required")
}

func TestCreatePrincipal_InvalidType(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "invalid-type",
		DisplayName: "Invalid",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, err.Error(), "invalid type")
}

func TestCreatePrincipal_InvalidPubkey(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	invalidPubkey := "not-a-valid-pubkey"

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "Invalid Key Agent",
		Pubkey:      &invalidPubkey,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, err.Error(), "invalid pubkey")
}

func TestCreatePrincipal_DuplicatePubkey(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create first agent with fingerprint
	fingerprint := strings.Repeat("e", 64)
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "existing-agent",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fingerprint,
		DisplayName: "Existing Agent",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	// Try to create another agent with same fingerprint
	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "Duplicate Key Agent",
		PubkeyFp:    &fingerprint,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.AlreadyExists, st.Code())
	assert.Contains(t, err.Error(), "already registered")
}

func TestCreatePrincipal_AuditLog(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "Audit Test Client",
		Roles:       []string{"member"},
	})
	require.NoError(t, err)

	// Check audit log
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	require.NoError(t, err)

	var found bool
	for _, entry := range entries {
		if entry.Action == store.AuditCreatePrincipal {
			found = true
			assert.Equal(t, "admin-1", entry.ActorPrincipalID)
			assert.Equal(t, resp.Id, entry.TargetID)
			assert.Equal(t, "principal", entry.TargetType)
			break
		}
	}
	assert.True(t, found, "expected to find create_principal audit entry")
}

// ============================================================================
// DeletePrincipal Tests
// ============================================================================

func TestDeletePrincipal_Success(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a principal to delete
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "to-delete",
		Type:        store.PrincipalTypeClient,
		DisplayName: "To Delete",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{
		Id: "to-delete",
	})
	require.NoError(t, err)

	// Verify deleted
	_, err = s.GetPrincipal(context.Background(), "to-delete")
	assert.ErrorIs(t, err, store.ErrPrincipalNotFound)
}

func TestDeletePrincipal_NotFound(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{
		Id: "nonexistent",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestDeletePrincipal_MissingID(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, err.Error(), "id required")
}

func TestDeletePrincipal_AuditLog(t *testing.T) {
	s := createTestStore(t)
	ctx := createAdminContext("admin-1")

	// Create a principal to delete
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "to-delete-audit",
		Type:        store.PrincipalTypeClient,
		DisplayName: "To Delete Audit",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{
		Id: "to-delete-audit",
	})
	require.NoError(t, err)

	// Check audit log
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	require.NoError(t, err)

	var found bool
	for _, entry := range entries {
		if entry.Action == store.AuditDeletePrincipal {
			found = true
			assert.Equal(t, "admin-1", entry.ActorPrincipalID)
			assert.Equal(t, "to-delete-audit", entry.TargetID)
			break
		}
	}
	assert.True(t, found, "expected to find delete_principal audit entry")
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestGeneratePrincipalID_Format(t *testing.T) {
	// Test client type
	clientID := generatePrincipalID(store.PrincipalTypeClient)
	assert.True(t, strings.HasPrefix(clientID, "client-"))
	parts := strings.Split(clientID, "-")
	assert.Len(t, parts, 3) // type, timestamp, random

	// Test agent type
	agentID := generatePrincipalID(store.PrincipalTypeAgent)
	assert.True(t, strings.HasPrefix(agentID, "agent-"))

	// IDs should be unique
	id1 := generatePrincipalID(store.PrincipalTypeClient)
	id2 := generatePrincipalID(store.PrincipalTypeClient)
	assert.NotEqual(t, id1, id2)
}

func TestFormatBase36(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{10, "a"},
		{35, "z"},
		{36, "10"},
		{100, "2s"},
	}

	for _, tc := range tests {
		result := formatBase36(tc.input)
		assert.Equal(t, tc.expected, result, "formatBase36(%d)", tc.input)
	}
}

func TestFormatHex4(t *testing.T) {
	tests := []struct {
		input    uint16
		expected string
	}{
		{0, "0000"},
		{1, "0001"},
		{255, "00ff"},
		{4096, "1000"},
		{65535, "ffff"},
	}

	for _, tc := range tests {
		result := formatHex4(tc.input)
		assert.Equal(t, tc.expected, result, "formatHex4(%d)", tc.input)
	}
}

func TestValidateCreatePrincipalRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *pb.CreatePrincipalRequest
		wantErr string
	}{
		{
			name:    "missing type",
			req:     &pb.CreatePrincipalRequest{DisplayName: "Test"},
			wantErr: "type required",
		},
		{
			name:    "missing display_name",
			req:     &pb.CreatePrincipalRequest{Type: "client"},
			wantErr: "display_name required",
		},
		{
			name: "valid request",
			req:  &pb.CreatePrincipalRequest{Type: "client", DisplayName: "Test"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreatePrincipalRequest(tc.req)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParsePrincipalType(t *testing.T) {
	tests := []struct {
		input   string
		want    store.PrincipalType
		wantErr bool
	}{
		{"client", store.PrincipalTypeClient, false},
		{"agent", store.PrincipalTypeAgent, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parsePrincipalType(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestResolveAgentFingerprint(t *testing.T) {
	// Test with valid pubkey
	validPubkey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"
	t.Run("with pubkey", func(t *testing.T) {
		fp, err := resolveAgentFingerprint(&pb.CreatePrincipalRequest{
			Pubkey: &validPubkey,
		})
		require.NoError(t, err)
		assert.Len(t, fp, 64) // SHA256 hex
	})

	// Test with fingerprint
	fingerprint := strings.Repeat("f", 64)
	t.Run("with fingerprint", func(t *testing.T) {
		fp, err := resolveAgentFingerprint(&pb.CreatePrincipalRequest{
			PubkeyFp: &fingerprint,
		})
		require.NoError(t, err)
		assert.Equal(t, fingerprint, fp)
	})

	// Test with neither
	t.Run("missing both", func(t *testing.T) {
		_, err := resolveAgentFingerprint(&pb.CreatePrincipalRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "require pubkey or pubkey_fp")
	})

	// Test with invalid pubkey
	invalidPubkey := "invalid"
	t.Run("invalid pubkey", func(t *testing.T) {
		_, err := resolveAgentFingerprint(&pb.CreatePrincipalRequest{
			Pubkey: &invalidPubkey,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pubkey")
	})
}

func TestAssignRoles(t *testing.T) {
	s := createTestStore(t)
	ctx := context.Background()

	// Create a principal
	require.NoError(t, s.CreatePrincipal(ctx, &store.Principal{
		ID:          "role-test",
		Type:        store.PrincipalTypeClient,
		DisplayName: "Role Test",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	verifier, _ := auth.NewJWTVerifier(testSecret)
	svc := NewPrincipalService(s, verifier)

	// Assign roles
	added := svc.assignRoles(ctx, "role-test", []string{"member", "admin"})
	assert.Len(t, added, 2)
	assert.Contains(t, added, "member")
	assert.Contains(t, added, "admin")

	// Verify roles were added
	roles, err := s.ListRoles(ctx, store.RoleSubjectPrincipal, "role-test")
	require.NoError(t, err)
	assert.Len(t, roles, 2)
}
