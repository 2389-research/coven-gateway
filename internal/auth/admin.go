// ABOUTME: Admin gate interceptor restricting AdminService to admin/owner roles
// ABOUTME: Used as second interceptor after authentication to enforce RBAC

package auth

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RequireAdmin returns a gRPC unary interceptor that enforces admin role
// for AdminService methods. Non-admin services pass through unchanged.
func RequireAdmin(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Skip for non-admin services
		if !strings.HasPrefix(info.FullMethod, "/coven.AdminService/") {
			return handler(ctx, req)
		}

		auth := FromContext(ctx)
		if auth == nil {
			logAuthFailure(logger, ctx, "not_authenticated")
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}

		if !auth.IsAdmin() {
			logAuthFailure(logger, ctx, "admin_required", "principal_id", auth.PrincipalID, "roles", auth.Roles)
			return nil, status.Error(codes.PermissionDenied, "admin role required")
		}

		return handler(ctx, req)
	}
}

// RequireAdminStream returns a gRPC stream interceptor that enforces admin role
// for AdminService streaming methods.
func RequireAdminStream(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip for non-admin services
		if !strings.HasPrefix(info.FullMethod, "/coven.AdminService/") {
			return handler(srv, ss)
		}

		auth := FromContext(ss.Context())
		if auth == nil {
			logAuthFailure(logger, ss.Context(), "not_authenticated")
			return status.Error(codes.Unauthenticated, "authentication required")
		}

		if !auth.IsAdmin() {
			logAuthFailure(logger, ss.Context(), "admin_required", "principal_id", auth.PrincipalID, "roles", auth.Roles)
			return status.Error(codes.PermissionDenied, "admin role required")
		}

		return handler(srv, ss)
	}
}
