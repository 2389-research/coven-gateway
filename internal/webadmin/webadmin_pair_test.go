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

	"github.com/google/uuid"

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

// pairRequest performs a POST /api/link/pair with the given JSON body.
func pairRequest(t *testing.T, a *Admin, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/link/pair", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkPair(rec, req)
	return rec
}

// createStoredPairToken mints a token row directly, returning the plaintext.
// A UUID suffix ensures the admin username is unique even when called multiple
// times within the same test (e.g. the reuse test).
func createStoredPairToken(t *testing.T, a *Admin, s *store.SQLiteStore, expiresAt time.Time) string {
	t.Helper()
	unique := strings.ReplaceAll(t.Name(), "/", "-") + "-" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	user := createAdminUserWithPassword(t, a, "tokadmin-"+unique, "password123")
	token, err := generatePairToken()
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}
	if _, err := s.CreatePairToken(context.Background(), hashPairToken(token), user.ID, expiresAt); err != nil {
		t.Fatalf("storing token: %v", err)
	}
	return token
}

func pairBody(token, fingerprint, deviceName string) string {
	b, err := json.Marshal(map[string]string{
		"token":       token,
		"fingerprint": fingerprint,
		"device_name": deviceName,
	})
	if err != nil {
		panic("pairBody marshal: " + err.Error())
	}
	return string(b)
}

