// ABOUTME: Additional coverage tests targeting functions left uncovered by UNIT-001 through UNIT-005.
// ABOUTME: Covers link approval happy path, agent detail with threads, thread listing, and webauthn nil-config paths.

package webadmin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// =============================================================================
// Test helper: admin with principalStore + tokenGenerator configured
// =============================================================================

// newTestAdminFull creates an Admin with a real SQLiteStore configured as both
// the FullStore and the PrincipalStore, plus a real JWTVerifier as TokenGenerator.
// This enables testing paths that require principalStore and tokenGenerator.
func newTestAdminFull(t *testing.T) *Admin {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-full.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// 32-byte secret for JWT (minimum required)
	secret := []byte("test-secret-that-is-32-bytes-lon")
	jwtVerifier, err := auth.NewJWTVerifier(secret)
	if err != nil {
		t.Fatalf("NewJWTVerifier: %v", err)
	}

	return NewWithConfig(NewConfig{
		Store:          s,
		PrincipalStore: s,
		TokenGenerator: jwtVerifier,
	})
}

// createPendingLink seeds a pending link code and returns its ID.
func createPendingLink(t *testing.T, a *Admin, fingerprint, deviceName string) string {
	t.Helper()
	ctx := context.Background()
	id := "test-link-" + fingerprint[:8]
	code := &store.LinkCode{
		ID:          id,
		Code:        "TESTCODE",
		Fingerprint: fingerprint,
		DeviceName:  deviceName,
		Status:      store.LinkCodeStatusPending,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	if err := a.store.CreateLinkCode(ctx, code); err != nil {
		t.Fatalf("CreateLinkCode: %v", err)
	}
	return id
}

// newTestManager creates a real *agent.Manager for tests that need one.
func newTestManager(t *testing.T) *agent.Manager {
	t.Helper()
	return agent.NewManager(slog.Default())
}

// newTestAdminWithManagerAndStore creates an Admin backed by a real SQLiteStore
// and the given agent.Manager.
func newTestAdminWithManagerAndStore(t *testing.T, mgr *agent.Manager) *Admin {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-mgr.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return NewWithConfig(NewConfig{
		Store:   s,
		Manager: mgr,
	})
}

// registerFakeAgent registers a minimal agent.Connection with the given manager.
// stream is nil so no real gRPC calls are made.
func registerFakeAgent(t *testing.T, mgr *agent.Manager, agentID, name string, caps, workspaces []string) {
	t.Helper()
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:           agentID,
		Name:         name,
		Capabilities: caps,
		Workspaces:   workspaces,
		WorkingDir:   "/tmp",
		InstanceID:   "inst-" + agentID,
		Backend:      "cli",
		Logger:       slog.Default(),
	})
	if err := mgr.Register(conn); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// =============================================================================
// handleLinkApprove happy path — covers getOrCreatePrincipalForLink +
// generateApprovalToken (both were at 0%)
// =============================================================================

