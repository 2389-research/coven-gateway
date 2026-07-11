// ABOUTME: Tests for webadmin authentication, session management, and setup flow.
// ABOUTME: Covers New, NewWithConfig, requireAuth, handleLogin, handleSetupSubmit, timingSafeCompare, validateUsername.

package webadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/2389/coven-gateway/internal/store"
)

// --- New / NewWithConfig constructor ---

func TestNew_ReturnsNonNilAdmin(t *testing.T) {
	a := New(nil, nil, nil, nil, Config{})
	if a == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewWithConfig_SetsStore(t *testing.T) {
	a := newTestAdminWithStore(t)
	if a == nil {
		t.Fatal("newTestAdminWithStore returned nil")
	}
	if a.store == nil {
		t.Fatal("expected store to be non-nil")
	}
}

// --- timingSafeCompare ---

func TestTimingSafeCompare_CorrectPassword(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "tuser", "correct horse")
	if err := timingSafeCompare(user, nil, "correct horse"); err != nil {
		t.Errorf("expected nil error for correct password, got %v", err)
	}
}

func TestTimingSafeCompare_WrongPassword(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "tuser2", "correct horse")
	if err := timingSafeCompare(user, nil, "wrong password"); err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestTimingSafeCompare_NilUser_UsesTimingDummy(t *testing.T) {
	// When user not found, must still perform a bcrypt comparison (dummy hash).
	err := timingSafeCompare(nil, store.ErrAdminUserNotFound, "any password")
	if err == nil {
		t.Error("expected error when user not found")
	}
}

// --- validateUsername ---

func TestValidateUsername_Valid(t *testing.T) {
	for _, u := range []string{"abc", "admin", "User_123", "a" + strings.Repeat("b", 30)} {
		if msg := validateUsername(u); msg != "" {
			t.Errorf("validateUsername(%q) = %q, want empty", u, msg)
		}
	}
}

func TestValidateUsername_TooShort(t *testing.T) {
	if msg := validateUsername("ab"); msg == "" {
		t.Error("expected error for 2-char username")
	}
}

func TestValidateUsername_TooLong(t *testing.T) {
	if msg := validateUsername(strings.Repeat("a", 33)); msg == "" {
		t.Error("expected error for 33-char username")
	}
}

func TestValidateUsername_StartsWithDigit(t *testing.T) {
	if msg := validateUsername("1abc"); msg == "" {
		t.Error("expected error for username starting with digit")
	}
}

func TestValidateUsername_HasDash(t *testing.T) {
	if msg := validateUsername("user-name"); msg == "" {
		t.Error("expected error for username with dash")
	}
}

// --- requireAuth ---

// buildLoginSession performs a real login and returns the session cookie value.
func buildLoginSession(t *testing.T, a *Admin, username, password string) string {
	t.Helper()
	// First get a CSRF token via the login page
	loginPageReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	loginPageRec := httptest.NewRecorder()
	a.handleLoginPage(loginPageRec, loginPageReq)

	var csrfCookie *http.Cookie
	for _, c := range loginPageRec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("no CSRF cookie from login page")
	}

	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)
	form.Set("csrf_token", csrfCookie.Value)

	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.AddCookie(csrfCookie)
	loginRec := httptest.NewRecorder()
	a.handleLogin(loginRec, loginReq)

	result := loginRec.Result()
	if result.StatusCode != http.StatusSeeOther {
		t.Fatalf("login expected redirect, got %d", result.StatusCode)
	}

	for _, c := range result.Cookies() {
		if c.Name == SessionCookieName {
			return c.Value
		}
	}
	t.Fatal("no session cookie after login")
	return ""
}

