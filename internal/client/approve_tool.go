// ABOUTME: ClientService gRPC handler for tool approval
// ABOUTME: Implements ApproveTool RPC for responding to tool approval requests

package client

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// ToolApprover defines the interface for sending tool approval responses.
type ToolApprover interface {
	SendToolApproval(agentID, toolID string, approved, approveAll bool) error
}

// SetToolApprover sets the tool approver for tool approval operations.
func (s *ClientService) SetToolApprover(approver ToolApprover) {
	s.approver = approver
}

// ApproveTool responds to a tool approval request from an agent.
func (s *ClientService) ApproveTool(ctx context.Context, req *pb.ApproveToolRequest) (*pb.ApproveToolResponse, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id required")
	}
	if req.ToolId == "" {
		return nil, status.Error(codes.InvalidArgument, "tool_id required")
	}

	if s.approver == nil {
		return &pb.ApproveToolResponse{
			Success: false,
			Error:   strPtr("tool approver not configured"),
		}, nil
	}

	err := s.approver.SendToolApproval(req.AgentId, req.ToolId, req.Approved, req.ApproveAll)
	if err != nil {
		return &pb.ApproveToolResponse{
			Success: false,
			Error:   strPtr(err.Error()),
		}, nil
	}

	return &pb.ApproveToolResponse{
		Success: true,
	}, nil
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
