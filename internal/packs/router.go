// ABOUTME: Routes tool calls from agents to the appropriate tool packs.
// ABOUTME: Handles request correlation, timeouts, and pack disconnection.

package packs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// ErrToolNotFound indicates the requested tool is not registered.
var ErrToolNotFound = errors.New("tool not found")

// ErrPackDisconnected indicates the pack that owns the tool has disconnected.
var ErrPackDisconnected = errors.New("pack disconnected")

// ErrDuplicateRequestID indicates the request ID is already in use.
var ErrDuplicateRequestID = errors.New("duplicate request ID")

// DefaultTimeout is the default timeout for tool execution.
const DefaultTimeout = 30 * time.Second

// Router routes tool calls to the appropriate packs and correlates responses.
type Router struct {
	registry *Registry
	logger   *slog.Logger
	timeout  time.Duration

	// pending tracks outstanding tool requests awaiting responses
	mu      sync.RWMutex
	pending map[string]chan *pb.ExecuteToolResponse
}

// RouterConfig contains configuration options for the Router.
type RouterConfig struct {
	Registry *Registry
	Logger   *slog.Logger
	Timeout  time.Duration
}

// NewRouter creates a new Router with the given configuration.
func NewRouter(cfg RouterConfig) *Router {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Router{
		registry: cfg.Registry,
		logger:   cfg.Logger,
		timeout:  timeout,
		pending:  make(map[string]chan *pb.ExecuteToolResponse),
	}
}

// RouteToolCall routes a tool call to the appropriate pack or builtin handler.
// Returns the ExecuteToolResponse or an error if the tool is not found, pack disconnected,
// context cancelled, or timeout exceeded.
func (r *Router) RouteToolCall(ctx context.Context, toolName, inputJSON, requestID string, agentID string) (*pb.ExecuteToolResponse, error) {
	// Check if it's a builtin tool first
	if builtin := r.registry.GetBuiltinTool(toolName); builtin != nil {
		r.logger.Info("→ dispatching to builtin",
			"tool_name", toolName,
			"request_id", requestID,
			"agent_id", agentID,
		)

		result, err := builtin.Handler(ctx, agentID, json.RawMessage(inputJSON))
		if err != nil {
			r.logger.Warn("builtin tool error",
				"tool_name", toolName,
				"request_id", requestID,
				"error", err,
			)
			return &pb.ExecuteToolResponse{
				RequestId: requestID,
				Result:    &pb.ExecuteToolResponse_Error{Error: err.Error()},
			}, nil
		}

		r.logger.Info("← builtin responded",
			"tool_name", toolName,
			"request_id", requestID,
		)
		return &pb.ExecuteToolResponse{
			RequestId: requestID,
			Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: string(result)},
		}, nil
	}

	// Look up the tool and its pack (external pack routing)
	tool, pack := r.registry.GetToolByName(toolName)
	if tool == nil || pack == nil {
		r.logger.Debug("tool not found in registry",
			"tool_name", toolName,
			"request_id", requestID,
		)
		return nil, ErrToolNotFound
	}

	// Create the request
	req := &pb.ExecuteToolRequest{
		ToolName:  toolName,
		InputJson: inputJSON,
		RequestId: requestID,
	}

	// Register the pending response channel
	respCh, err := r.createPendingRequest(requestID)
	if err != nil {
		return nil, err
	}
	defer r.closePendingRequest(requestID)

	// Determine timeout from tool definition or use default
	timeout := r.timeout
	if tool.Definition.GetTimeoutSeconds() > 0 {
		timeout = time.Duration(tool.Definition.GetTimeoutSeconds()) * time.Second
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Send request to pack's channel (with panic recovery for closed channels)
	if err := r.sendToPackChannel(ctx, pack, req, toolName, requestID); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case resp, ok := <-respCh:
		if !ok {
			r.logger.Warn("response channel closed unexpectedly",
				"tool_name", toolName,
				"pack_id", pack.ID,
				"request_id", requestID,
			)
			return nil, ErrPackDisconnected
		}
		r.logger.Info("  ← pack responded",
			"tool_name", toolName,
			"pack_id", pack.ID,
			"request_id", requestID,
		)
		return resp, nil
	case <-ctx.Done():
		r.logger.Warn("tool call timed out or cancelled",
			"tool_name", toolName,
			"pack_id", pack.ID,
			"request_id", requestID,
			"timeout", timeout,
			"error", ctx.Err(),
		)
		return nil, ctx.Err()
	}
}

