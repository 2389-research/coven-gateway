// ABOUTME: Tests for ClientService ListAgents RPC handler
// ABOUTME: Covers agent listing with workspace filtering and empty results

package client

import (
	"context"
	"testing"

	"github.com/2389/coven-gateway/internal/agent"
	pb "github.com/2389/coven-gateway/proto/coven"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAgentLister implements AgentLister for testing.
type mockAgentLister struct {
	agents []*agent.AgentInfo
}

func (m *mockAgentLister) ListAgents() []*agent.AgentInfo {
	return m.agents
}

func TestListAgents_Success(t *testing.T) {
	lister := &mockAgentLister{
		agents: []*agent.AgentInfo{
			{
				ID:         "agent-1",
				Name:       "Claude",
				Backend:    "mux",
				WorkingDir: "/home/user/project",
				Workspaces: []string{"dev", "test"},
				InstanceID: "abc123",
			},
			{
				ID:         "agent-2",
				Name:       "Assistant",
				Backend:    "cli",
				WorkingDir: "/tmp",
				Workspaces: []string{"prod"},
				InstanceID: "def456",
			},
		},
	}

	svc := &ClientService{agents: lister}

	resp, err := svc.ListAgents(context.Background(), &pb.ListAgentsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Agents, 2)

	// Verify first agent
	assert.Equal(t, "agent-1", resp.Agents[0].Id)
	assert.Equal(t, "Claude", resp.Agents[0].Name)
	assert.Equal(t, "mux", resp.Agents[0].Backend)
	assert.Equal(t, "/home/user/project", resp.Agents[0].WorkingDir)
	assert.True(t, resp.Agents[0].Connected)
	require.NotNil(t, resp.Agents[0].Metadata)
	assert.Equal(t, []string{"dev", "test"}, resp.Agents[0].Metadata.Workspaces)

	// Verify second agent
	assert.Equal(t, "agent-2", resp.Agents[1].Id)
	assert.Equal(t, "Assistant", resp.Agents[1].Name)
	assert.Equal(t, "cli", resp.Agents[1].Backend)
}

func TestListAgents_WorkspaceFilter(t *testing.T) {
	lister := &mockAgentLister{
		agents: []*agent.AgentInfo{
			{
				ID:         "agent-1",
				Name:       "Claude",
				Workspaces: []string{"dev", "test"},
			},
			{
				ID:         "agent-2",
				Name:       "Assistant",
				Workspaces: []string{"prod"},
			},
			{
				ID:         "agent-3",
				Name:       "Helper",
				Workspaces: []string{"dev"},
			},
		},
	}

	svc := &ClientService{agents: lister}

	// Filter by "dev" workspace
	workspace := "dev"
	resp, err := svc.ListAgents(context.Background(), &pb.ListAgentsRequest{
		Workspace: &workspace,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Agents, 2)

	// Should include agent-1 and agent-3
	ids := []string{resp.Agents[0].Id, resp.Agents[1].Id}
	assert.Contains(t, ids, "agent-1")
	assert.Contains(t, ids, "agent-3")
}

func TestListAgents_WorkspaceFilter_NoMatch(t *testing.T) {
	lister := &mockAgentLister{
		agents: []*agent.AgentInfo{
			{
				ID:         "agent-1",
				Name:       "Claude",
				Workspaces: []string{"dev"},
			},
		},
	}

	svc := &ClientService{agents: lister}

	// Filter by non-existent workspace
	workspace := "staging"
	resp, err := svc.ListAgents(context.Background(), &pb.ListAgentsRequest{
		Workspace: &workspace,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Agents, 0)
}

func TestListAgents_NoAgents(t *testing.T) {
	lister := &mockAgentLister{
		agents: []*agent.AgentInfo{},
	}

	svc := &ClientService{agents: lister}

	resp, err := svc.ListAgents(context.Background(), &pb.ListAgentsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Agents, 0)
}

func TestListAgents_NilAgentLister(t *testing.T) {
	// Service without agent lister configured
	svc := &ClientService{}

	resp, err := svc.ListAgents(context.Background(), &pb.ListAgentsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Agents, 0)
}

func TestListAgents_AgentWithoutMetadata(t *testing.T) {
	// Agent without workspaces or instance ID should not have metadata
	lister := &mockAgentLister{
		agents: []*agent.AgentInfo{
			{
				ID:      "agent-1",
				Name:    "Claude",
				Backend: "mux",
			},
		},
	}

	svc := &ClientService{agents: lister}

	resp, err := svc.ListAgents(context.Background(), &pb.ListAgentsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Agents, 1)
	assert.Nil(t, resp.Agents[0].Metadata)
}
