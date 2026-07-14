// ABOUTME: In-process rate limiter for failed admin login attempts.
// ABOUTME: Sliding one-minute window per username; mirrors internal/client's registration limiter.

package webadmin

import (
	"sync"
	"time"
)

const (
	maxLoginFailuresPerWindow = 5
	loginRateWindow           = time.Minute
	loginCleanupInterval      = 5 * time.Minute
)

// loginRateLimiter tracks recent failed login attempts per username.
// State is in-process only and resets on restart — acceptable for a
// single-instance gateway; bcrypt latency still bounds raw throughput.
// Map growth is bounded by the periodic cleanup: entries whose failures
// have all expired are removed every loginCleanupInterval.
type loginRateLimiter struct {
	mu          sync.Mutex
	failures    map[string][]time.Time
	lastCleanup time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		failures:    make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}
}

// loginLimiter gates handleLogin. Package-level like client.regRateLimiter;
// tests swap it with save/restore.
var loginLimiter = newLoginRateLimiter()

// tooMany reports whether username has exhausted its failure budget
// within the sliding window.
func (l *loginRateLimiter) tooMany(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-loginRateWindow)

	if now.Sub(l.lastCleanup) > loginCleanupInterval {
		l.cleanup(cutoff)
		l.lastCleanup = now
	}

	return len(l.validFailures(username, cutoff)) >= maxLoginFailuresPerWindow
}

// recordFailure notes a failed login attempt for username.
func (l *loginRateLimiter) recordFailure(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-loginRateWindow)
	l.failures[username] = append(l.validFailures(username, cutoff), time.Now())
}

// validFailures returns username's failures newer than cutoff and stores
// the pruned slice back (dropping the key entirely when empty).
// Callers must hold l.mu.
func (l *loginRateLimiter) validFailures(username string, cutoff time.Time) []time.Time {
	timestamps := l.failures[username]
	valid := make([]time.Time, 0, len(timestamps))
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	if len(valid) == 0 {
		delete(l.failures, username)
	} else {
		l.failures[username] = valid
	}
	return valid
}

// cleanup removes usernames whose failures have all expired.
// Callers must hold l.mu.
func (l *loginRateLimiter) cleanup(cutoff time.Time) {
	for username, timestamps := range l.failures {
		live := false
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				live = true
				break
			}
		}
		if !live {
			delete(l.failures, username)
		}
	}
}
