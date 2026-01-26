// ABOUTME: MCP-compatible HTTP server for external agents like Claude Code.
// ABOUTME: Implements Streamable HTTP transport (spec 2025-11-25) with session management.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/packs"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// Supported MCP protocol versions
var supportedProtocolVersions = map[string]bool{
	"2025-03-26": true,
	"2025-11-25": true,
}

// latestProtocolVersion is the version we advertise in initialize responses
const latestProtocolVersion = "2025-11-25"

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

// mcpSession tracks an active MCP client session.
type mcpSession struct {
	id              string
	protocolVersion string
	capabilities    []string
	ownerToken      string // auth token hash used to verify session ownership on DELETE
	createdAt       time.Time
}

// sessionStore manages active MCP sessions (in-memory).
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*mcpSession
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*mcpSession)}
}

func (s *sessionStore) create(protocolVersion string, caps []string, ownerToken string) *mcpSession {
	sess := &mcpSession{
		id:              uuid.New().String(),
		protocolVersion: protocolVersion,
		capabilities:    caps,
		ownerToken:      ownerToken,
		createdAt:       time.Now(),
	}
	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()
	return sess
}

func (s *sessionStore) get(id string) (*mcpSession, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	return sess, ok
}

func (s *sessionStore) delete(id string) bool {
	s.mu.Lock()
	_, existed := s.sessions[id]
	delete(s.sessions, id)
	s.mu.Unlock()
	return existed
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
// Conforms to MCP Streamable HTTP transport specification (2025-11-25).
type Server struct {
	registry    *packs.Registry
	router      *packs.Router
	logger      *slog.Logger
	verifier    auth.TokenVerifier
	tokenStore  *TokenStore
	requireAuth bool
	defaultCaps []string
	sessions    *sessionStore
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
		sessions:    newSessionStore(),
	}, nil
}

// RegisterRoutes registers the MCP endpoint on the given ServeMux.
// Supports both /mcp (bare) and /mcp/<token> (token-in-path) access patterns.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("/mcp/", s.handleMCP)
}

// handleMCP is the single MCP endpoint supporting POST, GET, and DELETE per the
// Streamable HTTP transport spec (2025-11-25).
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		// We don't support server-initiated SSE streams
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	case http.MethodDelete:
		s.handleDelete(w, r)
	default:
		w.Header().Set("Allow", "POST, GET, DELETE")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleDelete terminates a session per the Streamable HTTP spec.
