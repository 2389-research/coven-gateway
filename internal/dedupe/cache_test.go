// ABOUTME: Tests for the dedupe cache used to prevent duplicate message processing.
// ABOUTME: Validates TTL expiration, size limits, eviction, cleanup, and concurrency safety.

package dedupe

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_Check_NotSeen(t *testing.T) {
	cache := New(5*time.Minute, 100)
	defer cache.Close()

	// Key that was never marked should return false
	assert.False(t, cache.Check("never-seen-key"))
}

func TestCache_Check_Seen(t *testing.T) {
	cache := New(5*time.Minute, 100)
	defer cache.Close()

	// Mark a key
	cache.Mark("my-key")

	// Check should return true
	assert.True(t, cache.Check("my-key"))
}

func TestCache_Check_Expired(t *testing.T) {
	// Use a very short TTL for testing
	cache := New(10*time.Millisecond, 100)
	defer cache.Close()

	// Mark a key
	cache.Mark("expiring-key")

	// Should be seen initially
	assert.True(t, cache.Check("expiring-key"))

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Should no longer be seen after TTL
	assert.False(t, cache.Check("expiring-key"))
}

func TestCache_Mark(t *testing.T) {
	cache := New(5*time.Minute, 100)
	defer cache.Close()

	// Mark multiple keys
	cache.Mark("key-1")
	cache.Mark("key-2")
	cache.Mark("key-3")

	// All should be present
	assert.True(t, cache.Check("key-1"))
	assert.True(t, cache.Check("key-2"))
	assert.True(t, cache.Check("key-3"))

	// Unknown key should not be present
	assert.False(t, cache.Check("key-4"))
}

func TestCache_Mark_UpdatesTimestamp(t *testing.T) {
	// Use a short TTL
	cache := New(50*time.Millisecond, 100)
	defer cache.Close()

	// Mark a key
	cache.Mark("refresh-key")

	// Wait partway through TTL
	time.Sleep(30 * time.Millisecond)

	// Re-mark to refresh
	cache.Mark("refresh-key")

	// Wait another 30ms (would be past original TTL)
	time.Sleep(30 * time.Millisecond)

	// Should still be present because we refreshed
	assert.True(t, cache.Check("refresh-key"))
}

func TestCache_Eviction(t *testing.T) {
	// Small cache for testing eviction
	cache := New(5*time.Minute, 3)
	defer cache.Close()

	// Fill the cache
	cache.Mark("key-1")
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	cache.Mark("key-2")
	time.Sleep(1 * time.Millisecond)
	cache.Mark("key-3")

	// All three should be present
	assert.True(t, cache.Check("key-1"))
	assert.True(t, cache.Check("key-2"))
	assert.True(t, cache.Check("key-3"))

	// Add a fourth key - should evict the oldest (key-1)
	time.Sleep(1 * time.Millisecond)
	cache.Mark("key-4")

	// key-1 should be evicted (oldest)
	assert.False(t, cache.Check("key-1"), "oldest key should be evicted")

	// Other keys should remain
	assert.True(t, cache.Check("key-2"))
	assert.True(t, cache.Check("key-3"))
	assert.True(t, cache.Check("key-4"))
}

func TestCache_Cleanup(t *testing.T) {
	// Create cache with very short TTL and cleanup interval
	// Note: cleanup runs every minute by default, so we test that expired entries
	// are correctly identified, not the actual cleanup goroutine timing
	cache := New(10*time.Millisecond, 100)
	defer cache.Close()

	// Mark several keys
	cache.Mark("cleanup-1")
	cache.Mark("cleanup-2")
	cache.Mark("cleanup-3")

	// All should be present
	assert.True(t, cache.Check("cleanup-1"))
	assert.True(t, cache.Check("cleanup-2"))
	assert.True(t, cache.Check("cleanup-3"))

	// Wait for entries to expire
	time.Sleep(20 * time.Millisecond)

	// All should be expired now (Check returns false for expired)
	assert.False(t, cache.Check("cleanup-1"))
	assert.False(t, cache.Check("cleanup-2"))
	assert.False(t, cache.Check("cleanup-3"))

	// Test that cleanup actually removes entries (check internal state)
	// We'll trigger cleanup manually by calling the internal method
	cache.runCleanup()

	// Verify the map is empty after cleanup
	cache.mu.RLock()
	mapLen := len(cache.seen)
	cache.mu.RUnlock()
	assert.Equal(t, 0, mapLen, "cleanup should remove expired entries from map")
}

