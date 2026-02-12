// ABOUTME: Tests for ClientService RegisterAgent and RegisterClient RPC handlers
// ABOUTME: Covers validation, rate limiting, and registration scenarios

package client

import (
	"context"
	"fmt"
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

// =============================================================================
// Validation Tests
// =============================================================================

func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		wantCode    codes.Code
		wantErr     bool
	}{
		{
			name:        "valid simple name",
			displayName: "My Agent",
			wantErr:     false,
		},
		{
			name:        "valid with hyphens and underscores",
			displayName: "my-agent_v2",
			wantErr:     false,
		},
		{
			name:        "valid with periods",
			displayName: "agent.prod.v1",
			wantErr:     false,
		},
		{
			name:        "valid at max length",
			displayName: strings.Repeat("a", maxDisplayNameLength),
			wantErr:     false,
		},
		{
			name:        "empty name",
			displayName: "",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "whitespace only",
			displayName: "   ",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "tabs only",
			displayName: "\t\t",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "exceeds max length",
			displayName: strings.Repeat("a", maxDisplayNameLength+1),
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "contains invalid characters - at sign",
			displayName: "user@domain",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "contains invalid characters - exclamation",
			displayName: "Hello!",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "contains invalid characters - emoji",
			displayName: "Agent 🤖",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "contains invalid characters - slash",
			displayName: "path/to/agent",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDisplayName(tt.displayName)
			if tt.wantErr {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok, "expected gRPC status error")
				assert.Equal(t, tt.wantCode, st.Code())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateFingerprint(t *testing.T) {
	validFingerprint := strings.Repeat("ab", 32) // 64 hex chars

	tests := []struct {
		name        string
		fingerprint string
		wantCode    codes.Code
		wantErr     bool
	}{
		{
			name:        "valid fingerprint",
			fingerprint: validFingerprint,
			wantErr:     false,
		},
		{
			name:        "valid fingerprint uppercase",
			fingerprint: strings.ToUpper(validFingerprint),
			wantErr:     false,
		},
		{
			name:        "valid fingerprint mixed case",
			fingerprint: "ABCDEFabcdef0123456789ABCDEFabcdef0123456789ABCDEFabcdef01234567",
			wantErr:     false,
		},
		{
			name:        "empty fingerprint",
			fingerprint: "",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "too short",
			fingerprint: strings.Repeat("ab", 31), // 62 chars
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "too long",
			fingerprint: strings.Repeat("ab", 33), // 66 chars
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "invalid hex characters",
			fingerprint: strings.Repeat("gh", 32), // g and h are not hex
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
		{
			name:        "contains spaces",
			fingerprint: strings.Repeat("ab", 16) + " " + strings.Repeat("ab", 15) + "a",
			wantCode:    codes.InvalidArgument,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFingerprint(tt.fingerprint)
			if tt.wantErr {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok, "expected gRPC status error")
				assert.Equal(t, tt.wantCode, st.Code())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHasSufficientRole(t *testing.T) {
	tests := []struct {
		name   string
		roles  []string
		expect bool
	}{
		{
			name:   "no roles",
			roles:  []string{},
			expect: false,
		},
		{
			name:   "nil roles",
			roles:  nil,
			expect: false,
		},
		{
			name:   "viewer only",
			roles:  []string{"viewer"},
			expect: false,
		},
		{
			name:   "unknown role",
			roles:  []string{"unknown", "other"},
			expect: false,
		},
		{
			name:   "member role",
			roles:  []string{"member"},
			expect: true,
		},
		{
			name:   "admin role",
			roles:  []string{"admin"},
			expect: true,
		},
		{
			name:   "owner role",
			roles:  []string{"owner"},
			expect: true,
		},
		{
			name:   "member among other roles",
			roles:  []string{"viewer", "member"},
			expect: true,
		},
		{
			name:   "all privileged roles",
			roles:  []string{"member", "admin", "owner"},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSufficientRole(tt.roles)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestHandleCreatePrincipalError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantMsg  string
	}{
		{
			name:     "duplicate pubkey error",
			err:      store.ErrDuplicatePubkey,
			wantCode: codes.AlreadyExists,
			wantMsg:  "fingerprint already registered",
		},
		{
			name:     "generic error",
			err:      assert.AnError,
			wantCode: codes.Internal,
			wantMsg:  "failed to create principal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleCreatePrincipalError(tt.err)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "expected gRPC status error")
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Contains(t, st.Message(), tt.wantMsg)
		})
	}
}

// =============================================================================
// Rate Limiter Tests
// =============================================================================

func TestRateLimiter_AllowsInitialRegistrations(t *testing.T) {
	limiter := &registrationRateLimiter{
		records:     make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}

	// Should allow up to maxRegistrationsPerWindow registrations
	for i := range maxRegistrationsPerWindow {
		allowed := limiter.checkAndRecord("test-principal")
		assert.True(t, allowed, "registration %d should be allowed", i+1)
	}
}

func TestRateLimiter_BlocksExcessRegistrations(t *testing.T) {
	limiter := &registrationRateLimiter{
		records:     make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}

	// Fill up the allowance
	for range maxRegistrationsPerWindow {
		limiter.checkAndRecord("test-principal")
	}

	// Next registration should be blocked
	allowed := limiter.checkAndRecord("test-principal")
	assert.False(t, allowed, "registration should be blocked after limit")
}

func TestRateLimiter_AllowsDifferentPrincipals(t *testing.T) {
	limiter := &registrationRateLimiter{
		records:     make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}

	// Fill up principal A
	for range maxRegistrationsPerWindow {
		limiter.checkAndRecord("principal-a")
	}

	// Principal B should still be allowed
	allowed := limiter.checkAndRecord("principal-b")
	assert.True(t, allowed, "different principal should be allowed")
}

func TestRateLimiter_Cleanup(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-registrationRateWindow)

	limiter := &registrationRateLimiter{
		records: map[string][]time.Time{
			"expired": {cutoff.Add(-time.Second)},                          // All expired
			"mixed":   {cutoff.Add(-time.Second), cutoff.Add(time.Second)}, // One valid
			"valid":   {cutoff.Add(time.Second)},                           // All valid
		},
		lastCleanup: now,
	}

	limiter.mu.Lock()
	limiter.cleanup(cutoff)
	limiter.mu.Unlock()

	// "expired" should be removed
	_, hasExpired := limiter.records["expired"]
	assert.False(t, hasExpired, "fully expired entry should be removed")

	// "mixed" should remain (has valid timestamp)
	_, hasMixed := limiter.records["mixed"]
	assert.True(t, hasMixed, "entry with valid timestamps should remain")

	// "valid" should remain
	_, hasValid := limiter.records["valid"]
	assert.True(t, hasValid, "valid entry should remain")
}

// =============================================================================
// RegisterAgent Tests
// =============================================================================

func TestRegisterAgent_Success(t *testing.T) {
	s := createTestStore(t)

	// Seed a member principal who will register the agent
	member := seedPrincipal(t, s, "member-001", "Member User")

	// Create auth context with member role
	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	fingerprint := strings.Repeat("aa", 32)
	req := &pb.RegisterAgentRequest{
		DisplayName: "My New Agent",
		Fingerprint: fingerprint,
	}

	resp, err := svc.RegisterAgent(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.PrincipalId)
	assert.Equal(t, "approved", resp.Status)

	// Verify principal was created in store
	principal, err := s.GetPrincipal(ctx, resp.PrincipalId)
	require.NoError(t, err)
	assert.Equal(t, store.PrincipalTypeAgent, principal.Type)
	assert.Equal(t, "My New Agent", principal.DisplayName)
	assert.Equal(t, fingerprint, principal.PubkeyFP)
}

func TestRegisterAgent_PermissionDenied_InsufficientRoles(t *testing.T) {
	s := createTestStore(t)
	member := seedPrincipal(t, s, "viewer-001", "Viewer User")

	// Create auth context with insufficient roles (viewer cannot register agents)
	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"viewer"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	req := &pb.RegisterAgentRequest{
		DisplayName: "Agent",
		Fingerprint: strings.Repeat("bb", 32),
	}

	_, err := svc.RegisterAgent(ctx, req)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestRegisterAgent_ValidationErrors(t *testing.T) {
	s := createTestStore(t)
	member := seedPrincipal(t, s, "member-002", "Member")

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	tests := []struct {
		name        string
		displayName string
		fingerprint string
		wantCode    codes.Code
	}{
		{
			name:        "empty display name",
			displayName: "",
			fingerprint: strings.Repeat("cc", 32),
			wantCode:    codes.InvalidArgument,
		},
		{
			name:        "empty fingerprint",
			displayName: "Valid Name",
			fingerprint: "",
			wantCode:    codes.InvalidArgument,
		},
		{
			name:        "invalid fingerprint length",
			displayName: "Valid Name",
			fingerprint: "abc123",
			wantCode:    codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &pb.RegisterAgentRequest{
				DisplayName: tt.displayName,
				Fingerprint: tt.fingerprint,
			}
			_, err := svc.RegisterAgent(ctx, req)
			require.Error(t, err)
			st, _ := status.FromError(err)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}

func TestRegisterAgent_DuplicateFingerprint(t *testing.T) {
	s := createTestStore(t)
	member := seedPrincipal(t, s, "member-003", "Member")

	fingerprint := strings.Repeat("dd", 32)

	// First, create an existing principal with the same fingerprint
	existing := &store.Principal{
		ID:          "existing-agent",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fingerprint,
		DisplayName: "Existing Agent",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), existing))

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	req := &pb.RegisterAgentRequest{
		DisplayName: "Duplicate Agent",
		Fingerprint: fingerprint,
	}

	_, err := svc.RegisterAgent(ctx, req)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

// =============================================================================
// RegisterClient Tests
// =============================================================================

func TestRegisterClient_Success(t *testing.T) {
	s := createTestStore(t)

	// Seed a member principal who will register the client
	member := seedPrincipal(t, s, "member-010", "Member User")

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	fingerprint := strings.Repeat("ee", 32)
	req := &pb.RegisterClientRequest{
		DisplayName: "My New Client",
		Fingerprint: fingerprint,
	}

	resp, err := svc.RegisterClient(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.PrincipalId)
	assert.Equal(t, "approved", resp.Status)

	// Verify principal was created in store
	principal, err := s.GetPrincipal(ctx, resp.PrincipalId)
	require.NoError(t, err)
	assert.Equal(t, store.PrincipalTypeClient, principal.Type)
	assert.Equal(t, "My New Client", principal.DisplayName)
	assert.Equal(t, fingerprint, principal.PubkeyFP)
}

func TestRegisterClient_PermissionDenied_NoRoles(t *testing.T) {
	s := createTestStore(t)
	member := seedPrincipal(t, s, "viewer-010", "Viewer User")

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	req := &pb.RegisterClientRequest{
		DisplayName: "Client",
		Fingerprint: strings.Repeat("ff", 32),
	}

	_, err := svc.RegisterClient(ctx, req)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestRegisterClient_ValidationErrors(t *testing.T) {
	s := createTestStore(t)
	member := seedPrincipal(t, s, "member-011", "Member")

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"admin"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	tests := []struct {
		name        string
		displayName string
		fingerprint string
		wantCode    codes.Code
	}{
		{
			name:        "whitespace display name",
			displayName: "   ",
			fingerprint: strings.Repeat("00", 32),
			wantCode:    codes.InvalidArgument,
		},
		{
			name:        "invalid hex fingerprint",
			displayName: "Valid Name",
			fingerprint: strings.Repeat("zz", 32),
			wantCode:    codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &pb.RegisterClientRequest{
				DisplayName: tt.displayName,
				Fingerprint: tt.fingerprint,
			}
			_, err := svc.RegisterClient(ctx, req)
			require.Error(t, err)
			st, _ := status.FromError(err)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}

func TestRegisterClient_DuplicateFingerprint(t *testing.T) {
	s := createTestStore(t)
	member := seedPrincipal(t, s, "member-012", "Member")

	fingerprint := strings.Repeat("11", 32)

	// Create existing principal with the same fingerprint
	existing := &store.Principal{
		ID:          "existing-client",
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    fingerprint,
		DisplayName: "Existing Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), existing))

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"owner"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	req := &pb.RegisterClientRequest{
		DisplayName: "Duplicate Client",
		Fingerprint: fingerprint,
	}

	_, err := svc.RegisterClient(ctx, req)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

// =============================================================================
// Integration: Rate Limiting with Real Registrations
// =============================================================================

func TestRegisterAgent_RateLimitExceeded(t *testing.T) {
	// Save original rate limiter and restore after test
	originalLimiter := regRateLimiter
	t.Cleanup(func() {
		regRateLimiter = originalLimiter
	})

	// Reset global rate limiter for clean test
	regRateLimiter = &registrationRateLimiter{
		records:     make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}

	s := createTestStore(t)
	member := seedPrincipal(t, s, "member-rate-001", "Member")

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	// Register up to the limit
	for i := range maxRegistrationsPerWindow {
		// Use hex formatting for robustness if maxRegistrationsPerWindow > 10
		fingerprint := strings.Repeat("a", 60) + fmt.Sprintf("%04x", i)
		req := &pb.RegisterAgentRequest{
			DisplayName: "Agent",
			Fingerprint: fingerprint,
		}
		_, err := svc.RegisterAgent(ctx, req)
		require.NoError(t, err, "registration %d should succeed", i+1)
	}

	// Next registration should be rate limited
	req := &pb.RegisterAgentRequest{
		DisplayName: "One More Agent",
		Fingerprint: strings.Repeat("bb", 32),
	}
	_, err := svc.RegisterAgent(ctx, req)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Contains(t, st.Message(), "rate limit")
}

func TestRegisterClient_RateLimitExceeded(t *testing.T) {
	// Save original rate limiter and restore after test
	originalLimiter := regRateLimiter
	t.Cleanup(func() {
		regRateLimiter = originalLimiter
	})

	// Reset global rate limiter for clean test
	regRateLimiter = &registrationRateLimiter{
		records:     make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}

	s := createTestStore(t)
	member := seedPrincipal(t, s, "member-rate-002", "Member")

	authCtx := &auth.AuthContext{
		PrincipalID:   member.ID,
		PrincipalType: string(member.Type),
		Roles:         []string{"member"},
	}
	ctx := auth.WithAuth(context.Background(), authCtx)

	svc := NewClientService(s, s)

	// Register up to the limit
	for i := range maxRegistrationsPerWindow {
		// Use hex formatting for robustness if maxRegistrationsPerWindow > 10
		fingerprint := strings.Repeat("c", 60) + fmt.Sprintf("%04x", i)
		req := &pb.RegisterClientRequest{
			DisplayName: "Client",
			Fingerprint: fingerprint,
		}
		_, err := svc.RegisterClient(ctx, req)
		require.NoError(t, err, "registration %d should succeed", i+1)
	}

	// Next registration should be rate limited
	req := &pb.RegisterClientRequest{
		DisplayName: "One More Client",
		Fingerprint: strings.Repeat("dd", 32),
	}
	_, err := svc.RegisterClient(ctx, req)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}
