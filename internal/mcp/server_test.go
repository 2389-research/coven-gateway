// ABOUTME: Tests for the MCP HTTP server including tool listing and execution.
// ABOUTME: Validates auth handling, capability filtering, and error responses.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/2389/fold-gateway/internal/packs"
	pb "github.com/2389/fold-gateway/proto/fold"
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

func TestHandleListTools(t *testing.T) {
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

		req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}

		var response ListToolsResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response.Tools) != 3 {
			t.Errorf("expected 3 tools, got %d", len(response.Tools))
		}
	})

	t.Run("filters tools by capabilities when auth provided", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		verifier := &mockTokenVerifier{principalID: "admin"}
		server, err := NewServer(Config{
			Registry:      registry,
			Router:        router,
			Logger:        slog.Default(),
			TokenVerifier: verifier,
			RequireAuth:   false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}

		var response ListToolsResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Should get public-tool and admin-tool (admin has "admin" capability)
		// Won't get multi-cap-tool because that requires both admin AND superuser
		if len(response.Tools) != 2 {
			t.Errorf("expected 2 tools with admin cap, got %d", len(response.Tools))
		}
	})

	t.Run("rejects unauthenticated request when auth required", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)
		verifier := &mockTokenVerifier{principalID: "test"}

		server, err := NewServer(Config{
			Registry:      registry,
			Router:        router,
			Logger:        slog.Default(),
			RequireAuth:   true,
			TokenVerifier: verifier,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("uses default capabilities when no auth", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		server, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			Logger:      slog.Default(),
			RequireAuth: false,
			DefaultCaps: []string{"admin"},
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}

		var response ListToolsResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// With admin default cap, should get public-tool and admin-tool
		if len(response.Tools) != 2 {
			t.Errorf("expected 2 tools with admin default cap, got %d", len(response.Tools))
		}
	})

	t.Run("rejects non-GET requests", func(t *testing.T) {
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

		req := httptest.NewRequest(http.MethodPost, "/mcp/tools", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
		}
	})

	t.Run("returns correct tool format", func(t *testing.T) {
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

		req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		var response ListToolsResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Find public-tool and verify format
		var publicTool *ToolInfo
		for _, tool := range response.Tools {
			if tool.Name == "public-tool" {
				publicTool = &tool
				break
			}
		}

		if publicTool == nil {
			t.Fatal("public-tool not found in response")
		}

		if publicTool.Description != "A public tool for everyone" {
			t.Errorf("expected description 'A public tool for everyone', got '%s'", publicTool.Description)
		}

		// Verify input schema is valid JSON
		var schema map[string]interface{}
		if err := json.Unmarshal(publicTool.InputSchema, &schema); err != nil {
			t.Errorf("inputSchema is not valid JSON: %v", err)
		}
	})
}