// Verifies the caller owns the session to prevent unauthorized termination.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Bad Request: missing Mcp-Session-Id", http.StatusBadRequest)
		return
	}

	sess, ok := s.sessions.get(sessionID)
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Verify ownership: the DELETE request must carry the same auth as initialize
	if sess.ownerToken != "" {
		callerToken := s.extractOwnerToken(r)
		if callerToken != sess.ownerToken {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	s.sessions.delete(sessionID)
	s.logger.Info("MCP session terminated", "session_id", sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// handlePost processes JSON-RPC messages sent via HTTP POST.
func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	// Validate MCP-Protocol-Version header on non-initialize requests.
	// Per spec: server default assumption if missing is 2025-03-26.
	sessionID := r.Header.Get("Mcp-Session-Id")
	protoVersion := r.Header.Get("Mcp-Protocol-Version")

	// Read and parse the body first so we can check if this is an initialize request
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize+1))
	if err != nil {
		s.sendJSONRPCError(w, nil, JSONRPCParseError, "failed to read request body", nil)
		return
	}
	if int64(len(body)) > MaxRequestBodySize {
		s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "request body too large", nil)
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.sendJSONRPCError(w, nil, JSONRPCParseError, "invalid JSON", nil)
		return
	}

	if req.JSONRPC != "2.0" {
		s.sendJSONRPCError(w, req.ID, JSONRPCInvalidRequest, "invalid JSON-RPC version", nil)
		return
	}

	isInitialize := req.Method == "initialize"
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	// Validate protocol version header (not required on initialize)
	if !isInitialize && protoVersion != "" {
		if !supportedProtocolVersions[protoVersion] {
			http.Error(w, "Bad Request: unsupported MCP-Protocol-Version", http.StatusBadRequest)
			return
		}
	}

	// Validate session on non-initialize requests
	var caps []string
	if isInitialize {
		// Extract capabilities from auth for the new session
		authCaps, authErr := s.extractCapabilities(r)
		if authErr != nil {
			if errors.Is(authErr, errInvalidToken) {
				s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "invalid or expired token", nil)
				return
			}
			if s.requireAuth {
				s.sendJSONRPCError(w, nil, JSONRPCInvalidRequest, "authentication required", nil)
				return
			}
			authCaps = s.defaultCaps
		}
		caps = authCaps
	} else {
		// Non-initialize requests require a valid session
		if sessionID == "" {
			http.Error(w, "Bad Request: missing Mcp-Session-Id", http.StatusBadRequest)
			return
		}
		sess, ok := s.sessions.get(sessionID)
		if !ok {
			// Session expired or invalid - client must re-initialize
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		caps = sess.capabilities
	}

	s.logger.Debug("MCP request",
		"method", req.Method,
		"is_notification", isNotification,
		"session_id", sessionID,
	)

	// Handle notifications: accept and return HTTP 202 with no body
	if isNotification {
		if strings.HasPrefix(req.Method, "notifications/") {
			s.logger.Debug("accepted MCP notification", "method", req.Method)
		} else {
			s.logger.Warn("received notification for non-notification method", "method", req.Method)
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Route to appropriate handler
	switch req.Method {
	case "initialize":
		s.handleInitialize(w, r, req, caps)
	case "tools/list":
		s.handleToolsList(w, r, req, caps)
	case "tools/call":
		s.handleToolsCall(w, r, req, caps)
	default:
		s.sendJSONRPCError(w, req.ID, JSONRPCMethodNotFound, "method not found", nil)
	}
}

// handleInitialize handles the MCP initialize handshake and creates a session.
func (s *Server) handleInitialize(w http.ResponseWriter, r *http.Request, req JSONRPCRequest, caps []string) {
	// Derive an owner token from the request auth for session ownership verification.
	// Uses the raw path token or Authorization header as the identity binding.
	ownerToken := s.extractOwnerToken(r)

	// Create a new session for this client
	sess := s.sessions.create(latestProtocolVersion, caps, ownerToken)

	s.logger.Info("MCP session created",
		"session_id", sess.id,
		"protocol_version", sess.protocolVersion,
	)

	// Set the session ID header so the client can use it on subsequent requests
	w.Header().Set("Mcp-Session-Id", sess.id)

	result := map[string]any{
		"protocolVersion": latestProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "coven-gateway",
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
	// First try token from URL path (e.g., /mcp/<token>)
	if pathToken := strings.TrimPrefix(r.URL.Path, "/mcp/"); pathToken != "" && pathToken != r.URL.Path {
		// Normalize: trim trailing slashes and reject extra path segments
		pathToken = strings.TrimRight(pathToken, "/")
		if strings.Contains(pathToken, "/") {
			return nil, errInvalidToken
		}
		if s.tokenStore != nil {
			if caps := s.tokenStore.GetCapabilities(pathToken); caps != nil {
				return caps, nil
			}
		}
		return nil, errInvalidToken
	}

	// Fall back to token query parameter
	if token := r.URL.Query().Get("token"); token != "" {
		if s.tokenStore != nil {
			if caps := s.tokenStore.GetCapabilities(token); caps != nil {
				return caps, nil
			}
		}
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

// extractOwnerToken derives a stable identity string from the request's auth
// credentials. Used to bind sessions to their creator for ownership verification.
func (s *Server) extractOwnerToken(r *http.Request) string {
	// Path token takes priority
	if pathToken := strings.TrimPrefix(r.URL.Path, "/mcp/"); pathToken != "" && pathToken != r.URL.Path {
		return strings.TrimRight(pathToken, "/")
	}
	// Query token
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	// Authorization header
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
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
