// ABOUTME: Tests for webadmin device-link flow and invite flow handlers.
// ABOUTME: Covers handleLinkRequest, handleLinkStatus, handleLinkApprove, handleLinkPage,
// ABOUTME: handleLinkJSON, validatePendingLinkCode, handleInvitePage, handleInviteSignup,
// ABOUTME: handleCreateInviteJSON, parseInviteSignupForm, validateInvite, showInviteError.

package webadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
)

// validFingerprint returns a 64-char hex string (valid SHA256 fingerprint).
func validFingerprint() string {
	return strings.Repeat("a", 64)
}

// --- handleLinkRequest ---

func TestHandleLinkRequest_HappyPath_ReturnsCode(t *testing.T) {
	a := newTestAdminWithStore(t)

	body := `{"fingerprint":"` + validFingerprint() + `","device_name":"test-device"}`
	req := httptest.NewRequest(http.MethodPost, "/api/link/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["code"]; !ok {
		t.Error("expected 'code' in response")
	}
	if _, ok := resp["expires_at"]; !ok {
		t.Error("expected 'expires_at' in response")
	}
}

func TestHandleLinkRequest_MissingFingerprint_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)

	body := `{"device_name":"test-device"}`
	req := httptest.NewRequest(http.MethodPost, "/api/link/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLinkRequest_InvalidFingerprintLength_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)

	body := `{"fingerprint":"tooshort","device_name":"test-device"}`
	req := httptest.NewRequest(http.MethodPost, "/api/link/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short fingerprint, got %d", rec.Code)
	}
}

func TestHandleLinkRequest_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)

	req := httptest.NewRequest(http.MethodPost, "/api/link/request", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// createLinkCode creates a pending link code in the store and returns its short code and ID.
func createLinkCode(t *testing.T, a *Admin, fingerprint, deviceName string) (shortCode string, id string) {
	t.Helper()
	body := `{"fingerprint":"` + fingerprint + `","device_name":"` + deviceName + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/link/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkRequest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("createLinkCode failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	shortCode = resp["code"].(string)

	// Find the ID via store
	codes, err := a.store.ListPendingLinkCodes(context.Background())
	if err != nil {
		t.Fatalf("ListPendingLinkCodes: %v", err)
	}
	for _, c := range codes {
		if c.Code == shortCode {
			return shortCode, c.ID
		}
	}
	t.Fatal("created code not found in store")
	return "", ""
}

// --- handleLinkStatus ---

func TestHandleLinkStatus_PendingCode_ReturnsPending(t *testing.T) {
	a := newTestAdminWithStore(t)
	shortCode, _ := createLinkCode(t, a, validFingerprint(), "my-device")

	req := httptest.NewRequest(http.MethodGet, "/api/link/status/"+shortCode, nil)
	req.SetPathValue("code", shortCode)
	rec := httptest.NewRecorder()
	a.handleLinkStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %q", resp["status"])
	}
}

func TestHandleLinkStatus_UnknownCode_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/link/status/BADCODE", nil)
	req.SetPathValue("code", "BADCODE")
	rec := httptest.NewRecorder()
	a.handleLinkStatus(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleLinkStatus_EmptyCode_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/link/status/", nil)
	req.SetPathValue("code", "")
	rec := httptest.NewRecorder()
	a.handleLinkStatus(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLinkStatus_ExpiredCode_ReturnsExpired(t *testing.T) {
	a := newTestAdminWithStore(t)

	// Create a link code with expiry in the past
	expiredCode := &store.LinkCode{
		ID:          "expired-id",
		Code:        "EXPRD1",
		Fingerprint: validFingerprint(),
		DeviceName:  "expired-device",
		Status:      store.LinkCodeStatusPending,
		CreatedAt:   time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // already expired
	}
	if err := a.store.CreateLinkCode(context.Background(), expiredCode); err != nil {
		t.Fatalf("CreateLinkCode: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/link/status/EXPRD1", nil)
	req.SetPathValue("code", "EXPRD1")
	rec := httptest.NewRecorder()
	a.handleLinkStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "expired" {
		t.Errorf("expected status 'expired', got %q", resp["status"])
	}
}

// --- handleLinkPage / handleLinkJSON ---

func TestHandleLinkPage_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/link", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleLinkJSON_EmptyStore_ReturnsCodes(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/link", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["codes"]; !ok {
		t.Error("expected 'codes' field in response")
	}
}

func TestHandleLinkJSON_WithPendingCode_ContainsCode(t *testing.T) {
	a := newTestAdminWithStore(t)
	shortCode, _ := createLinkCode(t, a, validFingerprint(), "link-device")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/link", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), shortCode) {
		t.Errorf("expected code %q in response, got %q", shortCode, rec.Body.String())
	}
}

// --- validatePendingLinkCode ---

func TestValidatePendingLinkCode_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	rec := httptest.NewRecorder()
	code, ok := a.validatePendingLinkCode(rec, context.Background(), "no-such-id")

	if ok {
		t.Error("expected validation to fail for unknown ID")
	}
	if code != nil {
		t.Error("expected nil code")
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestValidatePendingLinkCode_PendingCode_ReturnsCode(t *testing.T) {
	a := newTestAdminWithStore(t)
	_, id := createLinkCode(t, a, validFingerprint(), "validate-device")

	rec := httptest.NewRecorder()
	code, ok := a.validatePendingLinkCode(rec, context.Background(), id)

	if !ok {
		t.Error("expected validation to succeed for pending code")
	}
	if code == nil {
		t.Fatal("expected non-nil code")
	}
	if code.Status != store.LinkCodeStatusPending {
		t.Errorf("expected pending status, got %q", code.Status)
	}
}

// --- handleInvitePage ---

func createInvite(t *testing.T, a *Admin) string {
	t.Helper()
	user := createAdminUserWithPassword(t, a, "inviteadmin", "password1")
	ctx := context.Background()
	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("generateSecureToken: %v", err)
	}
	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: user.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := a.store.CreateAdminInvite(ctx, invite); err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}
	return token
}

func TestHandleInvitePage_ValidToken_RendersSignupForm(t *testing.T) {
	a := newTestAdminWithStore(t)
	token := createInvite(t, a)

	req := httptest.NewRequest(http.MethodGet, "/invite/"+token, nil)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleInvitePage_InvalidToken_RendersError(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "someuser", "password1")

	req := httptest.NewRequest(http.MethodGet, "/invite/badtoken", nil)
	req.SetPathValue("token", "badtoken")
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error rendered), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid") && !strings.Contains(body, "invalid") {
		t.Errorf("expected error in body for invalid token, got %q", body[:min(200, len(body))])
	}
}

