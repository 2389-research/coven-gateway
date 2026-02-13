// ABOUTME: Tests for PrincipalService gRPC handlers
// ABOUTME: Covers ListPrincipals, CreatePrincipal, DeletePrincipal operations

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

// createPrincipalService creates a PrincipalService with the given store.
func createPrincipalService(t *testing.T, s *store.SQLiteStore) *PrincipalService {
	t.Helper()
	verifier, err := auth.NewJWTVerifier(testSecret)
	require.NoError(t, err)
	return NewPrincipalService(s, verifier)
}

// testFingerprint generates a valid 64-character hex fingerprint from a seed.
func testFingerprint(seed string) string {
	fp := seed
	for len(fp) < 64 {
		fp += seed
	}
	return strings.ReplaceAll(fp[:64], "-", "0")
}

// --- ListPrincipals Tests ---

func TestListPrincipals_Success(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create some principals directly in the store
	principals := []store.Principal{
		{
			ID:          "client-001",
			Type:        store.PrincipalTypeClient,
			PubkeyFP:    testFingerprint("client001"),
			DisplayName: "Test Client 1",
			Status:      store.PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC(),
		},
		{
			ID:          "agent-001",
			Type:        store.PrincipalTypeAgent,
			PubkeyFP:    testFingerprint("agent001"),
			DisplayName: "Test Agent 1",
			Status:      store.PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC(),
		},
	}

	for i := range principals {
		require.NoError(t, s.CreatePrincipal(context.Background(), &principals[i]))
	}

	// List all principals
	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Principals, 2)

	// Verify principal data is properly converted
	for _, p := range resp.Principals {
		assert.NotEmpty(t, p.Id)
		assert.NotEmpty(t, p.Type)
		assert.NotEmpty(t, p.DisplayName)
		assert.NotEmpty(t, p.Status)
		assert.NotEmpty(t, p.CreatedAt)
	}
}

func TestListPrincipals_Empty(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Principals)
}

func TestListPrincipals_FilterByType(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create mixed principals
	// Note: pubkey_fingerprint is NOT NULL UNIQUE, so even clients need unique values
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "client-001",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    testFingerprint("filterclient"),
		DisplayName: "Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "agent-001",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    testFingerprint("agent001"),
		DisplayName: "Agent",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	// Filter by client type
	clientType := "client"
	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{Type: &clientType})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 1)
	assert.Equal(t, "client", resp.Principals[0].Type)

	// Filter by agent type
	agentType := "agent"
	resp, err = svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{Type: &agentType})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 1)
	assert.Equal(t, "agent", resp.Principals[0].Type)
}

func TestListPrincipals_FilterByStatus(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create principals with different statuses
	// Note: pubkey_fingerprint is NOT NULL UNIQUE, so even clients need unique values
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "client-approved",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    testFingerprint("approved"),
		DisplayName: "Approved Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "client-pending",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    testFingerprint("pending"),
		DisplayName: "Pending Client",
		Status:      store.PrincipalStatusPending,
		CreatedAt:   time.Now().UTC(),
	}))

	// Filter by approved status
	approvedStatus := "approved"
	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{Status: &approvedStatus})
	require.NoError(t, err)
	assert.Len(t, resp.Principals, 1)
	assert.Equal(t, "approved", resp.Principals[0].Status)
}

func TestListPrincipals_IncludesRoles(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create a principal
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "client-001",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    testFingerprint("clientroles"),
		DisplayName: "Client With Roles",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	// Add roles
	require.NoError(t, s.AddRole(context.Background(), store.RoleSubjectPrincipal, "client-001", store.RoleAdmin))
	require.NoError(t, s.AddRole(context.Background(), store.RoleSubjectPrincipal, "client-001", store.RoleMember))

	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Principals, 1)
	assert.Len(t, resp.Principals[0].Roles, 2)
	assert.Contains(t, resp.Principals[0].Roles, "admin")
	assert.Contains(t, resp.Principals[0].Roles, "member")
}

func TestListPrincipals_IncludesPubkeyFP(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	fp := testFingerprint("testfp")
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "agent-001",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fp,
		DisplayName: "Agent With FP",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	resp, err := svc.ListPrincipals(ctx, &pb.ListPrincipalsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Principals, 1)
	require.NotNil(t, resp.Principals[0].PubkeyFp)
	assert.Equal(t, fp, *resp.Principals[0].PubkeyFp)
}

// --- CreatePrincipal Tests ---

func TestCreatePrincipal_ClientSuccess(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "New Client",
		Roles:       []string{"member"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.Id)
	assert.True(t, strings.HasPrefix(resp.Id, "client-"), "ID should start with 'client-'")
	assert.Equal(t, "client", resp.Type)
	assert.Equal(t, "New Client", resp.DisplayName)
	assert.Equal(t, "approved", resp.Status)
	assert.NotEmpty(t, resp.CreatedAt)
	assert.Contains(t, resp.Roles, "member")
}

