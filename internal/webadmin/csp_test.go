package webadmin

import (
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

	handler := CSPMiddleware(inner)
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

	handler := CSPMiddleware(inner)
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

	handler := CSPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := CSPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
