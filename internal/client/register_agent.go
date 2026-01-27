// ABOUTME: RegisterAgent RPC handler for self-registration of agents
// ABOUTME: Allows linked devices (members) to register their agent's SSH key

package client

import (
	"context"
	"encoding/hex"
	"log/slog"
	"regexp"
	"slices"
	"sync"
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

const (
	// Maximum display name length
	maxDisplayNameLength = 100

	// Rate limiting: max registrations per principal per window
	maxRegistrationsPerWindow = 5
	registrationRateWindow    = time.Minute
)

// displayNamePattern allows alphanumeric, hyphens, underscores, spaces, and basic punctuation
var displayNamePattern = regexp.MustCompile(`^[a-zA-Z0-9\-_\.\s]+$`)

// registrationRateLimiter tracks recent registration attempts per principal
type registrationRateLimiter struct {
	mu      sync.Mutex
	records map[string][]time.Time
}

var regRateLimiter = &registrationRateLimiter{
	records: make(map[string][]time.Time),
}

// checkAndRecord checks if a principal can register and records the attempt.
// Returns true if allowed, false if rate limited.
func (r *registrationRateLimiter) checkAndRecord(principalID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-registrationRateWindow)

	// Get existing timestamps and filter out expired ones
	timestamps := r.records[principalID]
	valid := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Check rate limit
	if len(valid) >= maxRegistrationsPerWindow {
		r.records[principalID] = valid
		return false
	}

	// Record this attempt
	r.records[principalID] = append(valid, now)
	return true
}

// RegisterAgent allows authenticated members to register an agent with their SSH key
func (s *ClientService) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	// Require member role for registration
	if !slices.Contains(authCtx.Roles, string(store.RoleMember)) &&
		!slices.Contains(authCtx.Roles, string(store.RoleAdmin)) &&
		!slices.Contains(authCtx.Roles, string(store.RoleOwner)) {
		return nil, status.Error(codes.PermissionDenied, "member role required to register agents")
	}

	// Rate limiting: prevent registration spam
	if !regRateLimiter.checkAndRecord(authCtx.PrincipalID) {
		slog.Warn("registration rate limit exceeded",
			"principal_id", authCtx.PrincipalID,
			"limit", maxRegistrationsPerWindow,
			"window", registrationRateWindow)
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded: max %d registrations per %v", maxRegistrationsPerWindow, registrationRateWindow)
	}

	// Validate display name
	if req.DisplayName == "" {
		return nil, status.Error(codes.InvalidArgument, "display_name required")
	}
	if len(req.DisplayName) > maxDisplayNameLength {
		return nil, status.Errorf(codes.InvalidArgument, "display_name exceeds %d characters", maxDisplayNameLength)
	}
	if !displayNamePattern.MatchString(req.DisplayName) {
		return nil, status.Error(codes.InvalidArgument, "display_name contains invalid characters (allowed: alphanumeric, hyphens, underscores, periods, spaces)")
	}

	// Validate fingerprint
	if req.Fingerprint == "" {
		return nil, status.Error(codes.InvalidArgument, "fingerprint required")
	}
	if len(req.Fingerprint) != 64 {
		return nil, status.Error(codes.InvalidArgument, "fingerprint must be 64 hex characters (SHA256)")
	}
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

	// Add member role - if this fails, we still have a valid principal but it won't be able to do much
	// Log and return error to let caller know the registration was incomplete
	if err := writer.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		slog.Error("failed to add member role to registered agent - registration incomplete",
			"agent_principal_id", principalID,
			"registered_by", authCtx.PrincipalID,
			"error", err)
		// Note: principal exists but has no role - admin will need to add role manually
		return nil, status.Errorf(codes.Internal, "agent created but role assignment failed: %v", err)
	}

	slog.Info("agent registered via self-registration",
		"agent_principal_id", principalID,
		"registered_by", authCtx.PrincipalID,
		"display_name", req.DisplayName,
	)

	return &pb.RegisterAgentResponse{
		PrincipalId: principalID,
		Status:      "approved",
	}, nil
}
