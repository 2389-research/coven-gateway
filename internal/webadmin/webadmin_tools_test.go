// ABOUTME: Tests for the webadmin tools page handler (handleToolsPage) and env key validation.
// ABOUTME: Verifies nil-safety, correct HTTP responses, and env key format rules.

package webadmin

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
)

// newTestAdmin creates a minimal Admin instance for handler testing.
func newTestAdmin(registry *packs.Registry) *Admin {
	return &Admin{
		registry: registry,
		logger:   slog.Default(),
	}
}

// requestWithUser attaches a test AdminUser to the request context,
// bypassing the requireAuth middleware for direct handler testing.
func requestWithUser(r *http.Request) *http.Request {
	user := &store.AdminUser{
		ID:          "test-user",
		Username:    "testadmin",
		DisplayName: "Test Admin",
	}
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return r.WithContext(ctx)
}

// --- handleToolsPage tests ---

func TestHandleToolsPage_RendersToolsPage(t *testing.T) {
	admin := newTestAdmin(nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/tools", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()

	admin.handleToolsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Tools") {
		t.Fatalf("expected page to contain 'Tools', got %q", body)
	}
}

// Tests for isValidEnvKey

func TestIsValidEnvKey_ValidKeys(t *testing.T) {
	validKeys := []string{
		"API_KEY",
		"ANTHROPIC_API_KEY",
		"_PRIVATE",
		"a",
		"A",
		"_",
		"MY_VAR_123",
		"lowercase",
		"MixedCase",
	}
	for _, key := range validKeys {
		if !isValidEnvKey(key) {
			t.Errorf("expected %q to be valid", key)
		}
	}
}

func TestIsValidEnvKey_InvalidKeys(t *testing.T) {
	invalidKeys := []string{
		"",                    // empty
		"123_STARTS_WITH_NUM", // starts with number
		"has-dash",            // contains dash
		"has.dot",             // contains dot
		"has space",           // contains space
		"has\ttab",            // contains tab
		"has\nnewline",        // contains newline
		"path/to/something",   // contains slash
		"$VAR",                // starts with $
	}
	for _, key := range invalidKeys {
		if isValidEnvKey(key) {
			t.Errorf("expected %q to be invalid", key)
		}
	}
}

func TestIsValidEnvKey_MaxLength(t *testing.T) {
	// 256 chars should be valid
	longValid := strings.Repeat("A", 256)
	if !isValidEnvKey(longValid) {
		t.Error("expected 256-char key to be valid")
	}

	// 257 chars should be invalid
	tooLong := strings.Repeat("A", 257)
	if isValidEnvKey(tooLong) {
		t.Error("expected 257-char key to be invalid")
	}
}
