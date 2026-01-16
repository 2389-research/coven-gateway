package store

import (
	"context"
	"fmt"
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

func TestStore_SaveMessage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread first
	thread := &Thread{
		ID:           "thread-123",
		FrontendName: "test-frontend",
		ExternalID:   "ext-123",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateThread(ctx, thread))

	msg := &Message{
		ID:        "msg-1",
		ThreadID:  "thread-123",
		Sender:    "user",
		Content:   "Hello",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	err := store.SaveMessage(ctx, msg)
	require.NoError(t, err)

	// Retrieve messages
	messages, err := store.GetThreadMessages(ctx, "thread-123", 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "Hello", messages[0].Content)
}

func TestStore_GetThreadMessages_Order(t *testing.T) {
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
	require.NoError(t, store.CreateThread(ctx, thread))

	// Save messages in order
	for i, content := range []string{"first", "second", "third"} {
		msg := &Message{
			ID:        fmt.Sprintf("msg-%d", i),
			ThreadID:  "thread-123",
			Sender:    "user",
			Content:   content,
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second).Truncate(time.Second),
		}
		require.NoError(t, store.SaveMessage(ctx, msg))
	}

	messages, err := store.GetThreadMessages(ctx, "thread-123", 0)
	require.NoError(t, err)
	require.Len(t, messages, 3)

	// Should be in chronological order
	assert.Equal(t, "first", messages[0].Content)
	assert.Equal(t, "second", messages[1].Content)
	assert.Equal(t, "third", messages[2].Content)
}
