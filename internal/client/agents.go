// ABOUTME: ClientService gRPC handler for listing connected agents
// ABOUTME: Implements ListAgents RPC for fetching available agents

package client

import (
	"context"

	"github.com/2389/fold-gateway/internal/agent"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// AgentLister defines the interface for listing connected agents
type AgentLister interface {
	ListAgents() []*agent.AgentInfo
}

// ListAgents returns all currently connected agents
func (s *ClientService) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	if s.agents == nil {
		return &pb.ListAgentsResponse{Agents: []*pb.AgentInfo{}}, nil
	}

	agentInfos := s.agents.ListAgents()

	// Filter by workspace if specified
	var filtered []*agent.AgentInfo
	if req.Workspace != nil && *req.Workspace != "" {
		workspace := *req.Workspace
		for _, a := range agentInfos {
			for _, ws := range a.Workspaces {
				if ws == workspace {
					filtered = append(filtered, a)
					break
				}
			}
		}
	} else {
		filtered = agentInfos
	}

	// Convert to proto
	agents := make([]*pb.AgentInfo, len(filtered))
	for i, a := range filtered {
		agents[i] = &pb.AgentInfo{
			Id:         a.ID,
			Name:       a.Name,
			Backend:    a.Backend,
			WorkingDir: a.WorkingDir,
			Connected:  true, // All agents from ListAgents are connected
		}

		// Include metadata if available
		if len(a.Workspaces) > 0 || a.InstanceID != "" {
			agents[i].Metadata = &pb.AgentMetadata{
				WorkingDirectory: a.WorkingDir,
				Workspaces:       a.Workspaces,
				Backend:          a.Backend,
			}
		}
	}

	return &pb.ListAgentsResponse{Agents: agents}, nil
}
