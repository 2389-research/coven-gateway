package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		store.Close()
	})

	return store
}

func TestStore_CreateThread(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	thread := &Thread{
		ID:           "thread-123",
		FrontendName: "test-frontend",
		ExternalID:   "ext-123",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetThread(ctx, "thread-123")
	require.NoError(t, err)
	assert.Equal(t, "thread-123", retrieved.ID)
}

func TestStore_CreateThread_Duplicate(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	thread := &Thread{
		ID:           "thread-123",
		FrontendName: "test-frontend",
		ExternalID:   "ext-123",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Second create should fail
	err = store.CreateThread(ctx, thread)
	assert.Error(t, err, "duplicate thread creation should fail")
}

func TestStore_GetThread_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetThread(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}
