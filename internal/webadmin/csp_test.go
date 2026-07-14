// ABOUTME: Tests for the security headers middleware (CSP policy selection and header values).
// ABOUTME: Covers prod/dev CSP, always-set headers, and HSTS gating on TLS.
package webadmin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/2389/coven-gateway/internal/assets"
)

func TestCSPMiddleware_SetsHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeadersMiddleware(false, inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("expected Content-Security-Policy header, got empty")
	}

	// Verify key directives are present.
	for _, directive := range []string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"form-action 'self'",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing directive %q; got: %s", directive, csp)
		}
	}
}

func TestCSPMiddleware_PreservesInnerHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test")
		w.WriteHeader(http.StatusTeapot)
	})

	handler := SecurityHeadersMiddleware(false, inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, rec.Code)
	}
	if rec.Header().Get("X-Custom") != "test" {
		t.Errorf("inner handler header not preserved")
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("CSP header missing on non-200 response")
	}
}

func TestCSPMiddleware_DevMode(t *testing.T) {
	// Save and restore manifest state.
	orig := assets.Manifest
	defer func() { assets.Manifest = orig }()

	assets.Manifest = nil // dev mode

	handler := SecurityHeadersMiddleware(false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "http://localhost:5173") {
		t.Errorf("dev CSP should allow localhost:5173; got: %s", csp)
	}
	if !strings.Contains(csp, "ws://localhost:5173") {
		t.Errorf("dev CSP should allow ws://localhost:5173 for HMR; got: %s", csp)
	}
	if !strings.Contains(csp, "'unsafe-eval'") {
		t.Errorf("dev CSP should allow 'unsafe-eval' for Vite error overlay; got: %s", csp)
	}
}

func TestCSPMiddleware_ProdMode(t *testing.T) {
	// Save and restore manifest state.
	orig := assets.Manifest
	defer func() { assets.Manifest = orig }()

	assets.Manifest = map[string]assets.ManifestEntry{
		"test": {File: "test.js", IsEntry: true},
	}

	handler := SecurityHeadersMiddleware(false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "localhost") {
		t.Errorf("prod CSP should not reference localhost; got: %s", csp)
	}
}

func TestSecurityHeaders_SetOnEveryResponse(t *testing.T) {
	handler := SecurityHeadersMiddleware(false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy missing")
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS must not be set on plain-HTTP connections, got %q", got)
	}
}

func TestSecurityHeaders_HSTSOnlyOverTLS(t *testing.T) {
	handler := SecurityHeadersMiddleware(false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	const want = "max-age=63072000; includeSubDomains"
	if got := rec.Header().Get("Strict-Transport-Security"); got != want {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, want)
	}
}

func TestSecurityHeaders_ForwardedProtoTrusted(t *testing.T) {
	cases := []struct {
		name     string
		trust    bool
		xfp      string
		wantHSTS bool
	}{
		{"trust off, header present", false, "https", false},
		{"trust on, header https", true, "https", true},
		{"trust on, header HTTPS uppercase", true, "HTTPS", true},
		{"trust on, header http", true, "http", false},
		{"trust on, no header", true, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := SecurityHeadersMiddleware(tc.trust, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.xfp != "" {
				req.Header.Set("X-Forwarded-Proto", tc.xfp)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			got := rec.Header().Get("Strict-Transport-Security") != ""
			if got != tc.wantHSTS {
				t.Errorf("HSTS emitted=%v, want %v", got, tc.wantHSTS)
			}
		})
	}
}
