// ABOUTME: MCP-compatible HTTP server for external agents like Claude Code.
// ABOUTME: Implements JSON-RPC 2.0 protocol over HTTP at /mcp endpoint.

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

// JSON-RPC 2.0 types

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	JSONRPCParseError     = -32700
	JSONRPCInvalidRequest = -32600
	JSONRPCMethodNotFound = -32601
	JSONRPCInvalidParams  = -32602
	JSONRPCInternalError  = -32603
)

// MCP-specific types

// MCPToolInfo represents an MCP tool definition.
type MCPToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPListToolsResult is the result for tools/list.
type MCPListToolsResult struct {
	Tools []MCPToolInfo `json:"tools"`
}

// MCPCallToolParams are the params for tools/call.
type MCPCallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// MCPCallToolResult is the result for tools/call.
type MCPCallToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents content in a tool result.
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Config holds configuration for the MCP server.
type Config struct {
	Registry      *packs.Registry
	Router        *packs.Router
	Logger        *slog.Logger
	TokenVerifier auth.TokenVerifier
	TokenStore    *TokenStore // Token-based auth (URL query param)
	RequireAuth   bool        // If true, reject requests without valid auth
	DefaultCaps   []string    // Capabilities to use when no auth is provided
}

// Server implements MCP-compatible HTTP endpoints for external agents.
type Server struct {
	registry    *packs.Registry
	router      *packs.Router
	logger      *slog.Logger
	verifier    auth.TokenVerifier
	tokenStore  *TokenStore
	requireAuth bool
	defaultCaps []string
}

// NewServer creates a new MCP server with the given configuration.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Registry == nil {
		return nil, errors.New("registry is required")
	}
	if cfg.Router == nil {
		return nil, errors.New("router is required")
	}
	if cfg.RequireAuth && cfg.TokenVerifier == nil && cfg.TokenStore == nil {
		return nil, errors.New("token verifier or token store required when auth is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

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
		tokenStore:  cfg.TokenStore,
		requireAuth: cfg.RequireAuth,
		defaultCaps: defaultCaps,
	}, nil
}

// RegisterRoutes registers the MCP endpoint on the given ServeMux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.handleMCP)
}

// handleMCP handles all MCP JSON-RPC requests at POST /mcp.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "method not allowed", nil)
		return
	}

	// Extract capabilities from auth
	caps, err := s.extractCapabilities(r)
	if err != nil {
		// If a token was explicitly provided but is invalid, always reject
		// This prevents privilege escalation from invalid->unauthenticated access
		if errors.Is(err, errInvalidToken) {
			s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "invalid or expired token", nil)
			return
		}
		// For other auth errors, check if auth is required
		if s.requireAuth {
			s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "authentication required", nil)
			return
		}
		caps = s.defaultCaps
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize+1))
	if err != nil {
		s.sendJSONRPCError(w, nil, JSONRPCParseError, "failed to read request body", nil)
		return
	}
	if int64(len(body)) > MaxRequestBodySize {
		s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "request body too large", nil)
		return
	}

	// Parse JSON-RPC request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.sendJSONRPCError(w, nil, JSONRPCParseError, "invalid JSON", nil)
		return
	}

	if req.JSONRPC != "2.0" {
		s.sendJSONRPCError(w, req.ID, JSONRPCInvalidRequest, "invalid JSON-RPC version", nil)
		return
	}

	s.logger.Debug("MCP request",
		"method", req.Method,
		"capabilities", caps,
	)

	// Route to appropriate handler
	switch req.Method {
	case "tools/list":
		s.handleToolsList(w, r, req, caps)
	case "tools/call":
		s.handleToolsCall(w, r, req, caps)
	case "initialize":
		s.handleInitialize(w, r, req)
	default:
		s.sendJSONRPCError(w, req.ID, JSONRPCMethodNotFound, "method not found", nil)
	}
}

// handleInitialize handles the MCP initialize handshake.
func (s *Server) handleInitialize(w http.ResponseWriter, _ *http.Request, req JSONRPCRequest) {
	result := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "fold-gateway",
			"version": "1.0.0",
		},
	}
	s.sendJSONRPCResult(w, req.ID, result)
}

// handleToolsList handles tools/list requests.
func (s *Server) handleToolsList(w http.ResponseWriter, _ *http.Request, req JSONRPCRequest, caps []string) {
	var tools []*pb.ToolDefinition
	if len(caps) == 0 {
		allTools := s.registry.GetAllTools()
		tools = make([]*pb.ToolDefinition, len(allTools))
		for i, t := range allTools {
			tools[i] = t.Definition
		}
	} else {
		tools = s.registry.GetToolsForCapabilities(caps)
	}

	result := MCPListToolsResult{
		Tools: make([]MCPToolInfo, len(tools)),
	}

	for i, tool := range tools {
		result.Tools[i] = MCPToolInfo{
			Name:        tool.GetName(),
			Description: tool.GetDescription(),
			InputSchema: json.RawMessage(tool.GetInputSchemaJson()),
		}
	}

	s.logger.Debug("tools/list",
		"count", len(tools),
		"capabilities", caps,
	)

	s.sendJSONRPCResult(w, req.ID, result)
}