func TestHandleExecuteTool(t *testing.T) {
	t.Run("rejects non-POST requests", func(t *testing.T) {
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

		req := httptest.NewRequest(http.MethodGet, "/mcp/tool", nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
		}
	})

	t.Run("rejects request with missing tool name", func(t *testing.T) {
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

		body := `{"arguments": {}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}

		var response ExecuteToolResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Error == "" {
			t.Error("expected error message in response")
		}
	})

	t.Run("returns 404 for unknown tool", func(t *testing.T) {
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

		body := `{"name": "nonexistent-tool", "arguments": {}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
		}
	})

	t.Run("rejects request without required capabilities", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)

		// Verifier returns a principal without admin capability
		verifier := &mockTokenVerifier{principalID: "regular-user"}
		server, err := NewServer(Config{
			Registry:      registry,
			Router:        router,
			Logger:        slog.Default(),
			TokenVerifier: verifier,
			RequireAuth:   false,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		// Try to execute admin-tool
		body := `{"name": "admin-tool", "arguments": {"command": "test"}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
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

		body := `not valid json`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("rejects unauthenticated request when auth required", func(t *testing.T) {
		registry := setupTestRegistry(t)
		router := setupTestRouter(t, registry)
		verifier := &mockTokenVerifier{principalID: "test"}

		server, err := NewServer(Config{
			Registry:      registry,
			Router:        router,
			Logger:        slog.Default(),
			RequireAuth:   true,
			TokenVerifier: verifier,
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		body := `{"name": "public-tool", "arguments": {}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("rejects request body too large", func(t *testing.T) {
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

		// Create body larger than MaxRequestBodySize (1MB)
		largeBody := make([]byte, MaxRequestBodySize+100)
		for i := range largeBody {
			largeBody[i] = 'x'
		}
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rr.Code)
		}
	})

	t.Run("rejects empty request body", func(t *testing.T) {
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

		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewReader([]byte{}))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}

		var response ExecuteToolResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if response.Error != "empty request body" {
			t.Errorf("expected error 'empty request body', got %q", response.Error)
		}
	})

	t.Run("handles null arguments gracefully", func(t *testing.T) {
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

		// Send a request with null arguments
		body := `{"name": "test-tool-1", "arguments": null}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		// Should not fail on the null arguments - tool execution may fail for other reasons
		// but the arguments should be converted to "{}" internally
		if rr.Code == http.StatusBadRequest {
			var response ExecuteToolResponse
			if err := json.NewDecoder(rr.Body).Decode(&response); err == nil {
				if strings.Contains(response.Error, "null") || strings.Contains(response.Error, "arguments") {
					t.Errorf("null arguments should be handled, but got error: %s", response.Error)
				}
			}
		}
	})
}

func TestNewServerValidation(t *testing.T) {
	registry := &packs.Registry{}
	router := &packs.Router{}

	t.Run("returns error when registry is nil", func(t *testing.T) {
		_, err := NewServer(Config{
			Registry: nil,
			Router:   router,
		})
		if err == nil {
			t.Error("expected error when registry is nil")
		}
		if err.Error() != "registry is required" {
			t.Errorf("expected 'registry is required', got %q", err.Error())
		}
	})

	t.Run("returns error when router is nil", func(t *testing.T) {
		_, err := NewServer(Config{
			Registry: registry,
			Router:   nil,
		})
		if err == nil {
			t.Error("expected error when router is nil")
		}
		if err.Error() != "router is required" {
			t.Errorf("expected 'router is required', got %q", err.Error())
		}
	})

	t.Run("returns error when RequireAuth but no TokenVerifier", func(t *testing.T) {
		_, err := NewServer(Config{
			Registry:      registry,
			Router:        router,
			RequireAuth:   true,
			TokenVerifier: nil,
		})
		if err == nil {
			t.Error("expected error when RequireAuth is true but TokenVerifier is nil")
		}
		if err.Error() != "token verifier required when auth is required" {
			t.Errorf("expected 'token verifier required when auth is required', got %q", err.Error())
		}
	})

	t.Run("succeeds with valid config", func(t *testing.T) {
		_, err := NewServer(Config{
			Registry:    registry,
			Router:      router,
			RequireAuth: false,
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestHasRequiredCapabilities(t *testing.T) {
	server := &Server{}

	t.Run("returns true when no capabilities required", func(t *testing.T) {
		if !server.hasRequiredCapabilities([]string{}, []string{}) {
			t.Error("expected true when no caps required")
		}
		if !server.hasRequiredCapabilities([]string{"admin"}, []string{}) {
			t.Error("expected true when caller has caps but none required")
		}
	})

	t.Run("returns true when caller has all required caps", func(t *testing.T) {
		if !server.hasRequiredCapabilities([]string{"admin", "superuser"}, []string{"admin"}) {
			t.Error("expected true when caller has required cap")
		}
		if !server.hasRequiredCapabilities([]string{"admin", "superuser"}, []string{"admin", "superuser"}) {
			t.Error("expected true when caller has all required caps")
		}
	})

	t.Run("returns false when caller missing required caps", func(t *testing.T) {
		if server.hasRequiredCapabilities([]string{}, []string{"admin"}) {
			t.Error("expected false when caller has no caps but cap required")
		}
		if server.hasRequiredCapabilities([]string{"admin"}, []string{"admin", "superuser"}) {
			t.Error("expected false when caller missing one required cap")
		}
	})
}

func TestExtractCapabilities(t *testing.T) {
	t.Run("returns error when no verifier configured", func(t *testing.T) {
		server := &Server{}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer test-token")

		_, err := server.extractCapabilities(req)
		if err == nil {
			t.Error("expected error when no verifier")
		}
	})

	t.Run("returns error when no auth header", func(t *testing.T) {
		verifier := &mockTokenVerifier{principalID: "test"}
		server := &Server{verifier: verifier}
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		_, err := server.extractCapabilities(req)
		if err == nil {
			t.Error("expected error when no auth header")
		}
	})

	t.Run("returns error for invalid header format", func(t *testing.T) {
		verifier := &mockTokenVerifier{principalID: "test"}
		server := &Server{verifier: verifier}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Basic credentials")

		_, err := server.extractCapabilities(req)
		if err == nil {
			t.Error("expected error for non-Bearer auth")
		}
	})

	t.Run("returns error for empty token", func(t *testing.T) {
		verifier := &mockTokenVerifier{principalID: "test"}
		server := &Server{verifier: verifier}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer ")

		_, err := server.extractCapabilities(req)
		if err == nil {
			t.Error("expected error for empty token")
		}
	})

	t.Run("returns error when token verification fails", func(t *testing.T) {
		verifier := &mockTokenVerifier{err: errors.New("invalid token")}
		server := &Server{verifier: verifier}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bad-token")

		_, err := server.extractCapabilities(req)
		if err == nil {
			t.Error("expected error when token invalid")
		}
	})

	t.Run("returns principal ID as capability on success", func(t *testing.T) {
		verifier := &mockTokenVerifier{principalID: "test-principal"}
		server := &Server{verifier: verifier}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")

		caps, err := server.extractCapabilities(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(caps) != 1 || caps[0] != "test-principal" {
			t.Errorf("expected [test-principal], got %v", caps)
		}
	})
}

func TestHandleToolError(t *testing.T) {
	registry := setupTestRegistry(t)
	router := setupTestRouter(t, registry)
	server, err := NewServer(Config{
		Registry: registry,
		Router:   router,
		Logger:   slog.Default(),
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	tests := []struct {
		name           string
		err            error
		expectedStatus int
	}{
		{
			name:           "tool not found",
			err:            packs.ErrToolNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "pack disconnected",
			err:            packs.ErrPackDisconnected,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "deadline exceeded",
			err:            context.DeadlineExceeded,
			expectedStatus: http.StatusGatewayTimeout,
		},
		{
			name:           "context canceled",
			err:            context.Canceled,
			expectedStatus: http.StatusRequestTimeout,
		},
		{
			name:           "duplicate request ID",
			err:            packs.ErrDuplicateRequestID,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "unknown error",
			err:            errors.New("something went wrong"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			server.handleToolError(rr, "test-tool", "req-123", tc.err)

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			var response ExecuteToolResponse
			if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if response.Error == "" {
				t.Error("expected error message in response")
			}
		})
	}
}

func TestRegisterRoutes(t *testing.T) {
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

	// Test that both routes are registered
	t.Run("tools endpoint registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mcp/tools", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		// Should not be 404
		if rr.Code == http.StatusNotFound {
			t.Error("/mcp/tools route not registered")
		}
	})

	t.Run("tool endpoint registered", func(t *testing.T) {
		body := `{"name": "test"}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tool", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		// Should not be 404 (might be 400 or 404 for tool not found, but not route not found)
		if rr.Code == http.StatusNotFound && rr.Body.String() == "404 page not found\n" {
			t.Error("/mcp/tool route not registered")
		}
	})
}
