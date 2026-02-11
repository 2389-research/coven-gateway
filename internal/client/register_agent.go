// ABOUTME: RegisterAgent RPC handler for self-registration of agents
// ABOUTME: Allows linked devices (members) to register their agent's SSH key

package client

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// PrincipalWriter defines the store operations needed for creating principals.
type PrincipalWriter interface {
	CreatePrincipal(ctx context.Context, p *store.Principal) error
	AddRole(ctx context.Context, subjectType store.RoleSubjectType, subjectID string, role store.RoleName) error
}

const (
	// Maximum display name length.
	maxDisplayNameLength = 100

	// Rate limiting: max registrations per principal per window.
	maxRegistrationsPerWindow = 5
	registrationRateWindow    = time.Minute
)

// displayNamePattern allows alphanumeric, hyphens, underscores, spaces, and basic punctuation.
var displayNamePattern = regexp.MustCompile(`^[a-zA-Z0-9\-_\.\s]+$`)

// registrationRateLimiter tracks recent registration attempts per principal.
// Includes periodic cleanup to prevent unbounded memory growth.
type registrationRateLimiter struct {
	mu          sync.Mutex
	records     map[string][]time.Time
	lastCleanup time.Time
}

// cleanupInterval determines how often we scan for stale entries.
const cleanupInterval = 5 * time.Minute

var regRateLimiter = &registrationRateLimiter{
	records:     make(map[string][]time.Time),
	lastCleanup: time.Now(),
}

// checkAndRecord checks if a principal can register and records the attempt.
// Returns true if allowed, false if rate limited.
func (r *registrationRateLimiter) checkAndRecord(principalID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-registrationRateWindow)

	// Periodic cleanup: remove entries with all expired timestamps
	if now.Sub(r.lastCleanup) > cleanupInterval {
		r.cleanup(cutoff)
		r.lastCleanup = now
	}

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

// cleanup removes entries where all timestamps have expired.
// Must be called with mu held.
func (r *registrationRateLimiter) cleanup(cutoff time.Time) {
	for id, timestamps := range r.records {
		hasValid := false
		for _, t := range timestamps {
			if t.After(cutoff) {
				hasValid = true
				break
			}
		}
		if !hasValid {
			delete(r.records, id)
		}
	}
}

// hasSufficientRole checks if the roles include member, admin, or owner.
func hasSufficientRole(roles []string) bool {
	return slices.Contains(roles, string(store.RoleMember)) ||
		slices.Contains(roles, string(store.RoleAdmin)) ||
		slices.Contains(roles, string(store.RoleOwner))
}

// validateDisplayName checks display name requirements.
func validateDisplayName(name string) error {
	if name == "" {
		return status.Error(codes.InvalidArgument, "display_name required")
	}
	// Reject whitespace-only names
	if strings.TrimSpace(name) == "" {
		return status.Error(codes.InvalidArgument, "display_name cannot be only whitespace")
	}
	if len(name) > maxDisplayNameLength {
		return status.Errorf(codes.InvalidArgument, "display_name exceeds %d characters", maxDisplayNameLength)
	}
	if !displayNamePattern.MatchString(name) {
		return status.Error(codes.InvalidArgument, "display_name contains invalid characters (allowed: alphanumeric, hyphens, underscores, periods, spaces)")
	}
	return nil
}

// validateFingerprint checks fingerprint format requirements.
func validateFingerprint(fp string) error {
	if fp == "" {
		return status.Error(codes.InvalidArgument, "fingerprint required")
	}
	if len(fp) != 64 {
		return status.Error(codes.InvalidArgument, "fingerprint must be 64 hex characters (SHA256)")
	}
	if _, err := hex.DecodeString(fp); err != nil {
		return status.Error(codes.InvalidArgument, "fingerprint must be valid hex characters")
	}
	return nil
}

// handleCreatePrincipalError converts creation errors to gRPC status.
func handleCreatePrincipalError(err error) error {
	if errors.Is(err, store.ErrDuplicatePubkey) {
		return status.Error(codes.AlreadyExists, "fingerprint already registered")
	}
	return status.Errorf(codes.Internal, "failed to create principal: %v", err)
}

// RegisterAgent allows authenticated members to register an agent with their SSH key.
func (s *ClientService) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	if !hasSufficientRole(authCtx.Roles) {
		return nil, status.Error(codes.PermissionDenied, "member role required to register agents")
	}

	if !regRateLimiter.checkAndRecord(authCtx.PrincipalID) {
		slog.Warn("registration rate limit exceeded",
			"principal_id", authCtx.PrincipalID,
			"limit", maxRegistrationsPerWindow,
			"window", registrationRateWindow)
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded: max %d registrations per %v", maxRegistrationsPerWindow, registrationRateWindow)
	}

	if err := validateDisplayName(req.DisplayName); err != nil {
		return nil, err
	}
	if err := validateFingerprint(req.Fingerprint); err != nil {
		return nil, err
	}

	writer, ok := s.principals.(PrincipalWriter)
	if !ok {
		return nil, status.Error(codes.Internal, "principal creation not available")
	}

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
		return nil, handleCreatePrincipalError(err)
	}

	if err := writer.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		slog.Error("failed to add member role to registered agent - registration incomplete",
			"agent_principal_id", principalID,
			"registered_by", authCtx.PrincipalID,
			"error", err)
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
