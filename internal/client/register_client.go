// ABOUTME: RegisterClient RPC handler for self-registration of clients (TUI, CLI)
// ABOUTME: Allows linked devices (members) to register their client's SSH key

package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// RegisterClient allows authenticated members to register a client with their SSH key.
func (s *ClientService) RegisterClient(ctx context.Context, req *pb.RegisterClientRequest) (*pb.RegisterClientResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	if !hasSufficientRole(authCtx.Roles) {
		return nil, status.Error(codes.PermissionDenied, "member role required to register clients")
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
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    req.Fingerprint,
		DisplayName: req.DisplayName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}

	if err := writer.CreatePrincipal(ctx, principal); err != nil {
		return nil, handleCreatePrincipalError(err)
	}

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
