// ABOUTME: gRPC interceptors for authenticating requests using JWT tokens
// ABOUTME: Extracts auth from metadata and populates context for handlers

package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/2389/fold-gateway/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// PrincipalStore defines the interface for retrieving principals
type PrincipalStore interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
}

// RoleStore defines the interface for retrieving roles
type RoleStore interface {
	ListRoles(ctx context.Context, subjectType store.RoleSubjectType, subjectID string) ([]store.RoleName, error)
}

// UnaryInterceptor returns a gRPC unary interceptor that authenticates requests
func UnaryInterceptor(principals PrincipalStore, roles RoleStore, tokens TokenVerifier) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		authCtx, err := extractAuth(ctx, principals, roles, tokens)
		if err != nil {
			return nil, err
		}

		ctx = WithAuth(ctx, authCtx)
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC stream interceptor that authenticates requests
func StreamInterceptor(principals PrincipalStore, roles RoleStore, tokens TokenVerifier) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		authCtx, err := extractAuth(ss.Context(), principals, roles, tokens)
		if err != nil {
			return err
		}

		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          WithAuth(ss.Context(), authCtx),
		}
		return handler(srv, wrapped)
	}
}

// wrappedServerStream wraps a grpc.ServerStream with a custom context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// extractAuth performs the authentication flow:
// 1. Get token from metadata: "authorization: Bearer <token>"
// 2. Verify token, extract principal_id
// 3. Lookup principal (return UNAUTHENTICATED if not found)
// 4. Check status: allow approved/online/offline, deny pending/revoked (PERMISSION_DENIED)
// 5. Lookup roles
// 6. Build AuthContext
func extractAuth(ctx context.Context, principals PrincipalStore, roles RoleStore, tokens TokenVerifier) (*AuthContext, error) {
	// Step 1: Get token from metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Step 2: Verify token, extract principal_id
	principalID, err := tokens.Verify(tokenString)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
	}

	// Step 3: Lookup principal
	principal, err := principals.GetPrincipal(ctx, principalID)
	if err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			return nil, status.Error(codes.Unauthenticated, "principal not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
	}

	// Step 4: Check status - allow approved/online/offline, deny pending/revoked
	switch principal.Status {
	case store.PrincipalStatusApproved, store.PrincipalStatusOnline, store.PrincipalStatusOffline:
		// allowed
	case store.PrincipalStatusPending:
		return nil, status.Error(codes.PermissionDenied, "principal status is pending")
	case store.PrincipalStatusRevoked:
		return nil, status.Error(codes.PermissionDenied, "principal has been revoked")
	default:
		return nil, status.Errorf(codes.Internal, "unknown principal status: %s", principal.Status)
	}

	// Step 5: Lookup roles
	roleNames, err := roles.ListRoles(ctx, store.RoleSubjectPrincipal, principalID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to lookup roles: %v", err)
	}

	// Convert role names to strings
	roleStrings := make([]string, len(roleNames))
	for i, r := range roleNames {
		roleStrings[i] = string(r)
	}

	// Step 6: Build AuthContext
	authCtx := &AuthContext{
		PrincipalID:   principalID,
		PrincipalType: string(principal.Type),
		MemberID:      nil, // always nil in v1
		Roles:         roleStrings,
	}

	return authCtx, nil
}