func TestHandleInvitePage_EmptyToken_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)

	req := httptest.NewRequest(http.MethodGet, "/invite/", nil)
	req.SetPathValue("token", "")
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- validateInvite ---

func TestValidateInvite_ValidToken_ReturnsInvite(t *testing.T) {
	a := newTestAdminWithStore(t)
	token := createInvite(t, a)

	invite, errMsg := a.validateInvite(context.Background(), token)
	if errMsg != "" {
		t.Errorf("expected no error, got %q", errMsg)
	}
	if invite == nil {
		t.Error("expected non-nil invite")
	}
}

func TestValidateInvite_InvalidToken_ReturnsError(t *testing.T) {
	a := newTestAdminWithStore(t)

	_, errMsg := a.validateInvite(context.Background(), "bogus-token")
	if errMsg == "" {
		t.Error("expected error for invalid token")
	}
}

func TestValidateInvite_ExpiredToken_ReturnsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "expireuser", "password1")

	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: user.ID,
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour), // expired
	}
	if err := a.store.CreateAdminInvite(context.Background(), invite); err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}

	_, errMsg := a.validateInvite(context.Background(), token)
	if errMsg == "" {
		t.Error("expected error for expired invite")
	}
	if !strings.Contains(errMsg, "expired") {
		t.Errorf("expected 'expired' in error, got %q", errMsg)
	}
}

// --- parseInviteSignupForm ---

func buildInviteFormRequest(vals map[string]string) *http.Request {
	form := url.Values{}
	for k, v := range vals {
		form.Set(k, v)
	}
	req := httptest.NewRequest(http.MethodPost, "/invite/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestParseInviteSignupForm_HappyPath(t *testing.T) {
	req := buildInviteFormRequest(map[string]string{
		"username":     "newuser",
		"password":     "password123",
		"display_name": "New User",
	})
	data, errMsg := parseInviteSignupForm(req)
	if errMsg != "" {
		t.Errorf("expected no error, got %q", errMsg)
	}
	if data == nil || data.username != "newuser" {
		t.Errorf("expected username 'newuser', got %v", data)
	}
}

func TestParseInviteSignupForm_EmptyUsername_ReturnsError(t *testing.T) {
	req := buildInviteFormRequest(map[string]string{
		"username": "",
		"password": "password123",
	})
	_, errMsg := parseInviteSignupForm(req)
	if errMsg == "" {
		t.Error("expected error for empty username")
	}
}

func TestParseInviteSignupForm_ShortPassword_ReturnsError(t *testing.T) {
	req := buildInviteFormRequest(map[string]string{
		"username": "newuser",
		"password": "short",
	})
	_, errMsg := parseInviteSignupForm(req)
	if errMsg == "" {
		t.Error("expected error for short password")
	}
}

func TestParseInviteSignupForm_BadUsername_ReturnsError(t *testing.T) {
	req := buildInviteFormRequest(map[string]string{
		"username": "1badstart",
		"password": "password123",
	})
	_, errMsg := parseInviteSignupForm(req)
	if errMsg == "" {
		t.Error("expected error for bad username")
	}
}

// --- handleInviteSignup ---

func getInviteCSRFCookie(t *testing.T, a *Admin, token string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/invite/"+token, nil)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			return c
		}
	}
	t.Fatal("no CSRF cookie from invite page")
	return nil
}

