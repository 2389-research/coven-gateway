// ABOUTME: AdminService gRPC handlers for binding management
// ABOUTME: Implements CRUD operations for channel-to-agent bindings with validation and audit logging

package admin

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// frontendPattern validates frontend names: lowercase alphanumeric and underscore, 1-50 chars.
var frontendPattern = regexp.MustCompile(`^[a-z0-9_]{1,50}$`)

// maxChannelIDLength is the maximum allowed length for channel IDs.
const maxChannelIDLength = 500

// BindingStore defines the store operations needed for binding management.
type BindingStore interface {
	CreateBindingV2(ctx context.Context, b *store.Binding) error
	GetBindingByID(ctx context.Context, id string) (*store.Binding, error)
	UpdateBinding(ctx context.Context, id, agentID string) error
	DeleteBindingByID(ctx context.Context, id string) error
	ListBindingsV2(ctx context.Context, f store.BindingFilter) ([]store.Binding, error)
	AppendAuditLog(ctx context.Context, e *store.AuditEntry) error
}

// AdminService implements the AdminService gRPC service.
type AdminService struct {
	pb.UnimplementedAdminServiceServer
	store BindingStore
}

// NewAdminService creates a new AdminService with the given store.
func NewAdminService(s BindingStore) *AdminService {
	return &AdminService{store: s}
}

// CreateBinding creates a new channel-to-agent binding.
func (s *AdminService) CreateBinding(ctx context.Context, req *pb.CreateBindingRequest) (*pb.Binding, error) {
	authCtx := auth.MustFromContext(ctx)

	// Validate request
	if err := validateFrontend(req.Frontend); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.ChannelId == "" {
		return nil, status.Error(codes.InvalidArgument, "channel_id required")
	}
	if len(req.ChannelId) > maxChannelIDLength {
		return nil, status.Errorf(codes.InvalidArgument, "channel_id exceeds %d characters", maxChannelIDLength)
	}
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id required")
	}

	// Create binding
	b := &store.Binding{
		ID:        uuid.New().String(),
		Frontend:  req.Frontend,
		ChannelID: req.ChannelId,
		AgentID:   req.AgentId,
		CreatedAt: time.Now().UTC(),
		CreatedBy: &authCtx.PrincipalID,
	}

	if err := s.store.CreateBindingV2(ctx, b); err != nil {
		if errors.Is(err, store.ErrDuplicateChannel) {
			return nil, status.Error(codes.AlreadyExists, "channel already bound")
		}
		if errors.Is(err, store.ErrAgentNotFound) {
			return nil, status.Error(codes.NotFound, "agent not found")
		}
		return nil, status.Error(codes.Internal, "failed to create binding")
	}

	// Audit log
	_ = s.store.AppendAuditLog(ctx, &store.AuditEntry{
		ActorPrincipalID: authCtx.PrincipalID,
		Action:           store.AuditCreateBinding,
		TargetType:       "binding",
		TargetID:         b.ID,
		Detail: map[string]any{
			"frontend":   b.Frontend,
			"channel_id": b.ChannelID,
			"agent_id":   b.AgentID,
		},
	})

	return toProtoBinding(b), nil
}

// UpdateBinding updates a binding's agent_id.
func (s *AdminService) UpdateBinding(ctx context.Context, req *pb.UpdateBindingRequest) (*pb.Binding, error) {
	authCtx := auth.MustFromContext(ctx)

	// Validate request
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id required")
	}

	// Update binding
	if err := s.store.UpdateBinding(ctx, req.Id, req.AgentId); err != nil {
		if errors.Is(err, store.ErrBindingNotFound) {
			return nil, status.Error(codes.NotFound, "binding not found")
		}
		if errors.Is(err, store.ErrAgentNotFound) {
			return nil, status.Error(codes.NotFound, "agent not found")
		}
		return nil, status.Error(codes.Internal, "failed to update binding")
	}

	// Get updated binding
	b, err := s.store.GetBindingByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to retrieve updated binding")
	}

	// Audit log
	_ = s.store.AppendAuditLog(ctx, &store.AuditEntry{
		ActorPrincipalID: authCtx.PrincipalID,
		Action:           store.AuditUpdateBinding,
		TargetType:       "binding",
		TargetID:         b.ID,
		Detail: map[string]any{
			"agent_id": req.AgentId,
		},
	})

	return toProtoBinding(b), nil
}

// DeleteBinding removes a binding by ID.
func (s *AdminService) DeleteBinding(ctx context.Context, req *pb.DeleteBindingRequest) (*pb.DeleteBindingResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	// Validate request
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}

	// Get binding before deletion for audit log
	b, err := s.store.GetBindingByID(ctx, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrBindingNotFound) {
			return nil, status.Error(codes.NotFound, "binding not found")
		}
		return nil, status.Error(codes.Internal, "failed to get binding")
	}

	// Delete binding
	if err := s.store.DeleteBindingByID(ctx, req.Id); err != nil {
		if errors.Is(err, store.ErrBindingNotFound) {
			return nil, status.Error(codes.NotFound, "binding not found")
		}
		return nil, status.Error(codes.Internal, "failed to delete binding")
	}

	// Audit log
	_ = s.store.AppendAuditLog(ctx, &store.AuditEntry{
		ActorPrincipalID: authCtx.PrincipalID,
		Action:           store.AuditDeleteBinding,
		TargetType:       "binding",
		TargetID:         req.Id,
		Detail: map[string]any{
			"frontend":   b.Frontend,
			"channel_id": b.ChannelID,
			"agent_id":   b.AgentID,
		},
	})

	return &pb.DeleteBindingResponse{}, nil
}

// ListBindings returns bindings matching the filter criteria.
func (s *AdminService) ListBindings(ctx context.Context, req *pb.ListBindingsRequest) (*pb.ListBindingsResponse, error) {
	// auth.MustFromContext is called by the interceptor, but we call it here
	// to ensure the context has auth even if this method is called directly in tests
	_ = auth.MustFromContext(ctx)

	filter := store.BindingFilter{
		Frontend: req.Frontend,
		AgentID:  req.AgentId,
	}

	bindings, err := s.store.ListBindingsV2(ctx, filter)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list bindings")
	}

	pbBindings := make([]*pb.Binding, len(bindings))
	for i := range bindings {
		pbBindings[i] = toProtoBinding(&bindings[i])
	}

	return &pb.ListBindingsResponse{Bindings: pbBindings}, nil
}

// validateFrontend validates the frontend name format.
func validateFrontend(frontend string) error {
	if frontend == "" {
		return errors.New("frontend required")
	}
	if !frontendPattern.MatchString(frontend) {
		return errors.New("frontend must be lowercase alphanumeric with underscores, 1-50 chars")
	}
	return nil
}

// toProtoBinding converts a store.Binding to a protobuf Binding.
func toProtoBinding(b *store.Binding) *pb.Binding {
	return &pb.Binding{
		Id:        b.ID,
		Frontend:  b.Frontend,
		ChannelId: b.ChannelID,
		AgentId:   b.AgentID,
		CreatedAt: b.CreatedAt.Format(time.RFC3339),
		CreatedBy: b.CreatedBy,
	}
}
