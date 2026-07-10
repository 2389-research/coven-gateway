// ABOUTME: Tests for CSRF token validation behavior in the admin UI.
// ABOUTME: Pins accept/reject semantics across form-field and header token sources.

package webadmin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// csrfRequest builds a POST with the given cookie and form token values.
// Empty cookieVal means no cookie is attached at all.
func csrfRequest(cookieVal, formVal string) *http.Request {
	form := url.Values{}
	if formVal != "" {
		form.Set("csrf_token", formVal)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookieVal != "" {
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: cookieVal})
	}
	return req
}

func TestValidateCSRF_MatchingFormToken(t *testing.T) {
	admin := newTestAdmin(nil)
	if !admin.validateCSRF(csrfRequest("tok-abc123", "tok-abc123")) {
		t.Error("matching cookie and form token should validate")
	}
}

func TestValidateCSRF_MatchingHeaderToken(t *testing.T) {
	admin := newTestAdmin(nil)
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "tok-abc123"})
	req.Header.Set("X-CSRF-Token", "tok-abc123")
	if !admin.validateCSRF(req) {
		t.Error("matching cookie and X-CSRF-Token header should validate")
	}
}

func TestValidateCSRF_MismatchedToken(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("tok-abc123", "tok-abc124")) {
		t.Error("mismatched token must not validate")
	}
}

func TestValidateCSRF_DifferentLengthToken(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("tok-abc123", "tok-abc")) {
		t.Error("different-length token must not validate")
	}
}

func TestValidateCSRF_MissingCookie(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("", "tok-abc123")) {
		t.Error("request without CSRF cookie must not validate")
	}
}

func TestValidateCSRF_EmptyFormAndHeader(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("tok-abc123", "")) {
		t.Error("request without any submitted token must not validate")
	}
}
