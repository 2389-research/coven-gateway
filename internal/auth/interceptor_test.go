// ABOUTME: Unit tests for gRPC auth interceptors
// ABOUTME: Tests authentication flow with mocked stores for unit isolation

package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/2389/fold-gateway/internal/store"
	"golang.org/x/crypto/ssh"
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

func (m *mockPrincipalStore) GetPrincipalByPubkey(ctx context.Context, fingerprint string) (*store.Principal, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.principal != nil && m.principal.PubkeyFP == fingerprint {
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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

			interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)
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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := UnaryInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := StreamInterceptor(principals, roles, verifier, nil, nil, nil)

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

	interceptor := StreamInterceptor(principals, roles, verifier, nil, nil, nil)

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

// mockPrincipalCreator implements PrincipalCreator for testing
type mockPrincipalCreator struct {
	created   []*store.Principal
	createErr error
}

func (m *mockPrincipalCreator) CreatePrincipal(ctx context.Context, p *store.Principal) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, p)
	return nil
}

// mockPrincipalStoreWithCreator implements both PrincipalStore and tracks created principals
type mockPrincipalStoreWithCreator struct {
	principals map[string]*store.Principal
	err        error
}

func newMockPrincipalStoreWithCreator() *mockPrincipalStoreWithCreator {
	return &mockPrincipalStoreWithCreator{
		principals: make(map[string]*store.Principal),
	}
}

func (m *mockPrincipalStoreWithCreator) GetPrincipal(ctx context.Context, id string) (*store.Principal, error) {
	if m.err != nil {
		return nil, m.err
	}
	if p, ok := m.principals[id]; ok {
		return p, nil
	}
	return nil, store.ErrPrincipalNotFound
}

func (m *mockPrincipalStoreWithCreator) GetPrincipalByPubkey(ctx context.Context, fingerprint string) (*store.Principal, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, p := range m.principals {
		if p.PubkeyFP == fingerprint {
			return p, nil
		}
	}
	return nil, store.ErrPrincipalNotFound
}

func (m *mockPrincipalStoreWithCreator) CreatePrincipal(ctx context.Context, p *store.Principal) error {
	if m.err != nil {
		return m.err
	}
	m.principals[p.ID] = p
	return nil
}

// generateTestKeyPairForInterceptor creates a new ed25519 key pair for testing
func generateTestKeyPairForInterceptor(t *testing.T) (ssh.Signer, ssh.PublicKey, string) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to create SSH public key: %v", err)
	}

	pubkeyStr := string(ssh.MarshalAuthorizedKey(sshPub))
	return signer, sshPub, pubkeyStr
}

// signMessageForInterceptor creates an SSH signature over a message
func signMessageForInterceptor(t *testing.T, signer ssh.Signer, message string) string {
	t.Helper()

	sig, err := signer.Sign(rand.Reader, []byte(message))
	if err != nil {
		t.Fatalf("failed to sign message: %v", err)
	}

	return base64.StdEncoding.EncodeToString(ssh.Marshal(sig))
}

// contextWithSSHAuth creates a context with SSH auth headers
func contextWithSSHAuth(pubkey, signature string, timestamp int64, nonce string) context.Context {
	md := metadata.New(map[string]string{
		SSHPubkeyHeader:    pubkey,
		SSHSignatureHeader: signature,
		SSHTimestampHeader: fmt.Sprintf("%d", timestamp),
		SSHNonceHeader:     nonce,
	})
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestExtractAuth_AutoCreatesPrincipalApproved(t *testing.T) {
	signer, pubkey, pubkeyStr := generateTestKeyPairForInterceptor(t)
	fingerprint := ComputeFingerprint(pubkey)

	principals := newMockPrincipalStoreWithCreator()
	roles := &mockRoleStore{
		roles: []store.RoleName{}, // No roles for new principal
	}
	sshVerifier := NewSSHVerifier()
	jwtVerifier, _ := NewJWTVerifier(interceptorTestSecret)

	config := &AuthConfig{
		AgentAutoRegistration: "approved",
	}

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessageForInterceptor(t, signer, message)

	ctx := contextWithSSHAuth(pubkeyStr, signature, timestamp, nonce)

	interceptor := UnaryInterceptor(principals, roles, jwtVerifier, sshVerifier, config, principals)

	handlerCalled := false
	var capturedCtx context.Context

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		capturedCtx = ctx
		return "response", nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called")
	}

	// Verify principal was auto-created
	if len(principals.principals) != 1 {
		t.Fatalf("expected 1 principal, got %d", len(principals.principals))
	}

	var createdPrincipal *store.Principal
	for _, p := range principals.principals {
		createdPrincipal = p
		break
	}

	if createdPrincipal.Type != store.PrincipalTypeAgent {
		t.Errorf("Type = %v, want %v", createdPrincipal.Type, store.PrincipalTypeAgent)
	}

	if createdPrincipal.Status != store.PrincipalStatusApproved {
		t.Errorf("Status = %v, want %v", createdPrincipal.Status, store.PrincipalStatusApproved)
	}

	if createdPrincipal.PubkeyFP != fingerprint {
		t.Errorf("PubkeyFP = %v, want %v", createdPrincipal.PubkeyFP, fingerprint)
	}

	// DisplayName should be "agent-" + last 8 chars of fingerprint
	expectedSuffix := fingerprint
	if len(expectedSuffix) > 8 {
		expectedSuffix = expectedSuffix[len(expectedSuffix)-8:]
	}
	expectedDisplayName := "agent-" + expectedSuffix
	if createdPrincipal.DisplayName != expectedDisplayName {
		t.Errorf("DisplayName = %v, want %v", createdPrincipal.DisplayName, expectedDisplayName)
	}

	// Verify auth context
	authCtx := FromContext(capturedCtx)
	if authCtx == nil {
		t.Fatal("AuthContext not set in context")
	}

	if authCtx.PrincipalID != createdPrincipal.ID {
		t.Errorf("PrincipalID = %q, want %q", authCtx.PrincipalID, createdPrincipal.ID)
	}

	if authCtx.PrincipalType != "agent" {
		t.Errorf("PrincipalType = %q, want %q", authCtx.PrincipalType, "agent")
	}
}

