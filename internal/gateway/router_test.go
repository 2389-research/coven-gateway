// ABOUTME: Tests for Router that connects bindings to message routing
// ABOUTME: Covers routing success, no binding, and agent offline scenarios

package gateway

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/2389/coven-gateway/internal/store"
)

// mockAgentChecker is a simple mock for testing router logic.
type mockAgentChecker struct {
	onlineAgents map[string]bool
}

func newMockAgentChecker() *mockAgentChecker {
	return &mockAgentChecker{
		onlineAgents: make(map[string]bool),
	}
}

func (m *mockAgentChecker) IsOnline(agentID string) bool {
	return m.onlineAgents[agentID]
}

func (m *mockAgentChecker) SetOnline(agentID string, online bool) {
	m.onlineAgents[agentID] = online
}

// mockBindingStore is a simple mock for unit testing router logic.
type mockBindingStore struct {
	bindings map[string]*store.Binding // key: "frontend:channelID"
}

func newMockBindingStore() *mockBindingStore {
	return &mockBindingStore{
		bindings: make(map[string]*store.Binding),
	}
}

func (m *mockBindingStore) GetBindingByChannel(ctx context.Context, frontend, channelID string) (*store.Binding, error) {
	key := frontend + ":" + channelID
	if b, ok := m.bindings[key]; ok {
		return b, nil
	}
	return nil, store.ErrBindingNotFound
}

func (m *mockBindingStore) AddBinding(frontend, channelID, agentID string) {
	key := frontend + ":" + channelID
	m.bindings[key] = &store.Binding{
		ID:        "test-binding",
		Frontend:  frontend,
		ChannelID: channelID,
		AgentID:   agentID,
		CreatedAt: time.Now(),
	}
}

// Unit Tests

func TestRouter_Route_Success(t *testing.T) {
	bindingStore := newMockBindingStore()
	agentChecker := newMockAgentChecker()

	bindingStore.AddBinding("matrix", "!room:example.org", "agent-001")
	agentChecker.SetOnline("agent-001", true)

	router := NewRouter(bindingStore, agentChecker)

	agentID, err := router.Route(context.Background(), "matrix", "!room:example.org")
	require.NoError(t, err)
	assert.Equal(t, "agent-001", agentID)
}

func TestRouter_Route_NoBinding(t *testing.T) {
	bindingStore := newMockBindingStore()
	agentChecker := newMockAgentChecker()

	router := NewRouter(bindingStore, agentChecker)

	_, err := router.Route(context.Background(), "matrix", "!nonexistent:example.org")
	assert.ErrorIs(t, err, ErrNoRoute)
}

func TestRouter_Route_AgentOffline(t *testing.T) {
	bindingStore := newMockBindingStore()
	agentChecker := newMockAgentChecker()

	bindingStore.AddBinding("slack", "C123456", "agent-002")
	agentChecker.SetOnline("agent-002", false) // agent is offline

	router := NewRouter(bindingStore, agentChecker)

	_, err := router.Route(context.Background(), "slack", "C123456")
	assert.ErrorIs(t, err, ErrAgentOffline)
}

func TestRouter_Route_StoreError(t *testing.T) {
	// Test that unexpected store errors are wrapped and returned
	agentChecker := newMockAgentChecker()

	// Create an error-returning binding store
	errorStore := &errorBindingStore{err: errors.New("database connection failed")}

	router := NewRouter(errorStore, agentChecker)

	_, err := router.Route(context.Background(), "matrix", "!room:example.org")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoRoute)
	assert.NotErrorIs(t, err, ErrAgentOffline)
	assert.Contains(t, err.Error(), "lookup binding")
}

type errorBindingStore struct {
	err error
}

func (e *errorBindingStore) GetBindingByChannel(ctx context.Context, frontend, channelID string) (*store.Binding, error) {
	return nil, e.err
}

// End-to-End Scenario Tests with real SQLite

func setupTestStoreForRouter(t *testing.T) *store.SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		s.Close()
	})

	return s
}

