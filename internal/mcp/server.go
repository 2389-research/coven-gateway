// ABOUTME: MCP-compatible HTTP server for external agents like Claude Code.
// ABOUTME: Exposes tool listing and execution via REST endpoints with optional JWT auth.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/2389/fold-gateway/internal/auth"
	"github.com/2389/fold-gateway/internal/packs"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// MaxRequestBodySize is the maximum allowed size for request bodies (1MB).
const MaxRequestBodySize = 1 << 20

// ToolInfo represents an MCP-compatible tool definition in the list response.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ListToolsResponse is the JSON response for GET /mcp/tools.
type ListToolsResponse struct {
	Tools []ToolInfo `json:"tools"`
}

// ExecuteToolRequest is the JSON request body for POST /mcp/tool.
type ExecuteToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ExecuteToolResponse is the JSON response for POST /mcp/tool.
type ExecuteToolResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Config holds configuration for the MCP server.
type Config struct {
	Registry      *packs.Registry
	Router        *packs.Router
	Logger        *slog.Logger
	TokenVerifier auth.TokenVerifier
	RequireAuth   bool     // If true, reject requests without valid auth
	DefaultCaps   []string // Capabilities to use when no auth is provided
}

// Server implements MCP-compatible HTTP endpoints for external agents.
type Server struct {
	registry    *packs.Registry
	router      *packs.Router
	logger      *slog.Logger
	verifier    auth.TokenVerifier
	requireAuth bool
	defaultCaps []string
}

// NewServer creates a new MCP server with the given configuration.
// Returns an error if Registry or Router are nil, or if RequireAuth is true without a TokenVerifier.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Registry == nil {
		return nil, errors.New("registry is required")
	}
	if cfg.Router == nil {
		return nil, errors.New("router is required")
	}
	if cfg.RequireAuth && cfg.TokenVerifier == nil {
		return nil, errors.New("token verifier required when auth is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Copy default caps to prevent aliasing
	var defaultCaps []string
	if len(cfg.DefaultCaps) > 0 {
		defaultCaps = make([]string, len(cfg.DefaultCaps))
		copy(defaultCaps, cfg.DefaultCaps)
	}

	return &Server{
		registry:    cfg.Registry,
		router:      cfg.Router,
		logger:      logger,
		verifier:    cfg.TokenVerifier,
		requireAuth: cfg.RequireAuth,
		defaultCaps: defaultCaps,
	}, nil
}

// RegisterRoutes registers the MCP endpoints on the given ServeMux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp/tools", s.handleListTools)
	mux.HandleFunc("/mcp/tool", s.handleExecuteTool)
}

// handleListTools handles GET /mcp/tools requests.
// Returns a list of available tools, optionally filtered by agent capabilities.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		s.sendJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract capabilities from auth token if present
	caps, err := s.extractCapabilities(r)
	if err != nil {
		if s.requireAuth {
			s.sendJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		// Use default capabilities when auth is optional and not provided
		caps = s.defaultCaps
	}

	// Get tools filtered by capabilities
	var tools []*pb.ToolDefinition
	if len(caps) == 0 {
		// No capabilities means get all tools (or use registry method that returns all)
		allTools := s.registry.GetAllTools()
		tools = make([]*pb.ToolDefinition, len(allTools))
		for i, t := range allTools {
			tools[i] = t.Definition
		}
	} else {
		tools = s.registry.GetToolsForCapabilities(caps)
	}

	// Convert to MCP response format
	response := ListToolsResponse{
		Tools: make([]ToolInfo, len(tools)),
	}

	for i, tool := range tools {
		response.Tools[i] = ToolInfo{
			Name:        tool.GetName(),
			Description: tool.GetDescription(),
			InputSchema: json.RawMessage(tool.GetInputSchemaJson()),
		}
	}

	s.logger.Debug("listing tools",
		"count", len(tools),
		"capabilities", caps,
	)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Warn("failed to encode list tools response", "error", err)
	}
}

