// ABOUTME: In-process rate limiter for failed /api/link/pair attempts.
// ABOUTME: Sliding one-minute window per remote IP; mirrors login_ratelimit.go.

package webadmin

import (
	"sync"
	"time"
)

const (
	maxPairFailuresPerWindow = 5
	pairRateWindow           = time.Minute
	pairCleanupInterval      = 5 * time.Minute
)

// pairRateLimiter tracks recent failed pair attempts per remote IP.
// Defense-in-depth only: the real protection is 256-bit single-use tokens
// with a 5-minute TTL. Behind tailscale funnel the observed remote address
// may be a shared ingress address, coarsening the limit — acceptable.
// State is in-process and resets on restart, like loginRateLimiter.
type pairRateLimiter struct {
	mu          sync.Mutex
	failures    map[string][]time.Time
	lastCleanup time.Time
}

func newPairRateLimiter() *pairRateLimiter {
	return &pairRateLimiter{
		failures:    make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}
}

// pairLimiter gates handleLinkPair. Package-level like loginLimiter;
// tests swap it with save/restore.
var pairLimiter = newPairRateLimiter()

// tooMany reports whether ip has exhausted its failure budget within the
// sliding window.
func (l *pairRateLimiter) tooMany(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-pairRateWindow)

	if now.Sub(l.lastCleanup) > pairCleanupInterval {
		l.cleanup(cutoff)
		l.lastCleanup = now
	}

	return len(l.validFailures(ip, cutoff)) >= maxPairFailuresPerWindow
}

// recordFailure notes a failed pair attempt for ip.
func (l *pairRateLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-pairRateWindow)
	l.failures[ip] = append(l.validFailures(ip, cutoff), time.Now())
}

// validFailures returns ip's failures newer than cutoff and stores the
// pruned slice back (dropping the key entirely when empty).
// Callers must hold l.mu.
func (l *pairRateLimiter) validFailures(ip string, cutoff time.Time) []time.Time {
	timestamps := l.failures[ip]
	valid := make([]time.Time, 0, len(timestamps))
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	if len(valid) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = valid
	}
	return valid
}

// cleanup removes IPs whose failures have all expired.
// Callers must hold l.mu.
func (l *pairRateLimiter) cleanup(cutoff time.Time) {
	for ip, timestamps := range l.failures {
		live := false
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				live = true
				break
			}
		}
		if !live {
			delete(l.failures, ip)
		}
	}
}
