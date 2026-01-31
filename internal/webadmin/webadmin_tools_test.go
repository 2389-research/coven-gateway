// ABOUTME: Tests for the webadmin tools page handlers (handleToolsPage, handleToolsList, handleStatsPacks).
// ABOUTME: Verifies nil-safety, pack grouping, sort ordering, and correct HTTP responses.

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
	pb "github.com/2389/coven-gateway/proto/coven"
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

// --- handleStatsPacks tests ---

func TestHandleStatsPacks_NilRegistry(t *testing.T) {
	admin := newTestAdmin(nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/stats/packs", nil)
	rec := httptest.NewRecorder()

	admin.handleStatsPacks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "0" {
		t.Fatalf("expected body %q, got %q", "0", body)
	}
}

func TestHandleStatsPacks_WithPacks(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())

	// Register three packs
	for _, id := range []string{"pack-a", "pack-b", "pack-c"} {
		manifest := &pb.PackManifest{
			PackId:  id,
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: id + "-tool", Description: "A tool", TimeoutSeconds: 10},
			},
		}
		if err := registry.RegisterPack(id, manifest); err != nil {
			t.Fatalf("failed to register pack %s: %v", id, err)
		}
	}

	admin := newTestAdmin(registry)
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/packs", nil)
	rec := httptest.NewRecorder()

	admin.handleStatsPacks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "3" {
		t.Fatalf("expected body %q, got %q", "3", body)
	}
}

func TestHandleStatsPacks_ContentType(t *testing.T) {
	admin := newTestAdmin(nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/stats/packs", nil)
	rec := httptest.NewRecorder()

	admin.handleStatsPacks(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("expected Content-Type %q, got %q", "text/html; charset=utf-8", ct)
	}
}

// --- handleToolsList tests ---

func TestHandleToolsList_NilRegistry(t *testing.T) {
	admin := newTestAdmin(nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No packs connected") {
		t.Fatalf("expected empty state message in response, got:\n%s", body)
	}
}

func TestHandleToolsList_EmptyRegistry(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())
	admin := newTestAdmin(registry)

	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No packs connected") {
		t.Fatalf("expected empty state message in response, got:\n%s", body)
	}
}

func TestHandleToolsList_GroupsToolsByPack(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())

	// Register pack with multiple tools
	manifest := &pb.PackManifest{
		PackId:  "analytics-pack",
		Version: "2.1.0",
		Tools: []*pb.ToolDefinition{
			{Name: "query-logs", Description: "Query log entries", TimeoutSeconds: 60},
			{Name: "count-events", Description: "Count events", TimeoutSeconds: 30},
		},
	}
	if err := registry.RegisterPack("analytics-pack", manifest); err != nil {
		t.Fatalf("failed to register pack: %v", err)
	}

	admin := newTestAdmin(registry)
	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Pack ID should appear
	if !strings.Contains(body, "analytics-pack") {
		t.Fatalf("expected pack ID in response, got:\n%s", body)
	}

	// Both tool names should appear
	if !strings.Contains(body, "query-logs") {
		t.Fatalf("expected tool name 'query-logs' in response, got:\n%s", body)
	}
	if !strings.Contains(body, "count-events") {
		t.Fatalf("expected tool name 'count-events' in response, got:\n%s", body)
	}

	// Version should appear
	if !strings.Contains(body, "2.1.0") {
		t.Fatalf("expected version '2.1.0' in response, got:\n%s", body)
	}
}

func TestHandleToolsList_PackWithZeroTools(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())

	// Register a pack with no tools
	manifest := &pb.PackManifest{
		PackId:  "empty-pack",
		Version: "0.1.0",
		Tools:   nil,
	}
	if err := registry.RegisterPack("empty-pack", manifest); err != nil {
		t.Fatalf("failed to register pack: %v", err)
	}

	admin := newTestAdmin(registry)
	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Pack should still appear even with no tools
	if !strings.Contains(body, "empty-pack") {
		t.Fatalf("expected pack ID 'empty-pack' in response even with zero tools, got:\n%s", body)
	}

	// Should NOT show the "no packs connected" message
	if strings.Contains(body, "No packs connected") {
		t.Fatalf("should not show empty state when packs exist, got:\n%s", body)
	}
}