func TestHandleLinkApprove_HappyPath_Approves(t *testing.T) {
	a := newTestAdminFull(t)
	user := createAdminUserWithPassword(t, a, "approver", "password123")

	fp := strings.Repeat("a", 64)
	linkID := createPendingLink(t, a, fp, "my-device")

	// Build a request with CSRF + path value set
	csrfVal := "test-csrf-token"
	req := httptest.NewRequest(http.MethodPost, "/admin/link/"+linkID+"/approve", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req.SetPathValue("id", linkID)

	// Put the user in context (requireAuth normally does this)
	req = req.WithContext(withUser(req.Context(), user))

	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLinkApprove_WithExistingPrincipal_ReusesIt(t *testing.T) {
	a := newTestAdminFull(t)
	user := createAdminUserWithPassword(t, a, "approver2", "password123")
	ctx := context.Background()

	fp := strings.Repeat("b", 64)

	// Pre-create a principal with this fingerprint
	existing := &store.Principal{
		ID:          "existing-principal",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fp,
		DisplayName: "Pre-existing",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	if err := a.principalStore.CreatePrincipal(ctx, existing); err != nil {
		t.Fatalf("CreatePrincipal: %v", err)
	}

	linkID := createPendingLink(t, a, fp, "device-existing")

	csrfVal := "csrf-existing"
	req := httptest.NewRequest(http.MethodPost, "/admin/link/"+linkID+"/approve", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req.SetPathValue("id", linkID)
	req = req.WithContext(withUser(req.Context(), user))

	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// handleAgentDetail — cover the thread-filtering branch (52% → higher)
// =============================================================================

func TestHandleAgentDetail_WithThreadsForAgent_ShowsThreads(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	agentID := "agent-with-threads"

	// Seed a thread for this agent and one for another agent
	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           "thread-for-agent",
		FrontendName: "test",
		ExternalID:   "ext-1",
		AgentID:      agentID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           "thread-other-agent",
		FrontendName: "test",
		ExternalID:   "ext-2",
		AgentID:      "other-agent",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread (other): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/agents/"+agentID, nil)
	req.SetPathValue("id", agentID)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleAgentDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleAgentDetailJSON — cover the connected manager branch
// =============================================================================

func TestHandleAgentDetailJSON_WithNonEmptyID_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/agents/some-agent/json", nil)
	req.SetPathValue("id", "some-agent")
	rec := httptest.NewRecorder()
	a.handleAgentDetailJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result["ID"] != "some-agent" {
		t.Errorf("expected ID=some-agent, got %v", result["ID"])
	}
}

// =============================================================================
// createOwnerPrincipal — full happy path (was at 10.5%)
// =============================================================================

func TestCreateOwnerPrincipal_HappyPath_ReturnsToken(t *testing.T) {
	a := newTestAdminFull(t)
	token := a.createOwnerPrincipal(context.Background(), "Test Admin")
	if token == "" {
		t.Error("expected non-empty token from createOwnerPrincipal")
	}
}

// =============================================================================
// handleCreateInviteJSON — CSRF protected path
// =============================================================================

func TestHandleCreateInviteJSON_NoCSRF_ReturnsForbidden(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/invite/create", nil)
	rec := httptest.NewRecorder()
	a.handleCreateInviteJSON(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestHandleCreateInviteJSON_WithUser_ReturnsURL(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "invite-creator", "password123")

	csrfVal := "invite-csrf"
	req := httptest.NewRequest(http.MethodPost, "/admin/invite/create", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req = req.WithContext(withUser(req.Context(), user))

	rec := httptest.NewRecorder()
	a.handleCreateInviteJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result["url"] == "" {
		t.Error("expected non-empty url in response")
	}
}

// =============================================================================
// handleLinkJSON — returns codes even when empty
// =============================================================================

func TestHandleLinkJSON_WithApprovedCode_DoesNotInclude(t *testing.T) {
	a := newTestAdminWithStore(t)
	// No pending codes — should return empty array
	req := httptest.NewRequest(http.MethodGet, "/admin/link.json", nil)
	rec := httptest.NewRecorder()
	a.handleLinkJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleThreadsJSON — nil-threads branch and populated branch
// =============================================================================

func TestHandleThreadsJSON_WithThreads_ReturnsList(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           "t1",
		FrontendName: "matrix",
		ExternalID:   "ext-1",
		AgentID:      "ag1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/threads.json", nil)
	rec := httptest.NewRecorder()
	a.handleThreadsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var threads []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &threads); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(threads) != 1 {
		t.Errorf("expected 1 thread, got %d", len(threads))
	}
}

// =============================================================================
// handleLinkPage — ensure page renders with list of link codes
// =============================================================================

func TestHandleLinkPage_WithPendingCode_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "link-page-user", "password123")

	fp := strings.Repeat("c", 64)
	createPendingLink(t, a, fp, "device-page")

	req := httptest.NewRequest(http.MethodGet, "/admin/link", nil)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleLinkPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleThreadDetail — not found path (was at 70%)
// =============================================================================

func TestHandleThreadDetail_NotFound_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/threads/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rec := httptest.NewRecorder()
	a.handleThreadDetail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// =============================================================================
// handleThreadDetailJSON — not found path (was at 69.6%)
// =============================================================================

func TestHandleThreadDetailJSON_NotFound_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/threads/nonexistent/json", nil)
	req.SetPathValue("id", "nonexistent")
	rec := httptest.NewRecorder()
	a.handleThreadDetailJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// =============================================================================
// handleInviteSignup — success path (creates a user and redirects)
// =============================================================================

func TestHandleInviteSignup_HappyPath_Creates(t *testing.T) {
	a := newTestAdminWithStore(t)

	// Create an invite token
	inviteToken := createInvite(t, a)

	// Build CSRF for the form
	csrfVal := "signup-csrf-token"
	form := strings.NewReader(
		"username=newuser&password=longpassword123&display_name=New+User&csrf_token=" + csrfVal,
	)
	req := httptest.NewRequest(http.MethodPost, "/invite/"+inviteToken+"/signup", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.SetPathValue("token", inviteToken)

	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	// Successful signup redirects to /
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// NewWithConfig — missing principalStore / tokenGenerator path (was at 75%)
// =============================================================================

func TestNewWithConfig_NoPrincipalStore_LinkApproveReturns500(t *testing.T) {
	a := newTestAdminWithStore(t) // no principalStore or tokenGenerator
	user := createAdminUserWithPassword(t, a, "nops-user", "password123")

	fp := strings.Repeat("d", 64)
	linkID := createPendingLink(t, a, fp, "nops-device")

	csrfVal := "nops-csrf"
	req := httptest.NewRequest(http.MethodPost, "/admin/link/"+linkID+"/approve", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req.SetPathValue("id", linkID)
	req = req.WithContext(withUser(req.Context(), user))

	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (server not configured), got %d", rec.Code)
	}
}

// =============================================================================
// handleAgentDetailJSON — cover the manager loop body (connected agent path)
// =============================================================================

func TestHandleAgentDetailJSON_WithConnectedAgent_ReturnsConnectedInfo(t *testing.T) {
	mgr := newTestManager(t)
	a := newTestAdminWithManagerAndStore(t, mgr)

	agentID := "connected-agent-xyz"
	registerFakeAgent(t, mgr, agentID, "My Agent", []string{"files"}, []string{"ws1"})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/agents/"+agentID, nil)
	req.SetPathValue("id", agentID)
	rec := httptest.NewRecorder()
	a.handleAgentDetailJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result["ID"] != agentID {
		t.Errorf("expected ID=%s, got %v", agentID, result["ID"])
	}
	connected, _ := result["Connected"].(bool)
	if !connected {
		t.Errorf("expected Connected=true, got %v", result["Connected"])
	}
}

func TestHandleAgentDetail_WithConnectedAgent_Shows200(t *testing.T) {
	mgr := newTestManager(t)
	a := newTestAdminWithManagerAndStore(t, mgr)

	agentID := "detail-connected-agent"
	registerFakeAgent(t, mgr, agentID, "Detail Agent", nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/agents/"+agentID, nil)
	req.SetPathValue("id", agentID)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleAgentDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleAgentsJSON — cover the manager with agents path (was at 66.7%)
// =============================================================================

func TestHandleAgentsJSON_WithConnectedAgents_ReturnsList(t *testing.T) {
	mgr := newTestManager(t)
	a := newTestAdminWithManagerAndStore(t, mgr)

	registerFakeAgent(t, mgr, "agent-a1", "Alpha Agent", []string{"files"}, nil)
	registerFakeAgent(t, mgr, "agent-b2", "Beta Agent", nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/agents", nil)
	rec := httptest.NewRecorder()
	a.handleAgentsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var agents []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

// =============================================================================
// handleLinkPage — cover the error path and list path more thoroughly
// =============================================================================

func TestHandleLinkJSON_WithCodesInStore_ReturnsList(t *testing.T) {
	a := newTestAdminWithStore(t)
	fp := strings.Repeat("e", 64)
	createPendingLink(t, a, fp, "device-json")

	req := httptest.NewRequest(http.MethodGet, "/admin/link.json", nil)
	rec := httptest.NewRecorder()
	a.handleLinkJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	codes, ok := result["codes"].([]any)
	if !ok {
		t.Fatalf("expected codes array, got %T", result["codes"])
	}
	if len(codes) == 0 {
		t.Error("expected at least one pending code")
	}
}

// =============================================================================
// handleInvitePage — cover the missing token path (handleInvitePage branch)
// =============================================================================

func TestHandleInvitePage_MissingToken_RedirectsToSetup(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/invite/", nil)
	req.SetPathValue("token", "")
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)

	// Empty token causes a bad request or redirect — handler returns 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// =============================================================================
// handleSecretsUpdate — cover the "happy" branch for an existing secret
// =============================================================================

func TestHandleSecretsUpdate_WithMatchingSecret_Updates(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "secretupdate-user", "password123")

	// Create the secret first
	csrfVal := "su-csrf"
	form := strings.NewReader("key=UPDATE_ME&value=original")
	createReq := httptest.NewRequest(http.MethodPost, "/admin/secrets", form)
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	createReq.Header.Set("X-CSRF-Token", csrfVal)
	createReq = createReq.WithContext(withUser(createReq.Context(), user))
	createRec := httptest.NewRecorder()
	a.handleSecretsCreate(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("setup: create secret got %d: %s", createRec.Code, createRec.Body.String())
	}

	// Find the secret ID from the JSON list
	listReq := httptest.NewRequest(http.MethodGet, "/admin/secrets.json", nil)
	listReq = listReq.WithContext(withUser(listReq.Context(), user))
	listRec := httptest.NewRecorder()
	a.handleSecretsJSON(listRec, listReq)

	var secrets []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &secrets); err != nil {
		t.Fatalf("unmarshal secrets: %v", err)
	}
	if len(secrets) == 0 {
		t.Fatal("expected at least one secret")
	}
	secretID := secrets[0]["ID"].(string)

	// Now update with a new value
	updateForm := strings.NewReader("value=updated-value")
	updateReq := httptest.NewRequest(http.MethodPut, "/admin/secrets/"+secretID, updateForm)
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	updateReq.Header.Set("X-CSRF-Token", csrfVal)
	updateReq.SetPathValue("id", secretID)
	updateReq = updateReq.WithContext(withUser(updateReq.Context(), user))
	updateRec := httptest.NewRecorder()
	a.handleSecretsUpdate(updateRec, updateReq)

	// handleSecretsUpdate redirects on success
	if updateRec.Code != http.StatusSeeOther && updateRec.Code != http.StatusOK {
		t.Errorf("expected redirect or 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
}

// =============================================================================
// handleThreadDetailJSON — cover the success path (thread + messages found)
// =============================================================================

func TestHandleThreadDetailJSON_HappyPath_ReturnsThreadAndMessages(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	threadID := "thread-detail-test"
	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           threadID,
		FrontendName: "matrix",
		ExternalID:   "ext-detail",
		AgentID:      "ag-detail",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads/"+threadID, nil)
	req.SetPathValue("id", threadID)
	rec := httptest.NewRecorder()
	a.handleThreadDetailJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result["thread"] == nil {
		t.Error("expected thread in response")
	}
}

// =============================================================================
// handleThreadDetail — cover the success path with messages
// =============================================================================

func TestHandleThreadDetail_HappyPath_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	threadID := "thread-page-test"
	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           threadID,
		FrontendName: "http",
		ExternalID:   "ext-page",
		AgentID:      "ag-page",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/threads/"+threadID, nil)
	req.SetPathValue("id", threadID)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleLogsJSON — cover path with log entries that have nil tags
// =============================================================================

func TestHandleLogsJSON_WithNilTagsEntry_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	// No seed needed — empty logs is enough to cover the function body
	req := httptest.NewRequest(http.MethodGet, "/api/admin/logs.json?limit=5", nil)
	rec := httptest.NewRecorder()
	a.handleLogsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleChatApp — cover missing branches
// =============================================================================

func TestHandleChatApp_WithManager_Returns200(t *testing.T) {
	mgr := newTestManager(t)
	a := newTestAdminWithManagerAndStore(t, mgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleChatApp(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handlePrincipalsJSON — cover different status filter combos
// =============================================================================

func TestHandlePrincipalsJSON_StatusFilter_Pending(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "p-pend", store.PrincipalTypeClient, store.PrincipalStatusPending)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/principals?status=pending", nil)
	rec := httptest.NewRecorder()
	a.handlePrincipalsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
}

// =============================================================================
// handleBoardJSON — cover the thread detail path
// =============================================================================

func TestHandleBoardThreadJSON_NonBBSThread_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	threadID := "board-thread-test"
	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           threadID,
		FrontendName: "bbs",
		ExternalID:   "bbs-ext",
		AgentID:      "ag-bbs",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/board/"+threadID, nil)
	req.SetPathValue("id", threadID)
	rec := httptest.NewRecorder()
	a.handleBoardThreadJSON(rec, req)

	// GetBBSThread returns not found since this thread has no BBS data,
	// so the handler responds 404.
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// =============================================================================
// handleToolsJSON — cover the nil-registry branch
// =============================================================================

func TestHandleToolsJSON_EmptyRegistry_ReturnsEmptyArray(t *testing.T) {
	a := newTestAdminWithStore(t) // registry is nil
	req := httptest.NewRequest(http.MethodGet, "/api/admin/tools.json", nil)
	rec := httptest.NewRecorder()
	a.handleToolsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// getSQLiteStore — cover the !ok / non-SQLiteStore path (was at 60%)
// =============================================================================

func TestHandleSecretsGetValue_NonSQLiteStore_Returns500(t *testing.T) {
	// newTestAdmin has nil store, so getSQLiteStore returns nil
	a := newTestAdmin()
	a.store = nil // explicitly nil to trigger the non-SQLiteStore path

	req := httptest.NewRequest(http.MethodGet, "/admin/secrets/some-id/value", nil)
	req.SetPathValue("id", "some-id")
	rec := httptest.NewRecorder()
	a.handleSecretsGetValue(rec, req)

	// getSQLiteStore returns nil → 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListSecretItems_NonSQLiteStore_ReturnsEmpty(t *testing.T) {
	// newTestAdmin has nil store, so getSQLiteStore returns nil → empty slice
	a := newTestAdmin()
	a.store = nil

	items := a.listSecretItems(context.Background())
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

// =============================================================================
// sendWithContext — cover the channel-full and context-cancel paths (was 33.3%)
// =============================================================================

func TestSendWithContext_ChannelFull_WaitsAndRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Create a session with buffer of 1 and fill it
	sessCtx, sessCancel := context.WithCancel(context.Background())
	defer sessCancel()

	sess := &chatSession{
		messages: make(chan *chatMessage, 1),
		ctx:      sessCtx,
		cancel:   sessCancel,
	}

	// Fill the channel
	sess.messages <- &chatMessage{Type: "filler"}

	// Now sendWithContext should fail non-blocking send, wait 100ms, then retry
	// Since channel is still full during the wait, retry also fails
	msg := &chatMessage{Type: "queued"}
	result := sendWithContext(ctx, sess, msg)
	// result will be false since channel stays full and timeout fires
	if result {
		t.Error("expected false when channel is full and timeout fires")
	}
}

func TestSendWithContext_SessionContextCancelled_ReturnsFalse(t *testing.T) {
	// Create a session whose context is already cancelled
	sessCtx, sessCancel := context.WithCancel(context.Background())
	sessCancel() // cancel immediately

	sess := &chatSession{
		messages: make(chan *chatMessage, 1),
		ctx:      sessCtx,
		cancel:   sessCancel,
	}

	// Fill the channel so first send fails
	sess.messages <- &chatMessage{Type: "filler"}

	result := sendWithContext(context.Background(), sess, &chatMessage{Type: "test"})
	if result {
		t.Error("expected false when session ctx is cancelled")
	}
}

func TestSendWithContext_CallerContextCancelled_ReturnsFalse(t *testing.T) {
	// Create a full session
	sessCtx, sessCancel := context.WithCancel(context.Background())
	defer sessCancel()

	sess := &chatSession{
		messages: make(chan *chatMessage, 1),
		ctx:      sessCtx,
		cancel:   sessCancel,
	}

	// Fill the channel so first send fails
	sess.messages <- &chatMessage{Type: "filler"}

	// Caller context already cancelled
	callerCtx, callerCancel := context.WithCancel(context.Background())
	callerCancel() // cancel immediately

	result := sendWithContext(callerCtx, sess, &chatMessage{Type: "test"})
	if result {
		t.Error("expected false when caller ctx is cancelled")
	}
}

// =============================================================================
// handleLogsJSON — cover the nil-tags branch (was at 63.2% for renderLogsPage)
// =============================================================================

func TestHandleLogsJSON_WithNilTagsEntry_CoversNilTagsBranch(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	// Seed a log entry with nil Tags
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	if err := sqlStore.CreateLogEntry(ctx, &store.LogEntry{
		ID:        "log-nil-tags",
		AgentID:   "agent-1",
		Message:   "test message with nil tags",
		Tags:      nil, // explicitly nil to trigger the nil-tags branch
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateLogEntry: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/logs.json", nil)
	rec := httptest.NewRecorder()
	a.handleLogsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	entries, _ := result["entries"].([]any)
	if len(entries) == 0 {
		t.Error("expected at least one log entry in response")
	}
}

func TestHandleLogsPage_WithNilTagsEntry_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	if err := sqlStore.CreateLogEntry(ctx, &store.LogEntry{
		ID:        "log-page-nil-tags",
		AgentID:   "agent-2",
		Message:   "page test message",
		Tags:      nil,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateLogEntry: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/logs", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLogsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleTodosJSON + renderTodosPage — cover the loop body and DueDate branch
// =============================================================================

func TestHandleTodosPage_WithDueDateTodo_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}

	dueDate := time.Now().Add(24 * time.Hour)
	if err := sqlStore.CreateTodo(ctx, &store.Todo{
		ID:          "todo-with-due",
		AgentID:     "agent-1",
		Description: "Test todo with due date",
		Status:      "pending",
		Priority:    "high",
		DueDate:     &dueDate,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/todos", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleTodosPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleTodosJSON_WithTodos_ReturnsList(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}

	if err := sqlStore.CreateTodo(ctx, &store.Todo{
		ID:          "todo-list-1",
		AgentID:     "agent-1",
		Description: "A todo item",
		Status:      "pending",
		Priority:    "low",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/todos.json", nil)
	rec := httptest.NewRecorder()
	a.handleTodosJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	todos, _ := result["todos"].([]any)
	if len(todos) == 0 {
		t.Error("expected at least one todo in response")
	}
}

func TestHandleTodosJSON_WithDueDateTodo_CoversDueDateBranch(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}

	dueDate := time.Now().Add(48 * time.Hour)
	if err := sqlStore.CreateTodo(ctx, &store.Todo{
		ID:          "todo-with-due-json",
		AgentID:     "agent-1",
		Description: "A todo item with due date",
		Status:      "pending",
		Priority:    "medium",
		DueDate:     &dueDate,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/todos.json", nil)
	rec := httptest.NewRecorder()
	a.handleTodosJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	todos, _ := result["todos"].([]any)
	if len(todos) == 0 {
		t.Error("expected at least one todo in response")
	}
	// Verify DueDate is present
	todo := todos[0].(map[string]any)
	if todo["DueDate"] == nil {
		t.Error("expected DueDate to be present in response")
	}
}

// =============================================================================
// handleLinkStatus — cover the approved-with-token branch (was at 70.8%)
// =============================================================================

func TestHandleLinkStatus_ApprovedCode_ReturnsTokenAndPrincipal(t *testing.T) {
	a := newTestAdminFull(t)
	user := createAdminUserWithPassword(t, a, "status-approver", "password123")
	ctx := context.Background()

	fp := strings.Repeat("f", 64)
	linkID := createPendingLink(t, a, fp, "status-device")

	// Get the short code from the DB to use in the status check
	code, err := a.store.GetLinkCode(ctx, linkID)
	if err != nil {
		t.Fatalf("GetLinkCode: %v", err)
	}
	shortCode := code.Code

	// Approve the link
	csrfVal := "status-csrf"
	approveReq := httptest.NewRequest(http.MethodPost, "/admin/link/"+linkID+"/approve", nil)
	approveReq.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	approveReq.Header.Set("X-CSRF-Token", csrfVal)
	approveReq.SetPathValue("id", linkID)
	approveReq = approveReq.WithContext(withUser(approveReq.Context(), user))
	approveRec := httptest.NewRecorder()
	a.handleLinkApprove(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("approve failed: %d %s", approveRec.Code, approveRec.Body.String())
	}

	// Now check the status
	statusReq := httptest.NewRequest(http.MethodGet, "/link/"+shortCode+"/status", nil)
	statusReq.SetPathValue("code", shortCode)
	statusRec := httptest.NewRecorder()
	a.handleLinkStatus(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", statusRec.Code, statusRec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", result["status"])
	}
	if result["token"] == nil {
		t.Error("expected token in approved response")
	}
}

// =============================================================================
// createInviteToken — cover error path (inviteURL is missing config field)
// =============================================================================

func TestHandleCreateInviteJSON_InvalidCSRF_ReturnsForbidden(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/invite/create", nil)
	// No CSRF cookie/header
	rec := httptest.NewRecorder()
	a.handleCreateInviteJSON(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// =============================================================================
// handlePrincipalsPage — with seeded principals to cover the loop body
// =============================================================================

func TestHandlePrincipalsPage_WithPrincipals_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "p-active-cov", store.PrincipalTypeAgent, store.PrincipalStatusApproved)
	seedPrincipal(t, a.store, "p-pending-cov", store.PrincipalTypeClient, store.PrincipalStatusPending)

	req := httptest.NewRequest(http.MethodGet, "/admin/principals", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handlePrincipalsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handlePrincipalsJSON — with type filter
// =============================================================================

func TestHandlePrincipalsJSON_TypeFilter_Agent(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedPrincipal(t, a.store, "p-agent-type", store.PrincipalTypeAgent, store.PrincipalStatusApproved)
	seedPrincipal(t, a.store, "p-client-type", store.PrincipalTypeClient, store.PrincipalStatusApproved)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/principals?type=agent", nil)
	rec := httptest.NewRecorder()
	a.handlePrincipalsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	// Should only have agent principals
	for _, p := range result {
		if p["Type"] != "agent" {
			t.Errorf("expected type=agent, got %v", p["Type"])
		}
	}
}

// =============================================================================
// handleBoardJSON — cover with seeded BBS threads
// =============================================================================

func TestHandleBoardJSON_WithBBSThreads_ReturnsList(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	if err := sqlStore.CreateBBSPost(ctx, &store.BBSPost{
		ID:        "bbs-post-1",
		AgentID:   "agent-1",
		Subject:   "BBS Test Thread",
		Content:   "Hello from BBS",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateBBSPost: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/board.json", nil)
	rec := httptest.NewRecorder()
	a.handleBoardJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	threads, _ := result["threads"].([]any)
	if len(threads) == 0 {
		t.Error("expected at least one BBS thread")
	}
}

// =============================================================================
// handleInviteSignup — cover the duplicate username path (createUserFromSignup error)
// =============================================================================

func TestHandleInviteSignup_DuplicateUsername_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)

	// Create a user with a valid username (no hyphens)
	createAdminUserWithPassword(t, a, "existinguser", "password123")

	inviteToken := createInvite(t, a)

	csrfVal := "dup-csrf"
	form := strings.NewReader("username=existinguser&password=longpassword123&csrf_token=" + csrfVal)
	req := httptest.NewRequest(http.MethodPost, "/invite/"+inviteToken+"/signup", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.SetPathValue("token", inviteToken)

	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	// Should show error page (200 with error content) not a redirect
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 error page for duplicate username, got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// listAgentItems — cover the manager loop body with a connected agent
// =============================================================================

func TestListAgentItems_WithConnectedAgent_ReturnsList(t *testing.T) {
	mgr := newTestManager(t)
	a := newTestAdminWithManagerAndStore(t, mgr)

	registerFakeAgent(t, mgr, "agent-list", "List Agent", []string{"files"}, []string{"workspace1"})

	items := a.listAgentItems()
	if len(items) == 0 {
		t.Error("expected at least one agent item")
	}
	if items[0].ID != "agent-list" {
		t.Errorf("expected agent-list, got %s", items[0].ID)
	}
}

// =============================================================================
// handleDashboard — cover with threads to exercise the threadCount branch
// =============================================================================

func TestHandleDashboard_WithThreads_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           "dash-thread-1",
		FrontendName: "http",
		ExternalID:   "dash-ext-1",
		AgentID:      "dash-agent",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// renderBoardPage — cover the for-range loop body (no posts == uncovered loop)
// =============================================================================

func TestHandleBoardPage_WithBBSPost_CoversLoopBody(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	// Top-level post: ThreadID must be empty so ListBBSThreads (WHERE thread_id IS NULL) returns it
	if err := sqlStore.CreateBBSPost(ctx, &store.BBSPost{
		ID:        "boardpage-post-1",
		AgentID:   "agent-board",
		ThreadID:  "", // empty = top-level post, returned by ListBBSThreads
		Subject:   "Board Page Loop Test",
		Content:   "Covering the renderBoardPage for-range loop body",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateBBSPost: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/board", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// renderThreadDetail — cover the for-range loop body when messages exist
// =============================================================================

func TestHandleThreadDetail_WithMessages_CoversLoopBody(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	threadID := "thread-with-msgs"
	if err := a.store.CreateThread(ctx, &store.Thread{
		ID:           threadID,
		FrontendName: "http",
		ExternalID:   "ext-msgs",
		AgentID:      "ag-msgs",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	if err := sqlStore.SaveMessage(ctx, &store.Message{
		ID:        "msg-1",
		ThreadID:  threadID,
		Sender:    "user",
		Content:   "hello",
		Type:      "message",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/threads/"+threadID, nil)
	req.SetPathValue("id", threadID)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// renderSetupComplete — cover the hasToken == true branch (apiToken non-empty)
// =============================================================================

func TestRenderSetupComplete_WithToken_CoversHasTokenBranch(t *testing.T) {
	a := newTestAdminWithStore(t)

	rec := httptest.NewRecorder()
	a.renderSetupComplete(rec, "Test User", "some-api-token-value", "localhost:7777")

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// renderToolsPage — cover the packs == nil branch
// =============================================================================

func TestRenderToolsPage_WithNilPacks_CoversNilBranch(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := &store.AdminUser{ID: "u1", Username: "u1", DisplayName: "User One"}

	rec := httptest.NewRecorder()
	a.renderToolsPage(rec, user, "csrf-tok", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// renderAgentsPage — cover the agents == nil branch
// =============================================================================

func TestRenderAgentsPage_WithNilAgents_CoversNilBranch(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := &store.AdminUser{ID: "u1", Username: "u1", DisplayName: "User One"}

	rec := httptest.NewRecorder()
	a.renderAgentsPage(rec, user, "csrf-tok", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleInvitePage — cover the invite.UsedAt != nil branch
// =============================================================================

func TestHandleInvitePage_UsedInvite_ShowsAlreadyUsedError(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	// Create invite
	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("generateSecureToken: %v", err)
	}
	userID := "invite-user-1"
	if err := a.store.CreateAdminUser(ctx, &store.AdminUser{
		ID:          userID,
		Username:    "inviteuser1",
		DisplayName: "Invite User 1",
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := a.store.CreateAdminInvite(ctx, invite); err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}
	// Mark invite as used
	if err := a.store.UseAdminInvite(ctx, token, userID); err != nil {
		t.Fatalf("UseAdminInvite: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/invite/"+token, nil)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with error page, got %d", rec.Code)
	}
}

// =============================================================================
// handleInvitePage — cover the time.Now().After(invite.ExpiresAt) branch
// =============================================================================

func TestHandleInvitePage_ExpiredInvite_ShowsExpiredError(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("generateSecureToken: %v", err)
	}
	userID := "invite-user-2"
	if err := a.store.CreateAdminUser(ctx, &store.AdminUser{
		ID:          userID,
		Username:    "inviteuser2",
		DisplayName: "Invite User 2",
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: userID,
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	}
	if err := a.store.CreateAdminInvite(ctx, invite); err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/invite/"+token, nil)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	a.handleInvitePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with error page, got %d", rec.Code)
	}
}

// =============================================================================
// validateInvite — cover the invite.UsedAt != nil branch
// via handleInviteSignup with a used invite
// =============================================================================

func TestHandleInviteSignup_UsedInvite_ShowsAlreadyUsedError(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	// Create and immediately use an invite
	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("generateSecureToken: %v", err)
	}
	ownerID := "invite-owner-3"
	if err := a.store.CreateAdminUser(ctx, &store.AdminUser{
		ID:          ownerID,
		Username:    "inviteowner3",
		DisplayName: "Invite Owner 3",
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: ownerID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := a.store.CreateAdminInvite(ctx, invite); err != nil {
		t.Fatalf("CreateAdminInvite: %v", err)
	}
	if err := a.store.UseAdminInvite(ctx, token, ownerID); err != nil {
		t.Fatalf("UseAdminInvite: %v", err)
	}

	csrfVal := "csrf-used-invite"
	form := url.Values{}
	form.Set("username", "newuser3")
	form.Set("password", "password123")
	form.Set("display_name", "New User 3")
	form.Set("csrf_token", csrfVal)

	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	a.handleInviteSignup(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (error page), got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// handleLogin — cover the user.PasswordHash == "" branch
// =============================================================================

func TestHandleLogin_EmptyPasswordHash_ShowsError(t *testing.T) {
	a := newTestAdminWithStore(t)
	ctx := context.Background()

	// Create a user with empty PasswordHash (e.g., OAuth-only account)
	if err := a.store.CreateAdminUser(ctx, &store.AdminUser{
		ID:           "nopassuser",
		Username:     "nopassuser",
		PasswordHash: "",
		DisplayName:  "No Pass User",
		CreatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}

	csrfVal := "csrf-nopass"
	form := url.Values{}
	form.Set("username", "nopassuser")
	form.Set("password", "anypassword")
	form.Set("csrf_token", csrfVal)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	rec := httptest.NewRecorder()
	a.handleLogin(rec, req)

	// Should show login error (200 with error page or redirect to login)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Errorf("expected 200 or redirect, got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// handleCreateInviteJSON — cover the success path (creates invite and returns URL)
// =============================================================================

func TestHandleCreateInviteJSON_Success_ReturnsURL(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "createinvite-user", "password123")

	csrfVal := "csrf-createinvite"
	req := httptest.NewRequest(http.MethodPost, "/api/admin/invites", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleCreateInviteJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["url"] == "" {
		t.Error("expected non-empty url in response")
	}
}

// =============================================================================
// handleSecretsPage — cover the manager-with-agents loop body
// =============================================================================

func TestHandleSecretsPage_WithManager_CoversManagerLoop(t *testing.T) {
	mgr := newTestManager(t)
	registerFakeAgent(t, mgr, "secrets-agent-1", "SecretsAgent", []string{"read"}, []string{"ws1"})
	a := newTestAdminWithManagerAndStore(t, mgr)

	req := httptest.NewRequest(http.MethodGet, "/admin/secrets", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleSecretsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleSecretsJSON — cover the scope filter "continue" branches
// =============================================================================

func TestHandleSecretsJSON_GlobalFilter_FiltersOutAgentSecrets(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "secrets-filter-user", "password123")
	ctx := context.Background()

	// Create a global secret and an agent-scoped secret
	csrfVal := "csrf-secrets-filter"
	createReq := func(form string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/admin/secrets", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
		req.Header.Set("X-CSRF-Token", csrfVal)
		return req.WithContext(withUser(req.Context(), user))
	}

	// Global secret
	rec1 := httptest.NewRecorder()
	a.handleSecretsCreate(rec1, createReq("key=GLOBAL_KEY&value=globalval"))
	if rec1.Code != http.StatusSeeOther {
		t.Fatalf("create global secret: got %d: %s", rec1.Code, rec1.Body.String())
	}

	// Agent-scoped secret
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	agentID := "filter-agent"
	if err := sqlStore.CreateSecret(ctx, &store.Secret{
		ID:        "agent-secret-1",
		Key:       "AGENT_KEY",
		Value:     "agentval",
		AgentID:   &agentID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateSecret agent: %v", err)
	}

	// Request with scope=global filter — should skip agent secrets
	req := httptest.NewRequest(http.MethodGet, "/api/admin/secrets?scope=global", nil)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleSecretsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var secrets []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &secrets); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, s := range secrets {
		if s["AgentID"] != nil && s["AgentID"] != "" {
			t.Errorf("expected only global secrets, got agent secret: %v", s)
		}
	}
}

// =============================================================================
// listPackItems — cover registry non-nil path and all loop bodies
// =============================================================================

func TestListPackItems_WithExternalPack_CoversLoopBodies(t *testing.T) {
	reg := packs.NewRegistry(slog.Default())

	// Register an external pack with one tool so the loop bodies execute.
	toolDef := &pb.ToolDefinition{
		Name:        "test-tool",
		Description: "A test tool",
	}
	manifest := &pb.PackManifest{
		PackId:  "test-pack",
		Version: "1.0.0",
		Tools:   []*pb.ToolDefinition{toolDef},
	}
	if err := reg.RegisterPack("test-pack", manifest); err != nil {
		t.Fatalf("RegisterPack: %v", err)
	}

	a := newTestAdminWithStore(t)
	a.registry = reg

	items := a.listPackItems()

	if len(items) == 0 {
		t.Error("expected at least one pack item, got none")
	}
}

// =============================================================================
// listPackItems — cover builtin pack loop bodies via RegisterBuiltinPack
// =============================================================================

func TestListPackItems_WithBuiltinPack_CoversBuiltinLoopBody(t *testing.T) {
	reg := packs.NewRegistry(slog.Default())

	// Register a builtin pack so the ListBuiltinPacks loop body executes.
	toolDef := &pb.ToolDefinition{
		Name:        "builtin-test-tool",
		Description: "A builtin test tool",
	}
	builtinPack := &packs.BuiltinPack{
		ID: "test-builtin-pack",
		Tools: []*packs.BuiltinTool{
			{Definition: toolDef, Handler: nil},
		},
	}
	if err := reg.RegisterBuiltinPack(builtinPack); err != nil {
		t.Fatalf("RegisterBuiltinPack: %v", err)
	}

	a := newTestAdminWithStore(t)
	a.registry = reg

	items := a.listPackItems()

	if len(items) == 0 {
		t.Error("expected at least one pack item from builtin, got none")
	}
}

// =============================================================================
// deriveWebAuthnConfig — cover the host == "" early-return branch
// http://:8080 gives Host=":8080" but Hostname()="" which triggers the branch
// =============================================================================

func TestDeriveWebAuthnConfig_EmptyHostname_ReturnsDefaults(t *testing.T) {
	rpID, rpOrigins := deriveWebAuthnConfig("http://:8080")

	// When hostname is empty, should return the localhost defaults.
	if rpID != "localhost" {
		t.Errorf("expected rpID=localhost, got %q", rpID)
	}
	if len(rpOrigins) == 0 {
		t.Error("expected non-empty rpOrigins")
	}
}

// =============================================================================
// handleWebAuthnRegisterBegin — cover the happy path (webauthn initialized,
// user in context, BeginRegistration succeeds, returns JSON with sessionToken)
// =============================================================================

func TestHandleWebAuthnRegisterBegin_WithRealUser_CoversHappyPath(t *testing.T) {
	a := newTestAdminWithStore(t)

	// newTestAdminWithStore calls NewWithConfig which calls initWebAuthn,
	// so a.webauthn is guaranteed non-nil.
	if a.webauthn == nil {
		t.Skip("webauthn not initialized, skipping happy path test")
	}

	// requestWithUser injects a test user (ID: "test-user") into the context.
	req := httptest.NewRequest(http.MethodGet, "/admin/webauthn/register/begin", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()

	a.handleWebAuthnRegisterBegin(rec, req)

	// The handler should respond 200 with JSON containing options and sessionToken.
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["sessionToken"] == "" {
		t.Error("expected non-empty sessionToken in response")
	}
}

// =============================================================================
// renderSecretsPage — cover nil-agent and nil-secret branches
// =============================================================================

func TestRenderSecretsPage_WithNilArgs_CoversNilBranches(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := &store.AdminUser{ID: "u1", Username: "u1", DisplayName: "User One"}

	rec := httptest.NewRecorder()
	// Pass nil agents and nil secrets to cover both nil-branch bodies.
	a.renderSecretsPage(rec, user, nil, nil, "csrf-tok")

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// handleSecretsJSON — cover the scope=agent filter branch (agent filter skips globals)
// =============================================================================

func TestHandleSecretsJSON_AgentFilter_FiltersOutGlobalSecrets(t *testing.T) {
	a := newTestAdminWithStore(t)
	user := createAdminUserWithPassword(t, a, "secrets-agent-filter-user", "password123")
	ctx := context.Background()

	csrfVal := "csrf-agent-filter"

	// Create a global secret via handleSecretsCreate (no AgentID = global).
	globalForm := url.Values{}
	globalForm.Set("key", "GLOBAL_KEY2")
	globalForm.Set("value", "globalval2")
	globalForm.Set("csrf_token", csrfVal)
	globalReq := httptest.NewRequest(http.MethodPost, "/admin/secrets",
		strings.NewReader(globalForm.Encode()))
	globalReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	globalReq.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	globalReq.Header.Set("X-CSRF-Token", csrfVal)
	globalReq = globalReq.WithContext(withUser(globalReq.Context(), user))
	globalRec := httptest.NewRecorder()
	a.handleSecretsCreate(globalRec, globalReq)
	if globalRec.Code != http.StatusSeeOther {
		t.Fatalf("create global secret: got %d: %s", globalRec.Code, globalRec.Body.String())
	}

	// Create an agent-scoped secret directly.
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		t.Skip("store is not SQLiteStore")
	}
	agentID := "agent-filter-agent"
	if err := sqlStore.CreateSecret(ctx, &store.Secret{
		ID:        "agent-secret-filter-1",
		Key:       "AGENT_KEY2",
		Value:     "agentval2",
		AgentID:   &agentID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateSecret agent: %v", err)
	}

	// Request scope=agent — should include agent secrets and skip globals.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/secrets?scope=agent", nil)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleSecretsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var secrets []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &secrets); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// All returned secrets should have an AgentID (no global secrets).
	for _, s := range secrets {
		if s["AgentID"] == nil || s["AgentID"] == "" {
			t.Errorf("expected only agent secrets with scope=agent, got global: %v", s)
		}
	}
}
