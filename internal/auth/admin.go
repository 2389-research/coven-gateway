// ABOUTME: Admin gate interceptor restricting AdminService to admin/owner roles
// ABOUTME: Used as second interceptor after authentication to enforce RBAC

package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RequireAdmin returns a gRPC unary interceptor that enforces admin role
// for AdminService methods. Non-admin services pass through unchanged.
func RequireAdmin() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Skip for non-admin services
		if !strings.HasPrefix(info.FullMethod, "/coven.AdminService/") {
			return handler(ctx, req)
		}

		auth := FromContext(ctx)
		if auth == nil {
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}

		if !auth.IsAdmin() {
			return nil, status.Error(codes.PermissionDenied, "admin role required")
		}

		return handler(ctx, req)
	}
}

// RequireAdminStream returns a gRPC stream interceptor that enforces admin role
// for AdminService streaming methods.
func RequireAdminStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip for non-admin services
		if !strings.HasPrefix(info.FullMethod, "/coven.AdminService/") {
			return handler(srv, ss)
		}

		auth := FromContext(ss.Context())
		if auth == nil {
			return status.Error(codes.Unauthenticated, "authentication required")
		}

		if !auth.IsAdmin() {
			return status.Error(codes.PermissionDenied, "admin role required")
		}

		return handler(srv, ss)
	}
}
