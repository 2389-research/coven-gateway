// ABOUTME: Tests for webadmin secrets, principals, agents, and tools CRUD handlers.
// ABOUTME: Covers create/read/update/delete cycles with real SQLiteStore and CSRF validation.

package webadmin

import (
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

// --- CSRF helpers for admin tests ---

// adminCSRFCookie gets a CSRF token cookie for admin handlers by hitting the login page.
func adminCSRFCookie(t *testing.T, a *Admin) *http.Cookie {
	t.Helper()
	// Need at least one admin user for login page to show (not redirect to setup)
	// If none exist, create a placeholder
	count, _ := a.store.CountAdminUsers(context.Background())
	if count == 0 {
		createAdminUserWithPassword(t, a, "csrfadmin", "password1")
	}

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	a.handleLoginPage(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			return c
		}
	}
	t.Fatal("no CSRF cookie from login page")
	return nil
}

// postWithCSRF sends a POST request to a handler with CSRF cookie and form field.
func postWithCSRF(t *testing.T, a *Admin, csrf *http.Cookie, path string, formVals url.Values, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	formVals.Set("csrf_token", csrf.Value)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(formVals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// deleteWithCSRF sends a DELETE request to a handler with CSRF cookie and X-CSRF-Token header.
func deleteWithCSRF(t *testing.T, a *Admin, csrf *http.Cookie, path string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(csrf)
	req.Header.Set("X-CSRF-Token", csrf.Value)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// --- Secrets CRUD ---

func TestHandleSecretsPage_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/secrets", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleSecretsCreate_HappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	form := url.Values{}
	form.Set("key", "MY_API_KEY")
	form.Set("value", "secret-value-123")
	rec := postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after create, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSecretsCreate_MissingCSRF_Forbidden(t *testing.T) {
	a := newTestAdminWithStore(t)

	form := url.Values{"key": {"MY_KEY"}, "value": {"val"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/secrets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsCreate(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestHandleSecretsCreate_BlankKey_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	form := url.Values{}
	form.Set("key", "")
	form.Set("value", "some-value")
	rec := postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleSecretsCreate_InvalidKey_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	form := url.Values{}
	form.Set("key", "has-dash")
	form.Set("value", "some-value")
	rec := postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid key, got %d", rec.Code)
	}
}

func TestHandleSecretsJSON_EmptyStore_ReturnsArray(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var items []any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func TestHandleSecrets_CreateThenGetValue_RoundTrip(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	// Create
	form := url.Values{}
	form.Set("key", "ROUND_TRIP_KEY")
	form.Set("value", "round-trip-value")
	rec := postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
	}

	// List to find the ID
	reqList := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	reqList = requestWithUser(reqList)
	recList := httptest.NewRecorder()
	a.handleSecretsJSON(recList, reqList)

	var items []map[string]any
	if err := json.Unmarshal(recList.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	var secretID string
	for _, item := range items {
		if item["Key"] == "ROUND_TRIP_KEY" {
			secretID = item["ID"].(string)
			break
		}
	}
	if secretID == "" {
		t.Fatal("created secret not found in list")
	}

	// Get value
	reqGet := httptest.NewRequest(http.MethodGet, "/admin/secrets/"+secretID+"/value", nil)
	reqGet.SetPathValue("id", secretID)
	reqGet = requestWithUser(reqGet)
	recGet := httptest.NewRecorder()
	a.handleSecretsGetValue(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Errorf("expected 200 for get-value, got %d", recGet.Code)
	}
	var valResp map[string]string
	if err := json.Unmarshal(recGet.Body.Bytes(), &valResp); err != nil {
		t.Fatalf("decode value response: %v", err)
	}
	if valResp["value"] != "round-trip-value" {
		t.Errorf("expected 'round-trip-value', got %q", valResp["value"])
	}
}

func TestHandleSecretsUpdate_HappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	// Create first
	form := url.Values{"key": {"UPDATE_KEY"}, "value": {"original"}}
	postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)

	// Find ID
	reqList := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	reqList = requestWithUser(reqList)
	recList := httptest.NewRecorder()
	a.handleSecretsJSON(recList, reqList)
	var items []map[string]any
	json.Unmarshal(recList.Body.Bytes(), &items) //nolint:errcheck
	var secretID string
	for _, item := range items {
		if item["Key"] == "UPDATE_KEY" {
			secretID = item["ID"].(string)
		}
	}
	if secretID == "" {
		t.Fatal("created secret not found")
	}

	// Update
	updateForm := url.Values{"value": {"updated-value"}}
	updateForm.Set("csrf_token", csrf.Value)
	reqUpdate := httptest.NewRequest(http.MethodPut, "/admin/secrets/"+secretID, strings.NewReader(updateForm.Encode()))
	reqUpdate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqUpdate.AddCookie(csrf)
	reqUpdate.SetPathValue("id", secretID)
	reqUpdate = requestWithUser(reqUpdate)
	recUpdate := httptest.NewRecorder()
	a.handleSecretsUpdate(recUpdate, reqUpdate)

	if recUpdate.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after update, got %d", recUpdate.Code)
	}

	// Verify new value
	reqGet := httptest.NewRequest(http.MethodGet, "/admin/secrets/"+secretID+"/value", nil)
	reqGet.SetPathValue("id", secretID)
	reqGet = requestWithUser(reqGet)
	recGet := httptest.NewRecorder()
	a.handleSecretsGetValue(recGet, reqGet)

	var valResp map[string]string
	json.Unmarshal(recGet.Body.Bytes(), &valResp) //nolint:errcheck
	if valResp["value"] != "updated-value" {
		t.Errorf("expected 'updated-value', got %q", valResp["value"])
	}
}

func TestHandleSecretsDelete_HappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	// Create
	form := url.Values{"key": {"DELETE_KEY"}, "value": {"to-delete"}}
	postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)

	// Find ID
	reqList := httptest.NewRequest(http.MethodGet, "/api/admin/secrets", nil)
	reqList = requestWithUser(reqList)
	recList := httptest.NewRecorder()
	a.handleSecretsJSON(recList, reqList)
	var items []map[string]any
	json.Unmarshal(recList.Body.Bytes(), &items) //nolint:errcheck
	var secretID string
	for _, item := range items {
		if item["Key"] == "DELETE_KEY" {
			secretID = item["ID"].(string)
		}
	}
	if secretID == "" {
		t.Fatal("created secret not found")
	}

	// Delete
	rec := deleteWithCSRF(t, a, csrf, "/admin/secrets/"+secretID, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", secretID)
		a.handleSecretsDelete(w, r)
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 after delete, got %d", rec.Code)
	}

	// Verify gone
	reqGet := httptest.NewRequest(http.MethodGet, "/admin/secrets/"+secretID+"/value", nil)
	reqGet.SetPathValue("id", secretID)
	reqGet = requestWithUser(reqGet)
	recGet := httptest.NewRecorder()
	a.handleSecretsGetValue(recGet, reqGet)
	if recGet.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", recGet.Code)
	}
}

