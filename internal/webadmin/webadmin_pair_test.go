// ABOUTME: Tests for QR pairing: token minting (admin) and device enrollment.
// ABOUTME: Uses a real SQLite store; asserts contract responses and audit writes.

package webadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
)

// newPairTestAdmin builds an Admin backed by a real store with principal
// support, returning the store for direct assertions.
func newPairTestAdmin(t *testing.T, cfg Config) (*Admin, *store.SQLiteStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	jwtVerifier, err := auth.NewJWTVerifier([]byte("test-secret-that-is-32-bytes-lon"))
	if err != nil {
		t.Fatalf("failed to create JWT verifier: %v", err)
	}
	a := NewWithConfig(NewConfig{
		Store:          s,
		Config:         cfg,
		PrincipalStore: s,
		TokenGenerator: jwtVerifier,
	})
	return a, s
}

// mintPairToken performs an authenticated, CSRF-valid mint request.
func mintPairToken(t *testing.T, a *Admin, user *store.AdminUser) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/link/pair-token", nil)
	csrfVal := "test-csrf-token-value"
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleMintPairToken(rec, req)
	return rec
}

type mintResponse struct {
	URL       string `json:"url"`
	QR        string `json:"qr"`
	ExpiresAt string `json:"expires_at"`
}

func decodeMintResponse(t *testing.T, rec *httptest.ResponseRecorder) mintResponse {
	t.Helper()
	var resp mintResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode mint response: %v (body: %s)", err, rec.Body.String())
	}
	return resp
}

func TestHandleMintPairToken_RequiresCSRF(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	user := createAdminUserWithPassword(t, a, "mintcsrf", "password123")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/link/pair-token", nil)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleMintPairToken(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 without CSRF, got %d", rec.Code)
	}
}

func TestHandleMintPairToken_RequiresUser(t *testing.T) {
	// The route sits behind requireAuth; this covers the handler's own guard.
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/link/pair-token", nil)
	csrfVal := "test-csrf-token-value"
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	rec := httptest.NewRecorder()
	a.handleMintPairToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a user in context, got %d", rec.Code)
	}
}

func TestHandleMintPairToken_HappyPath(t *testing.T) {
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net", GRPCPort: 50051})
	user := createAdminUserWithPassword(t, a, "mintadmin", "password123")

	rec := mintPairToken(t, a, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeMintResponse(t, rec)

	u, err := url.Parse(resp.URL)
	if err != nil || u.Scheme != "coven" || u.Host != "pair" {
		t.Fatalf("expected coven://pair URL, got %q (err: %v)", resp.URL, err)
	}
	q := u.Query()
	if q.Get("v") != "1" {
		t.Errorf("expected v=1, got %q", q.Get("v"))
	}
	if q.Get("host") != "gw.example.ts.net" {
		t.Errorf("expected host=gw.example.ts.net, got %q", q.Get("host"))
	}
	if q.Get("port") != "" {
		t.Errorf("expected default port 50051 omitted from payload, got %q", q.Get("port"))
	}
	token := q.Get("token")
	if token == "" {
		t.Fatal("expected token in payload URL")
	}

	if !strings.HasPrefix(resp.QR, "data:image/png;base64,") {
		t.Errorf("expected PNG data URI, got %.40q", resp.QR)
	}

	expires, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at not RFC3339: %v", err)
	}
	until := time.Until(expires)
	if until < 4*time.Minute || until > 6*time.Minute {
		t.Errorf("expected ~5 minute expiry, got %s", until)
	}

	pt, err := s.GetPairTokenByHash(context.Background(), hashPairToken(token))
	if err != nil {
		t.Fatalf("minted token hash not found in store: %v", err)
	}
	if pt.CreatedBy != user.ID {
		t.Errorf("expected created_by %q, got %q", user.ID, pt.CreatedBy)
	}

	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	if err != nil {
		t.Fatalf("listing audit log: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == store.AuditMintPairToken && e.TargetID == pt.ID && e.ActorPrincipalID == user.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected a mint_pair_token audit entry for the minted token")
	}
}

func TestHandleMintPairToken_IncludesNonDefaultPort(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net", GRPCPort: 9443})
	user := createAdminUserWithPassword(t, a, "portadmin", "password123")

	rec := mintPairToken(t, a, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeMintResponse(t, rec)

	u, err := url.Parse(resp.URL)
	if err != nil {
		t.Fatalf("parsing payload URL: %v", err)
	}
	if got := u.Query().Get("port"); got != "9443" {
		t.Errorf("expected port=9443 in payload, got %q", got)
	}
}

func TestHandleMintPairToken_RefusesIPLiteralHost(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://100.64.1.5"})
	user := createAdminUserWithPassword(t, a, "ipadmin", "password123")

	rec := mintPairToken(t, a, user)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for IP-literal base URL, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "tailnet DNS name") {
		t.Errorf("expected remediation message, got %q", rec.Body.String())
	}
}
