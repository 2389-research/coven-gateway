// ABOUTME: RegisterClient RPC handler for self-registration of clients (TUI, CLI)
// ABOUTME: Allows linked devices (members) to register their client's SSH key

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

// RegisterClient allows authenticated members to register a client with their SSH key
func (s *ClientService) RegisterClient(ctx context.Context, req *pb.RegisterClientRequest) (*pb.RegisterClientResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	// Require member role for registration
	if !slices.Contains(authCtx.Roles, string(store.RoleMember)) &&
		!slices.Contains(authCtx.Roles, string(store.RoleAdmin)) &&
		!slices.Contains(authCtx.Roles, string(store.RoleOwner)) {
		return nil, status.Error(codes.PermissionDenied, "member role required to register clients")
	}

	// Rate limiting: prevent registration spam (reuse agent rate limiter)
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

	// Create the client principal
	principalID := uuid.New().String()
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
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
		slog.Error("failed to add member role to registered client - registration incomplete",
			"client_principal_id", principalID,
			"registered_by", authCtx.PrincipalID,
			"error", err)
		return nil, status.Errorf(codes.Internal, "client created but role assignment failed: %v", err)
	}

	slog.Info("client registered via self-registration",
		"client_principal_id", principalID,
		"registered_by", authCtx.PrincipalID,
		"display_name", req.DisplayName,
	)

	return &pb.RegisterClientResponse{
		PrincipalId: principalID,
		Status:      "approved",
	}, nil
}
