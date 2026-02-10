// ABOUTME: Thread-safe TTL cache for deduplicating messages.
// ABOUTME: Used by bridges and clients to prevent duplicate message processing.

package dedupe

import (
	"container/list"
	"sync"
	"time"
)

// cacheEntry stores the timestamp and list element for a cached key.
type cacheEntry struct {
	timestamp time.Time
	element   *list.Element
}

// Cache provides a thread-safe, TTL-based, size-limited cache for tracking
// seen message keys. It is used to prevent duplicate processing of messages.
// Uses a doubly-linked list to maintain insertion order for O(1) eviction.
type Cache struct {
	mu      sync.RWMutex
	seen    map[string]*cacheEntry
	order   *list.List // List of keys in insertion order (oldest at front)
	ttl     time.Duration
	maxSize int
	done    chan struct{}
	closed  bool
}

// New creates a new dedupe cache with the specified TTL and maximum size.
// A background goroutine periodically cleans up expired entries.
func New(ttl time.Duration, maxSize int) *Cache {
	c := &Cache{
		seen:    make(map[string]*cacheEntry),
		order:   list.New(),
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

	entry, ok := c.seen[key]
	if !ok {
		return false
	}
	return time.Since(entry.timestamp) < c.ttl
}

// CheckAndMark atomically checks if a key has been seen and marks it if not.
// Returns true if the key was already seen (duplicate), false if it's new and now marked.
// This prevents TOCTOU race conditions that could occur with separate Check/Mark calls.
func (c *Cache) CheckAndMark(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.seen[key]
	if ok && time.Since(entry.timestamp) < c.ttl {
		return true // Already seen, reject
	}

	// Not seen (or expired), mark it
	c.markLocked(key)
	return false
}

// Mark records that a key has been seen. If the cache is at capacity,
// the oldest entry is evicted to make room.
func (c *Cache) Mark(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.markLocked(key)
}

// markLocked is the internal mark implementation. Must be called with mu held.
func (c *Cache) markLocked(key string) {
	now := time.Now()

	// If key already exists, update timestamp and move to back
	if entry, exists := c.seen[key]; exists {
		entry.timestamp = now
		c.order.MoveToBack(entry.element)
		return
	}

	// Evict oldest if at capacity
	if len(c.seen) >= c.maxSize {
		c.evictOldest()
	}

	// Add new entry
	elem := c.order.PushBack(key)
	c.seen[key] = &cacheEntry{
		timestamp: now,
		element:   elem,
	}
}

// evictOldest removes the oldest entry from the cache.
// Must be called with mu held. O(1) operation using linked list.
func (c *Cache) evictOldest() {
	front := c.order.Front()
	if front == nil {
		return
	}

	key, _ := front.Value.(string)
	c.order.Remove(front)
	delete(c.seen, key)
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
	for key, entry := range c.seen {
		if now.Sub(entry.timestamp) > c.ttl {
			c.order.Remove(entry.element)
			delete(c.seen, key)
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
