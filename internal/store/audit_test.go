// ABOUTME: Tests for audit log store operations
// ABOUTME: Covers Append and List with filtering for the audit_log table

package store

import (
	"context"
	"path/filepath"
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
	for i := range 3 {
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
	for i := range 5 {
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

func TestAuditLogAcceptsPairActions(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, action := range []AuditAction{AuditMintPairToken, AuditPairEnroll, AuditApproveLink} {
		err := s.AppendAuditLog(ctx, &AuditEntry{
			ActorPrincipalID: "actor-1",
			Action:           action,
			TargetType:       "principal",
			TargetID:         "target-1",
		})
		require.NoError(t, err, "action %s must pass the CHECK constraint", action)
	}

	entries, err := s.ListAuditLog(ctx, AuditFilter{})
	require.NoError(t, err)
	require.Len(t, entries, 3)
}

func TestAuditLogMigrationSurfacesProbeErrors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "probe-err-test.db")

	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// A closed handle makes the schema probe fail with a non-ErrNoRows error;
	// that must surface, not silently read as "no migration needed".
	err = s.migrateAuditLogCheckConstraint()
	require.ErrorContains(t, err, "checking audit_log schema")
}

func TestAuditLogMigrationAddsPairActions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migrate-test.db")

	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)

	// Simulate a pre-pair database: recreate audit_log with the old CHECK list.
	_, err = s.db.Exec(`DROP TABLE audit_log`)
	require.NoError(t, err)
	_, err = s.db.Exec(`CREATE TABLE audit_log (
		audit_id TEXT PRIMARY KEY,
		actor_principal_id TEXT NOT NULL,
		actor_member_id TEXT,
		action TEXT NOT NULL,
		target_type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		ts TEXT NOT NULL,
		detail_json TEXT,
		CHECK (action IN ('approve_principal', 'revoke_principal', 'grant_capability', 'revoke_capability', 'create_binding', 'update_binding', 'delete_binding', 'create_token', 'create_principal', 'delete_principal'))
	)`)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// Reopening runs migrations against the old-shaped table.
	s2, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	err = s2.AppendAuditLog(context.Background(), &AuditEntry{
		ActorPrincipalID: "actor-1",
		Action:           AuditMintPairToken,
		TargetType:       "pair_token",
		TargetID:         "tok-1",
	})
	require.NoError(t, err, "migrated table must accept mint_pair_token")
}
