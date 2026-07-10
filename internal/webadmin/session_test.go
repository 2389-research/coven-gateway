// ABOUTME: Tests for admin session and CSRF cookie attributes.
// ABOUTME: Asserts Secure/SameSite/HttpOnly on set and clear paths, over HTTP and TLS.

package webadmin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// findCookie returns the last Set-Cookie entry with the given name, or nil.
func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == name {
			found = c
		}
	}
	if found == nil {
		t.Fatalf("no Set-Cookie for %q", name)
	}
	return found
}

func TestHandleLogout_ClearCookieAttributes_PlainHTTP(t *testing.T) {
	admin := newTestAdmin()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()

	admin.handleLogout(rec, req)

	cookies := rec.Result().Cookies()

	session := findCookie(t, cookies, SessionCookieName)
	if session.MaxAge >= 0 {
		t.Errorf("session clear cookie MaxAge = %d, want negative", session.MaxAge)
	}
	if !session.HttpOnly {
		t.Error("session clear cookie must be HttpOnly")
	}
	if session.Secure {
		t.Error("session clear cookie must not be Secure over plain HTTP (must mirror set-time value)")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Errorf("session clear cookie SameSite = %v, want Lax (mirrors createSession)", session.SameSite)
	}

	csrf := findCookie(t, cookies, CSRFCookieName)
	if csrf.MaxAge >= 0 {
		t.Errorf("CSRF clear cookie MaxAge = %d, want negative", csrf.MaxAge)
	}
	if !csrf.HttpOnly {
		t.Error("CSRF clear cookie must be HttpOnly")
	}
	if csrf.Secure {
		t.Error("CSRF clear cookie must not be Secure over plain HTTP (must mirror set-time value)")
	}
	if csrf.SameSite != http.SameSiteStrictMode {
		t.Errorf("CSRF clear cookie SameSite = %v, want Strict (mirrors ensureCSRFToken)", csrf.SameSite)
	}
}

func TestHandleLogout_ClearCookieAttributes_TLS(t *testing.T) {
	admin := newTestAdmin()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()

	admin.handleLogout(rec, req)

	cookies := rec.Result().Cookies()
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		c := findCookie(t, cookies, name)
		if !c.Secure {
			t.Errorf("%s clear cookie must be Secure over TLS (must mirror set-time value)", name)
		}
	}
}
