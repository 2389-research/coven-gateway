// ABOUTME: Tests for the channel bindings store operations
// ABOUTME: Covers CRUD, filtering, and validation for frontend channel to agent bindings

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestAgent creates a principal with type="agent" for binding tests
func createTestAgent(t *testing.T, store *SQLiteStore, id string) {
	t.Helper()
	// Create a unique 64-char hex fingerprint from the full ID
	// Pad the ID with the ID repeated until we have 64 chars
	fp := id
	for len(fp) < 64 {
		fp += id
	}
	fp = strings.ReplaceAll(fp[:64], "-", "0") // replace dashes with 0 for valid hex
	p := &Principal{
		ID:          id,
		Type:        PrincipalTypeAgent,
		PubkeyFP:    fp,
		DisplayName: "Test Agent " + id,
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(context.Background(), p))
}

func TestBindingStore_Create(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create an agent first (required for foreign key validation)
	createTestAgent(t, store, "agent-001")

	createdBy := "admin-001"
	binding := &Binding{
		ID:        "binding-001",
		Frontend:  "matrix",
		ChannelID: "!room:example.org",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		CreatedBy: &createdBy,
	}

	err := store.CreateBindingV2(ctx, binding)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetBindingByID(ctx, "binding-001")
	require.NoError(t, err)
	assert.Equal(t, "binding-001", retrieved.ID)
	assert.Equal(t, "matrix", retrieved.Frontend)
	assert.Equal(t, "!room:example.org", retrieved.ChannelID)
	assert.Equal(t, "agent-001", retrieved.AgentID)
	assert.NotNil(t, retrieved.CreatedBy)
	assert.Equal(t, "admin-001", *retrieved.CreatedBy)
}

func TestBindingStore_Create_DuplicateChannel(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	binding1 := &Binding{
		ID:        "binding-001",
		Frontend:  "slack",
		ChannelID: "C123456",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateBindingV2(ctx, binding1))

	// Try to create another binding for the same frontend+channel
	binding2 := &Binding{
		ID:        "binding-002",
		Frontend:  "slack",
		ChannelID: "C123456", // same channel
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateBindingV2(ctx, binding2)
	assert.ErrorIs(t, err, ErrDuplicateChannel)
}

func TestBindingStore_Create_AgentNotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// No agent exists with this ID
	binding := &Binding{
		ID:        "binding-001",
		Frontend:  "matrix",
		ChannelID: "!room:example.org",
		AgentID:   "nonexistent-agent",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateBindingV2(ctx, binding)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestBindingStore_Create_AgentWrongType(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create a client principal (not an agent)
	client := &Principal{
		ID:          "client-001",
		Type:        PrincipalTypeClient,
		PubkeyFP:    strings.Repeat("b", 64),
		DisplayName: "Test Client",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, client))

	// Try to bind to the client (should fail, must be agent type)
	binding := &Binding{
		ID:        "binding-001",
		Frontend:  "matrix",
		ChannelID: "!room:example.org",
		AgentID:   "client-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateBindingV2(ctx, binding)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestBindingStore_Get(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	binding := &Binding{
		ID:        "binding-abc",
		Frontend:  "telegram",
		ChannelID: "chat_12345",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateBindingV2(ctx, binding))

	retrieved, err := store.GetBindingByID(ctx, "binding-abc")
	require.NoError(t, err)
	assert.Equal(t, "telegram", retrieved.Frontend)
	assert.Equal(t, "chat_12345", retrieved.ChannelID)
}

func TestBindingStore_Get_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetBindingByID(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrBindingNotFound)
}

func TestBindingStore_GetByChannel(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	binding := &Binding{
		ID:        "binding-xyz",
		Frontend:  "matrix",
		ChannelID: "!specific:server.org",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateBindingV2(ctx, binding))

	retrieved, err := store.GetBindingByChannel(ctx, "matrix", "!specific:server.org")
	require.NoError(t, err)
	assert.Equal(t, "binding-xyz", retrieved.ID)
	assert.Equal(t, "agent-001", retrieved.AgentID)
}

func TestBindingStore_GetByChannel_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetBindingByChannel(ctx, "matrix", "!nonexistent:server.org")
	assert.ErrorIs(t, err, ErrBindingNotFound)
}

func TestBindingStore_Update(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")
	createTestAgent(t, store, "agent-002")

	binding := &Binding{
		ID:        "binding-update",
		Frontend:  "slack",
		ChannelID: "C999",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateBindingV2(ctx, binding))

	// Update to point to different agent
	err := store.UpdateBinding(ctx, "binding-update", "agent-002")
	require.NoError(t, err)

	retrieved, err := store.GetBindingByID(ctx, "binding-update")
	require.NoError(t, err)
	assert.Equal(t, "agent-002", retrieved.AgentID)
}

func TestBindingStore_Update_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	err := store.UpdateBinding(ctx, "nonexistent", "agent-001")
	assert.ErrorIs(t, err, ErrBindingNotFound)
}