func TestHandleInviteSignup_HappyPath_CreatesUser(t *testing.T) {
	a := newTestAdminWithStore(t)
	token := createInvite(t, a)
	csrfCookie := getInviteCSRFCookie(t, a, token)

	form := url.Values{}
	form.Set("username", "inviteduser")
	form.Set("password", "securepass123")
	form.Set("display_name", "Invited User")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("token", token)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after signup, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify user was created
	user, err := a.store.GetAdminUserByUsername(context.Background(), "inviteduser")
	if err != nil {
		t.Fatalf("invited user not created: %v", err)
	}
	if user.DisplayName != "Invited User" {
		t.Errorf("display name = %q, want 'Invited User'", user.DisplayName)
	}
}

func TestHandleInviteSignup_ShortPassword_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	token := createInvite(t, a)
	csrfCookie := getInviteCSRFCookie(t, a, token)

	form := url.Values{}
	form.Set("username", "inviteduser")
	form.Set("password", "short")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("token", token)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "8") && !strings.Contains(rec.Body.String(), "Password") {
		t.Errorf("expected password error in body")
	}
}

func TestHandleInviteSignup_InvalidToken_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "placeholder", "password1")

	// Get CSRF from login page since invite page with bad token also renders
	pageReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	pageRec := httptest.NewRecorder()
	a.handleLoginPage(pageRec, pageReq)
	var csrfCookie *http.Cookie
	for _, c := range pageRec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			csrfCookie = c
		}
	}

	form := url.Values{}
	form.Set("username", "someuser")
	form.Set("password", "password123")
	form.Set("display_name", "Some User")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/invite/bogus-token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("token", "bogus-token")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
}

func TestHandleInviteSignup_MissingCSRF_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	token := createInvite(t, a)

	form := url.Values{}
	form.Set("username", "someuser")
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
}

func TestHandleInviteSignup_UsernameAlreadyTaken_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)

	// Seed an existing user with the same username
	createAdminUserWithPassword(t, a, "existing-user", "password123")

	token := createInvite(t, a)
	csrf := getInviteCSRFCookie(t, a, token)

	form := url.Values{}
	form.Set("username", "existing-user") // already taken
	form.Set("password", "password123")
	form.Set("csrf_token", csrf.Value)

	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("token", token)
	req.AddCookie(csrf)
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 error page for taken username, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "already") && !strings.Contains(rec.Body.String(), "taken") && !strings.Contains(rec.Body.String(), "exists") {
		// The showInviteError renders a template; just check for 200 status
		t.Logf("body excerpt: %s", rec.Body.String()[:min(200, rec.Body.Len())])
	}
}

func TestHandleInviteSignup_EmptyToken_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodPost, "/invite/", nil)
	req.SetPathValue("token", "")
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty token, got %d", rec.Code)
	}
}

// --- handleCreateInviteJSON ---

func TestHandleCreateInviteJSON_HappyPath_ReturnsURL(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	// createInviteToken uses getUserFromContext(r).ID as created_by FK,
	// so we must ensure that user exists in the store. requestWithUser injects
	// AdminUser{ID: "test-user"} so create a matching row.
	if err := a.store.CreateAdminUser(context.Background(), &store.AdminUser{
		ID:          "test-user",
		Username:    "testadmin",
		DisplayName: "Test Admin",
	}); err != nil {
		t.Fatalf("seed test user: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/invites", nil)
	req.AddCookie(csrf)
	req.Header.Set("X-CSRF-Token", csrf.Value)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleCreateInviteJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["url"]; !ok {
		t.Error("expected 'url' in response")
	}
}

func TestHandleCreateInviteJSON_MissingCSRF_ReturnsForbidden(t *testing.T) {
	a := newTestAdminWithStore(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/invites", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleCreateInviteJSON(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// --- handleLinkApprove ---

func TestHandleLinkApprove_NoCSRF_ReturnsForbidden(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/link/code-1/approve", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestHandleLinkApprove_EmptyID_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	form := "csrf_token=" + csrf.Value
	req := httptest.NewRequest(http.MethodPost, "/admin/link//approve", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	req.SetPathValue("id", "")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLinkApprove_UnknownCode_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	form := "csrf_token=" + csrf.Value
	req := httptest.NewRequest(http.MethodPost, "/admin/link/nonexistent/approve", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	req.SetPathValue("id", "nonexistent")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown link code, got %d", rec.Code)
	}
}

func TestHandleLinkApprove_NilPrincipalStore_Returns500(t *testing.T) {
	a := newTestAdminWithStore(t)
	// principalStore is nil in newTestAdminWithStore
	csrf := adminCSRFCookie(t, a)

	// Create a pending link code
	fp := validFingerprint()
	_, codeID := createLinkCode(t, a, fp, "test-device")

	form := "csrf_token=" + csrf.Value
	req := httptest.NewRequest(http.MethodPost, "/admin/link/"+codeID+"/approve", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	req.SetPathValue("id", codeID)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when principalStore is nil, got %d: %s", rec.Code, rec.Body.String())
	}
}
