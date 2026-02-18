// ABOUTME: Tests for HTTP authentication middleware
// ABOUTME: Covers token extraction, validation, principal lookup, and admin gate

package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
)

// httpTestSecret is a 32-byte secret that meets MinSecretLength requirement.
var httpTestSecret = []byte("http-middleware-test-secret-32b!")

func TestHTTPAuthMiddleware_ValidToken(t *testing.T) {
	verifier, err := NewJWTVerifier(httpTestSecret)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	principalID := "user-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusApproved,
		},
	}
	roles := &mockRoleStore{
		roles: []store.RoleName{store.RoleMember},
	}

	middleware := HTTPAuthMiddleware(principals, roles, verifier, nil)

	// Create test handler that checks context
	var gotAuthCtx *AuthContext
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthCtx = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Create request with valid token
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if gotAuthCtx == nil {
		t.Fatal("expected AuthContext in context")
	}
	if gotAuthCtx.PrincipalID != principalID {
		t.Errorf("expected principal ID '%s', got '%s'", principalID, gotAuthCtx.PrincipalID)
	}
	if gotAuthCtx.PrincipalType != "client" {
		t.Errorf("expected principal type 'client', got '%s'", gotAuthCtx.PrincipalType)
	}
	if len(gotAuthCtx.Roles) != 1 || gotAuthCtx.Roles[0] != "member" {
		t.Errorf("expected roles [member], got %v", gotAuthCtx.Roles)
	}
}

func TestHTTPAuthMiddleware_MissingAuthHeader(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	middleware := HTTPAuthMiddleware(principals, roles, verifier, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestHTTPAuthMiddleware_InvalidToken(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	middleware := HTTPAuthMiddleware(principals, roles, verifier, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestHTTPAuthMiddleware_RevokedPrincipal(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principalID := "user-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusRevoked,
		},
	}
	roles := &mockRoleStore{}

	middleware := HTTPAuthMiddleware(principals, roles, verifier, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}
}

func TestHTTPAuthMiddleware_PendingPrincipal(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principalID := "user-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusPending,
		},
	}
	roles := &mockRoleStore{}

	middleware := HTTPAuthMiddleware(principals, roles, verifier, nil)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}
}