func TestHandleLinkPair_HappyPath(t *testing.T) {
	saved := pairLimiter
	pairLimiter = newPairRateLimiter()
	defer func() { pairLimiter = saved }()
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	token := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	fp := validFingerprint()

	rec := pairRequest(t, a, pairBody(token, fp, "Test iPhone"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PrincipalID string `json:"principal_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.PrincipalID == "" {
		t.Fatal("expected principal_id in response")
	}

	// The device is enrolled: approved principal with the presented pubkey.
	p, err := s.GetPrincipalByPubkey(context.Background(), fp)
	if err != nil {
		t.Fatalf("looking up enrolled principal: %v", err)
	}
	if p.ID != resp.PrincipalID {
		t.Errorf("principal mismatch: response %q, store %q", resp.PrincipalID, p.ID)
	}
	if p.Status != store.PrincipalStatusApproved {
		t.Errorf("expected approved principal, got %q", p.Status)
	}
	if p.DisplayName != "Test iPhone" {
		t.Errorf("expected display name from device_name, got %q", p.DisplayName)
	}

	hasRole, err := s.HasRole(context.Background(), store.RoleSubjectPrincipal, resp.PrincipalID, store.RoleMember)
	if err != nil {
		t.Fatalf("checking member role: %v", err)
	}
	if !hasRole {
		t.Error("expected the enrolled principal to have the member role")
	}

	// The token is burned and linked.
	pt, err := s.GetPairTokenByHash(context.Background(), hashPairToken(token))
	if err != nil {
		t.Fatalf("looking up token: %v", err)
	}
	if pt.UsedAt == nil {
		t.Error("expected token consumed")
	}
	if pt.PrincipalID == nil || *pt.PrincipalID != resp.PrincipalID {
		t.Error("expected token linked to the enrolled principal")
	}

	// The enrollment is audited.
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	if err != nil {
		t.Fatalf("listing audit log: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == store.AuditPairEnroll && e.TargetID == resp.PrincipalID {
			found = true
			if reused, ok := e.Detail["reused_principal"].(bool); !ok || reused {
				t.Errorf("expected reused_principal=false detail, got %v", e.Detail["reused_principal"])
			}
		}
	}
	if !found {
		t.Error("expected a pair_enroll audit entry")
	}
}

func TestHandleLinkPair_ReusesPrincipalForKnownFingerprint(t *testing.T) {
	saved := pairLimiter
	pairLimiter = newPairRateLimiter()
	defer func() { pairLimiter = saved }()
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	fp := validFingerprint()

	first := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	rec := pairRequest(t, a, pairBody(first, fp, "Test iPhone"))
	if rec.Code != http.StatusOK {
		t.Fatalf("first pair: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var firstResp struct {
		PrincipalID string `json:"principal_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &firstResp)

	second := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	rec = pairRequest(t, a, pairBody(second, fp, "Test iPhone"))
	if rec.Code != http.StatusOK {
		t.Fatalf("second pair: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var secondResp struct {
		PrincipalID string `json:"principal_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &secondResp)

	if firstResp.PrincipalID != secondResp.PrincipalID {
		t.Errorf("expected the same principal for the same fingerprint, got %q then %q",
			firstResp.PrincipalID, secondResp.PrincipalID)
	}
}

func TestHandleLinkPair_Rejections(t *testing.T) {
	fp := validFingerprint()

	cases := []struct {
		name       string
		body       func(t *testing.T, a *Admin, s *store.SQLiteStore) string
		wantReason string
	}{
		{
			name: "malformed JSON",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return `{not json`
			},
			wantReason: "invalid request: body",
		},
		{
			name: "missing token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("", fp, "Test iPhone")
			},
			wantReason: "invalid request: token",
		},
		{
			name: "bad fingerprint",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("some-token", "too-short", "Test iPhone")
			},
			wantReason: "invalid request: fingerprint",
		},
		{
			name: "empty device name",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("some-token", fp, "")
			},
			wantReason: "invalid request: device_name",
		},
		{
			name: "oversized device name",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("some-token", fp, strings.Repeat("x", 101))
			},
			wantReason: "invalid request: device_name",
		},
		{
			name: "unknown token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("never-minted-token", fp, "Test iPhone")
			},
			wantReason: "invalid token",
		},
		{
			name: "expired token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				// Expired but present: skip eager cleanup by using a token
				// that handleLinkPair's DeleteExpiredPairTokens will remove —
				// removal yields "invalid token"; surviving yields "token
				// expired". Both are 401; assert only the status here.
				return pairBody(createStoredPairToken(t, a, s, time.Now().Add(-time.Minute)), fp, "Test iPhone")
			},
			wantReason: "", // reason depends on eager-cleanup timing; 401 is the contract
		},
		{
			name: "already used token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				token := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
				first := pairRequest(t, a, pairBody(token, fp, "Test iPhone"))
				if first.Code != http.StatusOK {
					t.Fatalf("setup pair failed: %d %s", first.Code, first.Body.String())
				}
				return pairBody(token, fp, "Test iPhone")
			},
			wantReason: "token already used",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saved := pairLimiter
			pairLimiter = newPairRateLimiter()
			defer func() { pairLimiter = saved }()
			a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
			rec := pairRequest(t, a, tc.body(t, a, s))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("401 body must be the JSON error envelope, got %q", rec.Body.String())
			}
			if tc.wantReason != "" && resp.Error != tc.wantReason {
				t.Errorf("expected reason %q, got %q", tc.wantReason, resp.Error)
			}
		})
	}
}

func TestHandleLinkPair_NilPrincipalStore_RecordsFailureAndReturns500(t *testing.T) {
	saved := pairLimiter
	pairLimiter = newPairRateLimiter()
	defer func() { pairLimiter = saved }()

	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	token := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	fp := validFingerprint()

	// Nil out the principalStore after setup so the enrollment hits that branch.
	a.principalStore = nil

	rec := pairRequest(t, a, pairBody(token, fp, "Test iPhone"))

	// Must be 500 with the standard error envelope.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("500 body must be JSON error envelope, got %q", rec.Body.String())
	}
	if resp.Error != "internal error" {
		t.Errorf("expected error %q, got %q", "internal error", resp.Error)
	}

	// The failure must have been recorded: one more failure trips the limiter.
	ip := "192.0.2.1" // default httptest.NewRequest RemoteAddr host
	for i := 1; i < maxPairFailuresPerWindow; i++ {
		pairLimiter.recordFailure(ip)
	}
	if !pairLimiter.tooMany(ip) {
		t.Error("expected failure to be recorded in pairLimiter; limiter not triggered after maxPairFailuresPerWindow total")
	}
}

func TestHandleLinkPair_RateLimited(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})

	saved := pairLimiter
	pairLimiter = newPairRateLimiter()
	defer func() { pairLimiter = saved }()

	for range maxPairFailuresPerWindow {
		pairLimiter.recordFailure("192.0.2.9")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/link/pair", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.9:5555"
	rec := httptest.NewRecorder()
	a.handleLinkPair(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when rate limited, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "too many attempts") {
		t.Errorf("expected 'too many attempts', got %q", rec.Body.String())
	}
}

func TestHandleLinkApprove_WritesAuditEntry(t *testing.T) {
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	user := createAdminUserWithPassword(t, a, "approveaudit", "password123")
	linkID := createPendingLink(t, a, validFingerprint(), "Audited Device")

	req := httptest.NewRequest(http.MethodPost, "/admin/link/"+linkID+"/approve", nil)
	csrfVal := "test-csrf-token-value"
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req.SetPathValue("id", linkID)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	lc, err := s.GetLinkCode(context.Background(), linkID)
	if err != nil {
		t.Fatalf("getting link code after approval: %v", err)
	}
	if lc.PrincipalID == nil {
		t.Fatal("expected link code to have a principal ID after approval")
	}
	approvedPrincipalID := *lc.PrincipalID

	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	if err != nil {
		t.Fatalf("listing audit log: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == store.AuditApproveLink && e.ActorPrincipalID == user.ID {
			found = true
			if e.Detail["device_name"] != "Audited Device" {
				t.Errorf("expected device_name detail, got %v", e.Detail["device_name"])
			}
			if e.Detail["link_code_id"] != linkID {
				t.Errorf("expected link_code_id detail %q, got %v", linkID, e.Detail["link_code_id"])
			}
			if e.TargetType != "principal" {
				t.Errorf("expected TargetType %q, got %q", "principal", e.TargetType)
			}
			if e.TargetID != approvedPrincipalID {
				t.Errorf("expected TargetID %q, got %q", approvedPrincipalID, e.TargetID)
			}
		}
	}
	if !found {
		t.Error("expected an approve_link audit entry")
	}
}
