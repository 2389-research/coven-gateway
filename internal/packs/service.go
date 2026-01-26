// ABOUTME: PackService gRPC handlers for tool pack registration and communication.
// ABOUTME: Manages pack streams, tool dispatch, and result correlation with timeout handling.

package packs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// DefaultToolTimeout is the default timeout for tool execution when not specified.
const DefaultToolTimeout = 30 * time.Second

// ErrToolTimeout indicates a tool execution timed out waiting for a response.
var ErrToolTimeout = errors.New("tool execution timed out")

// ErrPackChannelClosed indicates the pack's request channel was closed.
var ErrPackChannelClosed = errors.New("pack channel closed")

// PackServiceServer implements the PackService gRPC service.
type PackServiceServer struct {
	pb.UnimplementedPackServiceServer
	registry *Registry
	router   *Router // Router for delivering tool responses
	logger   *slog.Logger

	// pendingRequests maps request_id to a channel that receives the result.
	// Used to correlate ToolResult calls with DispatchTool waiters.
	pendingMu       sync.RWMutex
	pendingRequests map[string]chan *pb.ExecuteToolResponse
}

// NewPackServiceServer creates a new PackService with the given registry and router.
// The router is used to deliver tool responses to waiting callers.
func NewPackServiceServer(registry *Registry, router *Router, logger *slog.Logger) *PackServiceServer {
	return &PackServiceServer{
		registry:        registry,
		router:          router,
		logger:          logger,
		pendingRequests: make(map[string]chan *pb.ExecuteToolResponse),
	}
}

// Register handles a pack connecting with its manifest.
// The pack stays connected and receives tool execution requests via the stream.
// When the stream closes, the pack is unregistered.
func (s *PackServiceServer) Register(manifest *pb.PackManifest, stream grpc.ServerStreamingServer[pb.ExecuteToolRequest]) error {
	packID := manifest.GetPackId()
	if packID == "" {
		return status.Error(codes.InvalidArgument, "pack_id is required")
	}

	s.logger.Info("pack registering",
		"pack_id", packID,
		"version", manifest.GetVersion(),
		"tool_count", len(manifest.GetTools()),
	)

	// Register the pack with the registry
	if err := s.registry.RegisterPack(packID, manifest); err != nil {
		if errors.Is(err, ErrPackAlreadyRegistered) {
			return status.Errorf(codes.AlreadyExists, "pack %s already registered", packID)
		}
		if errors.Is(err, ErrToolCollision) {
			return status.Errorf(codes.AlreadyExists, "tool collision: %v", err)
		}
		return status.Errorf(codes.Internal, "registering pack: %v", err)
	}

	// Ensure we unregister on exit
	defer func() {
		s.registry.UnregisterPack(packID)
		s.logger.Info("pack disconnected", "pack_id", packID)
	}()

	// Get the pack's channel for receiving tool requests
	pack := s.registry.GetPack(packID)
	if pack == nil {
		return status.Error(codes.Internal, "pack disappeared after registration")
	}

	s.logger.Info("pack connected", "pack_id", packID)

	// Main loop: forward tool requests from the registry channel to the stream
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			// Stream context cancelled (client disconnected or server shutting down)
			return ctx.Err()

		case req, ok := <-pack.Channel:
			if !ok {
				// Channel closed, pack was unregistered
				return nil
			}

			// Forward the tool request to the pack via the stream
			if err := stream.Send(req); err != nil {
				s.logger.Error("failed to send tool request to pack",
					"pack_id", packID,
					"tool_name", req.GetToolName(),
					"request_id", req.GetRequestId(),
					"error", err,
				)
				return status.Errorf(codes.Internal, "sending tool request: %v", err)
			}

			s.logger.Debug("sent tool request to pack",
				"pack_id", packID,
				"tool_name", req.GetToolName(),
				"request_id", req.GetRequestId(),
			)
		}
	}
}