func TestCreatePrincipal_AgentWithPubkey(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

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
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	fp := testFingerprint("newagent")
	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "New Agent",
		PubkeyFp:    &fp,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, strings.HasPrefix(resp.Id, "agent-"), "ID should start with 'agent-'")
	assert.Equal(t, "agent", resp.Type)
	require.NotNil(t, resp.PubkeyFp)
	assert.Equal(t, fp, *resp.PubkeyFp)
}

func TestCreatePrincipal_MissingType(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "",
		DisplayName: "Test",
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "type")
}

func TestCreatePrincipal_MissingDisplayName(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "",
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "display_name")
}

func TestCreatePrincipal_InvalidType(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "invalid",
		DisplayName: "Test",
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid type")
}

func TestCreatePrincipal_AgentMissingPubkey(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "Agent Without Key",
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "pubkey")
}

func TestCreatePrincipal_InvalidPubkey(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

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
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	fp := testFingerprint("duplicate")

	// Create first agent
	_, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "First Agent",
		PubkeyFp:    &fp,
	})
	require.NoError(t, err)

	// Try to create second agent with same fingerprint
	_, err = svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "agent",
		DisplayName: "Second Agent",
		PubkeyFp:    &fp,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
	assert.Contains(t, st.Message(), "pubkey already registered")
}

func TestCreatePrincipal_WritesAuditLog(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "Audited Client",
		Roles:       []string{"member"},
	})
	require.NoError(t, err)

	// Verify audit log
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{
		TargetID: &resp.Id,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, "admin-1", entry.ActorPrincipalID)
	assert.Equal(t, store.AuditCreatePrincipal, entry.Action)
	assert.Equal(t, "principal", entry.TargetType)
	assert.Equal(t, resp.Id, entry.TargetID)
}

func TestCreatePrincipal_AssignsRoles(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "Client With Roles",
		Roles:       []string{"admin", "member"},
	})
	require.NoError(t, err)

	// Verify roles were assigned
	roles, err := s.ListRoles(context.Background(), store.RoleSubjectPrincipal, resp.Id)
	require.NoError(t, err)
	assert.Len(t, roles, 2)

	roleStrings := make([]string, len(roles))
	for i, r := range roles {
		roleStrings[i] = string(r)
	}
	assert.Contains(t, roleStrings, "admin")
	assert.Contains(t, roleStrings, "member")
}

// --- DeletePrincipal Tests ---

func TestDeletePrincipal_Success(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create a principal first
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          "to-delete",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    testFingerprint("todelete"),
		DisplayName: "Delete Me",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	// Delete it
	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{Id: "to-delete"})
	require.NoError(t, err)

	// Verify it's gone
	_, err = s.GetPrincipal(context.Background(), "to-delete")
	assert.ErrorIs(t, err, store.ErrPrincipalNotFound)
}

func TestDeletePrincipal_NotFound(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{Id: "nonexistent"})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestDeletePrincipal_MissingID(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{Id: ""})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "id")
}

func TestDeletePrincipal_WritesAuditLog(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create a principal first
	principalID := "to-audit-delete"
	require.NoError(t, s.CreatePrincipal(context.Background(), &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    testFingerprint("auditdelete"),
		DisplayName: "Delete Audit Test",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}))

	// Delete it
	_, err := svc.DeletePrincipal(ctx, &pb.DeletePrincipalRequest{Id: principalID})
	require.NoError(t, err)

	// Verify audit log
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{
		TargetID: &principalID,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, "admin-1", entry.ActorPrincipalID)
	assert.Equal(t, store.AuditDeletePrincipal, entry.Action)
	assert.Equal(t, "principal", entry.TargetType)
	assert.Equal(t, principalID, entry.TargetID)
}

// --- Helper Function Tests ---

func TestGeneratePrincipalID_Format(t *testing.T) {
	// Test that IDs have the expected format: {type}-{base36timestamp}-{hex4}
	clientID := generatePrincipalID(store.PrincipalTypeClient)
	assert.True(t, strings.HasPrefix(clientID, "client-"))
	parts := strings.Split(clientID, "-")
	assert.Len(t, parts, 3, "ID should have 3 parts: type-timestamp-random")
	assert.Len(t, parts[2], 4, "Random suffix should be 4 hex characters")

	agentID := generatePrincipalID(store.PrincipalTypeAgent)
	assert.True(t, strings.HasPrefix(agentID, "agent-"))
}

