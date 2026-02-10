// ABOUTME: End-to-end scenario tests for auth using real SQLite
// ABOUTME: Validates full auth flow without any mocking

package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// createTestStore creates a real SQLite store in a temp directory.
func createTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLite store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
		os.RemoveAll(tmpDir)
	})

	return s
}

// scenarioContextWithAuth creates a context with authorization header.
func scenarioContextWithAuth(token string) context.Context {
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + token,
	})
	return metadata.NewIncomingContext(context.Background(), md)
}

// scenarioTestSecret is a 32-byte secret that meets MinSecretLength requirement.
var scenarioTestSecret = []byte("scenario-test-secret-32-bytes!!!")

func TestScenario_FullAuthFlow(t *testing.T) {
	// 1. Create real SQLite store in temp dir
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create a real principal in the DB
	principalID := "principal-full-auth-test"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		DisplayName: "Test Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// 3. Add real roles to the principal
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleAdmin); err != nil {
		t.Fatalf("failed to add admin role: %v", err)
	}
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		t.Fatalf("failed to add member role: %v", err)
	}

	// 4. Generate a real JWT token
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 5. Call interceptor with real token
	interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)

	reqCtx := scenarioContextWithAuth(token)
	var capturedCtx context.Context
	handlerCalled := false

	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		capturedCtx = ctx
		return "success", nil
	}

	resp, err := interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}

	// 6. Verify AuthContext is populated correctly from real DB data
	if !handlerCalled {
		t.Fatal("handler was not called")
	}

	if resp != "success" {
		t.Errorf("unexpected response: %v", resp)
	}

	authCtx := FromContext(capturedCtx)
	if authCtx == nil {
		t.Fatal("AuthContext not found in context")
	}

	if authCtx.PrincipalID != principalID {
		t.Errorf("PrincipalID = %q, want %q", authCtx.PrincipalID, principalID)
	}

	if authCtx.PrincipalType != "client" {
		t.Errorf("PrincipalType = %q, want %q", authCtx.PrincipalType, "client")
	}

	// Verify roles are populated from real DB
	if len(authCtx.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d: %v", len(authCtx.Roles), authCtx.Roles)
	}

	if !authCtx.IsAdmin() {
		t.Error("IsAdmin() should be true (has admin role)")
	}

	// Verify MemberID is nil in v1
	if authCtx.MemberID != nil {
		t.Error("MemberID should be nil in v1")
	}
}

func TestScenario_RevokedPrincipalDenied(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal, then revoke it
	principalID := "principal-revoked-test"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    "revoked123revoked123revoked123revoked123revoked123revoked12345",
		DisplayName: "Revoked Agent",
		Status:      store.PrincipalStatusApproved, // Start approved
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// Revoke the principal
	if err := s.UpdatePrincipalStatus(ctx, principalID, store.PrincipalStatusRevoked); err != nil {
		t.Fatalf("failed to revoke principal: %v", err)
	}

	// 3. Generate valid JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Call interceptor
	interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)

	reqCtx := scenarioContextWithAuth(token)
	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called for revoked principal")
		return nil, errors.New("unexpected handler call")
	}

	_, err = interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)

	// 5. Verify PERMISSION_DENIED is returned
	if err == nil {
		t.Fatal("expected error for revoked principal")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
}

func TestScenario_ExpiredTokenRejected(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create valid principal
	principalID := "principal-expired-token-test"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "expired123expired123expired123expired123expired123expired12345",
		DisplayName: "Client with Expired Token",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// 3. Generate expired JWT (negative duration)
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, -time.Hour) // Expired 1 hour ago
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Call interceptor
	interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)

	reqCtx := scenarioContextWithAuth(token)
	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called for expired token")
		return nil, errors.New("unexpected handler call")
	}

	_, err = interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)

	// 5. Verify UNAUTHENTICATED is returned
	if err == nil {
		t.Fatal("expected error for expired token")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}

func TestScenario_PendingPrincipalDenied(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal with pending status
	principalID := "principal-pending-test"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypePack,
		PubkeyFP:    "pending123pending123pending123pending123pending123pending12345",
		DisplayName: "Pending Pack",
		Status:      store.PrincipalStatusPending,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// 3. Generate valid JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Call interceptor
	interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)

	reqCtx := scenarioContextWithAuth(token)
	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called for pending principal")
		return nil, errors.New("unexpected handler call")
	}

	_, err = interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)

	// 5. Verify PERMISSION_DENIED is returned
	if err == nil {
		t.Fatal("expected error for pending principal")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
}

func TestScenario_OnlineAndOfflineStatusesAllowed(t *testing.T) {
	s := createTestStore(t)
	ctx := context.Background()
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	testCases := []struct {
		name   string
		status store.PrincipalStatus
	}{
		{"online", store.PrincipalStatusOnline},
		{"offline", store.PrincipalStatusOffline},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			principalID := "principal-status-" + tc.name
			principal := &store.Principal{
				ID:   principalID,
				Type: store.PrincipalTypeAgent,
				// Make unique pubkey fingerprint for each test
				PubkeyFP:    "status" + tc.name + "status" + tc.name + "status" + tc.name + "status" + tc.name + string(rune('0'+i)),
				DisplayName: "Agent " + tc.name,
				Status:      tc.status,
				CreatedAt:   time.Now(),
			}
			if err := s.CreatePrincipal(ctx, principal); err != nil {
				t.Fatalf("failed to create principal: %v", err)
			}

			token, err := verifier.Generate(principalID, time.Hour)
			if err != nil {
				t.Fatalf("failed to generate token: %v", err)
			}

			interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)
			reqCtx := scenarioContextWithAuth(token)

			handlerCalled := false
			handler := func(ctx context.Context, req any) (any, error) {
				handlerCalled = true
				authCtx := FromContext(ctx)
				if authCtx == nil {
					t.Error("AuthContext not found")
				}
				return struct{}{}, nil
			}

			_, err = interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)
			if err != nil {
				t.Fatalf("unexpected error for status %s: %v", tc.name, err)
			}

			if !handlerCalled {
				t.Errorf("handler not called for status %s", tc.name)
			}
		})
	}
}

func TestScenario_NonexistentPrincipal(t *testing.T) {
	// 1. Create real SQLite store (empty - no principals)
	s := createTestStore(t)

	// 2. Generate JWT for principal that doesn't exist
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate("nonexistent-principal-id", time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 3. Call interceptor
	interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)

	reqCtx := scenarioContextWithAuth(token)
	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called for nonexistent principal")
		return nil, errors.New("unexpected handler call")
	}

	_, err = interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)

	// 4. Verify UNAUTHENTICATED is returned
	if err == nil {
		t.Fatal("expected error for nonexistent principal")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}

func TestScenario_PrincipalWithNoRoles(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal without adding any roles
	principalID := "principal-no-roles"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "noroles123noroles123noroles123noroles123noroles123noroles123",
		DisplayName: "Client No Roles",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// 3. Generate valid JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Call interceptor
	interceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)

	reqCtx := scenarioContextWithAuth(token)
	var capturedCtx context.Context

	handler := func(ctx context.Context, req any) (any, error) {
		capturedCtx = ctx
		return struct{}{}, nil
	}

	_, err = interceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5. Verify AuthContext has empty roles
	authCtx := FromContext(capturedCtx)
	if authCtx == nil {
		t.Fatal("AuthContext not found")
	}

	if len(authCtx.Roles) != 0 {
		t.Errorf("expected 0 roles, got %d: %v", len(authCtx.Roles), authCtx.Roles)
	}

	if authCtx.IsAdmin() {
		t.Error("IsAdmin() should be false with no roles")
	}
}
