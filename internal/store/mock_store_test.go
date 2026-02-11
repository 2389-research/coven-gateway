// ABOUTME: Unit tests for MockStore to ensure behavior matches SQLiteStore
// ABOUTME: Focuses on duplicate detection and edge cases specific to in-memory implementation

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockStore_CreateThread_Duplicate(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	thread := &Thread{
		ID:           "thread-123",
		FrontendName: "test-frontend",
		ExternalID:   "ext-123",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Second create with same (frontend_name, external_id) should fail
	thread2 := &Thread{
		ID:           "thread-456", // Different ID
		FrontendName: "test-frontend",
		ExternalID:   "ext-123",
		AgentID:      "agent-002",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err = store.CreateThread(ctx, thread2)
	assert.ErrorIs(t, err, ErrDuplicateThread, "duplicate (frontend_name, external_id) should return ErrDuplicateThread")
}

func TestMockStore_CreateThread_EmptyStringDuplicate(t *testing.T) {
	// SQLite enforces UNIQUE even for empty strings (not NULL).
	// MockStore must match this behavior.
	store := NewMockStore()
	ctx := context.Background()

	thread1 := &Thread{
		ID:           "thread-1",
		FrontendName: "",
		ExternalID:   "",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	err := store.CreateThread(ctx, thread1)
	require.NoError(t, err)

	// Second thread with both fields empty should fail
	thread2 := &Thread{
		ID:           "thread-2",
		FrontendName: "",
		ExternalID:   "",
		AgentID:      "agent-002",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err = store.CreateThread(ctx, thread2)
	assert.ErrorIs(t, err, ErrDuplicateThread, "duplicate empty strings should return ErrDuplicateThread (matches SQLite UNIQUE behavior)")
}

func TestMockStore_CreateThread_SingleEmptyField(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	// Create thread with frontend set, external empty
	thread1 := &Thread{
		ID:           "thread-1",
		FrontendName: "frontend",
		ExternalID:   "",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err := store.CreateThread(ctx, thread1)
	require.NoError(t, err)

	// Duplicate with same pattern should fail
	thread2 := &Thread{
		ID:           "thread-2",
		FrontendName: "frontend",
		ExternalID:   "",
		AgentID:      "agent-002",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err = store.CreateThread(ctx, thread2)
	assert.ErrorIs(t, err, ErrDuplicateThread, "duplicate (frontend, '') should fail")

	// Create thread with frontend empty, external set
	thread3 := &Thread{
		ID:           "thread-3",
		FrontendName: "",
		ExternalID:   "external",
		AgentID:      "agent-003",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err = store.CreateThread(ctx, thread3)
	require.NoError(t, err)

	// Duplicate with same pattern should fail
	thread4 := &Thread{
		ID:           "thread-4",
		FrontendName: "",
		ExternalID:   "external",
		AgentID:      "agent-004",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err = store.CreateThread(ctx, thread4)
	assert.ErrorIs(t, err, ErrDuplicateThread, "duplicate ('', external) should fail")
}

func TestMockStore_CreateThread_KeyCollisionPrevention(t *testing.T) {
	// Test that the null byte delimiter prevents key collisions.
	// With ":" delimiter, "a:b" + "c" and "a" + "b:c" would both produce "a:b:c".
	store := NewMockStore()
	ctx := context.Background()

	thread1 := &Thread{
		ID:           "thread-1",
		FrontendName: "a:b",
		ExternalID:   "c",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err := store.CreateThread(ctx, thread1)
	require.NoError(t, err)

	// This should NOT be considered a duplicate (different frontend/external)
	thread2 := &Thread{
		ID:           "thread-2",
		FrontendName: "a",
		ExternalID:   "b:c",
		AgentID:      "agent-002",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err = store.CreateThread(ctx, thread2)
	assert.NoError(t, err, "different (frontend, external) should not collide even with colons in values")
}

func TestMockStore_GetThreadByFrontendID(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	thread := &Thread{
		ID:           "thread-123",
		FrontendName: "test-frontend",
		ExternalID:   "ext-456",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Lookup by frontend ID
	found, err := store.GetThreadByFrontendID(ctx, "test-frontend", "ext-456")
	require.NoError(t, err)
	assert.Equal(t, "thread-123", found.ID)
	assert.Equal(t, "test-frontend", found.FrontendName)
	assert.Equal(t, "ext-456", found.ExternalID)
}

func TestMockStore_GetThreadByFrontendID_EmptyStrings(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	thread := &Thread{
		ID:           "thread-empty",
		FrontendName: "",
		ExternalID:   "",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Should be able to look up thread with empty strings
	found, err := store.GetThreadByFrontendID(ctx, "", "")
	require.NoError(t, err)
	assert.Equal(t, "thread-empty", found.ID)
}

func TestMockStore_GetThreadByFrontendID_NotFound(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	_, err := store.GetThreadByFrontendID(ctx, "nonexistent", "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}
