// ABOUTME: GetMe RPC handler for retrieving authenticated principal's identity
// ABOUTME: Allows clients to query their own identity information including roles

package client

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/2389/fold-gateway/internal/auth"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// GetMe returns the authenticated principal's identity information
func (s *ClientService) GetMe(ctx context.Context, _ *emptypb.Empty) (*pb.MeResponse, error) {
	authCtx := auth.MustFromContext(ctx)

	principal, err := s.principals.GetPrincipal(ctx, authCtx.PrincipalID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch principal")
	}

	return &pb.MeResponse{
		PrincipalId:   principal.ID,
		PrincipalType: string(principal.Type),
		DisplayName:   principal.DisplayName,
		Status:        string(principal.Status),
		Roles:         authCtx.Roles,
		// MemberId and MemberDisplayName are nil in v1
	}, nil
}
