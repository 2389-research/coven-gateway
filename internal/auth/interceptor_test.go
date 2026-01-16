// ABOUTME: Unit tests for gRPC auth interceptors
// ABOUTME: Tests authentication flow with mocked stores for unit isolation

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/2389/fold-gateway/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// interceptorTestSecret is a 32-byte secret that meets MinSecretLength requirement
var interceptorTestSecret = []byte("interceptor-test-secret-32bytes!")

// mockPrincipalStore implements PrincipalStore for testing
type mockPrincipalStore struct {
	principal *store.Principal
	err       error
}

func (m *mockPrincipalStore) GetPrincipal(ctx context.Context, id string) (*store.Principal, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.principal != nil && m.principal.ID == id {
		return m.principal, nil
	}
	return nil, store.ErrPrincipalNotFound
}

// mockRoleStore implements RoleStore for testing
type mockRoleStore struct {
	roles []store.RoleName
	err   error
}

func (m *mockRoleStore) ListRoles(ctx context.Context, subjectType store.RoleSubjectType, subjectID string) ([]store.RoleName, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.roles, nil
}

// Helper to create test context with authorization header
func contextWithAuth(token string) context.Context {
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + token,
	})
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestAuthInterceptor_ValidToken(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "principal-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusApproved,
		},
	}

	roles := &mockRoleStore{
		roles: []store.RoleName{store.RoleAdmin},
	}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)
	handlerCalled := false
	var capturedCtx context.Context

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		capturedCtx = ctx
		return "response", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called")
	}

	if resp != "response" {
		t.Errorf("response = %v, want %v", resp, "response")
	}

	// Verify auth context was set
	authCtx := FromContext(capturedCtx)
	if authCtx == nil {
		t.Fatal("AuthContext not set in context")
	}

	if authCtx.PrincipalID != principalID {
		t.Errorf("PrincipalID = %q, want %q", authCtx.PrincipalID, principalID)
	}

	if authCtx.PrincipalType != "client" {
		t.Errorf("PrincipalType = %q, want %q", authCtx.PrincipalType, "client")
	}

	if !authCtx.IsAdmin() {
		t.Error("IsAdmin() = false, want true")
	}
}

func TestAuthInterceptor_MissingHeader(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	// Context without authorization header
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_InvalidToken(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth("invalid-token")

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_PrincipalNotFound(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	token, _ := verifier.Generate("nonexistent-principal", time.Hour)

	principals := &mockPrincipalStore{
		// No principal configured, will return ErrPrincipalNotFound
	}
	roles := &mockRoleStore{}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}

func TestAuthInterceptor_PrincipalRevoked(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "revoked-principal"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusRevoked,
		},
	}
	roles := &mockRoleStore{}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
}

func TestAuthInterceptor_PrincipalPending(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "pending-principal"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeAgent,
			Status: store.PrincipalStatusPending,
		},
	}
	roles := &mockRoleStore{}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
}

func TestAuthInterceptor_AllowedStatuses(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	allowedStatuses := []store.PrincipalStatus{
		store.PrincipalStatusApproved,
		store.PrincipalStatusOnline,
		store.PrincipalStatusOffline,
	}

	for _, status := range allowedStatuses {
		t.Run(string(status), func(t *testing.T) {
			principalID := "principal-" + string(status)
			token, _ := verifier.Generate(principalID, time.Hour)

			principals := &mockPrincipalStore{
				principal: &store.Principal{
					ID:     principalID,
					Type:   store.PrincipalTypeClient,
					Status: status,
				},
			}
			roles := &mockRoleStore{
				roles: []store.RoleName{store.RoleMember},
			}

			interceptor := UnaryInterceptor(principals, roles, verifier)
			ctx := contextWithAuth(token)

			handlerCalled := false
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				handlerCalled = true
				return nil, nil
			}

			_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
			if err != nil {
				t.Fatalf("unexpected error for status %s: %v", status, err)
			}

			if !handlerCalled {
				t.Errorf("handler not called for allowed status %s", status)
			}
		})
	}
}

func TestAuthInterceptor_StoreError(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "principal-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		err: errors.New("database connection failed"),
	}
	roles := &mockRoleStore{}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.Internal {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Internal)
	}
}

func TestAuthInterceptor_RoleStoreError(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "principal-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusApproved,
		},
	}
	roles := &mockRoleStore{
		err: errors.New("failed to list roles"),
	}

	interceptor := UnaryInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, interceptErr := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.Internal {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Internal)
	}
}

// mockServerStream implements grpc.ServerStream for testing StreamInterceptor
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func TestStreamInterceptor_ValidToken(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "principal-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusApproved,
		},
	}

	roles := &mockRoleStore{
		roles: []store.RoleName{store.RoleAdmin},
	}

	interceptor := StreamInterceptor(principals, roles, verifier)

	ctx := contextWithAuth(token)
	stream := &mockServerStream{ctx: ctx}

	handlerCalled := false
	var capturedStream grpc.ServerStream

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		handlerCalled = true
		capturedStream = ss
		return nil
	}

	interceptErr := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if interceptErr != nil {
		t.Fatalf("interceptor error = %v", interceptErr)
	}

	if !handlerCalled {
		t.Error("handler was not called")
	}

	// Verify auth context was set in the wrapped stream's context
	authCtx := FromContext(capturedStream.Context())
	if authCtx == nil {
		t.Fatal("AuthContext not set in stream context")
	}

	if authCtx.PrincipalID != principalID {
		t.Errorf("PrincipalID = %q, want %q", authCtx.PrincipalID, principalID)
	}

	if authCtx.PrincipalType != "client" {
		t.Errorf("PrincipalType = %q, want %q", authCtx.PrincipalType, "client")
	}

	if !authCtx.IsAdmin() {
		t.Error("IsAdmin() = false, want true")
	}
}

func TestStreamInterceptor_MissingHeader(t *testing.T) {
	verifier, err := NewJWTVerifier(interceptorTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	interceptor := StreamInterceptor(principals, roles, verifier)

	// Context without authorization header
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))
	stream := &mockServerStream{ctx: ctx}

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		t.Error("handler should not be called")
		return nil
	}

	interceptErr := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if interceptErr == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(interceptErr)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", interceptErr)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}