// handleToolsCall handles tools/call requests.
func (s *Server) handleToolsCall(w http.ResponseWriter, r *http.Request, req JSONRPCRequest, caps []string) {
	// Parse params
	var params MCPCallToolParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendJSONRPCError(w, req.ID, JSONRPCInvalidParams, "invalid params", nil)
			return
		}
	}

	if params.Name == "" {
		s.sendJSONRPCError(w, req.ID, JSONRPCInvalidParams, "tool name is required", nil)
		return
	}

	// Get tool definition to check capabilities
	toolDef := s.router.GetToolDefinition(params.Name)
	if toolDef == nil {
		s.sendJSONRPCError(w, req.ID, JSONRPCInvalidParams, "tool not found", nil)
		return
	}

	// Verify caller has required capabilities
	if !s.hasRequiredCapabilities(caps, toolDef.GetRequiredCapabilities()) {
		s.sendJSONRPCError(w, req.ID, JSONRPCInvalidRequest, "insufficient capabilities for this tool", nil)
		return
	}

	// Generate request ID for correlation
	requestID := uuid.New().String()

	// Convert arguments to JSON string
	inputJSON := string(params.Arguments)
	if inputJSON == "" || inputJSON == "null" {
		inputJSON = "{}"
	}

	s.logger.Debug("tools/call",
		"tool_name", params.Name,
		"request_id", requestID,
	)

	// Route the tool call
	resp, err := s.router.RouteToolCall(r.Context(), params.Name, inputJSON, requestID)
	if err != nil {
		s.handleToolError(w, req.ID, params.Name, requestID, err)
		return
	}

	// Build MCP result
	var result MCPCallToolResult
	if errStr := resp.GetError(); errStr != "" {
		result = MCPCallToolResult{
			Content: []MCPContent{{Type: "text", Text: errStr}},
			IsError: true,
		}
	} else {
		result = MCPCallToolResult{
			Content: []MCPContent{{Type: "text", Text: resp.GetOutputJson()}},
		}
	}

	s.logger.Debug("tools/call complete",
		"tool_name", params.Name,
		"request_id", requestID,
		"is_error", result.IsError,
	)

	s.sendJSONRPCResult(w, req.ID, result)
}

// errInvalidToken is returned when a token is provided but invalid/expired.
// This is distinct from "no auth" - if a token was provided, we should reject
// invalid tokens rather than falling through to unauthenticated access.
var errInvalidToken = errors.New("invalid or expired token")

// extractCapabilities extracts capabilities from the request.
func (s *Server) extractCapabilities(r *http.Request) ([]string, error) {
	// First try token query parameter (preferred for agent MCP access)
	if token := r.URL.Query().Get("token"); token != "" {
		if s.tokenStore != nil {
			if caps := s.tokenStore.GetCapabilities(token); caps != nil {
				return caps, nil
			}
		}
		// Token was provided but is invalid - this should always error
		// (don't fall through to unauthenticated access)
		return nil, errInvalidToken
	}

	// Fall back to Authorization header (for JWT-based auth)
	if s.verifier == nil {
		return nil, errors.New("no authentication provided")
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing authorization")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errors.New("invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return nil, errors.New("empty token")
	}

	principalID, err := s.verifier.Verify(token)
	if err != nil {
		return nil, err
	}

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

// handleToolError handles errors from tool execution.
func (s *Server) handleToolError(w http.ResponseWriter, id json.RawMessage, toolName, requestID string, err error) {
	s.logger.Warn("tool execution failed",
		"tool_name", toolName,
		"request_id", requestID,
		"error", err,
	)

	code := JSONRPCInternalError
	message := "tool execution failed"

	switch {
	case errors.Is(err, packs.ErrToolNotFound):
		code = JSONRPCInvalidParams
		message = "tool not found"
	case errors.Is(err, packs.ErrPackDisconnected):
		message = "tool pack unavailable"
	case errors.Is(err, packs.ErrDuplicateRequestID):
		message = "duplicate request ID"
	case errors.Is(err, context.DeadlineExceeded):
		message = "tool execution timed out"
	case errors.Is(err, context.Canceled):
		message = "request cancelled"
	}

	s.sendJSONRPCError(w, id, code, message, nil)
}

// sendJSONRPCResult sends a successful JSON-RPC response.
func (s *Server) sendJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Warn("failed to encode JSON-RPC response", "error", err)
	}
}

// sendJSONRPCError sends a JSON-RPC error response.
func (s *Server) sendJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string, data any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Warn("failed to encode JSON-RPC error response", "error", err)
	}
}
