// ABOUTME: Tests for AdminService bindings gRPC handlers
// ABOUTME: Covers CRUD operations, validation, and audit logging for channel bindings

package admin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/2389/fold-gateway/internal/auth"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// createTestStore creates a real SQLite store in a temp directory
func createTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLite store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
		os.RemoveAll(tmpDir)
	})

	return s
}

// createAdminService creates an AdminService with the given store
func createAdminService(t *testing.T, s *store.SQLiteStore) *AdminService {
	t.Helper()
	return NewAdminService(s)
}

// createTestAgent creates a principal with type="agent" for binding tests
func createTestAgent(t *testing.T, s *store.SQLiteStore, id string) {
	t.Helper()
	// Create a unique 64-char hex fingerprint from the full ID
	fp := id
	for len(fp) < 64 {
		fp += id
	}
	fp = strings.ReplaceAll(fp[:64], "-", "0") // replace dashes with 0 for valid hex
	p := &store.Principal{
		ID:          id,
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fp,
		DisplayName: "Test Agent " + id,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, s.CreatePrincipal(context.Background(), p))
}

// createAdminContext creates a context with admin auth for testing
func createAdminContext(principalID string) context.Context {
	authCtx := &auth.AuthContext{
		PrincipalID:   principalID,
		PrincipalType: "client",
		Roles:         []string{"admin"},
	}
	return auth.WithAuth(context.Background(), authCtx)
}

func TestCreateBinding_Success(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	// Create an agent first
	createTestAgent(t, s, "agent-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}

	binding, err := svc.CreateBinding(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, binding)

	// Verify binding has expected values
	assert.NotEmpty(t, binding.Id)
	assert.Equal(t, "matrix", binding.Frontend)
	assert.Equal(t, "!room:example.org", binding.ChannelId)
	assert.Equal(t, "agent-001", binding.AgentId)
	assert.NotEmpty(t, binding.CreatedAt)
	assert.NotNil(t, binding.CreatedBy)
	assert.Equal(t, "admin-001", *binding.CreatedBy)
}

func TestCreateBinding_MissingFrontend(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}

	_, err := svc.CreateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "frontend")
}

func TestCreateBinding_MissingChannelID(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "",
		AgentId:   "agent-001",
	}

	_, err := svc.CreateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "channel_id")
}

func TestCreateBinding_MissingAgentID(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "",
	}

	_, err := svc.CreateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "agent_id")
}

func TestCreateBinding_AgentNotFound(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "nonexistent-agent",
	}

	_, err := svc.CreateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "agent not found")
}

func TestCreateBinding_DuplicateChannel(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}

	// Create first binding
	_, err := svc.CreateBinding(ctx, req)
	require.NoError(t, err)

	// Try to create duplicate binding
	_, err = svc.CreateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
	assert.Contains(t, st.Message(), "already bound")
}

func TestCreateBinding_WritesAuditLog(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	req := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}

	binding, err := svc.CreateBinding(ctx, req)
	require.NoError(t, err)

	// Verify audit log was written
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{
		TargetID: &binding.Id,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, "admin-001", entry.ActorPrincipalID)
	assert.Equal(t, store.AuditCreateBinding, entry.Action)
	assert.Equal(t, "binding", entry.TargetType)
	assert.Equal(t, binding.Id, entry.TargetID)
}

func TestUpdateBinding_Success(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")
	createTestAgent(t, s, "agent-002")

	// Create initial binding
	createReq := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}
	created, err := svc.CreateBinding(ctx, createReq)
	require.NoError(t, err)

	// Update to different agent
	updateReq := &pb.UpdateBindingRequest{
		Id:      created.Id,
		AgentId: "agent-002",
	}

	updated, err := svc.UpdateBinding(ctx, updateReq)
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Equal(t, created.Id, updated.Id)
	assert.Equal(t, "agent-002", updated.AgentId)
}

func TestUpdateBinding_NotFound(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	req := &pb.UpdateBindingRequest{
		Id:      "nonexistent-binding",
		AgentId: "agent-001",
	}

	_, err := svc.UpdateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestUpdateBinding_AgentNotFound(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	// Create initial binding
	createReq := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}
	created, err := svc.CreateBinding(ctx, createReq)
	require.NoError(t, err)

	// Try to update to nonexistent agent
	updateReq := &pb.UpdateBindingRequest{
		Id:      created.Id,
		AgentId: "nonexistent-agent",
	}

	_, err = svc.UpdateBinding(ctx, updateReq)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "agent not found")
}