// ToolResult handles a pack sending back the result of a tool execution.
// Routes the result to the Router which will deliver it to the waiting caller.
func (s *PackServiceServer) ToolResult(ctx context.Context, resp *pb.ExecuteToolResponse) (*emptypb.Empty, error) {
	requestID := resp.GetRequestId()

	status := "success"
	if resp.GetError() != "" {
		status = "error"
	}
	s.logger.Info("  ← tool result from pack",
		"request_id", requestID,
		"status", status,
		"output_bytes", len(resp.GetOutputJson()),
	)

	// Route the response to the waiting caller via the Router
	if s.router != nil {
		s.router.HandleToolResponse(resp)
	} else {
		// Fallback to local pending requests (for backward compatibility)
		s.pendingMu.Lock()
		ch, exists := s.pendingRequests[requestID]
		if exists {
			delete(s.pendingRequests, requestID)
		}
		s.pendingMu.Unlock()

		if !exists {
			s.logger.Warn("received result for unknown request",
				"request_id", requestID,
			)
			return &emptypb.Empty{}, nil
		}

		select {
		case ch <- resp:
			s.logger.Debug("delivered tool result",
				"request_id", requestID,
			)
		default:
			s.logger.Warn("failed to deliver tool result (channel full or closed)",
				"request_id", requestID,
			)
		}
	}

	return &emptypb.Empty{}, nil
}

// CreatePendingRequest creates a pending request channel for the given request ID.
// Returns a channel that will receive the ExecuteToolResponse when ToolResult is called.
// The channel has buffer size 1 to prevent blocking.
func (s *PackServiceServer) CreatePendingRequest(requestID string) chan *pb.ExecuteToolResponse {
	ch := make(chan *pb.ExecuteToolResponse, 1)

	s.pendingMu.Lock()
	s.pendingRequests[requestID] = ch
	s.pendingMu.Unlock()

	s.logger.Debug("created pending request",
		"request_id", requestID,
	)

	return ch
}

// CancelPendingRequest removes a pending request without delivering a result.
// Used for cleanup after timeout or context cancellation.
func (s *PackServiceServer) CancelPendingRequest(requestID string) {
	s.pendingMu.Lock()
	delete(s.pendingRequests, requestID)
	s.pendingMu.Unlock()

	s.logger.Debug("cancelled pending request",
		"request_id", requestID,
	)
}

// DispatchTool sends a tool execution request to a pack and waits for the result.
// Returns the tool response or an error if the operation times out or is cancelled.
func (s *PackServiceServer) DispatchTool(ctx context.Context, pack *Pack, toolName, inputJSON string, timeout time.Duration) (*pb.ExecuteToolResponse, error) {
	if pack == nil {
		return nil, errors.New("pack is nil")
	}

	if timeout <= 0 {
		timeout = DefaultToolTimeout
	}

	// Generate unique request ID
	requestID := uuid.New().String()

	// Create pending request channel before sending
	resultCh := s.CreatePendingRequest(requestID)
	defer s.CancelPendingRequest(requestID) // Cleanup if we don't receive result

	// Build the request
	req := &pb.ExecuteToolRequest{
		ToolName:  toolName,
		InputJson: inputJSON,
		RequestId: requestID,
	}

	s.logger.Info("dispatching tool",
		"pack_id", pack.ID,
		"tool_name", toolName,
		"request_id", requestID,
		"timeout", timeout,
	)

	// Create a single timeout context for both send and receive
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Send request to pack's channel using safe Send method
	if err := pack.Send(timeoutCtx, req); err != nil {
		if errors.Is(err, ErrPackClosed) {
			return nil, ErrPackChannelClosed
		}
		return nil, err
	}

	// Wait for result
	select {
	case result := <-resultCh:
		s.logger.Info("tool execution completed",
			"pack_id", pack.ID,
			"tool_name", toolName,
			"request_id", requestID,
			"has_error", result.GetError() != "",
		)
		return result, nil

	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s.logger.Warn("tool execution timed out",
			"pack_id", pack.ID,
			"tool_name", toolName,
			"request_id", requestID,
			"timeout", timeout,
		)
		return nil, ErrToolTimeout
	}
}

// GetPendingRequestCount returns the number of pending requests (for monitoring).
func (s *PackServiceServer) GetPendingRequestCount() int {
	s.pendingMu.RLock()
	defer s.pendingMu.RUnlock()
	return len(s.pendingRequests)
}