// sendToPackChannel attempts to send a request to the pack's channel.
// Returns ErrPackDisconnected if the channel is closed.
func (r *Router) sendToPackChannel(ctx context.Context, pack *Pack, req *pb.ExecuteToolRequest, toolName, requestID string) error {
	err := pack.Send(ctx, req)
	if errors.Is(err, ErrPackClosed) {
		r.logger.Warn("pack channel closed while sending request",
			"tool_name", toolName,
			"pack_id", pack.ID,
			"request_id", requestID,
		)
		return ErrPackDisconnected
	}
	if err != nil {
		return err
	}
	r.logger.Info("  → routed to pack",
		"tool_name", toolName,
		"pack_id", pack.ID,
		"request_id", requestID,
	)
	return nil
}

// HandleToolResponse routes an incoming tool response to the waiting caller.
// This should be called by the pack service when it receives a response from a pack.
func (r *Router) HandleToolResponse(resp *pb.ExecuteToolResponse) {
	requestID := resp.GetRequestId()

	// Hold the lock while sending to prevent the channel from being closed
	// by closePendingRequest between lookup and send
	r.mu.Lock()
	ch, ok := r.pending[requestID]
	if !ok {
		r.mu.Unlock()
		r.logger.Warn("received response for unknown request",
			"request_id", requestID,
		)
		return
	}

	// Non-blocking send to avoid deadlock if channel is full.
	// Since we hold the write lock, the channel cannot be closed during this send.
	select {
	case ch <- resp:
		r.mu.Unlock()
	default:
		r.mu.Unlock()
		r.logger.Warn("response channel full, dropping tool response",
			"request_id", requestID,
		)
	}
}

// HasTool checks if a tool with the given name exists in the registry (external or builtin).
func (r *Router) HasTool(toolName string) bool {
	// Check builtins first
	if r.registry.IsBuiltin(toolName) {
		return true
	}
	// Check external pack tools
	tool, _ := r.registry.GetToolByName(toolName)
	return tool != nil
}

// GetToolDefinition returns the tool definition for a given tool name.
// Returns nil if the tool is not found.
func (r *Router) GetToolDefinition(toolName string) *pb.ToolDefinition {
	// Check builtins first
	if builtin := r.registry.GetBuiltinTool(toolName); builtin != nil {
		return builtin.Definition
	}
	// Check external pack tools
	tool, _ := r.registry.GetToolByName(toolName)
	if tool == nil {
		return nil
	}
	return tool.Definition
}

// createPendingRequest registers a new pending request and returns a channel for the response.
// Returns ErrDuplicateRequestID if a request with the same ID is already pending.
func (r *Router) createPendingRequest(requestID string) (chan *pb.ExecuteToolResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.pending[requestID]; exists {
		return nil, ErrDuplicateRequestID
	}

	ch := make(chan *pb.ExecuteToolResponse, 1)
	r.pending[requestID] = ch
	return ch, nil
}

// closePendingRequest closes and removes the response channel for a request.
func (r *Router) closePendingRequest(requestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ch, ok := r.pending[requestID]; ok {
		close(ch)
		delete(r.pending, requestID)
	}
}

// PendingCount returns the number of pending tool requests (for testing/monitoring).
func (r *Router) PendingCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pending)
}

// Close cancels all pending requests and clears the router state.
// This should be called during graceful shutdown to unblock any waiting callers.
func (r *Router) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close all pending response channels to unblock waiters
	for requestID, ch := range r.pending {
		close(ch)
		delete(r.pending, requestID)
	}

	r.logger.Info("router closed", "pending_cancelled", len(r.pending))
}
