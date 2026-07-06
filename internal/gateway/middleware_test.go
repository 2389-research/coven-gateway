// ABOUTME: Tests for gateway HTTP middleware.
// ABOUTME: Verifies request body size limits are enforced.

package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBytesMiddleware_RejectsOversizedBody(t *testing.T) {
	var readErr error
	h := maxBytesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	oversized := strings.NewReader(strings.Repeat("a", int(MaxAPIBodySize)+1))
	req := httptest.NewRequest(http.MethodPost, "/api/send", oversized)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
	if readErr == nil {
		t.Fatal("expected a read error from the oversized body, got nil")
	}
}

func TestMaxBytesMiddleware_AllowsNormalBody(t *testing.T) {
	h := maxBytesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/send",
		strings.NewReader(`{"sender":"me","content":"hi"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
