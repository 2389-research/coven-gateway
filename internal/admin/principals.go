// ABOUTME: AdminService gRPC handlers for principal management
// ABOUTME: Implements create, list, delete operations for principals

package admin

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// PrincipalStore defines the interface for principal operations.
type PrincipalStore interface {
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
	CreatePrincipal(ctx context.Context, p *store.Principal) error
	DeletePrincipal(ctx context.Context, id string) error
	ListPrincipals(ctx context.Context, filter store.PrincipalFilter) ([]store.Principal, error)
	AddRole(ctx context.Context, subjectType store.RoleSubjectType, subjectID string, role store.RoleName) error
	ListRoles(ctx context.Context, subjectType store.RoleSubjectType, subjectID string) ([]store.RoleName, error)
	AppendAuditLog(ctx context.Context, entry *store.AuditEntry) error
}

// PrincipalService extends TokenService with principal management capabilities.
type PrincipalService struct {
	*TokenService
	principalStore PrincipalStore
}

// NewPrincipalService creates an AdminService with full principal management capabilities.
func NewPrincipalService(s interface {
	BindingStore
	PrincipalStore
}, tokenGen TokenGenerator) *PrincipalService {
	return &PrincipalService{
		TokenService:   NewTokenService(s, tokenGen, s),
		principalStore: s,
	}
}

// ListPrincipals returns a list of principals matching the filter.
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

// validateCreatePrincipalRequest validates basic request fields.
func validateCreatePrincipalRequest(req *pb.CreatePrincipalRequest) error {
	if req.Type == "" {
		return status.Error(codes.InvalidArgument, "type required")
	}
	if req.DisplayName == "" {
		return status.Error(codes.InvalidArgument, "display_name required")
	}
	return nil
}

// parsePrincipalType validates and converts the type string.
func parsePrincipalType(typeStr string) (store.PrincipalType, error) {
	pType := store.PrincipalType(typeStr)
	switch pType {
	case store.PrincipalTypeClient, store.PrincipalTypeAgent:
		return pType, nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "invalid type: %s (use 'client' or 'agent')", typeStr)
	}
}

// resolveAgentFingerprint extracts fingerprint from pubkey or uses provided fingerprint.
func resolveAgentFingerprint(req *pb.CreatePrincipalRequest) (string, error) {
	switch {
	case req.Pubkey != nil && *req.Pubkey != "":
		fp, err := auth.ParseFingerprintFromKey(*req.Pubkey)
		if err != nil {
			return "", status.Errorf(codes.InvalidArgument, "invalid pubkey: %v", err)
		}
		return fp, nil
	case req.PubkeyFp != nil && *req.PubkeyFp != "":
		return *req.PubkeyFp, nil
	default:
		return "", status.Error(codes.InvalidArgument, "agent principals require pubkey or pubkey_fp")
	}
}

// assignRoles attempts to add roles to principal, returning successfully added roles.
// Invalid or duplicate roles are logged and skipped (best-effort assignment).
func (s *PrincipalService) assignRoles(ctx context.Context, principalID string, roles []string) []string {
	var added []string
	for _, roleStr := range roles {
		role := store.RoleName(roleStr)
		if err := s.principalStore.AddRole(ctx, store.RoleSubjectPrincipal, principalID, role); err != nil {
			slog.Warn("failed to assign role to principal",
				"principal_id", principalID,
				"role", roleStr,
				"error", err,
			)
			continue // Best effort - skip on error
		}
		added = append(added, roleStr)
	}
	return added
}

// CreatePrincipal creates a new principal.
func (s *PrincipalService) CreatePrincipal(ctx context.Context, req *pb.CreatePrincipalRequest) (*pb.Principal, error) {
	authCtx := auth.MustFromContext(ctx)

	if err := validateCreatePrincipalRequest(req); err != nil {
		return nil, err
	}

	pType, err := parsePrincipalType(req.Type)
	if err != nil {
		return nil, err
	}

	var fingerprint string
	if pType == store.PrincipalTypeAgent {
		fingerprint, err = resolveAgentFingerprint(req)
		if err != nil {
			return nil, err
		}
	}

	principalID := generatePrincipalID(pType)
	p := &store.Principal{
		ID:          principalID,
		Type:        pType,
		PubkeyFP:    fingerprint,
		DisplayName: req.DisplayName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.principalStore.CreatePrincipal(ctx, p); err != nil {
		if errors.Is(err, store.ErrDuplicatePubkey) {
			return nil, status.Error(codes.AlreadyExists, "pubkey already registered to another principal")
		}
		return nil, status.Errorf(codes.Internal, "failed to create principal: %v", err)
	}

	roleStrings := s.assignRoles(ctx, principalID, req.Roles)

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

// DeletePrincipal deletes a principal.
func (s *PrincipalService) DeletePrincipal(ctx context.Context, req *pb.DeletePrincipalRequest) (*pb.DeletePrincipalResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}

	// Check principal exists
	_, err := s.principalStore.GetPrincipal(ctx, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			return nil, status.Error(codes.NotFound, "principal not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
	}

	// Delete
	if err := s.principalStore.DeletePrincipal(ctx, req.Id); err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
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

// generatePrincipalID creates a new unique principal ID.
// Format: {type}-{timestamp_base36}-{random_hex}
// Uses a delimiter between timestamp and random suffix for unambiguous parsing.
func generatePrincipalID(pType store.PrincipalType) string {
	timestamp := time.Now().UnixMilli()
	random := generateRandomSuffix()
	// Use 4-char hex for the random suffix (fixed width, easy to parse)
	return string(pType) + "-" + formatBase36(timestamp) + "-" + formatHex4(random)
}

// generateRandomSuffix returns a cryptographically random 16-bit value.
// Falls back to timestamp-based entropy if crypto/rand fails.
func generateRandomSuffix() uint16 {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback: use nanosecond timestamp truncated to 16 bits
		nanos := time.Now().UnixNano()
		return uint16(nanos & 0xFFFF) //nolint:gosec // intentional truncation for fallback
	}
	return binary.BigEndian.Uint16(b[:])
}

// formatBase36 converts an int64 to base36 string.
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

// formatHex4 formats a uint16 as a 4-character lowercase hex string.
func formatHex4(n uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{
		digits[(n>>12)&0xf],
		digits[(n>>8)&0xf],
		digits[(n>>4)&0xf],
		digits[n&0xf],
	})
}
