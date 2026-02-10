// ABOUTME: Unit tests for admin gate interceptor
// ABOUTME: Tests role-based access control for AdminService methods

package auth

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAdminGate_AdminCan(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with admin auth
	authCtx := &AuthContext{
		PrincipalID:   "admin-principal",
		PrincipalType: "client",
		Roles:         []string{"admin"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "success", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AdminService/ListPrincipals"}
	resp, err := interceptor(ctx, nil, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called")
	}

	if resp != "success" {
		t.Errorf("response = %v, want %v", resp, "success")
	}
}

func TestAdminGate_NonAdminCannot(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with member (non-admin) auth
	authCtx := &AuthContext{
		PrincipalID:   "member-principal",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called for non-admin")
		return nil, errors.New("unexpected handler call")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AdminService/ListPrincipals"}
	_, err := interceptor(ctx, nil, info, handler)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}

	if st.Message() != "admin role required" {
		t.Errorf("message = %q, want %q", st.Message(), "admin role required")
	}
}

func TestAdminGate_OwnerCan(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with owner auth (owner is also considered admin)
	authCtx := &AuthContext{
		PrincipalID:   "owner-principal",
		PrincipalType: "client",
		Roles:         []string{"owner"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "owner-success", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AdminService/RevokePrincipal"}
	resp, err := interceptor(ctx, nil, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called for owner")
	}

	if resp != "owner-success" {
		t.Errorf("response = %v, want %v", resp, "owner-success")
	}
}

func TestAdminGate_ClientServiceOpen(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with member (non-admin) auth
	authCtx := &AuthContext{
		PrincipalID:   "member-principal",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "client-success", nil
	}

	// ClientService is not an AdminService, should pass through
	info := &grpc.UnaryServerInfo{FullMethod: "/coven.ClientService/SendMessage"}
	resp, err := interceptor(ctx, nil, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called for ClientService")
	}

	if resp != "client-success" {
		t.Errorf("response = %v, want %v", resp, "client-success")
	}
}

func TestAdminGate_AgentServiceOpen(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with member (non-admin) auth
	authCtx := &AuthContext{
		PrincipalID:   "agent-principal",
		PrincipalType: "agent",
		Roles:         []string{"member"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "agent-success", nil
	}

	// AgentService is not an AdminService, should pass through
	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AgentService/Register"}
	resp, err := interceptor(ctx, nil, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called for AgentService")
	}

	if resp != "agent-success" {
		t.Errorf("response = %v, want %v", resp, "agent-success")
	}
}

func TestAdminGate_NoAuthContext(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context without any auth
	ctx := context.Background()

	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called without auth")
		return nil, errors.New("unexpected handler call")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AdminService/ListPrincipals"}
	_, err := interceptor(ctx, nil, info, handler)

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

	if st.Message() != "authentication required" {
		t.Errorf("message = %q, want %q", st.Message(), "authentication required")
	}
}

func TestAdminGateStream_AdminCan(t *testing.T) {
	interceptor := RequireAdminStream()

	// Create context with admin auth
	authCtx := &AuthContext{
		PrincipalID:   "admin-principal",
		PrincipalType: "client",
		Roles:         []string{"admin"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	stream := &mockServerStream{ctx: ctx}

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/coven.AdminService/WatchPrincipals"}
	err := interceptor(nil, stream, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called")
	}
}

func TestAdminGateStream_NonAdminCannot(t *testing.T) {
	interceptor := RequireAdminStream()

	// Create context with member (non-admin) auth
	authCtx := &AuthContext{
		PrincipalID:   "member-principal",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	stream := &mockServerStream{ctx: ctx}

	handler := func(srv any, ss grpc.ServerStream) error {
		t.Error("handler should not be called for non-admin")
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/coven.AdminService/WatchPrincipals"}
	err := interceptor(nil, stream, info, handler)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
}

func TestAdminGateStream_NonAdminServicePassThrough(t *testing.T) {
	interceptor := RequireAdminStream()

	// Create context with member (non-admin) auth
	authCtx := &AuthContext{
		PrincipalID:   "member-principal",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	stream := &mockServerStream{ctx: ctx}

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	// Non-admin service should pass through even for non-admin users
	info := &grpc.StreamServerInfo{FullMethod: "/coven.CovenControl/Stream"}
	err := interceptor(nil, stream, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called for non-admin service")
	}
}

func TestAdminGateStream_NoAuthContext(t *testing.T) {
	interceptor := RequireAdminStream()

	// Create context without any auth
	ctx := context.Background()

	stream := &mockServerStream{ctx: ctx}

	handler := func(srv any, ss grpc.ServerStream) error {
		t.Error("handler should not be called without auth")
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/coven.AdminService/WatchPrincipals"}
	err := interceptor(nil, stream, info, handler)

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

func TestAdminGate_EmptyRoles(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with auth but no roles
	authCtx := &AuthContext{
		PrincipalID:   "no-roles-principal",
		PrincipalType: "client",
		Roles:         []string{},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handler := func(ctx context.Context, req any) (any, error) {
		t.Error("handler should not be called with no roles")
		return nil, errors.New("unexpected handler call")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AdminService/ListPrincipals"}
	_, err := interceptor(ctx, nil, info, handler)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("status code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
}

func TestAdminGate_MultipleRolesIncludingAdmin(t *testing.T) {
	interceptor := RequireAdmin()

	// Create context with multiple roles including admin
	authCtx := &AuthContext{
		PrincipalID:   "multi-role-principal",
		PrincipalType: "client",
		Roles:         []string{"member", "admin", "viewer"},
	}
	ctx := WithAuth(context.Background(), authCtx)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "multi-success", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/coven.AdminService/ListPrincipals"}
	resp, err := interceptor(ctx, nil, info, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !handlerCalled {
		t.Error("handler was not called")
	}

	if resp != "multi-success" {
		t.Errorf("response = %v, want %v", resp, "multi-success")
	}
}