func TestHandleToolsList_SortsByPackIDAndToolName(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())

	// Register packs in reverse alphabetical order
	manifestZ := &pb.PackManifest{
		PackId:  "zulu-pack",
		Version: "1.0.0",
		Tools: []*pb.ToolDefinition{
			{Name: "zebra-tool", Description: "Z tool", TimeoutSeconds: 10},
			{Name: "alpha-tool", Description: "A tool", TimeoutSeconds: 10},
		},
	}
	if err := registry.RegisterPack("zulu-pack", manifestZ); err != nil {
		t.Fatalf("failed to register zulu-pack: %v", err)
	}

	manifestA := &pb.PackManifest{
		PackId:  "alpha-pack",
		Version: "1.0.0",
		Tools: []*pb.ToolDefinition{
			{Name: "yankee-tool", Description: "Y tool", TimeoutSeconds: 10},
			{Name: "bravo-tool", Description: "B tool", TimeoutSeconds: 10},
		},
	}
	if err := registry.RegisterPack("alpha-pack", manifestA); err != nil {
		t.Fatalf("failed to register alpha-pack: %v", err)
	}

	admin := newTestAdmin(registry)
	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify pack order: alpha-pack should come before zulu-pack
	alphaIdx := strings.Index(body, "alpha-pack")
	zuluIdx := strings.Index(body, "zulu-pack")
	if alphaIdx == -1 || zuluIdx == -1 {
		t.Fatalf("expected both pack IDs in response, got:\n%s", body)
	}
	if alphaIdx >= zuluIdx {
		t.Fatalf("expected alpha-pack before zulu-pack (sort by pack ID), alpha at %d, zulu at %d", alphaIdx, zuluIdx)
	}

	// Verify tool order within alpha-pack: bravo-tool should come before yankee-tool
	bravoIdx := strings.Index(body, "bravo-tool")
	yankeeIdx := strings.Index(body, "yankee-tool")
	if bravoIdx == -1 || yankeeIdx == -1 {
		t.Fatalf("expected both tool names in alpha-pack, got:\n%s", body)
	}
	if bravoIdx >= yankeeIdx {
		t.Fatalf("expected bravo-tool before yankee-tool (sort by tool name), bravo at %d, yankee at %d", bravoIdx, yankeeIdx)
	}

	// Verify tool order within zulu-pack: alpha-tool should come before zebra-tool
	alphaToolIdx := strings.Index(body, "alpha-tool")
	zebraToolIdx := strings.Index(body, "zebra-tool")
	if alphaToolIdx == -1 || zebraToolIdx == -1 {
		t.Fatalf("expected both tool names in zulu-pack, got:\n%s", body)
	}
	if alphaToolIdx >= zebraToolIdx {
		t.Fatalf("expected alpha-tool before zebra-tool (sort by tool name), alpha at %d, zebra at %d", alphaToolIdx, zebraToolIdx)
	}
}

func TestHandleToolsList_RendersToolDetails(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())

	manifest := &pb.PackManifest{
		PackId:  "detail-pack",
		Version: "3.0.0",
		Tools: []*pb.ToolDefinition{
			{
				Name:                 "admin-query",
				Description:          "Execute admin queries",
				TimeoutSeconds:       45,
				RequiredCapabilities: []string{"admin", "read"},
			},
		},
	}
	if err := registry.RegisterPack("detail-pack", manifest); err != nil {
		t.Fatalf("failed to register pack: %v", err)
	}

	admin := newTestAdmin(registry)
	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	body := rec.Body.String()

	// Check tool name
	if !strings.Contains(body, "admin-query") {
		t.Fatalf("expected tool name 'admin-query' in response, got:\n%s", body)
	}

	// Check description
	if !strings.Contains(body, "Execute admin queries") {
		t.Fatalf("expected description in response, got:\n%s", body)
	}

	// Check timeout
	if !strings.Contains(body, "45s") {
		t.Fatalf("expected timeout '45s' in response, got:\n%s", body)
	}

	// Check capabilities
	if !strings.Contains(body, "admin") {
		t.Fatalf("expected capability 'admin' in response, got:\n%s", body)
	}
	if !strings.Contains(body, "read") {
		t.Fatalf("expected capability 'read' in response, got:\n%s", body)
	}
}

func TestHandleToolsList_MultiplePacks(t *testing.T) {
	registry := packs.NewRegistry(slog.Default())

	// Register multiple packs each with tools
	packs := []struct {
		id    string
		tools []string
	}{
		{"network-pack", []string{"ping", "traceroute"}},
		{"storage-pack", []string{"list-buckets", "get-object"}},
		{"compute-pack", []string{"list-instances"}},
	}

	for _, p := range packs {
		tools := make([]*pb.ToolDefinition, len(p.tools))
		for i, name := range p.tools {
			tools[i] = &pb.ToolDefinition{Name: name, Description: name + " desc", TimeoutSeconds: 30}
		}
		manifest := &pb.PackManifest{
			PackId:  p.id,
			Version: "1.0.0",
			Tools:   tools,
		}
		if err := registry.RegisterPack(p.id, manifest); err != nil {
			t.Fatalf("failed to register pack %s: %v", p.id, err)
		}
	}

	admin := newTestAdmin(registry)
	req := httptest.NewRequest(http.MethodGet, "/admin/tools/list", nil)
	rec := httptest.NewRecorder()

	admin.handleToolsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// All pack IDs should be present
	for _, p := range packs {
		if !strings.Contains(body, p.id) {
			t.Errorf("expected pack ID %q in response", p.id)
		}
		for _, tool := range p.tools {
			if !strings.Contains(body, tool) {
				t.Errorf("expected tool name %q from pack %q in response", tool, p.id)
			}
		}
	}

	// Verify sort order: compute-pack < network-pack < storage-pack
	computeIdx := strings.Index(body, "compute-pack")
	networkIdx := strings.Index(body, "network-pack")
	storageIdx := strings.Index(body, "storage-pack")
	if computeIdx >= networkIdx || networkIdx >= storageIdx {
		t.Fatalf("expected packs in alphabetical order: compute(%d) < network(%d) < storage(%d)", computeIdx, networkIdx, storageIdx)
	}
}

// --- handleToolsPage tests ---
// Note: handleToolsPage now redirects to /admin/ (chat app) since tools are in settings modal

func TestHandleToolsPage_RedirectsToChatApp(t *testing.T) {
	admin := newTestAdmin(nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/tools", nil)
	req = requestWithUser(req)
	rec := httptest.NewRecorder()

	admin.handleToolsPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 (redirect), got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/" {
		t.Fatalf("expected redirect to /, got %q", location)
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