// createTestAgentForRouter creates a principal with type="agent" for binding tests.
func createTestAgentForRouter(t *testing.T, s *store.SQLiteStore, id string) {
	t.Helper()
	fp := id
	for len(fp) < 64 {
		fp += id
	}
	fp = strings.ReplaceAll(fp[:64], "-", "0")
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

func TestRouting_EndToEnd(t *testing.T) {
	// Test the full flow: real SQLite store with binding -> router lookup

	t.Run("successful route with real store", func(t *testing.T) {
		// Set up real SQLite store
		s := setupTestStoreForRouter(t)
		ctx := context.Background()

		// Create an agent principal in the store
		createTestAgentForRouter(t, s, "agent-alpha")

		// Create a binding: matrix:!room → agent
		binding := &store.Binding{
			ID:        "binding-001",
			Frontend:  "matrix",
			ChannelID: "!myroom:example.org",
			AgentID:   "agent-alpha",
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, s.CreateBindingV2(ctx, binding))

		// Create a mock agent checker that says agent is online
		agentChecker := newMockAgentChecker()
		agentChecker.SetOnline("agent-alpha", true)

		// Create router with real store
		router := NewRouter(s, agentChecker)

		// Call Route and verify it returns the agent_id
		agentID, err := router.Route(ctx, "matrix", "!myroom:example.org")
		require.NoError(t, err)
		assert.Equal(t, "agent-alpha", agentID)
	})

	t.Run("route when binding exists but agent offline", func(t *testing.T) {
		s := setupTestStoreForRouter(t)
		ctx := context.Background()

		createTestAgentForRouter(t, s, "agent-beta")

		binding := &store.Binding{
			ID:        "binding-002",
			Frontend:  "slack",
			ChannelID: "C987654",
			AgentID:   "agent-beta",
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, s.CreateBindingV2(ctx, binding))

		agentChecker := newMockAgentChecker()
		agentChecker.SetOnline("agent-beta", false) // offline

		router := NewRouter(s, agentChecker)

		_, err := router.Route(ctx, "slack", "C987654")
		assert.ErrorIs(t, err, ErrAgentOffline)
	})

	t.Run("route when no binding exists", func(t *testing.T) {
		s := setupTestStoreForRouter(t)
		ctx := context.Background()

		agentChecker := newMockAgentChecker()

		router := NewRouter(s, agentChecker)

		_, err := router.Route(ctx, "telegram", "chat_99999")
		assert.ErrorIs(t, err, ErrNoRoute)
	})

	t.Run("route with multiple bindings - correct routing", func(t *testing.T) {
		s := setupTestStoreForRouter(t)
		ctx := context.Background()

		createTestAgentForRouter(t, s, "agent-one")
		createTestAgentForRouter(t, s, "agent-two")

		// Binding 1: matrix room -> agent-one
		b1 := &store.Binding{
			ID:        "binding-m1",
			Frontend:  "matrix",
			ChannelID: "!room1:server.org",
			AgentID:   "agent-one",
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, s.CreateBindingV2(ctx, b1))

		// Binding 2: matrix room -> agent-two
		b2 := &store.Binding{
			ID:        "binding-m2",
			Frontend:  "matrix",
			ChannelID: "!room2:server.org",
			AgentID:   "agent-two",
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, s.CreateBindingV2(ctx, b2))

		agentChecker := newMockAgentChecker()
		agentChecker.SetOnline("agent-one", true)
		agentChecker.SetOnline("agent-two", true)

		router := NewRouter(s, agentChecker)

		// Route room1 -> should get agent-one
		agentID, err := router.Route(ctx, "matrix", "!room1:server.org")
		require.NoError(t, err)
		assert.Equal(t, "agent-one", agentID)

		// Route room2 -> should get agent-two
		agentID, err = router.Route(ctx, "matrix", "!room2:server.org")
		require.NoError(t, err)
		assert.Equal(t, "agent-two", agentID)
	})
}
