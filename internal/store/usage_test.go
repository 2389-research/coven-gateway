// ABOUTME: Tests for token usage tracking functionality
// ABOUTME: Covers SaveUsage, LinkUsageToMessage, GetThreadUsage, GetUsageStats

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_SaveUsage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create a thread first (for foreign key)
	thread := &Thread{
		ID:           "thread-usage-001",
		FrontendName: "test-frontend",
		ExternalID:   "ext-001",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreateThread(ctx, thread))

	usage := &TokenUsage{
		ID:               uuid.New().String(),
		ThreadID:         "thread-usage-001",
		RequestID:        "req-001",
		AgentID:          "agent-001",
		InputTokens:      1000,
		OutputTokens:     500,
		CacheReadTokens:  200,
		CacheWriteTokens: 100,
		ThinkingTokens:   50,
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}

	err := store.SaveUsage(ctx, usage)
	require.NoError(t, err)

	// Verify by getting thread usage
	usages, err := store.GetThreadUsage(ctx, "thread-usage-001")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, int32(1000), usages[0].InputTokens)
	assert.Equal(t, int32(500), usages[0].OutputTokens)
	assert.Equal(t, "agent-001", usages[0].AgentID)
}

func TestStore_LinkUsageToMessage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread
	thread := &Thread{
		ID:           "thread-link-001",
		FrontendName: "test-frontend",
		ExternalID:   "ext-link-001",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, store.CreateThread(ctx, thread))

	// Create usage without message ID
	usage := &TokenUsage{
		ID:           uuid.New().String(),
		ThreadID:     "thread-link-001",
		RequestID:    "req-link-001",
		AgentID:      "agent-001",
		InputTokens:  500,
		OutputTokens: 250,
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, store.SaveUsage(ctx, usage))

	// Link to message
	messageID := "msg-final-001"
	err := store.LinkUsageToMessage(ctx, "req-link-001", messageID)
	require.NoError(t, err)

	// Verify the link
	usages, err := store.GetThreadUsage(ctx, "thread-link-001")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, messageID, usages[0].MessageID)
}

func TestStore_GetThreadUsage_Empty(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	usages, err := store.GetThreadUsage(ctx, "nonexistent-thread")
	require.NoError(t, err)
	assert.Empty(t, usages)
}

func TestStore_GetUsageStats_NoFilter(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread
	thread := &Thread{
		ID:           "thread-stats-001",
		FrontendName: "test-frontend",
		ExternalID:   "ext-stats-001",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, store.CreateThread(ctx, thread))

	// Add multiple usage records
	for i := 0; i < 3; i++ {
		usage := &TokenUsage{
			ID:               uuid.New().String(),
			ThreadID:         "thread-stats-001",
			RequestID:        "req-stats-" + string(rune('a'+i)),
			AgentID:          "agent-001",
			InputTokens:      100,
			OutputTokens:     50,
			CacheReadTokens:  20,
			CacheWriteTokens: 10,
			ThinkingTokens:   5,
			CreatedAt:        time.Now().UTC(),
		}
		require.NoError(t, store.SaveUsage(ctx, usage))
	}

	stats, err := store.GetUsageStats(ctx, UsageFilter{})
	require.NoError(t, err)

	assert.Equal(t, int64(300), stats.TotalInput)     // 100 * 3
	assert.Equal(t, int64(150), stats.TotalOutput)    // 50 * 3
	assert.Equal(t, int64(60), stats.TotalCacheRead)  // 20 * 3
	assert.Equal(t, int64(30), stats.TotalCacheWrite) // 10 * 3
	assert.Equal(t, int64(15), stats.TotalThinking)   // 5 * 3
	assert.Equal(t, int64(3), stats.RequestCount)
	assert.Equal(t, int64(465), stats.TotalTokens) // 300 + 150 + 15
}

func TestStore_GetUsageStats_FilterByAgent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread
	thread := &Thread{
		ID:           "thread-filter-001",
		FrontendName: "test-frontend",
		ExternalID:   "ext-filter-001",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, store.CreateThread(ctx, thread))

	// Add usage for agent-001
	require.NoError(t, store.SaveUsage(ctx, &TokenUsage{
		ID:           uuid.New().String(),
		ThreadID:     "thread-filter-001",
		RequestID:    "req-agent1",
		AgentID:      "agent-001",
		InputTokens:  100,
		OutputTokens: 50,
		CreatedAt:    time.Now().UTC(),
	}))

	// Add usage for agent-002
	require.NoError(t, store.SaveUsage(ctx, &TokenUsage{
		ID:           uuid.New().String(),
		ThreadID:     "thread-filter-001",
		RequestID:    "req-agent2",
		AgentID:      "agent-002",
		InputTokens:  200,
		OutputTokens: 100,
		CreatedAt:    time.Now().UTC(),
	}))

	// Filter by agent-001
	agentID := "agent-001"
	stats, err := store.GetUsageStats(ctx, UsageFilter{AgentID: &agentID})
	require.NoError(t, err)

	assert.Equal(t, int64(100), stats.TotalInput)
	assert.Equal(t, int64(50), stats.TotalOutput)
	assert.Equal(t, int64(1), stats.RequestCount)
}

func TestStore_GetUsageStats_FilterByTime(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread
	thread := &Thread{
		ID:           "thread-time-001",
		FrontendName: "test-frontend",
		ExternalID:   "ext-time-001",
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, store.CreateThread(ctx, thread))

	// Add usage from yesterday
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	require.NoError(t, store.SaveUsage(ctx, &TokenUsage{
		ID:           uuid.New().String(),
		ThreadID:     "thread-time-001",
		RequestID:    "req-yesterday",
		AgentID:      "agent-001",
		InputTokens:  100,
		OutputTokens: 50,
		CreatedAt:    yesterday,
	}))

	// Add usage from today
	now := time.Now().UTC()
	require.NoError(t, store.SaveUsage(ctx, &TokenUsage{
		ID:           uuid.New().String(),
		ThreadID:     "thread-time-001",
		RequestID:    "req-today",
		AgentID:      "agent-001",
		InputTokens:  200,
		OutputTokens: 100,
		CreatedAt:    now,
	}))

	// Filter for today only
	today := time.Now().UTC().Truncate(24 * time.Hour)
	stats, err := store.GetUsageStats(ctx, UsageFilter{Since: &today})
	require.NoError(t, err)

	// Should only include today's usage
	assert.Equal(t, int64(200), stats.TotalInput)
	assert.Equal(t, int64(100), stats.TotalOutput)
	assert.Equal(t, int64(1), stats.RequestCount)
}

func TestMockStore_UsageStore(t *testing.T) {
	mockStore := NewMockStore()
	ctx := context.Background()

	// Save usage
	usage := &TokenUsage{
		ID:           "usage-001",
		ThreadID:     "thread-001",
		RequestID:    "req-001",
		AgentID:      "agent-001",
		InputTokens:  100,
		OutputTokens: 50,
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, mockStore.SaveUsage(ctx, usage))

	// Link to message
	require.NoError(t, mockStore.LinkUsageToMessage(ctx, "req-001", "msg-001"))

	// Get thread usage
	usages, err := mockStore.GetThreadUsage(ctx, "thread-001")
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, "msg-001", usages[0].MessageID)

	// Get stats
	stats, err := mockStore.GetUsageStats(ctx, UsageFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(100), stats.TotalInput)
	assert.Equal(t, int64(50), stats.TotalOutput)
	assert.Equal(t, int64(1), stats.RequestCount)
}