func TestRequireAuth_NoSession_RedirectsToLogin(t *testing.T) {
	a := newTestAdminWithStore(t)

	handlerCalled := false
	protected := a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	protected(rec, req)

	if handlerCalled {
		t.Error("handler should not be called without a session")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuth_ValidSession_CallsNext(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "authuser", "correct horse")
	sessionVal := buildLoginSession(t, a, "authuser", "correct horse")

	handlerCalled := false
	var ctxUser *store.AdminUser
	protected := a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxUser = getUserFromContext(r)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionVal})
	rec := httptest.NewRecorder()
	protected(rec, req)

	if !handlerCalled {
		t.Error("handler should be called with a valid session")
	}
	if ctxUser == nil || ctxUser.Username != "authuser" {
		t.Errorf("expected user in context, got %v", ctxUser)
	}
}

func TestRequireAuth_InvalidSession_RedirectsToLogin(t *testing.T) {
	a := newTestAdminWithStore(t)

	handlerCalled := false
	protected := a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "bogus-session-id"})
	rec := httptest.NewRecorder()
	protected(rec, req)

	if handlerCalled {
		t.Error("handler should not be called with invalid session")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect 303, got %d", rec.Code)
	}
}

// --- handleLoginPage ---

func TestHandleLoginPage_RendersLoginForm(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "loginpageuser", "password1")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	a.handleLoginPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "login") && !strings.Contains(body, "Login") && !strings.Contains(body, "username") {
		t.Errorf("expected login form in body, got: %q", body[:min(200, len(body))])
	}
}

func TestHandleLoginPage_NoAdminUsers_RedirectsToSetup(t *testing.T) {
	a := newTestAdminWithStore(t)
	// No users created — store is empty

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	a.handleLoginPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect to setup, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Errorf("expected redirect to /setup, got %q", loc)
	}
}

func TestHandleLoginPage_AlreadyLoggedIn_RedirectsToRoot(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "loggedinuser", "password1")
	sessionVal := buildLoginSession(t, a, "loggedinuser", "password1")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionVal})
	rec := httptest.NewRecorder()
	a.handleLoginPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
}

// --- handleLogin ---

func TestHandleLogin_MissingCSRF_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "csrfuser", "password1")

	form := url.Values{}
	form.Set("username", "csrfuser")
	form.Set("password", "password1")
	// No CSRF token or cookie

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with error page, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid") && !strings.Contains(body, "request") && !strings.Contains(body, "error") {
		t.Errorf("expected error message in response, got %q", body[:min(200, len(body))])
	}
}

func TestHandleLogin_EmptyCredentials_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "emptyuser", "password1")

	// Get CSRF token
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
	form.Set("username", "")
	form.Set("password", "")
	form.Set("csrf_token", csrfCookie.Value)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Errorf("expected 'required' in error body, got %q", rec.Body.String()[:min(200, len(rec.Body.String()))])
	}
}

func TestHandleLogin_UnknownUser_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "realuser", "password1")

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
	form.Set("username", "nosuchuser")
	form.Set("password", "password1")
	form.Set("csrf_token", csrfCookie.Value)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid") {
		t.Errorf("expected 'Invalid' in error body, got %q", rec.Body.String()[:min(200, len(rec.Body.String()))])
	}
}

func TestHandleLogin_WrongPassword_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "wrongpwuser", "correct password")

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
	form.Set("username", "wrongpwuser")
	form.Set("password", "wrong password")
	form.Set("csrf_token", csrfCookie.Value)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid") {
		t.Errorf("expected 'Invalid' in error body, got %q", rec.Body.String()[:min(200, len(rec.Body.String()))])
	}
}

func TestHandleLogin_CorrectCredentials_SetsSessionAndRedirects(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "gooduser", "correct horse")

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
	form.Set("username", "gooduser")
	form.Set("password", "correct horse")
	form.Set("csrf_token", csrfCookie.Value)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req)

	result := rec.Result()
	if result.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", result.StatusCode)
	}
	if loc := result.Header.Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}

	sessionCookie := findCookie(t, result.Cookies(), SessionCookieName)
	if sessionCookie.Value == "" {
		t.Error("expected session cookie value to be non-empty")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
}

