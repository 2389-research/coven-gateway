// ABOUTME: Thread-safe TTL cache for deduplicating messages.
// ABOUTME: Used by bridges and clients to prevent duplicate message processing.

package dedupe

import (
	"sync"
	"time"
)

// Cache provides a thread-safe, TTL-based, size-limited cache for tracking
// seen message keys. It is used to prevent duplicate processing of messages.
type Cache struct {
	mu      sync.RWMutex
	seen    map[string]time.Time
	ttl     time.Duration
	maxSize int
	done    chan struct{}
	closed  bool
}

// New creates a new dedupe cache with the specified TTL and maximum size.
// A background goroutine periodically cleans up expired entries.
func New(ttl time.Duration, maxSize int) *Cache {
	c := &Cache{
		seen:    make(map[string]time.Time),
		ttl:     ttl,
		maxSize: maxSize,
		done:    make(chan struct{}),
	}
	go c.cleanup()
	return c
}

// Check returns true if the key has been seen and is not expired.
func (c *Cache) Check(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t, ok := c.seen[key]
	if !ok {
		return false
	}
	return time.Since(t) < c.ttl
}

// Mark records that a key has been seen. If the cache is at capacity,
// the oldest entry is evicted to make room.
func (c *Cache) Mark(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest if at capacity (and key is not already present)
	if _, exists := c.seen[key]; !exists && len(c.seen) >= c.maxSize {
		c.evictOldest()
	}

	c.seen[key] = time.Now()
}

// evictOldest removes the oldest entry from the cache.
// Must be called with mu held.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, t := range c.seen {
		if oldestKey == "" || t.Before(oldestTime) {
			oldestKey = k
			oldestTime = t
		}
	}

	if oldestKey != "" {
		delete(c.seen, oldestKey)
	}
}

// cleanup runs in a background goroutine, periodically removing expired entries.
func (c *Cache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.runCleanup()
		case <-c.done:
			return
		}
	}
}

// runCleanup removes all expired entries from the cache.
func (c *Cache) runCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, t := range c.seen {
		if now.Sub(t) > c.ttl {
			delete(c.seen, k)
		}
	}
}

// Close stops the background cleanup goroutine. It is safe to call multiple times.
func (c *Cache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		close(c.done)
		c.closed = true
	}
}
