// ABOUTME: RegisterAgent RPC handler for self-registration of agents
// ABOUTME: Allows linked devices (members) to register their agent's SSH key

package client

import (
	"context"
	"encoding/hex"
	"log/slog"
	"slices"
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

// Maximum display name length
const maxDisplayNameLength = 100

// RegisterAgent allows authenticated members to register an agent with their SSH key
func (s *ClientService) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	// Require member role for registration
	if !slices.Contains(authCtx.Roles, string(store.RoleMember)) &&
		!slices.Contains(authCtx.Roles, string(store.RoleAdmin)) &&
		!slices.Contains(authCtx.Roles, string(store.RoleOwner)) {
		return nil, status.Error(codes.PermissionDenied, "member role required to register agents")
	}

	// Validate request
	if req.DisplayName == "" {
		return nil, status.Error(codes.InvalidArgument, "display_name required")
	}
	if len(req.DisplayName) > maxDisplayNameLength {
		return nil, status.Errorf(codes.InvalidArgument, "display_name exceeds %d characters", maxDisplayNameLength)
	}
	if req.Fingerprint == "" {
		return nil, status.Error(codes.InvalidArgument, "fingerprint required")
	}
	if len(req.Fingerprint) != 64 {
		return nil, status.Error(codes.InvalidArgument, "fingerprint must be 64 hex characters (SHA256)")
	}
	// Validate fingerprint is valid hex
	if _, err := hex.DecodeString(req.Fingerprint); err != nil {
		return nil, status.Error(codes.InvalidArgument, "fingerprint must be valid hex characters")
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
		slog.Error("failed to add member role to registered agent",
			"agent_principal_id", principalID,
			"registered_by", authCtx.PrincipalID,
			"error", err)
	}

	slog.Info("agent registered via self-registration",
		"agent_principal_id", principalID,
		"registered_by", authCtx.PrincipalID,
		"display_name", req.DisplayName,
		"fingerprint", req.Fingerprint[:16]+"...",
	)

	return &pb.RegisterAgentResponse{
		PrincipalId: principalID,
		Status:      "approved",
	}, nil
}
