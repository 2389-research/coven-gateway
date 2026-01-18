// ABOUTME: gRPC interceptors for authenticating requests using JWT or SSH keys
// ABOUTME: Extracts auth from metadata and populates context for handlers

package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/2389/fold-gateway/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// PrincipalStore defines the interface for retrieving principals
type PrincipalStore interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
	GetPrincipalByPubkey(ctx context.Context, fingerprint string) (*store.Principal, error)
}

// PrincipalCreator can create new principals (for auto-registration)
type PrincipalCreator interface {
	CreatePrincipal(ctx context.Context, p *store.Principal) error
}

// RoleStore defines the interface for retrieving roles
type RoleStore interface {
	ListRoles(ctx context.Context, subjectType store.RoleSubjectType, subjectID string) ([]store.RoleName, error)
}

// AuthConfig holds auth configuration options
type AuthConfig struct {
	AgentAutoRegistration string // "approved", "pending", or "disabled"
}

// UnaryInterceptor returns a gRPC unary interceptor that authenticates requests.
// The optional config and creator parameters enable agent auto-registration.
func UnaryInterceptor(principals PrincipalStore, roles RoleStore, tokens TokenVerifier, sshVerifier *SSHVerifier, config *AuthConfig, creator PrincipalCreator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		authCtx, err := extractAuth(ctx, principals, roles, tokens, sshVerifier, config, creator)
		if err != nil {
			return nil, err
		}

		ctx = WithAuth(ctx, authCtx)
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC stream interceptor that authenticates requests.
// The optional config and creator parameters enable agent auto-registration.
func StreamInterceptor(principals PrincipalStore, roles RoleStore, tokens TokenVerifier, sshVerifier *SSHVerifier, config *AuthConfig, creator PrincipalCreator) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		authCtx, err := extractAuth(ss.Context(), principals, roles, tokens, sshVerifier, config, creator)
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
// For SSH auth (agents):
//  1. Extract SSH headers (x-ssh-pubkey, x-ssh-signature, x-ssh-timestamp, x-ssh-nonce)
//  2. Verify signature over "timestamp|nonce"
//  3. Compute fingerprint and lookup principal by pubkey
//     3a. If principal not found and auto-registration is enabled, create a new principal
//
// For JWT auth (clients):
//  1. Get token from metadata: "authorization: Bearer <token>"
//  2. Verify token, extract principal_id
//  3. Lookup principal by ID
//
// Common steps:
//  4. Check status: allow approved/online/offline, deny pending/revoked (PERMISSION_DENIED)
//  5. Lookup roles
//  6. Build AuthContext
func extractAuth(ctx context.Context, principals PrincipalStore, roles RoleStore, tokens TokenVerifier, sshVerifier *SSHVerifier, config *AuthConfig, creator PrincipalCreator) (*AuthContext, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	var principal *store.Principal
	var principalID string
	var wasAutoRegistered bool // Track if we just created this principal

	// Try SSH auth first (for agents)
	if sshReq := ExtractSSHAuthFromMetadata(md); sshReq != nil {
		if sshVerifier == nil {
			return nil, status.Error(codes.Unauthenticated, "SSH authentication not configured")
		}

		// Validate all required fields are present
		if sshReq.Pubkey == "" {
			return nil, status.Error(codes.Unauthenticated, "missing SSH public key")
		}
		if sshReq.Signature == "" {
			return nil, status.Error(codes.Unauthenticated, "missing SSH signature")
		}
		if sshReq.Timestamp == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing SSH timestamp")
		}
		if sshReq.Nonce == "" {
			return nil, status.Error(codes.Unauthenticated, "missing SSH nonce")
		}

		// Verify the SSH signature
		fingerprint, err := sshVerifier.Verify(sshReq)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "SSH auth failed: %v", err)
		}

		// Lookup principal by fingerprint
		p, err := principals.GetPrincipalByPubkey(ctx, fingerprint)
		if err != nil {
			if errors.Is(err, store.ErrPrincipalNotFound) {
				// Check if auto-registration is enabled
				if config == nil || config.AgentAutoRegistration == "disabled" || config.AgentAutoRegistration == "" {
					return nil, status.Error(codes.Unauthenticated, "unknown public key")
				}

				// Auto-registration is enabled - create a new principal
				if creator == nil {
					return nil, status.Error(codes.Internal, "auto-registration enabled but no principal creator configured")
				}

				// Determine status based on config
				principalStatus := store.PrincipalStatusPending
				if config.AgentAutoRegistration == "approved" {
					principalStatus = store.PrincipalStatusApproved
				}

				// Create the new principal with a descriptive display name
				// Use last 8 chars of fingerprint for identification
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

				p = newPrincipal
				wasAutoRegistered = true
			} else {
				return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
			}
		}
		principal = p
		principalID = p.ID
	} else {
		// Fall back to JWT auth (for clients)
		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		authHeader := authHeaders[0]
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Verify token, extract principal_id
		pid, err := tokens.Verify(tokenString)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		principalID = pid

		// Lookup principal by ID
		p, err := principals.GetPrincipal(ctx, principalID)
		if err != nil {
			if errors.Is(err, store.ErrPrincipalNotFound) {
				return nil, status.Error(codes.Unauthenticated, "principal not found")
			}
			return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
		}
		principal = p
	}

	// Check status - allow approved/online/offline, deny pending/revoked
	switch principal.Status {
	case store.PrincipalStatusApproved, store.PrincipalStatusOnline, store.PrincipalStatusOffline:
		// allowed
	case store.PrincipalStatusPending:
		if wasAutoRegistered {
			return nil, status.Errorf(codes.PermissionDenied, "agent registered with pending status (principal_id: %s) - admin approval required", principal.ID)
		}
		return nil, status.Error(codes.PermissionDenied, "principal status is pending - admin approval required")
	case store.PrincipalStatusRevoked:
		return nil, status.Error(codes.PermissionDenied, "principal has been revoked")
	default:
		return nil, status.Errorf(codes.Internal, "unknown principal status: %s", principal.Status)
	}

	// Lookup roles
	roleNames, err := roles.ListRoles(ctx, store.RoleSubjectPrincipal, principalID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to lookup roles: %v", err)
	}

	// Convert role names to strings
	roleStrings := make([]string, len(roleNames))
	for i, r := range roleNames {
		roleStrings[i] = string(r)
	}

	// Build AuthContext
	authCtx := &AuthContext{
		PrincipalID:   principalID,
		PrincipalType: string(principal.Type),
		MemberID:      nil, // always nil in v1
		Roles:         roleStrings,
	}

	return authCtx, nil
}
