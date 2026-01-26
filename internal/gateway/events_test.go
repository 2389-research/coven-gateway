// ABOUTME: Tests for ledger event recording with actor attribution
// ABOUTME: Covers scenarios for different auth contexts (client, bridge, agent, no auth)

package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
)

// strPtr returns a pointer to the given string
func strPtr(s string) *string {
	return &s
}

// setupTestGatewayWithStore creates a minimal gateway with a mock store for testing
func setupTestGatewayWithStore(t *testing.T) (*Gateway, *store.MockStore) {
	t.Helper()
	mockStore := store.NewMockStore()

	gw := &Gateway{
		store: mockStore,
	}

	return gw, mockStore
}

func TestRecordEvent_FromClient(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// Set up auth context for a client
	authCtx := &auth.AuthContext{
		PrincipalID:   "principal-client-123",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	ctx = auth.WithAuth(ctx, authCtx)

	// Create an event without actor fields
	event := &store.LedgerEvent{
		ID:              "event-from-client",
		ConversationKey: "tui:client-123:pane-1",
		Direction:       store.EventDirectionInbound,
		Author:          "harper",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            store.EventTypeMessage,
		Text:            strPtr("Hello from TUI"),
	}

	// Record the event - should populate actor from auth context
	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	// Verify the event was saved with actor attribution
	saved, err := mockStore.GetEvent(ctx, "event-from-client")
	require.NoError(t, err)
	require.NotNil(t, saved.ActorPrincipalID)
	assert.Equal(t, "principal-client-123", *saved.ActorPrincipalID)
	assert.Nil(t, saved.ActorMemberID) // no member linked in v1
}

func TestRecordEvent_FromClientWithMember(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// Set up auth context for a client with a linked member
	memberID := "member-harper"
	authCtx := &auth.AuthContext{
		PrincipalID:   "principal-client-456",
		PrincipalType: "client",
		MemberID:      &memberID,
		Roles:         []string{"admin"},
	}
	ctx = auth.WithAuth(ctx, authCtx)

	event := &store.LedgerEvent{
		ID:              "event-with-member",
		ConversationKey: "web:session-789",
		Direction:       store.EventDirectionInbound,
		Author:          "harper",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            store.EventTypeMessage,
		Text:            strPtr("Hello from web"),
	}

	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	saved, err := mockStore.GetEvent(ctx, "event-with-member")
	require.NoError(t, err)
	require.NotNil(t, saved.ActorPrincipalID)
	require.NotNil(t, saved.ActorMemberID)
	assert.Equal(t, "principal-client-456", *saved.ActorPrincipalID)
	assert.Equal(t, "member-harper", *saved.ActorMemberID)
}

func TestRecordEvent_FromBridge(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// Set up auth context for a bridge (e.g., Matrix bridge)
	// Bridge is authenticated as a principal, but the remote user is not
	authCtx := &auth.AuthContext{
		PrincipalID:   "principal-matrix-bridge",
		PrincipalType: "client", // bridges are clients
		Roles:         []string{"member"},
	}
	ctx = auth.WithAuth(ctx, authCtx)

	// Remote user info would be in raw payload or detail_json
	event := &store.LedgerEvent{
		ID:              "event-from-bridge",
		ConversationKey: "matrix:!room123:server.com",
		Direction:       store.EventDirectionInbound,
		Author:          "@user:matrix.org", // remote user, but actor is the bridge
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            store.EventTypeMessage,
		Text:            strPtr("Hello from Matrix"),
		RawTransport:    strPtr("matrix"),
	}

	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	saved, err := mockStore.GetEvent(ctx, "event-from-bridge")
	require.NoError(t, err)
	require.NotNil(t, saved.ActorPrincipalID)
	assert.Equal(t, "principal-matrix-bridge", *saved.ActorPrincipalID)
	assert.Nil(t, saved.ActorMemberID)
	// Author field preserves the remote user identity
	assert.Equal(t, "@user:matrix.org", saved.Author)
}

func TestRecordEvent_FromAgent(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// Set up auth context for an agent
	authCtx := &auth.AuthContext{
		PrincipalID:   "principal-agent-001",
		PrincipalType: "agent",
		Roles:         []string{"member"},
	}
	ctx = auth.WithAuth(ctx, authCtx)

	event := &store.LedgerEvent{
		ID:              "event-from-agent",
		ConversationKey: "tui:client-123:pane-1",
		Direction:       store.EventDirectionOutbound,
		Author:          "agent-code-assistant",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            store.EventTypeMessage,
		Text:            strPtr("Here is the code you requested..."),
	}

	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	saved, err := mockStore.GetEvent(ctx, "event-from-agent")
	require.NoError(t, err)
	require.NotNil(t, saved.ActorPrincipalID)
	assert.Equal(t, "principal-agent-001", *saved.ActorPrincipalID)
	assert.Nil(t, saved.ActorMemberID)
}

func TestRecordEvent_FromPack(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// Set up auth context for a pack
	authCtx := &auth.AuthContext{
		PrincipalID:   "principal-pack-elevenlabs",
		PrincipalType: "pack",
		Roles:         []string{"member"},
	}
	ctx = auth.WithAuth(ctx, authCtx)

	event := &store.LedgerEvent{
		ID:              "event-from-pack",
		ConversationKey: "pack:events",
		Direction:       store.EventDirectionInbound,
		Author:          "elevenlabs-pack",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            store.EventTypeSystem,
		Text:            strPtr("Audio playback completed"),
	}

	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	saved, err := mockStore.GetEvent(ctx, "event-from-pack")
	require.NoError(t, err)
	require.NotNil(t, saved.ActorPrincipalID)
	assert.Equal(t, "principal-pack-elevenlabs", *saved.ActorPrincipalID)
	assert.Nil(t, saved.ActorMemberID)
}

func TestRecordEvent_NoAuthContext(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// No auth context - could be a system event or pre-auth event
	event := &store.LedgerEvent{
		ID:              "event-no-auth",
		ConversationKey: "system:startup",
		Direction:       store.EventDirectionOutbound,
		Author:          "system",
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		Type:            store.EventTypeSystem,
		Text:            strPtr("Gateway started"),
	}

	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	saved, err := mockStore.GetEvent(ctx, "event-no-auth")
	require.NoError(t, err)
	assert.Nil(t, saved.ActorPrincipalID)
	assert.Nil(t, saved.ActorMemberID)
}

func TestRecordEvent_DoesNotOverrideExistingActor(t *testing.T) {
	gw, mockStore := setupTestGatewayWithStore(t)
	ctx := context.Background()

	// Set up auth context
	authCtx := &auth.AuthContext{
		PrincipalID:   "principal-current",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	ctx = auth.WithAuth(ctx, authCtx)

	// Event already has actor set (e.g., forwarding an event)
	existingActor := "principal-original"
	event := &store.LedgerEvent{
		ID:               "event-existing-actor",
		ConversationKey:  "test:conv",
		Direction:        store.EventDirectionInbound,
		Author:           "user",
		Timestamp:        time.Now().UTC().Truncate(time.Second),
		Type:             store.EventTypeMessage,
		Text:             strPtr("Forwarded message"),
		ActorPrincipalID: &existingActor, // already set
	}

	err := gw.recordEvent(ctx, event)
	require.NoError(t, err)

	saved, err := mockStore.GetEvent(ctx, "event-existing-actor")
	require.NoError(t, err)
	require.NotNil(t, saved.ActorPrincipalID)
	// Should NOT override existing actor
	assert.Equal(t, "principal-original", *saved.ActorPrincipalID)
}
