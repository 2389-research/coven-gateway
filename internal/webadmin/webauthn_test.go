// ABOUTME: Tests for WebAuthn/Passkey authentication handlers
// ABOUTME: Covers session store, config derivation, request parsing, and handler edge cases

package webadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/go-webauthn/webauthn/webauthn"
)

// ============================================================================
// deriveWebAuthnConfig tests
// ============================================================================

func TestDeriveWebAuthnConfig_EmptyURL(t *testing.T) {
	rpID, rpOrigins := deriveWebAuthnConfig("")

	if rpID != "localhost" {
		t.Errorf("rpID = %q, want %q", rpID, "localhost")
	}
	if len(rpOrigins) != 2 {
		t.Errorf("rpOrigins length = %d, want 2", len(rpOrigins))
	}
}

func TestDeriveWebAuthnConfig_InvalidURL(t *testing.T) {
	rpID, rpOrigins := deriveWebAuthnConfig("not-a-valid-url")

	if rpID != "localhost" {
		t.Errorf("rpID = %q, want %q for invalid URL", rpID, "localhost")
	}
	if len(rpOrigins) != 2 {
		t.Errorf("rpOrigins length = %d, want 2 for invalid URL", len(rpOrigins))
	}
}

func TestDeriveWebAuthnConfig_ValidHTTPS(t *testing.T) {
	rpID, rpOrigins := deriveWebAuthnConfig("https://gateway.example.com")

	if rpID != "gateway.example.com" {
		t.Errorf("rpID = %q, want %q", rpID, "gateway.example.com")
	}
	if len(rpOrigins) < 1 {
		t.Fatal("expected at least one origin")
	}
	if rpOrigins[0] != "https://gateway.example.com" {
		t.Errorf("rpOrigins[0] = %q, want %q", rpOrigins[0], "https://gateway.example.com")
	}
}

func TestDeriveWebAuthnConfig_ValidHTTP(t *testing.T) {
	rpID, rpOrigins := deriveWebAuthnConfig("http://localhost:8080")

	if rpID != "localhost" {
		t.Errorf("rpID = %q, want %q", rpID, "localhost")
	}
	// Should include both http and https variants
	hasHTTP := false
	for _, o := range rpOrigins {
		if strings.HasPrefix(o, "http://") {
			hasHTTP = true
			break
		}
	}
	if !hasHTTP {
		t.Error("expected http origin in rpOrigins")
	}
}

func TestDeriveWebAuthnConfig_WithPort(t *testing.T) {
	rpID, rpOrigins := deriveWebAuthnConfig("https://gateway.tailnet.ts.net:443")

	if rpID != "gateway.tailnet.ts.net" {
		t.Errorf("rpID = %q, want %q", rpID, "gateway.tailnet.ts.net")
	}
	if len(rpOrigins) == 0 {
		t.Error("expected at least one origin")
	}
}

// ============================================================================
// webAuthnUser adapter tests
// ============================================================================

func TestWebAuthnUser_WebAuthnID(t *testing.T) {
	user := &webAuthnUser{
		user: &store.AdminUser{ID: "user-123"},
	}

	id := user.WebAuthnID()
	if string(id) != "user-123" {
		t.Errorf("WebAuthnID() = %q, want %q", string(id), "user-123")
	}
}

func TestWebAuthnUser_WebAuthnName(t *testing.T) {
	user := &webAuthnUser{
		user: &store.AdminUser{Username: "testuser"},
	}

	name := user.WebAuthnName()
	if name != "testuser" {
		t.Errorf("WebAuthnName() = %q, want %q", name, "testuser")
	}
}

func TestWebAuthnUser_WebAuthnDisplayName_WithDisplayName(t *testing.T) {
	user := &webAuthnUser{
		user: &store.AdminUser{
			Username:    "testuser",
			DisplayName: "Test User",
		},
	}

	displayName := user.WebAuthnDisplayName()
	if displayName != "Test User" {
		t.Errorf("WebAuthnDisplayName() = %q, want %q", displayName, "Test User")
	}
}