func TestHandleSecretsGetValue_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/secrets/nonexistent/value", nil)
	req.SetPathValue("id", "nonexistent")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsGetValue(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleSecretsUpdate_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	updateForm := url.Values{"value": {"new-value"}}
	updateForm.Set("csrf_token", csrf.Value)
	req := httptest.NewRequest(http.MethodPut, "/admin/secrets/no-such-id", strings.NewReader(updateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrf)
	req.SetPathValue("id", "no-such-id")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsUpdate(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown secret update, got %d", rec.Code)
	}
}

func TestHandleSecretsDelete_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	rec := deleteWithCSRF(t, a, csrf, "/admin/secrets/no-such-id", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "no-such-id")
		a.handleSecretsDelete(w, r)
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleSecretsJSON_ScopeFilter_Global(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	// Create a global secret (no agent_id)
	form := url.Values{"key": {"GLOBAL_KEY"}, "value": {"global-val"}}
	postWithCSRF(t, a, csrf, "/admin/secrets", form, a.handleSecretsCreate)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/secrets?scope=global", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "GLOBAL_KEY") {
		t.Errorf("expected global key in global-scoped response")
	}
}

// --- Principals ---

func seedPrincipal(t *testing.T, s FullStore, id string, ptype store.PrincipalType, status store.PrincipalStatus) {
	t.Helper()
	p := &store.Principal{
		ID:          id,
		Type:        ptype,
		PubkeyFP:    "fp-" + id,
		DisplayName: "Principal " + id,
		Status:      status,
		CreatedAt:   time.Now(),
	}
	if err := s.(*store.SQLiteStore).CreatePrincipal(context.Background(), p); err != nil {
		t.Fatalf("CreatePrincipal: %v", err)
	}
}

