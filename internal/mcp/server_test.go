// ABOUTME: Tests for the MCP HTTP server including tool listing and execution.
// ABOUTME: Validates JSON-RPC protocol, auth handling, capability filtering.

package mcp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/packs"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// mockTokenVerifier implements auth.TokenVerifier for testing.
type mockTokenVerifier struct {
	principalID string
	err         error
}

func (m *mockTokenVerifier) Verify(token string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.principalID, nil
}

// setupTestRegistry creates a registry with test tools.
func setupTestRegistry(t *testing.T) *packs.Registry {
	t.Helper()
	registry := packs.NewRegistry(slog.Default())

	manifest := &pb.PackManifest{
		PackId:  "test-pack",
		Version: "1.0.0",
		Tools: []*pb.ToolDefinition{
			{
				Name:            "public-tool",
				Description:     "A public tool for everyone",
				InputSchemaJson: `{"type": "object", "properties": {"input": {"type": "string"}}}`,
			},
			{
				Name:                 "admin-tool",
				Description:          "An admin-only tool",
				InputSchemaJson:      `{"type": "object", "properties": {"command": {"type": "string"}}}`,
				RequiredCapabilities: []string{"admin"},
			},
			{
				Name:                 "multi-cap-tool",
				Description:          "Requires multiple capabilities",
				InputSchemaJson:      `{"type": "object"}`,
				RequiredCapabilities: []string{"admin", "superuser"},
			},
		},
	}

	if err := registry.RegisterPack("test-pack", manifest); err != nil {
		t.Fatalf("failed to register test pack: %v", err)
	}

	return registry
}

// setupTestRouter creates a router with the given registry.
func setupTestRouter(t *testing.T, registry *packs.Registry) *packs.Router {
	t.Helper()
	return packs.NewRouter(packs.RouterConfig{
		Registry: registry,
		Logger:   slog.Default(),
		Timeout:  5 * time.Second,
	})
}

// makeJSONRPCRequest creates a JSON-RPC request body.
func makeJSONRPCRequest(method string, params any) []byte {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	body, _ := json.Marshal(req)
	return body
}

// initializeSession sends an initialize request and returns the session ID.
// tokenQueryParam is appended as ?token= query parameter if non-empty.
func initializeSession(t *testing.T, mux http.Handler, tokenQueryParam string) string {
	t.Helper()
	body := makeJSONRPCRequest("initialize", nil)
	url := "/mcp"
	if tokenQueryParam != "" {
		url += "?token=" + tokenQueryParam
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("initialize failed with status %d: %s", rr.Code, rr.Body.String())
	}
	sessionID := rr.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("initialize did not return Mcp-Session-Id header")
	}
	return sessionID
}

// initializeSessionWithPathToken sends an initialize request using /mcp/<token> path format.
func initializeSessionWithPathToken(t *testing.T, mux http.Handler, token string) string {
	t.Helper()
	body := makeJSONRPCRequest("initialize", nil)
	url := "/mcp/" + token
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("initialize with path token failed with status %d: %s", rr.Code, rr.Body.String())
	}
	sessionID := rr.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("initialize did not return Mcp-Session-Id header")
	}
	return sessionID
}

func TestHandleInitialize(t *testing.T) {
	registry := setupTestRegistry(t)
	router := setupTestRouter(t, registry)

	server, err := NewServer(Config{
		Registry:    registry,
		Router:      router,
		Logger:      slog.Default(),
		RequireAuth: false,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body := makeJSONRPCRequest("initialize", nil)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Verify session ID header is set
	sessionID := rr.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Error("expected Mcp-Session-Id header in response")
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map, got %T", resp.Result)
	}

	if pv, _ := result["protocolVersion"].(string); pv != latestProtocolVersion {
		t.Errorf("expected protocolVersion %q, got %q", latestProtocolVersion, pv)
	}
}

func TestHandleToolsList(t *testing.T) {
	t.Run("returns all tools when no auth required", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		// Initialize session first
		sessionID := initializeSession(t, mux, "")

		body := makeJSONRPCRequest("tools/list", nil)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", sessionID)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp JSONRPCResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error)
		}

		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("expected result to be map, got %T", resp.Result)
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatalf("expected tools array in result")
		}

		if len(tools) != 3 {
			t.Errorf("expected 3 tools, got %d", len(tools))
		}
	})

	t.Run("filters tools by token capabilities", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		tokenStore := NewTokenStore()
		token := tokenStore.CreateToken([]string{"admin"})

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			TokenStore:  tokenStore,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		// Initialize session with token to capture capabilities
		sessionID := initializeSession(t, mux, token)

		body := makeJSONRPCRequest("tools/list", nil)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", sessionID)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var resp JSONRPCResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("expected result to be map")
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatalf("expected tools array")
		}

		// Should get public-tool and admin-tool (not multi-cap-tool which needs admin+superuser)
		if len(tools) != 2 {
			t.Errorf("expected 2 tools for admin capability, got %d", len(tools))
		}
	})

	t.Run("filters tools by path-based token", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		tokenStore := NewTokenStore()
		token := tokenStore.CreateToken([]string{"admin"})

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			TokenStore:  tokenStore,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		// Initialize session with token in URL path (/mcp/<token>)
		sessionID := initializeSessionWithPathToken(t, mux, token)

		body := makeJSONRPCRequest("tools/list", nil)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", sessionID)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var resp JSONRPCResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("expected result to be map")
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatalf("expected tools array")
		}

		// Same filtering as query param: admin gets public-tool + admin-tool
		if len(tools) != 2 {
			t.Errorf("expected 2 tools for path-based admin token, got %d", len(tools))
		}
	})

	t.Run("rejects requests without session ID", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		body := makeJSONRPCRequest("tools/list", nil)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rr.Code)
		}
	})

	t.Run("rejects requests with invalid session ID", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		body := makeJSONRPCRequest("tools/list", nil)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", "nonexistent-session")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rr.Code)
		}
	})
}