func TestGeneratePrincipalID_Unique(t *testing.T) {
	// Generate multiple IDs and verify uniqueness
	// Note: In a tight loop, collisions are possible due to 16-bit random suffix
	// and same-millisecond timestamps. Real-world usage has time between calls.
	ids := make(map[string]bool)
	duplicates := 0
	for range 100 {
		id := generatePrincipalID(store.PrincipalTypeClient)
		if ids[id] {
			duplicates++
		}
		ids[id] = true
	}
	// Allow a small number of duplicates in tight loop, but not excessive
	assert.Less(t, duplicates, 5, "Too many duplicate IDs generated")
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
		{1000, "rs"},
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
		{15, "000f"},
		{255, "00ff"},
		{4095, "0fff"},
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
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid request",
			req:     &pb.CreatePrincipalRequest{Type: "client", DisplayName: "Test"},
			wantErr: false,
		},
		{
			name:    "missing type",
			req:     &pb.CreatePrincipalRequest{Type: "", DisplayName: "Test"},
			wantErr: true,
			errMsg:  "type",
		},
		{
			name:    "missing display_name",
			req:     &pb.CreatePrincipalRequest{Type: "client", DisplayName: ""},
			wantErr: true,
			errMsg:  "display_name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreatePrincipalRequest(tc.req)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParsePrincipalType(t *testing.T) {
	tests := []struct {
		input    string
		expected store.PrincipalType
		wantErr  bool
	}{
		{"client", store.PrincipalTypeClient, false},
		{"agent", store.PrincipalTypeAgent, false},
		{"invalid", "", true},
		{"", "", true},
		{"CLIENT", "", true}, // case sensitive
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result, err := parsePrincipalType(tc.input)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestResolveAgentFingerprint_WithPubkey(t *testing.T) {
	// Valid SSH public key (ed25519)
	validPubkey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGvMD0YhM8XQ5S/4t6GVXdU5YJ5d+/9/Sd3TThQwEqbP test@example.com"

	req := &pb.CreatePrincipalRequest{
		Pubkey: &validPubkey,
	}

	fp, err := resolveAgentFingerprint(req)
	require.NoError(t, err)
	assert.NotEmpty(t, fp)
	// Fingerprint should be a 64-character hex string (SHA256)
	assert.Len(t, fp, 64)
	// Verify it's all hex characters
	for _, c := range fp {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"fingerprint should be lowercase hex, got char: %c", c)
	}
}

func TestResolveAgentFingerprint_InvalidPubkey(t *testing.T) {
	invalidPubkey := "not-a-valid-pubkey"

	req := &pb.CreatePrincipalRequest{
		Pubkey: &invalidPubkey,
	}

	_, err := resolveAgentFingerprint(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pubkey")
}

func TestResolveAgentFingerprint_PrefersPubkeyOverPubkeyFp(t *testing.T) {
	// When both pubkey and pubkey_fp are provided, pubkey takes precedence
	validPubkey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGvMD0YhM8XQ5S/4t6GVXdU5YJ5d+/9/Sd3TThQwEqbP test@example.com"
	manualFp := testFingerprint("manual")

	req := &pb.CreatePrincipalRequest{
		Pubkey:   &validPubkey,
		PubkeyFp: &manualFp,
	}

	fp, err := resolveAgentFingerprint(req)
	require.NoError(t, err)
	// Should use derived fingerprint, not the manual one
	assert.NotEqual(t, manualFp, fp)
	assert.Len(t, fp, 64)
}

func TestResolveAgentFingerprint_MissingBoth(t *testing.T) {
	_, err := resolveAgentFingerprint(&pb.CreatePrincipalRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pubkey")
}

func TestAssignRoles_InvalidRoleNames(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create a principal first
	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "Test Client",
		Roles:       []string{"invalid_role", "also_invalid"},
	})
	require.NoError(t, err) // Principal creation succeeds

	// But invalid roles should not be assigned (assignRoles skips on error)
	roles, err := s.ListRoles(context.Background(), store.RoleSubjectPrincipal, resp.Id)
	require.NoError(t, err)
	assert.Empty(t, roles, "invalid roles should not be assigned")
}

func TestAssignRoles_MixedValidAndInvalidRoles(t *testing.T) {
	s := createTestStore(t)
	svc := createPrincipalService(t, s)
	ctx := createAdminContext("admin-1")

	// Create a principal with mixed valid and invalid roles
	resp, err := svc.CreatePrincipal(ctx, &pb.CreatePrincipalRequest{
		Type:        "client",
		DisplayName: "Test Client",
		Roles:       []string{"admin", "invalid_role", "member"},
	})
	require.NoError(t, err)

	// Only valid roles should be assigned
	roles, err := s.ListRoles(context.Background(), store.RoleSubjectPrincipal, resp.Id)
	require.NoError(t, err)
	assert.Len(t, roles, 2)

	roleStrings := make([]string, len(roles))
	for i, r := range roles {
		roleStrings[i] = string(r)
	}
	assert.Contains(t, roleStrings, "admin")
	assert.Contains(t, roleStrings, "member")
	assert.NotContains(t, roleStrings, "invalid_role")
}