func TestDeleteBinding_Success(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	// Create binding
	createReq := &pb.CreateBindingRequest{
		Frontend:  "matrix",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}
	created, err := svc.CreateBinding(ctx, createReq)
	require.NoError(t, err)

	// Delete binding
	deleteReq := &pb.DeleteBindingRequest{Id: created.Id}
	_, err = svc.DeleteBinding(ctx, deleteReq)
	require.NoError(t, err)

	// Verify binding is gone
	listReq := &pb.ListBindingsRequest{}
	listResp, err := svc.ListBindings(ctx, listReq)
	require.NoError(t, err)
	assert.Empty(t, listResp.Bindings)
}

func TestDeleteBinding_NotFound(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.DeleteBindingRequest{Id: "nonexistent-binding"}

	_, err := svc.DeleteBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestListBindings_All(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")
	createTestAgent(t, s, "agent-002")

	// Create multiple bindings
	bindings := []struct {
		frontend  string
		channelID string
		agentID   string
	}{
		{"matrix", "!room1:example.org", "agent-001"},
		{"slack", "C001", "agent-001"},
		{"matrix", "!room2:example.org", "agent-002"},
	}

	for _, b := range bindings {
		req := &pb.CreateBindingRequest{
			Frontend:  b.frontend,
			ChannelId: b.channelID,
			AgentId:   b.agentID,
		}
		_, err := svc.CreateBinding(ctx, req)
		require.NoError(t, err)
	}

	// List all bindings
	listReq := &pb.ListBindingsRequest{}
	resp, err := svc.ListBindings(ctx, listReq)
	require.NoError(t, err)
	assert.Len(t, resp.Bindings, 3)
}

func TestListBindings_ByFrontend(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	// Create bindings for different frontends
	bindings := []struct {
		frontend  string
		channelID string
	}{
		{"matrix", "!room1:example.org"},
		{"slack", "C001"},
		{"matrix", "!room2:example.org"},
	}

	for _, b := range bindings {
		req := &pb.CreateBindingRequest{
			Frontend:  b.frontend,
			ChannelId: b.channelID,
			AgentId:   "agent-001",
		}
		_, err := svc.CreateBinding(ctx, req)
		require.NoError(t, err)
	}

	// List only matrix bindings
	frontend := "matrix"
	listReq := &pb.ListBindingsRequest{Frontend: &frontend}
	resp, err := svc.ListBindings(ctx, listReq)
	require.NoError(t, err)
	assert.Len(t, resp.Bindings, 2)

	for _, b := range resp.Bindings {
		assert.Equal(t, "matrix", b.Frontend)
	}
}

func TestListBindings_ByAgentID(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")
	createTestAgent(t, s, "agent-002")

	// Create bindings for different agents
	bindings := []struct {
		frontend  string
		channelID string
		agentID   string
	}{
		{"matrix", "!room1:example.org", "agent-001"},
		{"slack", "C001", "agent-002"},
		{"matrix", "!room2:example.org", "agent-001"},
	}

	for _, b := range bindings {
		req := &pb.CreateBindingRequest{
			Frontend:  b.frontend,
			ChannelId: b.channelID,
			AgentId:   b.agentID,
		}
		_, err := svc.CreateBinding(ctx, req)
		require.NoError(t, err)
	}

	// List only agent-001 bindings
	agentID := "agent-001"
	listReq := &pb.ListBindingsRequest{AgentId: &agentID}
	resp, err := svc.ListBindings(ctx, listReq)
	require.NoError(t, err)
	assert.Len(t, resp.Bindings, 2)

	for _, b := range resp.Bindings {
		assert.Equal(t, "agent-001", b.AgentId)
	}
}

func TestCreateBinding_InvalidFrontendFormat(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	createTestAgent(t, s, "agent-001")

	// Frontend with invalid characters
	req := &pb.CreateBindingRequest{
		Frontend:  "MATRIX-!@#$",
		ChannelId: "!room:example.org",
		AgentId:   "agent-001",
	}

	_, err := svc.CreateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestUpdateBinding_MissingID(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.UpdateBindingRequest{
		Id:      "",
		AgentId: "agent-001",
	}

	_, err := svc.UpdateBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "id")
}

func TestDeleteBinding_MissingID(t *testing.T) {
	s := createTestStore(t)
	svc := createAdminService(t, s)
	ctx := createAdminContext("admin-001")

	req := &pb.DeleteBindingRequest{Id: ""}

	_, err := svc.DeleteBinding(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "id")
}