func TestBindingStore_Update_AgentNotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	binding := &Binding{
		ID:        "binding-update",
		Frontend:  "slack",
		ChannelID: "C999",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateBindingV2(ctx, binding))

	// Try to update to nonexistent agent
	err := store.UpdateBinding(ctx, "binding-update", "nonexistent-agent")
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestBindingStore_Delete(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	binding := &Binding{
		ID:        "binding-delete",
		Frontend:  "matrix",
		ChannelID: "!delete:server.org",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateBindingV2(ctx, binding))

	err := store.DeleteBindingByID(ctx, "binding-delete")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetBindingByID(ctx, "binding-delete")
	assert.ErrorIs(t, err, ErrBindingNotFound)
}

func TestBindingStore_Delete_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	err := store.DeleteBindingByID(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrBindingNotFound)
}

func TestBindingStore_List_All(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")
	createTestAgent(t, store, "agent-002")

	// Create bindings for different frontends and agents
	bindings := []*Binding{
		{ID: "b1", Frontend: "matrix", ChannelID: "!room1:s.org", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "b2", Frontend: "slack", ChannelID: "C001", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "b3", Frontend: "matrix", ChannelID: "!room2:s.org", AgentID: "agent-002", CreatedAt: time.Now().UTC().Truncate(time.Second)},
	}

	for _, b := range bindings {
		require.NoError(t, store.CreateBindingV2(ctx, b))
	}

	result, err := store.ListBindingsV2(ctx, BindingFilter{})
	require.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestBindingStore_List_ByFrontend(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	bindings := []*Binding{
		{ID: "b1", Frontend: "matrix", ChannelID: "!room1:s.org", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "b2", Frontend: "slack", ChannelID: "C001", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "b3", Frontend: "matrix", ChannelID: "!room2:s.org", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
	}

	for _, b := range bindings {
		require.NoError(t, store.CreateBindingV2(ctx, b))
	}

	frontend := "matrix"
	result, err := store.ListBindingsV2(ctx, BindingFilter{Frontend: &frontend})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	for _, b := range result {
		assert.Equal(t, "matrix", b.Frontend)
	}
}

func TestBindingStore_List_ByAgent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")
	createTestAgent(t, store, "agent-002")

	bindings := []*Binding{
		{ID: "b1", Frontend: "matrix", ChannelID: "!room1:s.org", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "b2", Frontend: "slack", ChannelID: "C001", AgentID: "agent-002", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "b3", Frontend: "matrix", ChannelID: "!room2:s.org", AgentID: "agent-001", CreatedAt: time.Now().UTC().Truncate(time.Second)},
	}

	for _, b := range bindings {
		require.NoError(t, store.CreateBindingV2(ctx, b))
	}

	agentID := "agent-001"
	result, err := store.ListBindingsV2(ctx, BindingFilter{AgentID: &agentID})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	for _, b := range result {
		assert.Equal(t, "agent-001", b.AgentID)
	}
}

func TestBindingStore_Create_NilCreatedBy(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-001")

	binding := &Binding{
		ID:        "binding-nil-creator",
		Frontend:  "matrix",
		ChannelID: "!room:example.org",
		AgentID:   "agent-001",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		CreatedBy: nil, // explicitly nil
	}

	err := store.CreateBindingV2(ctx, binding)
	require.NoError(t, err)

	retrieved, err := store.GetBindingByID(ctx, "binding-nil-creator")
	require.NoError(t, err)
	assert.Nil(t, retrieved.CreatedBy)
}

func TestBindingWithWorkingDir(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create an agent principal first
	createTestAgent(t, store, "agent-uuid")

	binding := &Binding{
		ID:         "binding-uuid",
		Frontend:   "matrix",
		ChannelID:  "!room:server",
		AgentID:    "agent-uuid",
		WorkingDir: "/projects/website",
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateBindingV2(ctx, binding)
	require.NoError(t, err)

	// Retrieve and verify working_dir is preserved
	got, err := store.GetBindingByChannel(ctx, "matrix", "!room:server")
	require.NoError(t, err)
	assert.Equal(t, "/projects/website", got.WorkingDir)
}

func TestBindingWithWorkingDir_NilIsEmpty(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-empty-wd")

	// Binding without WorkingDir set (should default to empty string)
	binding := &Binding{
		ID:        "binding-empty-wd",
		Frontend:  "slack",
		ChannelID: "C12345",
		AgentID:   "agent-empty-wd",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateBindingV2(ctx, binding)
	require.NoError(t, err)

	got, err := store.GetBindingByID(ctx, "binding-empty-wd")
	require.NoError(t, err)
	assert.Equal(t, "", got.WorkingDir)
}

func TestBindingWithWorkingDir_ListPreserves(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	createTestAgent(t, store, "agent-list-wd")

	binding := &Binding{
		ID:         "binding-list-wd",
		Frontend:   "telegram",
		ChannelID:  "chat_789",
		AgentID:    "agent-list-wd",
		WorkingDir: "/home/user/myproject",
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateBindingV2(ctx, binding)
	require.NoError(t, err)

	// List bindings and verify working_dir is preserved
	bindings, err := store.ListBindingsV2(ctx, BindingFilter{})
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "/home/user/myproject", bindings[0].WorkingDir)
}
