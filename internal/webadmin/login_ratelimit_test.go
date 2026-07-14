// ABOUTME: Tests for the failed-login rate limiter.
// ABOUTME: Covers window budget, per-username isolation, expiry, and handleLogin gating.

package webadmin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginRateLimiter_AllowsUnderBudget(t *testing.T) {
	l := newLoginRateLimiter()
	for range maxLoginFailuresPerWindow - 1 {
		l.recordFailure("admin")
	}
	if l.tooMany("admin") {
		t.Errorf("%d failures should not trip a budget of %d", maxLoginFailuresPerWindow-1, maxLoginFailuresPerWindow)
	}
}

func TestLoginRateLimiter_BlocksAtBudget(t *testing.T) {
	l := newLoginRateLimiter()
	for range maxLoginFailuresPerWindow {
		l.recordFailure("admin")
	}
	if !l.tooMany("admin") {
		t.Errorf("%d failures must trip the limiter", maxLoginFailuresPerWindow)
	}
}

func TestLoginRateLimiter_PerUsernameIsolation(t *testing.T) {
	l := newLoginRateLimiter()
	for range maxLoginFailuresPerWindow {
		l.recordFailure("alice")
	}
	if l.tooMany("bob") {
		t.Error("failures for alice must not block bob")
	}
}

func TestLoginRateLimiter_ExpiredFailuresDoNotCount(t *testing.T) {
	l := newLoginRateLimiter()
	stale := time.Now().Add(-2 * loginRateWindow)
	for range maxLoginFailuresPerWindow {
		l.failures["admin"] = append(l.failures["admin"], stale)
	}
	if l.tooMany("admin") {
		t.Error("failures older than the window must not count")
	}
}

func TestLoginRateLimiter_CleanupRemovesExpiredEntries(t *testing.T) {
	l := newLoginRateLimiter()
	// Pre-load stale failures for two users.
	stale := time.Now().Add(-2 * loginRateWindow)
	l.failures["alice"] = []time.Time{stale, stale}
	l.failures["bob"] = []time.Time{stale}
	// Force the cleanup trigger by backdating lastCleanup.
	l.lastCleanup = time.Now().Add(-2 * loginCleanupInterval)

	// tooMany triggers cleanup when lastCleanup is overdue.
	if l.tooMany("alice") {
		t.Error("stale failures must not block alice")
	}
	l.mu.Lock()
	_, alicePresent := l.failures["alice"]
	_, bobPresent := l.failures["bob"]
	l.mu.Unlock()
	if alicePresent || bobPresent {
		t.Error("cleanup must remove users with all-expired failures")
	}
}

func TestHandleLogin_RateLimited(t *testing.T) {
	// Swap the package-level limiter (save/restore, same pattern csp_test.go
	// uses for assets.Manifest).
	orig := loginLimiter
	defer func() { loginLimiter = orig }()
	loginLimiter = newLoginRateLimiter()
	for range maxLoginFailuresPerWindow {
		loginLimiter.recordFailure("admin")
	}

	// newTestAdmin() leaves store nil: if the limiter fails to gate before
	// the store lookup, this test panics — proving the gate runs first is
	// the point.
	admin := newTestAdmin()

	form := url.Values{
		"username":   {"admin"},
		"password":   {"hunter2"},
		"csrf_token": {"tok"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "tok"})
	rec := httptest.NewRecorder()

	admin.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("blocked login should re-render the login page with status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Too many login attempts") {
		t.Error("blocked login should show the rate-limit message")
	}
}
