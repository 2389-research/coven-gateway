// ABOUTME: AdminService gRPC handlers for principal management
// ABOUTME: Implements create, list, delete operations for principals

package admin

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/fold-gateway/internal/auth"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// PrincipalStore defines the interface for principal operations
type PrincipalStore interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
	CreatePrincipal(ctx context.Context, p *store.Principal) error
	DeletePrincipal(ctx context.Context, id string) error
	ListPrincipals(ctx context.Context, filter store.PrincipalFilter) ([]store.Principal, error)
	AddRole(ctx context.Context, subjectType store.RoleSubjectType, subjectID string, role store.RoleName) error
	ListRoles(ctx context.Context, subjectType store.RoleSubjectType, subjectID string) ([]store.RoleName, error)
	AppendAuditLog(ctx context.Context, entry *store.AuditEntry) error
}

// PrincipalService extends TokenService with principal management capabilities
type PrincipalService struct {
	*TokenService
	principalStore PrincipalStore
}

// NewPrincipalService creates an AdminService with full principal management capabilities
func NewPrincipalService(s interface {
	BindingStore
	PrincipalStore
}, tokenGen TokenGenerator) *PrincipalService {
	return &PrincipalService{
		TokenService:   NewTokenService(s, tokenGen, s),
		principalStore: s,
	}
}

// ListPrincipals returns a list of principals matching the filter
func (s *PrincipalService) ListPrincipals(ctx context.Context, req *pb.ListPrincipalsRequest) (*pb.ListPrincipalsResponse, error) {
	filter := store.PrincipalFilter{}

	// Apply type filter if provided
	if req.Type != nil {
		pType := store.PrincipalType(*req.Type)
		filter.Type = &pType
	}

	// Apply status filter if provided
	if req.Status != nil {
		pStatus := store.PrincipalStatus(*req.Status)
		filter.Status = &pStatus
	}

	principals, err := s.principalStore.ListPrincipals(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list principals: %v", err)
	}

	// Convert to proto
	result := make([]*pb.Principal, len(principals))
	for i := range principals {
		p := &principals[i]
		// Get roles for this principal
		roles, _ := s.principalStore.ListRoles(ctx, store.RoleSubjectPrincipal, p.ID)
		roleStrings := make([]string, len(roles))
		for j, r := range roles {
			roleStrings[j] = string(r)
		}

		proto := &pb.Principal{
			Id:          p.ID,
			Type:        string(p.Type),
			DisplayName: p.DisplayName,
			Status:      string(p.Status),
			CreatedAt:   p.CreatedAt.Format(time.RFC3339),
			Roles:       roleStrings,
		}
		if p.PubkeyFP != "" {
			proto.PubkeyFp = &p.PubkeyFP
		}
		result[i] = proto
	}

	return &pb.ListPrincipalsResponse{
		Principals: result,
	}, nil
}

// CreatePrincipal creates a new principal
func (s *PrincipalService) CreatePrincipal(ctx context.Context, req *pb.CreatePrincipalRequest) (*pb.Principal, error) {
	authCtx := auth.MustFromContext(ctx)

	// Validate request
	if req.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "type required")
	}
	if req.DisplayName == "" {
		return nil, status.Error(codes.InvalidArgument, "display_name required")
	}

	// Validate type
	pType := store.PrincipalType(req.Type)
	switch pType {
	case store.PrincipalTypeClient, store.PrincipalTypeAgent:
		// valid
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid type: %s (use 'client' or 'agent')", req.Type)
	}

	// Agents must have a pubkey
	var fingerprint string
	if pType == store.PrincipalTypeAgent {
		if req.Pubkey != nil && *req.Pubkey != "" {
			// Parse the pubkey and compute fingerprint
			fp, err := auth.ParseFingerprintFromKey(*req.Pubkey)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid pubkey: %v", err)
			}
			fingerprint = fp
		} else if req.PubkeyFp != nil && *req.PubkeyFp != "" {
			// Use provided fingerprint directly
			fingerprint = *req.PubkeyFp
		} else {
			return nil, status.Error(codes.InvalidArgument, "agent principals require pubkey or pubkey_fp")
		}
	}

	// Generate principal ID
	principalID := generatePrincipalID(pType)

	// Create principal
	p := &store.Principal{
		ID:          principalID,
		Type:        pType,
		PubkeyFP:    fingerprint,
		DisplayName: req.DisplayName,
		Status:      store.PrincipalStatusApproved, // Auto-approve admin-created principals
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.principalStore.CreatePrincipal(ctx, p); err != nil {
		if err == store.ErrDuplicatePubkey {
			return nil, status.Error(codes.AlreadyExists, "pubkey already registered to another principal")
		}
		return nil, status.Errorf(codes.Internal, "failed to create principal: %v", err)
	}

	// Add requested roles
	roleStrings := []string{}
	for _, roleStr := range req.Roles {
		role := store.RoleName(roleStr)
		if err := s.principalStore.AddRole(ctx, store.RoleSubjectPrincipal, principalID, role); err != nil {
			// Best effort - log but don't fail
			continue
		}
		roleStrings = append(roleStrings, roleStr)
	}

	// Audit log
	_ = s.principalStore.AppendAuditLog(ctx, &store.AuditEntry{
		ActorPrincipalID: authCtx.PrincipalID,
		Action:           store.AuditCreatePrincipal,
		TargetType:       "principal",
		TargetID:         principalID,
		Detail: map[string]any{
			"type":         req.Type,
			"display_name": req.DisplayName,
			"roles":        req.Roles,
		},
	})

	// Build response
	proto := &pb.Principal{
		Id:          principalID,
		Type:        string(pType),
		DisplayName: p.DisplayName,
		Status:      string(p.Status),
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
		Roles:       roleStrings,
	}
	if fingerprint != "" {
		proto.PubkeyFp = &fingerprint
	}

	return proto, nil
}

// DeletePrincipal deletes a principal
func (s *PrincipalService) DeletePrincipal(ctx context.Context, req *pb.DeletePrincipalRequest) (*pb.DeletePrincipalResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}

	// Check principal exists
	_, err := s.principalStore.GetPrincipal(ctx, req.Id)
	if err != nil {
		if err == store.ErrPrincipalNotFound {
			return nil, status.Error(codes.NotFound, "principal not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
	}

	// Delete
	if err := s.principalStore.DeletePrincipal(ctx, req.Id); err != nil {
		if err == store.ErrPrincipalNotFound {
			return nil, status.Error(codes.NotFound, "principal not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete principal: %v", err)
	}

	// Audit log
	_ = s.principalStore.AppendAuditLog(ctx, &store.AuditEntry{
		ActorPrincipalID: authCtx.PrincipalID,
		Action:           store.AuditDeletePrincipal,
		TargetType:       "principal",
		TargetID:         req.Id,
	})

	return &pb.DeletePrincipalResponse{}, nil
}

// generatePrincipalID creates a new unique principal ID
func generatePrincipalID(pType store.PrincipalType) string {
	// Use type prefix + timestamp + random suffix
	timestamp := time.Now().UnixMilli()
	return string(pType) + "-" + formatBase36(timestamp)
}

// formatBase36 converts an int64 to base36 string
func formatBase36(n int64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(digits[n%36]) + result
		n /= 36
	}
	return result
}