func TestCache_Concurrent(t *testing.T) {
	cache := New(5*time.Minute, 1000)
	defer cache.Close()

	const numGoroutines = 100
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent marks and checks
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := "key-" + string(rune('A'+id%26)) + "-" + string(rune('0'+j%10))
				cache.Mark(key)
				cache.Check(key)
			}
		}(i)
	}

	wg.Wait()

	// No panics or race conditions - test passes if we get here
	// Also verify cache is still functional
	cache.Mark("final-key")
	assert.True(t, cache.Check("final-key"))
}

func TestCache_Close(t *testing.T) {
	cache := New(5*time.Minute, 100)

	cache.Mark("before-close")
	assert.True(t, cache.Check("before-close"))

	// Close should not panic and should stop the cleanup goroutine
	cache.Close()

	// Multiple closes should not panic
	cache.Close()
}

func TestCache_ConfiguredDefaults(t *testing.T) {
	// Test with the expected production config values
	cache := New(5*time.Minute, 100_000)
	defer cache.Close()

	// Basic operations should work
	cache.Mark("prod-key")
	assert.True(t, cache.Check("prod-key"))
}

func TestCache_CheckAndMark_NewKey(t *testing.T) {
	cache := New(5*time.Minute, 100)
	defer cache.Close()

	// First call for a new key should return false (not seen) and mark it
	result := cache.CheckAndMark("new-key")
	assert.False(t, result, "first CheckAndMark should return false for new key")

	// Key should now be marked
	assert.True(t, cache.Check("new-key"), "key should be marked after CheckAndMark")
}

func TestCache_CheckAndMark_SeenKey(t *testing.T) {
	cache := New(5*time.Minute, 100)
	defer cache.Close()

	// Mark the key first
	cache.Mark("existing-key")

	// CheckAndMark should return true (already seen)
	result := cache.CheckAndMark("existing-key")
	assert.True(t, result, "CheckAndMark should return true for already-seen key")
}

func TestCache_CheckAndMark_Expired(t *testing.T) {
	// Use a very short TTL for testing
	cache := New(10*time.Millisecond, 100)
	defer cache.Close()

	// Mark via CheckAndMark
	result := cache.CheckAndMark("expiring-key")
	assert.False(t, result, "first CheckAndMark should return false")

	// Should be seen immediately
	assert.True(t, cache.CheckAndMark("expiring-key"), "should be seen before expiry")

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Should not be seen after expiry
	assert.False(t, cache.CheckAndMark("expiring-key"), "should not be seen after expiry")
}

func TestCache_CheckAndMark_Atomic(t *testing.T) {
	cache := New(5*time.Minute, 100)
	defer cache.Close()

	const numGoroutines = 100

	// Count how many goroutines successfully "won" (got false)
	var successCount int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// All goroutines try to CheckAndMark the same key simultaneously
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Only one goroutine should get false (first one)
			if !cache.CheckAndMark("contested-key") {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Exactly one goroutine should have succeeded
	assert.Equal(t, int32(1), successCount,
		"exactly one goroutine should win the race for CheckAndMark")
}

func TestCache_EvictionOrder(t *testing.T) {
	// Test that eviction properly removes oldest entry (O(1) using linked list)
	cache := New(5*time.Minute, 3)
	defer cache.Close()

	// Add keys in order
	cache.Mark("first")
	time.Sleep(1 * time.Millisecond)
	cache.Mark("second")
	time.Sleep(1 * time.Millisecond)
	cache.Mark("third")

	// All should be present
	assert.True(t, cache.Check("first"))
	assert.True(t, cache.Check("second"))
	assert.True(t, cache.Check("third"))

	// Add fourth - should evict "first" (oldest)
	cache.Mark("fourth")

	assert.False(t, cache.Check("first"), "first should be evicted")
	assert.True(t, cache.Check("second"))
	assert.True(t, cache.Check("third"))
	assert.True(t, cache.Check("fourth"))

	// Add fifth - should evict "second"
	cache.Mark("fifth")

	assert.False(t, cache.Check("second"), "second should be evicted")
	assert.True(t, cache.Check("third"))
	assert.True(t, cache.Check("fourth"))
	assert.True(t, cache.Check("fifth"))
}
