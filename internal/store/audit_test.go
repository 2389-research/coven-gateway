// ABOUTME: Tests for audit log store operations
// ABOUTME: Covers Append and List with filtering for the audit_log table

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditStore_Append(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	entry := &AuditEntry{
		ActorPrincipalID: "principal-123",
		Action:           AuditApprovePrincipal,
		TargetType:       "principal",
		TargetID:         "principal-456",
		Detail:           map[string]any{"reason": "approved by admin"},
	}

	err := store.AppendAuditLog(ctx, entry)
	require.NoError(t, err)

	// Should have generated ID and timestamp
	assert.NotEmpty(t, entry.ID)
	assert.False(t, entry.Timestamp.IsZero())
}

func TestAuditStore_List_NoFilter(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Append multiple entries
	for i, action := range []AuditAction{AuditApprovePrincipal, AuditRevokePrincipal, AuditCreateBinding} {
		entry := &AuditEntry{
			ActorPrincipalID: "principal-123",
			Action:           action,
			TargetType:       "principal",
			TargetID:         generateTestID("target", i),
			Timestamp:        time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, store.AppendAuditLog(ctx, entry))
	}

	entries, err := store.ListAuditLog(ctx, AuditFilter{})
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// Should be newest first
	assert.Equal(t, AuditCreateBinding, entries[0].Action)
}

func TestAuditStore_List_BySince(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	baseTime := now.Add(-time.Hour)

	// Create entries at different times
	for i := 0; i < 3; i++ {
		entry := &AuditEntry{
			ActorPrincipalID: "principal-123",
			Action:           AuditApprovePrincipal,
			TargetType:       "principal",
			TargetID:         generateTestID("target", i),
			Timestamp:        baseTime.Add(time.Duration(i) * 10 * time.Minute),
		}
		require.NoError(t, store.AppendAuditLog(ctx, entry))
	}

	// Filter to entries after 15 minutes in
	since := baseTime.Add(15 * time.Minute)
	entries, err := store.ListAuditLog(ctx, AuditFilter{Since: &since})
	require.NoError(t, err)
	assert.Len(t, entries, 1) // Only entry at 20 minutes
}

func TestAuditStore_List_ByActor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create entries with different actors
	for i, actor := range []string{"actor-1", "actor-2", "actor-1"} {
		entry := &AuditEntry{
			ActorPrincipalID: actor,
			Action:           AuditApprovePrincipal,
			TargetType:       "principal",
			TargetID:         generateTestID("target", i),
		}
		require.NoError(t, store.AppendAuditLog(ctx, entry))
	}

	actor := "actor-1"
	entries, err := store.ListAuditLog(ctx, AuditFilter{ActorPrincipalID: &actor})
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	for _, e := range entries {
		assert.Equal(t, "actor-1", e.ActorPrincipalID)
	}
}

func TestAuditStore_List_ByAction(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create entries with different actions
	actions := []AuditAction{AuditApprovePrincipal, AuditRevokePrincipal, AuditApprovePrincipal}
	for i, action := range actions {
		entry := &AuditEntry{
			ActorPrincipalID: "principal-123",
			Action:           action,
			TargetType:       "principal",
			TargetID:         generateTestID("target", i),
		}
		require.NoError(t, store.AppendAuditLog(ctx, entry))
	}

	action := AuditApprovePrincipal
	entries, err := store.ListAuditLog(ctx, AuditFilter{Action: &action})
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	for _, e := range entries {
		assert.Equal(t, AuditApprovePrincipal, e.Action)
	}
}

func TestAuditStore_List_ByTarget(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create entries with different targets
	entries := []struct {
		targetType string
		targetID   string
	}{
		{"principal", "p-1"},
		{"binding", "b-1"},
		{"principal", "p-1"},
	}
	for _, e := range entries {
		entry := &AuditEntry{
			ActorPrincipalID: "principal-123",
			Action:           AuditApprovePrincipal,
			TargetType:       e.targetType,
			TargetID:         e.targetID,
		}
		require.NoError(t, store.AppendAuditLog(ctx, entry))
	}

	targetType := "principal"
	targetID := "p-1"
	results, err := store.ListAuditLog(ctx, AuditFilter{
		TargetType: &targetType,
		TargetID:   &targetID,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestAuditStore_List_Pagination(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create 5 entries
	for i := 0; i < 5; i++ {
		entry := &AuditEntry{
			ActorPrincipalID: "principal-123",
			Action:           AuditApprovePrincipal,
			TargetType:       "principal",
			TargetID:         generateTestID("target", i),
		}
		require.NoError(t, store.AppendAuditLog(ctx, entry))
	}

	// Get first 2
	entries, err := store.ListAuditLog(ctx, AuditFilter{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestAuditStore_Append_WithMemberID(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	memberID := "member-xyz"
	entry := &AuditEntry{
		ActorPrincipalID: "principal-123",
		ActorMemberID:    &memberID,
		Action:           AuditApprovePrincipal,
		TargetType:       "principal",
		TargetID:         "principal-456",
	}

	err := store.AppendAuditLog(ctx, entry)
	require.NoError(t, err)

	entries, err := store.ListAuditLog(ctx, AuditFilter{})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].ActorMemberID)
	assert.Equal(t, memberID, *entries[0].ActorMemberID)
}
