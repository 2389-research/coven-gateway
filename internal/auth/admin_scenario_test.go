// ABOUTME: End-to-end scenario tests for admin gate using real SQLite
// ABOUTME: Validates full auth + admin gate interceptor chain without mocking

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/2389/fold-gateway/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestScenario_AdminGateFullFlow(t *testing.T) {
	// 1. Create real SQLite store in temp dir
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal with admin role
	principalID := "admin-gate-test-principal"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "admingatetest123admingatetest123admingatetest123admingatetest1",
		DisplayName: "Admin Gate Test Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// Add admin role
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleAdmin); err != nil {
		t.Fatalf("failed to add admin role: %v", err)
	}

	// 3. Generate real JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Chain auth interceptor -> admin gate interceptor
	authInterceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)
	adminGateInterceptor := RequireAdmin()

	reqCtx := scenarioContextWithAuth(token)

	// First apply auth interceptor
	var authCtx context.Context
	authHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		authCtx = ctx
		return nil, nil
	}

	_, err = authInterceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, authHandler)
	if err != nil {
		t.Fatalf("auth interceptor error: %v", err)
	}

	// 5. Verify AdminService call succeeds through admin gate
	handlerCalled := false
	adminHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "admin-success", nil
	}

	adminInfo := &grpc.UnaryServerInfo{FullMethod: "/fold.AdminService/ListPrincipals"}
	resp, err := adminGateInterceptor(authCtx, nil, adminInfo, adminHandler)

	if err != nil {
		t.Fatalf("admin gate error: %v", err)
	}

	if !handlerCalled {
		t.Error("admin handler was not called")
	}

	if resp != "admin-success" {
		t.Errorf("response = %v, want %v", resp, "admin-success")
	}

	// Verify the auth context has admin role
	auth := FromContext(authCtx)
	if auth == nil {
		t.Fatal("AuthContext not found in context")
	}

	if !auth.IsAdmin() {
		t.Error("IsAdmin() should be true")
	}
}

func TestScenario_NonAdminBlockedByGate(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal with only member role (not admin)
	principalID := "non-admin-gate-test-principal"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "nonadmingatetest123nonadmingatetest123nonadmingatetest123nona",
		DisplayName: "Non-Admin Gate Test Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// Add only member role
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		t.Fatalf("failed to add member role: %v", err)
	}

	// 3. Generate real JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Chain auth interceptor -> admin gate interceptor
	authInterceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)
	adminGateInterceptor := RequireAdmin()

	reqCtx := scenarioContextWithAuth(token)

	// First apply auth interceptor
	var authCtx context.Context
	authHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		authCtx = ctx
		return nil, nil
	}

	_, err = authInterceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, authHandler)
	if err != nil {
		t.Fatalf("auth interceptor error: %v", err)
	}

	// 5. Verify AdminService call returns PERMISSION_DENIED
	adminHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Error("admin handler should not be called for non-admin")
		return nil, nil
	}

	adminInfo := &grpc.UnaryServerInfo{FullMethod: "/fold.AdminService/ListPrincipals"}
	_, err = adminGateInterceptor(authCtx, nil, adminInfo, adminHandler)

	if err == nil {
		t.Fatal("expected error for non-admin principal")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}

	// Verify the auth context does not have admin role
	auth := FromContext(authCtx)
	if auth == nil {
		t.Fatal("AuthContext not found in context")
	}

	if auth.IsAdmin() {
		t.Error("IsAdmin() should be false")
	}
}

func TestScenario_OwnerPassesAdminGate(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal with owner role (owner is treated as admin)
	principalID := "owner-gate-test-principal"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "ownergatetest123ownergatetest123ownergatetest123ownergatetest",
		DisplayName: "Owner Gate Test Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// Add owner role
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleOwner); err != nil {
		t.Fatalf("failed to add owner role: %v", err)
	}

	// 3. Generate real JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Chain auth interceptor -> admin gate interceptor
	authInterceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)
	adminGateInterceptor := RequireAdmin()

	reqCtx := scenarioContextWithAuth(token)

	// First apply auth interceptor
	var authCtx context.Context
	authHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		authCtx = ctx
		return nil, nil
	}

	_, err = authInterceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, authHandler)
	if err != nil {
		t.Fatalf("auth interceptor error: %v", err)
	}

	// 5. Verify AdminService call succeeds for owner
	handlerCalled := false
	adminHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "owner-success", nil
	}

	adminInfo := &grpc.UnaryServerInfo{FullMethod: "/fold.AdminService/RevokePrincipal"}
	resp, err := adminGateInterceptor(authCtx, nil, adminInfo, adminHandler)

	if err != nil {
		t.Fatalf("admin gate error for owner: %v", err)
	}

	if !handlerCalled {
		t.Error("admin handler was not called for owner")
	}

	if resp != "owner-success" {
		t.Errorf("response = %v, want %v", resp, "owner-success")
	}

	// Verify owner passes IsAdmin check
	auth := FromContext(authCtx)
	if auth == nil {
		t.Fatal("AuthContext not found in context")
	}

	if !auth.IsAdmin() {
		t.Error("IsAdmin() should be true for owner")
	}
}