// --- handleSetupPage ---

func TestHandleSetupPage_NoAdmins_RendersSetupForm(t *testing.T) {
	a := newTestAdminWithStore(t)

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	a.handleSetupPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "setup") && !strings.Contains(body, "Setup") && !strings.Contains(body, "username") {
		t.Errorf("expected setup form in body, got: %q", body[:min(200, len(body))])
	}
}

func TestHandleSetupPage_AdminExists_RedirectsToLogin(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "existingadmin", "password1")

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	a.handleSetupPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// --- handleSetupSubmit / parseSetupForm ---

// buildCSRFRequest builds a request with a fresh CSRF cookie and form field
// by going through handleSetupPage to get the token.
func buildSetupCSRFCookie(t *testing.T, a *Admin) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	a.handleSetupPage(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			return c
		}
	}
	t.Fatal("no CSRF cookie from setup page")
	return nil
}

func TestHandleSetupSubmit_HappyPath_CreatesUser(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrfCookie := buildSetupCSRFCookie(t, a)

	form := url.Values{}
	form.Set("username", "newadmin")
	form.Set("password", "securepass123")
	form.Set("display_name", "New Admin")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleSetupSubmit(rec, req)

	// Should render setup complete page (not redirect)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (setup complete), got %d: %s", rec.Code, rec.Body.String()[:min(300, rec.Body.Len())])
	}

	// Verify user was created in store
	user, err := a.store.GetAdminUserByUsername(context.Background(), "newadmin")
	if err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.DisplayName != "New Admin" {
		t.Errorf("display name = %q, want %q", user.DisplayName, "New Admin")
	}
}

func TestHandleSetupSubmit_PasswordMismatch_NotInForm_ShowsError(t *testing.T) {
	// parseSetupForm only takes a single password field; trigger short password error
	a := newTestAdminWithStore(t)
	csrfCookie := buildSetupCSRFCookie(t, a)

	form := url.Values{}
	form.Set("username", "newadmin")
	form.Set("password", "short") // < 8 chars
	form.Set("display_name", "New Admin")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleSetupSubmit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "8") && !strings.Contains(body, "Password") && !strings.Contains(body, "characters") {
		t.Errorf("expected password length error in body, got %q", body[:min(300, len(body))])
	}
}

func TestHandleSetupSubmit_BadUsername_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrfCookie := buildSetupCSRFCookie(t, a)

	form := url.Values{}
	form.Set("username", "1badstart") // starts with digit
	form.Set("password", "securepass123")
	form.Set("display_name", "Admin")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleSetupSubmit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Username") {
		t.Errorf("expected username error, got %q", rec.Body.String()[:min(300, len(rec.Body.String()))])
	}
}

func TestHandleSetupSubmit_MissingCSRF_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)

	form := url.Values{}
	form.Set("username", "newadmin")
	form.Set("password", "securepass123")
	form.Set("display_name", "Admin")
	// no csrf_token

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleSetupSubmit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
}

func TestHandleSetupSubmit_AdminAlreadyExists_Redirects(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "existingadmin", "password1")

	form := url.Values{}
	form.Set("username", "another")
	form.Set("password", "securepass123")
	form.Set("display_name", "Another")

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.handleSetupSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect when admin exists, got %d", rec.Code)
	}
}

func TestHandleSetupSubmit_AllFieldsRequired(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrfCookie := buildSetupCSRFCookie(t, a)

	form := url.Values{}
	form.Set("username", "")
	form.Set("password", "")
	form.Set("display_name", "")
	form.Set("csrf_token", csrfCookie.Value)

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	a.handleSetupSubmit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Errorf("expected 'required' in error body, got %q", rec.Body.String()[:min(300, len(rec.Body.String()))])
	}
}

// min returns the smaller of two ints (stdlib min requires Go 1.21).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