func TestHandleToolsCall(t *testing.T) {
	t.Run("returns error for unknown tool", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		sessionID := initializeSession(t, mux, "")

		body := makeJSONRPCRequest("tools/call", map[string]any{
			"name":      "nonexistent-tool",
			"arguments": map[string]any{},
		})
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", sessionID)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		var resp JSONRPCResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Error == nil {
			t.Error("expected error response for unknown tool")
		}

		if resp.Error.Code != JSONRPCInvalidParams {
			t.Errorf("expected error code %d, got %d", JSONRPCInvalidParams, resp.Error.Code)
		}
	})

	t.Run("returns error for missing tool name", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			Logger:      slog.Default(),
			RequireAuth: false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		sessionID := initializeSession(t, mux, "")

		body := makeJSONRPCRequest("tools/call", map[string]any{
			"arguments": map[string]any{},
		})
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", sessionID)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		var resp JSONRPCResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Error == nil {
			t.Error("expected error response for missing tool name")
		}
	})
}

func TestTokenStore(t *testing.T) {
	t.Run("create and retrieve token", func(t *testing.T) {
		store := NewTokenStore()
		caps := []string{"read", "write"}

		token := store.CreateToken(caps)
		if token == "" {
			t.Error("expected non-empty token")
		}

		retrieved := store.GetCapabilities(token)
		if len(retrieved) != 2 {
			t.Errorf("expected 2 capabilities, got %d", len(retrieved))
		}

		if retrieved[0] != "read" || retrieved[1] != "write" {
			t.Errorf("unexpected capabilities: %v", retrieved)
		}
	})

	t.Run("invalid token returns nil", func(t *testing.T) {
		store := NewTokenStore()

		caps := store.GetCapabilities("invalid-token")
		if caps != nil {
			t.Error("expected nil for invalid token")
		}
	})

	t.Run("invalidate token", func(t *testing.T) {
		store := NewTokenStore()
		token := store.CreateToken([]string{"test"})

		// Token should exist
		if store.GetCapabilities(token) == nil {
			t.Error("token should exist before invalidation")
		}

		store.InvalidateToken(token)

		// Token should not exist
		if store.GetCapabilities(token) != nil {
			t.Error("token should not exist after invalidation")
		}
	})
}

func TestMethodNotAllowed(t *testing.T) {
	registry := setupTestRegistry(t)
	router := setupTestRouter(t, registry)

	server, err := NewServer(Config{
		Registry:    registry,
		Router:      router,
		Logger:      slog.Default(),
		RequireAuth: false,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	t.Run("GET returns 405 (no SSE support)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rr.Code)
		}
	})

	t.Run("unsupported methods return 405 with Allow header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/mcp", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rr.Code)
		}
		if allow := rr.Header().Get("Allow"); allow != "POST, GET, DELETE" {
			t.Errorf("expected Allow: POST, GET, DELETE, got %q", allow)
		}
	})
}

func TestSessionTermination(t *testing.T) {
	registry := setupTestRegistry(t)
	router := setupTestRouter(t, registry)

	server, err := NewServer(Config{
		Registry:    registry,
		Router:      router,
		Logger:      slog.Default(),
		RequireAuth: false,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	// Create a session
	sessionID := initializeSession(t, mux, "")

	// Verify session works
	body := makeJSONRPCRequest("tools/list", nil)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tools/list failed before delete: %d", rr.Code)
	}

	// Delete the session
	delReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	delReq.Header.Set("Mcp-Session-Id", sessionID)
	delRR := httptest.NewRecorder()
	mux.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusNoContent {
		t.Errorf("expected 204 on delete, got %d", delRR.Code)
	}

	// Session should no longer work
	body = makeJSONRPCRequest("tools/list", nil)
	req = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 after session delete, got %d", rr.Code)
	}
}

func TestNotificationHandling(t *testing.T) {
	registry := setupTestRegistry(t)
	router := setupTestRouter(t, registry)

	server, err := NewServer(Config{
		Registry:    registry,
		Router:      router,
		Logger:      slog.Default(),
		RequireAuth: false,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	sessionID := initializeSession(t, mux, "")

	// Send a notification (no id field)
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	body, _ := json.Marshal(notification)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202 for notification, got %d", rr.Code)
	}

	if rr.Body.Len() != 0 {
		t.Errorf("expected empty body for notification, got %q", rr.Body.String())
	}
}

func TestUnknownMethod(t *testing.T) {
	registry := setupTestRegistry(t)
	router := setupTestRouter(t, registry)

	server, err := NewServer(Config{
		Registry:    registry,
		Router:      router,
		Logger:      slog.Default(),
		RequireAuth: false,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	sessionID := initializeSession(t, mux, "")

	body := makeJSONRPCRequest("unknown/method", nil)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Error("expected error for unknown method")
	}

	if resp.Error.Code != JSONRPCMethodNotFound {
		t.Errorf("expected error code %d, got %d", JSONRPCMethodNotFound, resp.Error.Code)
	}
}