func TestScenario_NonAdminServiceBypassesGate(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal with only member role
	principalID := "member-bypass-test-principal"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "memberbypasstest123memberbypasstest123memberbypasstest123memb",
		DisplayName: "Member Bypass Test Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// Add only member role
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		t.Fatalf("failed to add member role: %v", err)
	}

	// 3. Generate real JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Chain auth interceptor -> admin gate interceptor
	authInterceptor := UnaryInterceptor(s, s, verifier, nil, nil, nil)
	adminGateInterceptor := RequireAdmin()

	reqCtx := scenarioContextWithAuth(token)

	// First apply auth interceptor
	var authCtx context.Context
	authHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		authCtx = ctx
		return nil, nil
	}

	_, err = authInterceptor(reqCtx, nil, &grpc.UnaryServerInfo{}, authHandler)
	if err != nil {
		t.Fatalf("auth interceptor error: %v", err)
	}

	// 5. Verify non-admin service call succeeds (bypasses admin gate)
	handlerCalled := false
	clientHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "client-success", nil
	}

	// ClientService should bypass the admin gate
	clientInfo := &grpc.UnaryServerInfo{FullMethod: "/fold.ClientService/SendMessage"}
	resp, err := adminGateInterceptor(authCtx, nil, clientInfo, clientHandler)

	if err != nil {
		t.Fatalf("client service error: %v", err)
	}

	if !handlerCalled {
		t.Error("client handler was not called")
	}

	if resp != "client-success" {
		t.Errorf("response = %v, want %v", resp, "client-success")
	}
}

func TestScenario_StreamAdminGateFullFlow(t *testing.T) {
	// 1. Create real SQLite store
	s := createTestStore(t)
	ctx := context.Background()

	// 2. Create principal with admin role
	principalID := "stream-admin-gate-test"
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    "streamadmingatetest123streamadmingatetest123streamadmingate12",
		DisplayName: "Stream Admin Gate Test Client",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := s.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("failed to create principal: %v", err)
	}

	// Add admin role
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleAdmin); err != nil {
		t.Fatalf("failed to add admin role: %v", err)
	}

	// 3. Generate real JWT
	verifier, err := NewJWTVerifier(scenarioTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}
	token, err := verifier.Generate(principalID, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Chain stream auth interceptor -> admin gate stream interceptor
	authInterceptor := StreamInterceptor(s, s, verifier, nil, nil, nil)
	adminGateInterceptor := RequireAdminStream()

	reqCtx := scenarioContextWithAuth(token)
	stream := &mockServerStream{ctx: reqCtx}

	// First apply stream auth interceptor
	var authStream grpc.ServerStream
	authHandler := func(srv interface{}, ss grpc.ServerStream) error {
		authStream = ss
		return nil
	}

	err = authInterceptor(nil, stream, &grpc.StreamServerInfo{}, authHandler)
	if err != nil {
		t.Fatalf("stream auth interceptor error: %v", err)
	}

	// 5. Verify AdminService stream call succeeds
	handlerCalled := false
	adminHandler := func(srv interface{}, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	adminInfo := &grpc.StreamServerInfo{FullMethod: "/fold.AdminService/WatchPrincipals"}
	err = adminGateInterceptor(nil, authStream, adminInfo, adminHandler)

	if err != nil {
		t.Fatalf("stream admin gate error: %v", err)
	}

	if !handlerCalled {
		t.Error("stream admin handler was not called")
	}

	// Verify auth context
	auth := FromContext(authStream.Context())
	if auth == nil {
		t.Fatal("AuthContext not found in stream context")
	}

	if !auth.IsAdmin() {
		t.Error("IsAdmin() should be true")
	}
}
