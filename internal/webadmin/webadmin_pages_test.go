// ABOUTME: Tests for webadmin route registration and read-only page/JSON handlers.
// ABOUTME: Covers RegisterRoutes, dashboard, threads, board, logs, usage, and todos handlers.

package webadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
)

// --- Route registration ---

func TestRegisterRoutes_AuthRequired_RedirectsToLogin(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "routeuser", "password1")

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	// Unauthenticated request to admin page should redirect to login
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect to login, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected Location /login, got %q", loc)
	}
}

func TestRegisterRoutes_PublicRoutes_Accessible(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "routeuser2", "password1")

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	// Login page should be public
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /login, got %d", rec.Code)
	}
}

func TestRegisterRoutes_HealthStream_Accessible(t *testing.T) {
	a := newTestAdminWithStore(t)
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	// Health stream should be public
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/health/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// It's a streaming endpoint; expect 200 or context-cancel (not 404 or 403)
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusForbidden {
		t.Errorf("expected health stream to be accessible, got %d", rec.Code)
	}
}

func TestRegisterRootRoutes_RegistersLoginRoute(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "routeuser3", "password1")

	mux := http.NewServeMux()
	a.registerRootRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /login via registerRootRoutes, got %d", rec.Code)
	}
}

func TestRegisterAdminRoutes_RegistersDashboardRoute(t *testing.T) {
	a := newTestAdminWithStore(t)
	createAdminUserWithPassword(t, a, "routeuser4", "password1")
	sessionVal := buildLoginSession(t, a, "routeuser4", "password1")

	mux := http.NewServeMux()
	a.registerAdminRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionVal})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /admin/ via registerAdminRoutes, got %d", rec.Code)
	}
}

// --- handleDashboard ---

func TestHandleDashboard_EmptyStore_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// --- handleDashboardJSON ---

func TestHandleDashboardJSON_Returns200WithExpectedFields(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleDashboardJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	for _, field := range []string{"agentCount", "packCount", "threadCount", "usage"} {
		if _, ok := resp[field]; !ok {
			t.Errorf("expected field %q in dashboard JSON", field)
		}
	}
}

// --- handleThreadsPage / handleThreadsJSON ---

func seedTestThread(t *testing.T, s FullStore, id, _ string) {
	t.Helper()
	thread := &store.Thread{
		ID:           id,
		FrontendName: "test-frontend",
		ExternalID:   "ext-" + id,
		AgentID:      "agent-001",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := s.CreateThread(context.Background(), thread); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
}

func TestHandleThreadsPage_EmptyStore_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/threads", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleThreadsJSON_EmptyStore_ReturnsEmptyArray(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var threads []any
	if err := json.Unmarshal(rec.Body.Bytes(), &threads); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("expected empty array, got %d threads", len(threads))
	}
}

func TestHandleThreadsJSON_WithThread_ContainsThreadID(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedTestThread(t, a.store, "test-thread-1", "Test Thread")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "test-thread-1") {
		t.Errorf("expected thread ID in response, got %q", rec.Body.String())
	}
}

// --- handleThreadDetail / handleThreadDetailJSON ---

func TestHandleThreadDetail_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/threads/no-such-thread", nil)
	req.SetPathValue("id", "no-such-thread")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleThreadDetail_KnownID_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedTestThread(t, a.store, "known-thread", "Known Thread")

	req := httptest.NewRequest(http.MethodGet, "/admin/threads/known-thread", nil)
	req.SetPathValue("id", "known-thread")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleThreadDetailJSON_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads/no-such", nil)
	req.SetPathValue("id", "no-such")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetailJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleThreadDetailJSON_KnownID_Returns200WithFields(t *testing.T) {
	a := newTestAdminWithStore(t)
	seedTestThread(t, a.store, "detail-thread", "Detail Thread")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads/detail-thread", nil)
	req.SetPathValue("id", "detail-thread")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetailJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["thread"]; !ok {
		t.Error("expected 'thread' field in response")
	}
	if _, ok := resp["messages"]; !ok {
		t.Error("expected 'messages' field in response")
	}
}

// --- handleLogsPage / handleLogsJSON ---

func TestHandleLogsPage_EmptyStore_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/logs", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLogsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func seedLogEntry(t *testing.T, s *store.SQLiteStore, msg string) {
	t.Helper()
	entry := &store.LogEntry{
		ID:        "log-" + msg,
		AgentID:   "agent-001",
		Message:   msg,
		Tags:      []string{"test"},
		CreatedAt: time.Now(),
	}
	if err := s.CreateLogEntry(context.Background(), entry); err != nil {
		t.Fatalf("CreateLogEntry: %v", err)
	}
}

func TestHandleLogsJSON_WithEntry_ContainsMessage(t *testing.T) {
	a := newTestAdminWithStore(t)
	sqlStore := a.store.(*store.SQLiteStore)
	seedLogEntry(t, sqlStore, "unique-log-message")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/logs", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLogsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "unique-log-message") {
		t.Errorf("expected log message in response, got %q", rec.Body.String())
	}
}

