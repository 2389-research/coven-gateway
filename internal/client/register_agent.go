// ABOUTME: RegisterAgent RPC handler for self-registration of agents
// ABOUTME: Allows linked devices (members) to register their agent's SSH key

package client

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// PrincipalWriter defines the store operations needed for creating principals
type PrincipalWriter interface {
	CreatePrincipal(ctx context.Context, p *store.Principal) error
	AddRole(ctx context.Context, subjectType store.RoleSubjectType, subjectID string, role store.RoleName) error
}

// RegisterAgent allows authenticated members to register an agent with their SSH key
func (s *ClientService) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	// Validate request
	if req.DisplayName == "" {
		return nil, status.Error(codes.InvalidArgument, "display_name required")
	}
	if req.Fingerprint == "" {
		return nil, status.Error(codes.InvalidArgument, "fingerprint required")
	}
	if len(req.Fingerprint) != 64 {
		return nil, status.Error(codes.InvalidArgument, "fingerprint must be 64 hex characters (SHA256)")
	}

	// Need principal writer capability
	writer, ok := s.principals.(PrincipalWriter)
	if !ok {
		return nil, status.Error(codes.Internal, "principal creation not available")
	}

	// Create the agent principal
	principalID := uuid.New().String()
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    req.Fingerprint,
		DisplayName: req.DisplayName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}

	if err := writer.CreatePrincipal(ctx, principal); err != nil {
		if err == store.ErrDuplicatePubkey {
			return nil, status.Error(codes.AlreadyExists, "fingerprint already registered")
		}
		return nil, status.Errorf(codes.Internal, "failed to create principal: %v", err)
	}

	// Add member role
	if err := writer.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		// Log but don't fail - principal is created
		// TODO: add logging
	}

	// Log the registration (using caller's principal)
	_ = authCtx.PrincipalID // registered by this principal

	return &pb.RegisterAgentResponse{
		PrincipalId: principalID,
		Status:      "approved",
	}, nil
}