// handleExecuteTool handles POST /mcp/tool requests.
// Executes a tool and returns the result.
func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.sendJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check auth if required
	caps, err := s.extractCapabilities(r)
	if err != nil {
		if s.requireAuth {
			s.sendJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		caps = s.defaultCaps
	}

	// Read body with size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize+1))
	if err != nil {
		s.sendJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if int64(len(body)) > MaxRequestBodySize {
		s.sendJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	// Parse request body
	var req ExecuteToolRequest
	if err := json.Unmarshal(body, &req); err != nil {
		if len(body) == 0 {
			s.sendJSONError(w, http.StatusBadRequest, "empty request body")
		} else {
			s.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return
	}

	if req.Name == "" {
		s.sendJSONError(w, http.StatusBadRequest, "tool name is required")
		return
	}

	// Get tool definition to check capabilities
	toolDef := s.router.GetToolDefinition(req.Name)
	if toolDef == nil {
		s.sendJSONError(w, http.StatusNotFound, "tool not found")
		return
	}

	// Verify caller has required capabilities
	if !s.hasRequiredCapabilities(caps, toolDef.GetRequiredCapabilities()) {
		s.sendJSONError(w, http.StatusForbidden, "insufficient capabilities for this tool")
		return
	}

	// Generate request ID for correlation
	requestID := uuid.New().String()

	// Convert arguments to JSON string
	inputJSON := string(req.Arguments)
	if inputJSON == "" || inputJSON == "null" {
		inputJSON = "{}"
	}

	s.logger.Debug("executing tool",
		"tool_name", req.Name,
		"request_id", requestID,
	)

	// Route the tool call
	resp, err := s.router.RouteToolCall(r.Context(), req.Name, inputJSON, requestID)
	if err != nil {
		s.handleToolError(w, req.Name, requestID, err)
		return
	}

	// Build response
	var response ExecuteToolResponse
	if errStr := resp.GetError(); errStr != "" {
		response.Error = errStr
	} else {
		response.Result = json.RawMessage(resp.GetOutputJson())
	}

	s.logger.Debug("tool execution complete",
		"tool_name", req.Name,
		"request_id", requestID,
		"has_error", response.Error != "",
	)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Warn("failed to encode tool execution response", "error", err)
	}
}

// extractCapabilities extracts capabilities from the Authorization header JWT.
// Returns the capabilities claim from the token, or an error if token is missing/invalid.
func (s *Server) extractCapabilities(r *http.Request) ([]string, error) {
	if s.verifier == nil {
		return nil, errors.New("no token verifier configured")
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing authorization header")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errors.New("invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return nil, errors.New("empty token")
	}

	// Verify token and extract principal ID
	principalID, err := s.verifier.Verify(token)
	if err != nil {
		return nil, err
	}

	// For now, we use the principal ID as a capability
	// In the future, this could be extended to look up capabilities from a store
	return []string{principalID}, nil
}

// hasRequiredCapabilities checks if the caller has all required capabilities.
func (s *Server) hasRequiredCapabilities(callerCaps, requiredCaps []string) bool {
	if len(requiredCaps) == 0 {
		return true
	}

	capSet := make(map[string]struct{}, len(callerCaps))
	for _, cap := range callerCaps {
		capSet[cap] = struct{}{}
	}

	for _, req := range requiredCaps {
		if _, has := capSet[req]; !has {
			return false
		}
	}
	return true
}

// handleToolError handles errors from tool execution and sends appropriate HTTP response.
func (s *Server) handleToolError(w http.ResponseWriter, toolName, requestID string, err error) {
	s.logger.Warn("tool execution failed",
		"tool_name", toolName,
		"request_id", requestID,
		"error", err,
	)

	switch {
	case errors.Is(err, packs.ErrToolNotFound):
		s.sendJSONError(w, http.StatusNotFound, "tool not found")
	case errors.Is(err, packs.ErrPackDisconnected):
		s.sendJSONError(w, http.StatusServiceUnavailable, "tool pack unavailable")
	case errors.Is(err, context.DeadlineExceeded):
		s.sendJSONError(w, http.StatusGatewayTimeout, "tool execution timed out")
	case errors.Is(err, context.Canceled):
		s.sendJSONError(w, http.StatusRequestTimeout, "request cancelled")
	default:
		s.sendJSONError(w, http.StatusInternalServerError, "tool execution failed")
	}
}

// sendJSONError writes a JSON error response.
func (s *Server) sendJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ExecuteToolResponse{Error: message}); err != nil {
		s.logger.Warn("failed to encode error response", "error", err)
	}
}