func TestWebAuthnUser_WebAuthnDisplayName_FallbackToUsername(t *testing.T) {
	user := &webAuthnUser{
		user: &store.AdminUser{
			Username:    "testuser",
			DisplayName: "",
		},
	}

	displayName := user.WebAuthnDisplayName()
	if displayName != "testuser" {
		t.Errorf("WebAuthnDisplayName() = %q, want %q (fallback to username)", displayName, "testuser")
	}
}

func TestWebAuthnUser_WebAuthnCredentials_Empty(t *testing.T) {
	user := &webAuthnUser{
		user:  &store.AdminUser{ID: "user-123"},
		creds: nil,
	}

	creds := user.WebAuthnCredentials()
	if len(creds) != 0 {
		t.Errorf("WebAuthnCredentials() length = %d, want 0", len(creds))
	}
}

func TestWebAuthnUser_WebAuthnCredentials_WithCredentials(t *testing.T) {
	user := &webAuthnUser{
		user: &store.AdminUser{ID: "user-123"},
		creds: []*store.WebAuthnCredential{
			{
				ID:              "cred-1",
				CredentialID:    []byte("credential-id-1"),
				PublicKey:       []byte("public-key-1"),
				AttestationType: "none",
				SignCount:       5,
				Transports:      `["usb","nfc"]`,
			},
		},
	}

	creds := user.WebAuthnCredentials()
	if len(creds) != 1 {
		t.Fatalf("WebAuthnCredentials() length = %d, want 1", len(creds))
	}
	if !bytes.Equal(creds[0].ID, []byte("credential-id-1")) {
		t.Errorf("credential ID mismatch")
	}
	if creds[0].Authenticator.SignCount != 5 {
		t.Errorf("SignCount = %d, want 5", creds[0].Authenticator.SignCount)
	}
}

// ============================================================================
// webAuthnSessionStore tests
// ============================================================================

func TestWebAuthnSessionStore_SetGet(t *testing.T) {
	store := newWebAuthnSessionStore()
	defer store.Close()

	session := &webauthn.SessionData{
		Challenge: "test-challenge",
	}

	store.Set("token-1", session, "user-123")

	got, userID, ok := store.Get("token-1")
	if !ok {
		t.Fatal("expected session to be found")
	}
	if got.Challenge != "test-challenge" {
		t.Errorf("Challenge = %q, want %q", got.Challenge, "test-challenge")
	}
	if userID != "user-123" {
		t.Errorf("userID = %q, want %q", userID, "user-123")
	}
}

func TestWebAuthnSessionStore_GetNonExistent(t *testing.T) {
	store := newWebAuthnSessionStore()
	defer store.Close()

	_, _, ok := store.Get("nonexistent")
	if ok {
		t.Error("expected session not to be found")
	}
}

func TestWebAuthnSessionStore_Delete(t *testing.T) {
	store := newWebAuthnSessionStore()
	defer store.Close()

	session := &webauthn.SessionData{Challenge: "test"}
	store.Set("token-1", session, "user-123")

	store.Delete("token-1")

	_, _, ok := store.Get("token-1")
	if ok {
		t.Error("expected session to be deleted")
	}
}

func TestWebAuthnSessionStore_SessionExpiry(t *testing.T) {
	store := newWebAuthnSessionStore()
	defer store.Close()

	// Manually set an expired session
	store.mu.Lock()
	store.sessions["expired"] = &sessionData{
		session:   &webauthn.SessionData{Challenge: "expired"},
		userID:    "user-1",
		expiresAt: time.Now().Add(-time.Hour), // expired 1 hour ago
	}
	store.mu.Unlock()

	_, _, ok := store.Get("expired")
	if ok {
		t.Error("expected expired session not to be returned")
	}
}

// ============================================================================
// Request parsing tests
// ============================================================================