func TestHandleLogsJSON_CustomLimit_Accepted(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/logs?limit=25", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLogsJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// --- handleTodosPage / handleTodosJSON ---

func TestHandleTodosPage_EmptyStore_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/todos", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleTodosPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func seedTodo(t *testing.T, s *store.SQLiteStore, desc string) {
	t.Helper()
	todo := &store.Todo{
		ID:          "todo-" + desc,
		AgentID:     "agent-001",
		Description: desc,
		Status:      "pending",
		Priority:    "medium",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.CreateTodo(context.Background(), todo); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
}

func TestHandleTodosJSON_WithTodo_ContainsTodo(t *testing.T) {
	a := newTestAdminWithStore(t)
	sqlStore := a.store.(*store.SQLiteStore)
	seedTodo(t, sqlStore, "unique-todo-item")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/todos", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleTodosJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unique-todo-item") {
		t.Errorf("expected todo in response, got %q", rec.Body.String())
	}
}

func TestHandleTodosJSON_EmptyStore_ReturnsEmptyTodos(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/todos", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleTodosJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["todos"]; !ok {
		t.Error("expected 'todos' field in response")
	}
}

// --- handleBoardPage / handleBoardJSON / handleBoardThreadJSON ---

func seedBBSPost(t *testing.T, s *store.SQLiteStore, id, subject string) {
	t.Helper()
	post := &store.BBSPost{
		ID:        id,
		AgentID:   "agent-001",
		ThreadID:  "",
		Subject:   subject,
		Content:   "Content for " + subject,
		CreatedAt: time.Now(),
	}
	if err := s.CreateBBSPost(context.Background(), post); err != nil {
		t.Fatalf("CreateBBSPost: %v", err)
	}
}

func TestHandleBoardPage_EmptyStore_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/board", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleBoardJSON_WithPost_ContainsSubject(t *testing.T) {
	a := newTestAdminWithStore(t)
	sqlStore := a.store.(*store.SQLiteStore)
	seedBBSPost(t, sqlStore, "bbs-post-1", "Unique BBS Subject")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/board", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Unique BBS Subject") {
		t.Errorf("expected post subject in response, got %q", rec.Body.String())
	}
}

func TestHandleBoardThreadJSON_UnknownID_Returns404(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/board/no-such-thread", nil)
	req.SetPathValue("id", "no-such-thread")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardThreadJSON(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleBoardThreadJSON_KnownID_Returns200WithFields(t *testing.T) {
	a := newTestAdminWithStore(t)
	sqlStore := a.store.(*store.SQLiteStore)
	seedBBSPost(t, sqlStore, "bbs-known-thread", "Known Thread Subject")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/board/bbs-known-thread", nil)
	req.SetPathValue("id", "bbs-known-thread")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardThreadJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["post"]; !ok {
		t.Error("expected 'post' field in response")
	}
	if _, ok := resp["replies"]; !ok {
		t.Error("expected 'replies' field in response")
	}
}

func TestHandleBoardThreadJSON_EmptyID_Returns400(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/board/", nil)
	req.SetPathValue("id", "")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardThreadJSON(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- handleUsagePage / handleUsageJSON ---

func TestHandleUsagePage_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleUsagePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleUsageJSON_Returns200WithFields(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleUsageJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := resp["totalTokens"]; !ok {
		t.Error("expected 'totalTokens' field in response")
	}
}

func TestHandleUsageJSON_WithSinceFilter(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage?since=2024-01-01T00:00:00Z", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleUsageJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with since filter, got %d", rec.Code)
	}
}

func TestHandleUsageJSON_WithUntilFilter(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage?until=2099-01-01T00:00:00Z", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleUsageJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with until filter, got %d", rec.Code)
	}
}

func TestHandleUsageJSON_WithInvalidSince_Ignores(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage?since=not-a-date", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleUsageJSON(rec, req)
	// Invalid date is silently ignored; response is still 200
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for invalid since, got %d", rec.Code)
	}
}

// --- handleThreadDetail / handleThreadDetailJSON empty ID coverage ---

func TestHandleThreadDetail_EmptyID_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/threads/", nil)
	req.SetPathValue("id", "")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetail(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleThreadDetailJSON_EmptyID_ReturnsBadRequest(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads/", nil)
	req.SetPathValue("id", "")
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadDetailJSON(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- handleBoardJSON limit coverage ---

func TestHandleBoardJSON_WithValidLimit_Returns200(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/board?limit=10", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleBoardJSON_WithInvalidLimit_UsesDefault(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/board?limit=notanumber", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleBoardJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with invalid limit, got %d", rec.Code)
	}
}

// --- handleThreadsJSON with data ---

func TestHandleThreadsJSON_EmptyStore_ReturnsEmptyJSONArray(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/threads", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleThreadsJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var threads []any
	if err := json.Unmarshal(rec.Body.Bytes(), &threads); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("expected empty threads, got %d", len(threads))
	}
}

// --- handleTodosJSON with limit ---

func TestHandleTodosJSON_WithLimit(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/todos?limit=5", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleTodosJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleTodosJSON_WithInvalidLimit_UsesDefault(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/todos?limit=badvalue", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleTodosJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// --- handleLogsJSON with custom limit ---

func TestHandleLogsJSON_WithCustomLimit(t *testing.T) {
	a := newTestAdminWithStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/logs?limit=20", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()
	a.handleLogsJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