func TestExtractAuth_AutoCreatesPrincipalPending(t *testing.T) {
	signer, pubkey, pubkeyStr := generateTestKeyPairForInterceptor(t)
	fingerprint := ComputeFingerprint(pubkey)

	principals := newMockPrincipalStoreWithCreator()
	roles := &mockRoleStore{
		roles: []store.RoleName{},
	}
	sshVerifier := NewSSHVerifier()
	jwtVerifier, _ := NewJWTVerifier(interceptorTestSecret)

	config := &AuthConfig{
		AgentAutoRegistration: "pending",
	}

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessageForInterceptor(t, signer, message)

	ctx := contextWithSSHAuth(pubkeyStr, signature, timestamp, nonce)

	interceptor := UnaryInterceptor(principals, roles, jwtVerifier, sshVerifier, config, principals)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called for pending principal")
		return nil, nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	// Should fail because pending principals are not allowed to proceed
	if err == nil {
		t.Fatal("expected error for pending principal, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}

	// Verify principal was auto-created with pending status
	if len(principals.principals) != 1 {
		t.Fatalf("expected 1 principal, got %d", len(principals.principals))
	}

	var createdPrincipal *store.Principal
	for _, p := range principals.principals {
		createdPrincipal = p
		break
	}

	if createdPrincipal.Type != store.PrincipalTypeAgent {
		t.Errorf("Type = %v, want %v", createdPrincipal.Type, store.PrincipalTypeAgent)
	}

	if createdPrincipal.Status != store.PrincipalStatusPending {
		t.Errorf("Status = %v, want %v", createdPrincipal.Status, store.PrincipalStatusPending)
	}

	if createdPrincipal.PubkeyFP != fingerprint {
		t.Errorf("PubkeyFP = %v, want %v", createdPrincipal.PubkeyFP, fingerprint)
	}
}

func TestExtractAuth_RejectsUnknownWhenDisabled(t *testing.T) {
	signer, _, pubkeyStr := generateTestKeyPairForInterceptor(t)

	principals := newMockPrincipalStoreWithCreator()
	roles := &mockRoleStore{}
	sshVerifier := NewSSHVerifier()
	jwtVerifier, _ := NewJWTVerifier(interceptorTestSecret)

	config := &AuthConfig{
		AgentAutoRegistration: "disabled",
	}

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessageForInterceptor(t, signer, message)

	ctx := contextWithSSHAuth(pubkeyStr, signature, timestamp, nonce)

	interceptor := UnaryInterceptor(principals, roles, jwtVerifier, sshVerifier, config, principals)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}

	// Verify no principal was created
	if len(principals.principals) != 0 {
		t.Errorf("expected 0 principals, got %d", len(principals.principals))
	}
}

func TestExtractAuth_RejectsUnknownWhenConfigNil(t *testing.T) {
	signer, _, pubkeyStr := generateTestKeyPairForInterceptor(t)

	principals := newMockPrincipalStoreWithCreator()
	roles := &mockRoleStore{}
	sshVerifier := NewSSHVerifier()
	jwtVerifier, _ := NewJWTVerifier(interceptorTestSecret)

	timestamp := time.Now().Unix()
	nonce := "test-nonce-12345"
	message := fmt.Sprintf("%d|%s", timestamp, nonce)
	signature := signMessageForInterceptor(t, signer, message)

	ctx := contextWithSSHAuth(pubkeyStr, signature, timestamp, nonce)

	// config is nil, so auto-registration should be disabled
	interceptor := UnaryInterceptor(principals, roles, jwtVerifier, sshVerifier, nil, nil)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("status code = %v, want %v", st.Code(), codes.Unauthenticated)
	}
}