func TestParseWebAuthnRegisterRequest_Valid(t *testing.T) {
	body := `{"sessionToken": "abc123", "response": {"test": "data"}}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	req, err := parseWebAuthnRegisterRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.sessionToken != "abc123" {
		t.Errorf("sessionToken = %q, want %q", req.sessionToken, "abc123")
	}
	if req.response == nil {
		t.Error("expected response to be non-nil")
	}
}

func TestParseWebAuthnRegisterRequest_InvalidJSON(t *testing.T) {
	body := `not valid json`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	_, err := parseWebAuthnRegisterRequest(r)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseWebAuthnLoginRequest_Valid(t *testing.T) {
	body := `{"sessionToken": "xyz789", "response": {"id": "cred-id"}}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	req, err := parseWebAuthnLoginRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.sessionToken != "xyz789" {
		t.Errorf("sessionToken = %q, want %q", req.sessionToken, "xyz789")
	}
}

func TestParseWebAuthnLoginRequest_InvalidJSON(t *testing.T) {
	body := `{invalid`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	_, err := parseWebAuthnLoginRequest(r)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ============================================================================
// makeCredentialFinder tests
// ============================================================================

func TestMakeCredentialFinder_MatchingUserHandle(t *testing.T) {
	waUser := &webAuthnUser{
		user: &store.AdminUser{ID: "user-123"},
	}

	finder := makeCredentialFinder(waUser, "user-123")

	result, err := finder([]byte("raw-id"), []byte("user-123"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != waUser {
		t.Error("expected finder to return waUser")
	}
}

func TestMakeCredentialFinder_EmptyUserHandle(t *testing.T) {
	waUser := &webAuthnUser{
		user: &store.AdminUser{ID: "user-123"},
	}

	finder := makeCredentialFinder(waUser, "user-123")

	// Empty user handle should still work
	result, err := finder([]byte("raw-id"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != waUser {
		t.Error("expected finder to return waUser")
	}
}

func TestMakeCredentialFinder_MismatchedUserHandle(t *testing.T) {
	waUser := &webAuthnUser{
		user: &store.AdminUser{ID: "user-123"},
	}

	finder := makeCredentialFinder(waUser, "user-123")

	_, err := finder([]byte("raw-id"), []byte("different-user"))
	if err == nil {
		t.Error("expected error for mismatched user handle")
	}
}

// ============================================================================
// Handler edge case tests
// ============================================================================

// mockWebAuthnStore implements the store methods needed for WebAuthn handler tests.
type mockWebAuthnStore struct {
	FullStore
	adminUser   *store.AdminUser
	credentials []*store.WebAuthnCredential
	credByID    *store.WebAuthnCredential
	err         error
}

func (m *mockWebAuthnStore) GetAdminUser(_ context.Context, _ string) (*store.AdminUser, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.adminUser, nil
}

func (m *mockWebAuthnStore) GetWebAuthnCredentialsByUser(_ context.Context, _ string) ([]*store.WebAuthnCredential, error) {
	return m.credentials, m.err
}

func (m *mockWebAuthnStore) GetWebAuthnCredentialByCredentialID(_ context.Context, _ []byte) (*store.WebAuthnCredential, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.credByID != nil {
		return m.credByID, nil
	}
	return nil, store.ErrNotFound
}

func (m *mockWebAuthnStore) CreateWebAuthnCredential(_ context.Context, _ *store.WebAuthnCredential) error {
	return m.err
}

func (m *mockWebAuthnStore) UpdateWebAuthnCredentialSignCount(_ context.Context, _ string, _ uint32) error {
	return m.err
}

// newTestAdmin creates an Admin instance for testing.
func newWebAuthnTestAdmin(t *testing.T, store FullStore) *Admin {
	t.Helper()
	return &Admin{
		store:  store,
		logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		config: Config{BaseURL: "https://test.example.com"},
	}
}

func TestHandleWebAuthnRegisterBegin_NotConfigured(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})
	// webauthn is nil (not configured)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/register/begin", nil)
	rec := httptest.NewRecorder()

	admin.handleWebAuthnRegisterBegin(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "WebAuthn not configured") {
		t.Errorf("body = %q, want to contain 'WebAuthn not configured'", rec.Body.String())
	}
}

func TestHandleWebAuthnRegisterBegin_NotAuthenticated(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})
	// Initialize webauthn
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/register/begin", nil)
	// No user in context
	rec := httptest.NewRecorder()

	admin.handleWebAuthnRegisterBegin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleWebAuthnRegisterFinish_NotConfigured(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/register/finish", nil)
	rec := httptest.NewRecorder()

	admin.handleWebAuthnRegisterFinish(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleWebAuthnRegisterFinish_NotAuthenticated(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/register/finish", nil)
	rec := httptest.NewRecorder()

	admin.handleWebAuthnRegisterFinish(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleWebAuthnRegisterFinish_InvalidRequest(t *testing.T) {
	mockStore := &mockWebAuthnStore{
		adminUser: &store.AdminUser{ID: "user-1", Username: "test"},
	}
	admin := newWebAuthnTestAdmin(t, mockStore)
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	body := `invalid json`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/register/finish", strings.NewReader(body))
	req = req.WithContext(withUser(req.Context(), mockStore.adminUser))
	rec := httptest.NewRecorder()

	admin.handleWebAuthnRegisterFinish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWebAuthnRegisterFinish_InvalidSession(t *testing.T) {
	mockStore := &mockWebAuthnStore{
		adminUser: &store.AdminUser{ID: "user-1", Username: "test"},
	}
	admin := newWebAuthnTestAdmin(t, mockStore)
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	body := `{"sessionToken": "nonexistent", "response": {}}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/register/finish", strings.NewReader(body))
	req = req.WithContext(withUser(req.Context(), mockStore.adminUser))
	rec := httptest.NewRecorder()

	admin.handleWebAuthnRegisterFinish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Invalid or expired session") {
		t.Errorf("body = %q, want to contain 'Invalid or expired session'", rec.Body.String())
	}
}

func TestHandleWebAuthnLoginBegin_NotConfigured(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/login/begin", nil)
	rec := httptest.NewRecorder()

	admin.handleWebAuthnLoginBegin(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleWebAuthnLoginBegin_Success(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/login/begin", nil)
	rec := httptest.NewRecorder()

	admin.handleWebAuthnLoginBegin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check response structure
	var resp struct {
		Options      json.RawMessage `json:"options"`
		SessionToken string          `json:"sessionToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.SessionToken == "" {
		t.Error("expected sessionToken in response")
	}
	if resp.Options == nil {
		t.Error("expected options in response")
	}
}

func TestHandleWebAuthnLoginFinish_NotConfigured(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})

	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/login/finish", nil)
	rec := httptest.NewRecorder()

	admin.handleWebAuthnLoginFinish(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleWebAuthnLoginFinish_InvalidRequest(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	body := `invalid json`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/login/finish", strings.NewReader(body))
	rec := httptest.NewRecorder()

	admin.handleWebAuthnLoginFinish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWebAuthnLoginFinish_InvalidSession(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})
	_ = admin.initWebAuthn()
	defer admin.webauthnSessions.Close()

	body := `{"sessionToken": "nonexistent", "response": {}}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/webauthn/login/finish", strings.NewReader(body))
	rec := httptest.NewRecorder()

	admin.handleWebAuthnLoginFinish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Invalid or expired session") {
		t.Errorf("body = %q, want to contain 'Invalid or expired session'", rec.Body.String())
	}
}

func TestHandleLookupError_NotFound(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})

	rec := httptest.NewRecorder()
	admin.handleLookupError(rec, store.ErrNotFound)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "Unknown credential") {
		t.Errorf("body = %q, want to contain 'Unknown credential'", rec.Body.String())
	}
}

func TestHandleLookupError_OtherError(t *testing.T) {
	admin := newWebAuthnTestAdmin(t, &mockWebAuthnStore{})

	rec := httptest.NewRecorder()
	admin.handleLookupError(rec, context.DeadlineExceeded)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// Helper for setting user in context
// ============================================================================

// withUser adds an admin user to the request context.
// This matches the pattern used by the actual middleware.
func withUser(ctx context.Context, user *store.AdminUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}