func TestRequireAdminHTTP_WithAdmin(t *testing.T) {
	middleware := RequireAdminHTTP(nil)

	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create request with admin AuthContext
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	authCtx := &AuthContext{
		PrincipalID:   "admin-1",
		PrincipalType: "client",
		Roles:         []string{"admin"},
	}
	req = req.WithContext(WithAuth(req.Context(), authCtx))
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

func TestRequireAdminHTTP_WithOwner(t *testing.T) {
	middleware := RequireAdminHTTP(nil)

	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	authCtx := &AuthContext{
		PrincipalID:   "owner-1",
		PrincipalType: "client",
		Roles:         []string{"owner"},
	}
	req = req.WithContext(WithAuth(req.Context(), authCtx))
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

func TestRequireAdminHTTP_WithoutAdmin(t *testing.T) {
	middleware := RequireAdminHTTP(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	authCtx := &AuthContext{
		PrincipalID:   "member-1",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	req = req.WithContext(WithAuth(req.Context(), authCtx))
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}
}

func TestRequireAdminHTTP_NoAuthContext(t *testing.T) {
	middleware := RequireAdminHTTP(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	// No AuthContext in request
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestOptionalAuthMiddleware_NoToken(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	middleware := OptionalAuthMiddleware(principals, roles, verifier)

	var gotAuthCtx *AuthContext
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthCtx = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if gotAuthCtx != nil {
		t.Errorf("expected nil AuthContext, got %+v", gotAuthCtx)
	}
}

func TestOptionalAuthMiddleware_ValidToken(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principalID := "user-123"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusApproved,
		},
	}
	roles := &mockRoleStore{
		roles: []store.RoleName{store.RoleMember},
	}

	middleware := OptionalAuthMiddleware(principals, roles, verifier)

	var gotAuthCtx *AuthContext
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthCtx = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if gotAuthCtx == nil {
		t.Fatal("expected AuthContext in context")
	}
	if gotAuthCtx.PrincipalID != principalID {
		t.Errorf("expected principal ID '%s', got '%s'", principalID, gotAuthCtx.PrincipalID)
	}
}

func TestOptionalAuthMiddleware_InvalidToken(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	middleware := OptionalAuthMiddleware(principals, roles, verifier)

	var gotAuthCtx *AuthContext
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthCtx = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	// Should still succeed, just without AuthContext
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if gotAuthCtx != nil {
		t.Errorf("expected nil AuthContext for invalid token, got %+v", gotAuthCtx)
	}
}

// httpTestLogHandler captures log records for testing HTTP auth logging.
type httpTestLogHandler struct {
	records []slog.Record
}

func (h *httpTestLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *httpTestLogHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *httpTestLogHandler) WithGroup(_ string) slog.Handler              { return h }
func (h *httpTestLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *httpTestLogHandler) hasRecordWithReason(reason string) bool {
	for _, r := range h.records {
		var foundReason string
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "reason" {
				foundReason = a.Value.String()
				return false
			}
			return true
		})
		if foundReason == reason {
			return true
		}
	}
	return false
}

func (h *httpTestLogHandler) lastRecordMessage() string {
	if len(h.records) == 0 {
		return ""
	}
	return h.records[len(h.records)-1].Message
}

func TestHTTPAuthMiddleware_LogsFailure_MissingHeader(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	handler := &httpTestLogHandler{}
	logger := slog.New(handler)

	middleware := HTTPAuthMiddleware(principals, roles, verifier, logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	// Verify log was written
	if len(handler.records) == 0 {
		t.Fatal("expected log record, got none")
	}

	if !strings.Contains(handler.lastRecordMessage(), "http auth failure") {
		t.Errorf("expected 'http auth failure' in message, got %q", handler.lastRecordMessage())
	}

	if !handler.hasRecordWithReason("token_extraction_failed") {
		t.Error("expected log record with reason 'token_extraction_failed'")
	}
}

func TestHTTPAuthMiddleware_LogsFailure_InvalidToken(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	handler := &httpTestLogHandler{}
	logger := slog.New(handler)

	middleware := HTTPAuthMiddleware(principals, roles, verifier, logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	if !handler.hasRecordWithReason("token_verification_failed") {
		t.Error("expected log record with reason 'token_verification_failed'")
	}
}

func TestHTTPAuthMiddleware_LogsFailure_PrincipalNotFound(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principalID := "nonexistent-principal"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	handler := &httpTestLogHandler{}
	logger := slog.New(handler)

	middleware := HTTPAuthMiddleware(principals, roles, verifier, logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	if !handler.hasRecordWithReason("principal_not_found") {
		t.Error("expected log record with reason 'principal_not_found'")
	}
}

func TestHTTPAuthMiddleware_LogsFailure_RevokedPrincipal(t *testing.T) {
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principalID := "revoked-principal"
	token, _ := verifier.Generate(principalID, time.Hour)

	principals := &mockPrincipalStore{
		principal: &store.Principal{
			ID:     principalID,
			Type:   store.PrincipalTypeClient,
			Status: store.PrincipalStatusRevoked,
		},
	}
	roles := &mockRoleStore{}

	handler := &httpTestLogHandler{}
	logger := slog.New(handler)

	middleware := HTTPAuthMiddleware(principals, roles, verifier, logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	if !handler.hasRecordWithReason("principal_status_invalid") {
		t.Error("expected log record with reason 'principal_status_invalid'")
	}
}

func TestRequireAdminHTTP_LogsFailure_NotAdmin(t *testing.T) {
	handler := &httpTestLogHandler{}
	logger := slog.New(handler)

	middleware := RequireAdminHTTP(logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	authCtx := &AuthContext{
		PrincipalID:   "member-1",
		PrincipalType: "client",
		Roles:         []string{"member"},
	}
	req = req.WithContext(WithAuth(req.Context(), authCtx))
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	if !handler.hasRecordWithReason("admin_required") {
		t.Error("expected log record with reason 'admin_required'")
	}
}

func TestHTTPAuthMiddleware_NoLoggerNoError(t *testing.T) {
	// Verify that passing nil logger doesn't cause a panic
	verifier, _ := NewJWTVerifier(httpTestSecret)
	principals := &mockPrincipalStore{}
	roles := &mockRoleStore{}

	middleware := HTTPAuthMiddleware(principals, roles, verifier, nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}