func TestHandlePrincipalsPage_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/principals", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handlePrincipalsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandlePrincipalsJSON_EmptyStore_ReturnsArray(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/principals", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handlePrincipalsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var items []any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func TestHandlePrincipalApprove_HappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "p-pending", store.PrincipalTypeClient, store.PrincipalStatusPending)
	csrf := adminCSRFCookie(t, a)

	rec := postWithCSRF(t, a, csrf, "/admin/principals/p-pending/approve", url.Values{}, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "p-pending")
		a.handlePrincipalApprove(w, r)
	})

	// Returns 200 with HTML snippet (not redirect)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "approved") {
		t.Errorf("expected 'approved' in response, got %q", rec.Body.String())
	}

	// Verify store state
	p, err := a.store.GetPrincipal(context.Background(), "p-pending")
	if err != nil {
		t.Fatalf("GetPrincipal: %v", err)
	}
	if p.Status != store.PrincipalStatusApproved {
		t.Errorf("expected status approved, got %q", p.Status)
	}
}

func TestHandlePrincipalApprove_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	rec := postWithCSRF(t, a, csrf, "/admin/principals/no-such/approve", url.Values{}, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "no-such")
		a.handlePrincipalApprove(w, r)
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlePrincipalRevoke_HappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "p-active", store.PrincipalTypeClient, store.PrincipalStatusApproved)
	csrf := adminCSRFCookie(t, a)

	rec := postWithCSRF(t, a, csrf, "/admin/principals/p-active/revoke", url.Values{}, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "p-active")
		a.handlePrincipalRevoke(w, r)
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "revoked") {
		t.Errorf("expected 'revoked' in response")
	}
}

func TestHandlePrincipalRevoke_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	rec := postWithCSRF(t, a, csrf, "/admin/principals/no-such/revoke", url.Values{}, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "no-such")
		a.handlePrincipalRevoke(w, r)
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlePrincipalDelete_HappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "p-delete", store.PrincipalTypeClient, store.PrincipalStatusPending)
	csrf := adminCSRFCookie(t, a)

	rec := deleteWithCSRF(t, a, csrf, "/admin/principals/p-delete", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "p-delete")
		a.handlePrincipalDelete(w, r)
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 after delete, got %d", rec.Code)
	}

	// Verify gone from store
	_, err := a.store.GetPrincipal(context.Background(), "p-delete")
	if err == nil {
		t.Error("expected principal to be deleted")
	}
}

func TestHandlePrincipalDelete_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	rec := deleteWithCSRF(t, a, csrf, "/admin/principals/no-such", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "no-such")
		a.handlePrincipalDelete(w, r)
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- Agents ---

func TestHandleAgentsPage_NilManager_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	// manager is nil by default in newTestAdminWithStore
	req := httptest.NewRequest(http.MethodGet, "/admin/agents", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleAgentsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with nil manager, got %d", rec.Code)
	}
}

func TestHandleAgentRevoke_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	csrf := adminCSRFCookie(t, a)

	rec := postWithCSRF(t, a, csrf, "/admin/agents/no-such/revoke", url.Values{}, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "no-such")
		a.handleAgentRevoke(w, r)
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleAgentRevoke_KnownAgent_RedirectsAndRevokes(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "agent-p-1", store.PrincipalTypeAgent, store.PrincipalStatusApproved)
	csrf := adminCSRFCookie(t, a)

	rec := postWithCSRF(t, a, csrf, "/admin/agents/agent-p-1/revoke", url.Values{}, func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "agent-p-1")
		a.handleAgentRevoke(w, r)
	})

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after revoke, got %d", rec.Code)
	}
}

// --- Tools JSON ---

func TestHandleToolsJSON_NilRegistry_ReturnsEmptyArray(t *testing.T) {
	a := newTestAdminWithStore(t)
	// registry is nil

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tools", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleToolsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var items []any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty array with nil registry, got %d items", len(items))
	}
}

func TestSortedToolItems_SortsAlphabetically(t *testing.T) {
	tools := []toolItem{
		{Name: "zebra"},
		{Name: "alpha"},
		{Name: "medium"},
	}
	result := sortedToolItems(tools)
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	if result[0].Name != "alpha" || result[1].Name != "medium" || result[2].Name != "zebra" {
		t.Errorf("sortedToolItems order wrong: got %v", result)
	}
}

func TestSortedToolItems_NilInput_ReturnsEmpty(t *testing.T) {
	result := sortedToolItems(nil)
	if result == nil || len(result) != 0 {
		t.Errorf("expected empty slice for nil input, got %v", result)
	}
}
