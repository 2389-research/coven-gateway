// ABOUTME: gRPC interceptors for authenticating requests using JWT or SSH keys
// ABOUTME: Extracts auth from metadata and populates context for handlers

package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// PrincipalStore defines the interface for retrieving principals.
type PrincipalStore interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
	GetPrincipalByPubkey(ctx context.Context, fingerprint string) (*store.Principal, error)
}

// PrincipalCreator can create new principals (for auto-registration).
type PrincipalCreator interface {
	CreatePrincipal(ctx context.Context, p *store.Principal) error
}

// RoleStore defines the interface for retrieving roles.
type RoleStore interface {
	ListRoles(ctx context.Context, subjectType store.RoleSubjectType, subjectID string) ([]store.RoleName, error)
}

// AuthConfig holds auth configuration options.
type AuthConfig struct {
	AgentAutoRegistration string // "approved", "pending", or "disabled"
}

// logAuthFailure logs an authentication failure with structured context.
func logAuthFailure(logger *slog.Logger, ctx context.Context, reason string, attrs ...any) {
	if logger == nil {
		return
	}
	// Extract peer address if available
	baseAttrs := []any{"reason", reason}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		baseAttrs = append(baseAttrs, "peer_addr", p.Addr.String())
	}
	baseAttrs = append(baseAttrs, attrs...)
	logger.Warn("auth failure", baseAttrs...)
}

// UnaryInterceptor returns a gRPC unary interceptor that authenticates requests.
// The optional config and creator parameters enable agent auto-registration.
// The optional logger enables auth failure logging for security monitoring.
func UnaryInterceptor(principals PrincipalStore, roles RoleStore, tokens TokenVerifier, sshVerifier *SSHVerifier, config *AuthConfig, creator PrincipalCreator, logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		authCtx, err := extractAuth(ctx, principals, roles, tokens, sshVerifier, config, creator, logger)
		if err != nil {
			return nil, err
		}

		ctx = WithAuth(ctx, authCtx)
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC stream interceptor that authenticates requests.
// The optional config and creator parameters enable agent auto-registration.
// The optional logger enables auth failure logging for security monitoring.
func StreamInterceptor(principals PrincipalStore, roles RoleStore, tokens TokenVerifier, sshVerifier *SSHVerifier, config *AuthConfig, creator PrincipalCreator, logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		authCtx, err := extractAuth(ss.Context(), principals, roles, tokens, sshVerifier, config, creator, logger)
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

// NoAuthUnaryInterceptor returns a gRPC unary interceptor that injects an anonymous
// auth context when authentication is disabled. This prevents handlers that call
// MustFromContext from panicking.
func NoAuthUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Inject anonymous auth context
		authCtx := &AuthContext{
			PrincipalID:   "anonymous",
			PrincipalType: "anonymous",
			MemberID:      nil,
			Roles:         []string{"admin"}, // Grant admin role when auth is disabled
		}
		ctx = WithAuth(ctx, authCtx)
		return handler(ctx, req)
	}
}

// NoAuthStreamInterceptor returns a gRPC stream interceptor that injects an anonymous
// auth context when authentication is disabled.
func NoAuthStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Inject anonymous auth context
		authCtx := &AuthContext{
			PrincipalID:   "anonymous",
			PrincipalType: "anonymous",
			MemberID:      nil,
			Roles:         []string{"admin"}, // Grant admin role when auth is disabled
		}
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          WithAuth(ss.Context(), authCtx),
		}
		return handler(srv, wrapped)
	}
}

// wrappedServerStream wraps a grpc.ServerStream with a custom context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context.
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// extractAuth performs the authentication flow:
// sshAuthResult holds the result of SSH authentication.
type sshAuthResult struct {
	principal      *store.Principal
	autoRegistered bool
}

// validateSSHRequest checks that all required SSH fields are present.
func validateSSHRequest(req *SSHAuthRequest) error {
	if req.Pubkey == "" {
		return status.Error(codes.Unauthenticated, "missing SSH public key")
	}
	if req.Signature == "" {
		return status.Error(codes.Unauthenticated, "missing SSH signature")
	}
	if req.Timestamp == 0 {
		return status.Error(codes.Unauthenticated, "missing SSH timestamp")
	}
	if req.Nonce == "" {
		return status.Error(codes.Unauthenticated, "missing SSH nonce")
	}
	return nil
}

// autoRegisterPrincipal creates a new principal for auto-registration.
func autoRegisterPrincipal(ctx context.Context, fingerprint string, config *AuthConfig, creator PrincipalCreator) (*store.Principal, error) {
	if config == nil || config.AgentAutoRegistration == "disabled" || config.AgentAutoRegistration == "" {
		return nil, status.Error(codes.Unauthenticated, "unknown public key")
	}
	if creator == nil {
		return nil, status.Error(codes.Internal, "auto-registration enabled but no principal creator configured")
	}

	principalStatus := store.PrincipalStatusPending
	if config.AgentAutoRegistration == "approved" {
		principalStatus = store.PrincipalStatusApproved
	}

	shortFP := fingerprint
	if len(shortFP) > 8 {
		shortFP = shortFP[len(shortFP)-8:]
	}

	newPrincipal := &store.Principal{
		ID:          uuid.New().String(),
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fingerprint,
		DisplayName: "agent-" + shortFP,
		Status:      principalStatus,
		CreatedAt:   time.Now().UTC(),
	}

	if err := creator.CreatePrincipal(ctx, newPrincipal); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to auto-create principal: %v", err)
	}
	return newPrincipal, nil
}

