// Package dedupe provides message deduplication using a time-based cache.
//
// # Overview
//
// The dedupe package prevents duplicate message processing when the same
// message arrives through multiple paths (e.g., bridge connections, retries).
//
// # How It Works
//
// Messages are identified by a caller-provided key (e.g., idempotency key,
// transport message ID). The cache tracks seen keys with expiration times:
//
//  1. Check if key exists in cache
//  2. If exists and not expired, skip (duplicate)
//  3. If not exists or expired, process and add to cache
//
// # Usage
//
// Create a cache with TTL:
//
//	cache := dedupe.NewCache(5 * time.Minute)
//
// Check for duplicates:
//
//	if cache.IsDuplicate(messageKey) {
//	    // Skip duplicate message
//	    return
//	}
//
// The cache automatically removes expired entries during operations.
//
// # Thread Safety
//
// The cache is safe for concurrent use. Multiple goroutines can check
// and add entries simultaneously.
//
// # Bridge Deduplication
//
// Primary use case: Matrix/Slack bridges that may deliver the same
// message multiple times due to:
//
//   - Network retries
//   - Multiple bridge instances
//   - Federation delays
//
// # Configuration
//
// The TTL should be long enough to catch retries but short enough
// to allow intentional re-sends:
//
//   - Recommended: 5 minutes
//   - Minimum: 1 minute
//   - Maximum: 1 hour
package dedupe
