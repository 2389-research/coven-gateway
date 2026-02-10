// ABOUTME: AdminService gRPC handlers for token management
// ABOUTME: Implements token generation for client principals

package admin

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// Default TTL for tokens: 30 days.
const defaultTokenTTL = 30 * 24 * time.Hour

// Maximum TTL for tokens: 365 days.
const maxTokenTTL = 365 * 24 * time.Hour

// TokenGenerator generates JWT tokens.
type TokenGenerator interface {
	Generate(principalID string, ttl time.Duration) (string, error)
}

// PrincipalLookup looks up principals by ID.
type PrincipalLookup interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
}

// TokenService extends AdminService with token management capabilities.
type TokenService struct {
	*AdminService
	tokenGen   TokenGenerator
	principals PrincipalLookup
}

// NewTokenService creates an AdminService with token management capabilities.
func NewTokenService(s BindingStore, tokenGen TokenGenerator, principals PrincipalLookup) *TokenService {
	return &TokenService{
		AdminService: NewAdminService(s),
		tokenGen:     tokenGen,
		principals:   principals,
	}
}

// CreateToken generates a JWT token for a principal.
func (s *TokenService) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.CreateTokenResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	// Validate request
	if req.PrincipalId == "" {
		return nil, status.Error(codes.InvalidArgument, "principal_id required")
	}

	// Check token generator is configured
	if s.tokenGen == nil {
		return nil, status.Error(codes.FailedPrecondition, "token generation not configured (no jwt_secret)")
	}

	// Verify target principal exists
	principal, err := s.principals.GetPrincipal(ctx, req.PrincipalId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "principal not found")
	}

	// Only allow token generation for approved principals
	if principal.Status != store.PrincipalStatusApproved {
		return nil, status.Errorf(codes.FailedPrecondition, "principal status is %s, must be approved", principal.Status)
	}

	// Determine TTL
	ttl := defaultTokenTTL
	if req.TtlSeconds > 0 {
		ttl = time.Duration(req.TtlSeconds) * time.Second
		if ttl > maxTokenTTL {
			return nil, status.Errorf(codes.InvalidArgument, "ttl_seconds exceeds maximum of %d", int64(maxTokenTTL.Seconds()))
		}
	}

	// Generate token
	token, err := s.tokenGen.Generate(req.PrincipalId, ttl)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	expiresAt := time.Now().Add(ttl).UTC()

	// Audit log (ignore error - best effort)
	_ = s.store.AppendAuditLog(ctx, &store.AuditEntry{
		ActorPrincipalID: authCtx.PrincipalID,
		Action:           store.AuditCreateToken,
		TargetType:       "principal",
		TargetID:         req.PrincipalId,
		Detail: map[string]any{
			"ttl_seconds": int64(ttl.Seconds()),
			"expires_at":  expiresAt.Format(time.RFC3339),
		},
	})

	return &pb.CreateTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}