// authenticateWithSSH handles SSH-based authentication for agents.
func authenticateWithSSH(ctx context.Context, req *SSHAuthRequest, verifier *SSHVerifier, principals PrincipalStore, config *AuthConfig, creator PrincipalCreator) (*sshAuthResult, error) {
	if verifier == nil {
		return nil, status.Error(codes.Unauthenticated, "SSH authentication not configured")
	}

	if err := validateSSHRequest(req); err != nil {
		return nil, err
	}

	fingerprint, err := verifier.Verify(req)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "SSH auth failed: %v", err)
	}

	p, err := principals.GetPrincipalByPubkey(ctx, fingerprint)
	if err == nil {
		return &sshAuthResult{principal: p, autoRegistered: false}, nil
	}

	if !errors.Is(err, store.ErrPrincipalNotFound) {
		return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
	}

	newPrincipal, err := autoRegisterPrincipal(ctx, fingerprint, config, creator)
	if err != nil {
		return nil, err
	}
	return &sshAuthResult{principal: newPrincipal, autoRegistered: true}, nil
}

// authenticateWithJWT handles JWT-based authentication for clients.
func authenticateWithJWT(ctx context.Context, md metadata.MD, tokens TokenVerifier, principals PrincipalStore) (*store.Principal, error) {
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	principalID, err := tokens.Verify(tokenString)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
	}

	p, err := principals.GetPrincipal(ctx, principalID)
	if err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			return nil, status.Error(codes.Unauthenticated, "principal not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
	}
	return p, nil
}

// validatePrincipalStatusGRPC validates that the principal has an allowed status for gRPC.
func validatePrincipalStatusGRPC(p *store.Principal, autoRegistered bool) error {
	switch p.Status {
	case store.PrincipalStatusApproved, store.PrincipalStatusOnline, store.PrincipalStatusOffline:
		return nil
	case store.PrincipalStatusPending:
		if autoRegistered {
			return status.Errorf(codes.PermissionDenied, "agent registered with pending status (principal_id: %s) - admin approval required", p.ID)
		}
		return status.Error(codes.PermissionDenied, "principal status is pending - admin approval required")
	case store.PrincipalStatusRevoked:
		return status.Error(codes.PermissionDenied, "principal has been revoked")
	default:
		return status.Errorf(codes.Internal, "unknown principal status: %s", p.Status)
	}
}

// buildAuthContextGRPC creates the AuthContext from a principal and roles for gRPC.
func buildAuthContextGRPC(ctx context.Context, p *store.Principal, roles RoleStore) (*AuthContext, error) {
	roleNames, err := roles.ListRoles(ctx, store.RoleSubjectPrincipal, p.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to lookup roles: %v", err)
	}

	roleStrings := make([]string, len(roleNames))
	for i, r := range roleNames {
		roleStrings[i] = string(r)
	}

	return &AuthContext{
		PrincipalID:   p.ID,
		PrincipalType: string(p.Type),
		MemberID:      nil,
		Roles:         roleStrings,
	}, nil
}

// extractAuth extracts authentication context from gRPC metadata.
// Supports SSH auth for agents and JWT auth for clients.
// The optional logger enables auth failure logging for security monitoring.
func extractAuth(ctx context.Context, principals PrincipalStore, roles RoleStore, tokens TokenVerifier, sshVerifier *SSHVerifier, config *AuthConfig, creator PrincipalCreator, logger *slog.Logger) (*AuthContext, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		logAuthFailure(logger, ctx, "missing_metadata")
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	var principal *store.Principal
	var autoRegistered bool

	// Try SSH auth first (for agents)
	if sshReq := ExtractSSHAuthFromMetadata(md); sshReq != nil {
		result, err := authenticateWithSSH(ctx, sshReq, sshVerifier, principals, config, creator)
		if err != nil {
			logAuthFailure(logger, ctx, "ssh_auth_failed", "error", err.Error())
			return nil, err
		}
		principal = result.principal
		autoRegistered = result.autoRegistered
	} else {
		p, err := authenticateWithJWT(ctx, md, tokens, principals)
		if err != nil {
			logAuthFailure(logger, ctx, "jwt_auth_failed", "error", err.Error())
			return nil, err
		}
		principal = p
	}

	if err := validatePrincipalStatusGRPC(principal, autoRegistered); err != nil {
		logAuthFailure(logger, ctx, "principal_status_invalid", "principal_id", principal.ID, "status", string(principal.Status))
		return nil, err
	}

	return buildAuthContextGRPC(ctx, principal, roles)
}
