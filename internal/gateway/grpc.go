// ABOUTME: FoldControl gRPC service implementation for agent communication
// ABOUTME: Handles bidirectional streaming for agent registration, heartbeats, and message routing

package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/auth"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// foldControlServer implements the FoldControl gRPC service.
type foldControlServer struct {
	pb.UnimplementedFoldControlServer
	gateway *Gateway
	logger  *slog.Logger
}

// newFoldControlServer creates a new FoldControl service instance.
func newFoldControlServer(gw *Gateway, logger *slog.Logger) *foldControlServer {
	return &foldControlServer{
		gateway: gw,
		logger:  logger,
	}
}

// AgentStream handles the bidirectional streaming connection with an agent.
// Protocol flow:
// 1. Agent sends RegisterAgent message
// 2. Server responds with Welcome message
// 3. Agent sends Heartbeat or MessageResponse messages
// 4. Server sends SendMessage or Shutdown messages
func (s *foldControlServer) AgentStream(stream pb.FoldControl_AgentStreamServer) error {
	s.logger.Debug("AgentStream handler invoked, waiting for registration")

	// Send headers immediately to unblock tonic clients waiting for response headers
	// This is needed because grpc-go delays headers until first Send() by default
	if err := stream.SendHeader(nil); err != nil {
		s.logger.Error("failed to send initial headers", "error", err)
	}

	// Wait for registration message
	msg, err := stream.Recv()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return status.Errorf(codes.Internal, "receiving first message: %v", err)
	}

	// First message must be a registration
	reg := msg.GetRegister()
	if reg == nil {
		return status.Error(codes.InvalidArgument, "first message must be RegisterAgent")
	}

	// Validate required fields
	if reg.GetAgentId() == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Extract authenticated principal from auth context
	authCtx := auth.FromContext(stream.Context())
	var principalID string
	if authCtx != nil {
		principalID = authCtx.PrincipalID
	}

	// Extract metadata from registration
	var workspaces []string
	var workingDir string
	var backend string
	if metadata := reg.GetMetadata(); metadata != nil {
		workspaces = metadata.GetWorkspaces()
		workingDir = metadata.GetWorkingDirectory()
		backend = metadata.GetBackend()
	}

	// Generate short instance ID for binding commands
	// 12 chars provides ~48 bits of entropy, collision probability < 0.1% at 1M agents
	instanceID := uuid.New().String()[:12]

	// Create connection for this agent
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:           reg.GetAgentId(),
		Name:         reg.GetName(),
		Capabilities: reg.GetCapabilities(),
		PrincipalID:  principalID,
		Workspaces:   workspaces,
		WorkingDir:   workingDir,
		InstanceID:   instanceID,
		Backend:      backend,
		Stream:       stream,
		Logger:       s.logger.With("agent_id", reg.GetAgentId()),
	})

	// Register the agent with the manager
	if err := s.gateway.agentManager.Register(conn); err != nil {
		if errors.Is(err, agent.ErrAgentAlreadyRegistered) {
			return status.Errorf(codes.AlreadyExists, "agent %s already registered", reg.GetAgentId())
		}
		return status.Errorf(codes.Internal, "registering agent: %v", err)
	}

	// Generate MCP token for this agent's capabilities
	var mcpToken string
	if s.gateway.mcpTokens != nil {
		mcpToken = s.gateway.mcpTokens.CreateToken(reg.GetCapabilities())
		s.logger.Debug("created MCP token for agent",
			"agent_id", reg.GetAgentId(),
			"capabilities", reg.GetCapabilities(),
		)
	}

	// Ensure we unregister on exit and invalidate MCP token
	defer func() {
		s.gateway.agentManager.Unregister(conn.ID)
		if s.gateway.mcpTokens != nil && mcpToken != "" {
			s.gateway.mcpTokens.InvalidateToken(mcpToken)
			s.logger.Debug("invalidated MCP token for agent", "agent_id", conn.ID)
		}
	}()

	// Get available pack tools filtered by agent's capabilities
	var availableTools []*pb.ToolDefinition
	if s.gateway.packRegistry != nil {
		availableTools = s.gateway.packRegistry.GetToolsForCapabilities(reg.GetCapabilities())
		s.logger.Debug("filtered pack tools for agent",
			"agent_id", reg.GetAgentId(),
			"capabilities", reg.GetCapabilities(),
			"tool_count", len(availableTools),
		)
	}

	// Send welcome message with instance ID, principal ID, available tools, MCP token and endpoint
	welcome := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Welcome{
			Welcome: &pb.Welcome{
				ServerId:       s.gateway.serverID,
				AgentId:        reg.GetAgentId(),
				InstanceId:     instanceID,
				PrincipalId:    principalID,
				AvailableTools: availableTools,
				McpToken:       mcpToken,
				McpEndpoint:    s.gateway.mcpEndpoint,
			},
		},
	}

	if err := stream.Send(welcome); err != nil {
		return status.Errorf(codes.Internal, "sending welcome: %v", err)
	}

	// Auto-grant leader role if agent has "leader" capability
	s.maybeGrantLeaderRole(stream.Context(), principalID, reg.GetCapabilities())

	// Main receive loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				s.logger.Debug("agent stream EOF", "agent_id", conn.ID)
				return nil
			}
			// Check for context cancellation
			if status.Code(err) == codes.Canceled {
				s.logger.Debug("agent stream cancelled", "agent_id", conn.ID)
				return nil
			}
			s.logger.Error("receiving message", "error", err, "agent_id", conn.ID)
			return status.Errorf(codes.Internal, "receiving message: %v", err)
		}

		// Handle different message types
		switch payload := msg.GetPayload().(type) {
		case *pb.AgentMessage_Heartbeat:
			s.handleHeartbeat(conn, payload.Heartbeat)

		case *pb.AgentMessage_Response:
			s.handleResponse(conn, payload.Response)

		case *pb.AgentMessage_Register:
			// Registration should only happen once at the start
			s.logger.Warn("received duplicate registration", "agent_id", conn.ID)

		case *pb.AgentMessage_ExecutePackTool:
			s.handleExecutePackTool(stream, conn, payload.ExecutePackTool)

		default:
			s.logger.Warn("received unknown message type", "agent_id", conn.ID)
		}
	}
}

// handleHeartbeat processes a heartbeat message from an agent
func (s *foldControlServer) handleHeartbeat(conn *agent.Connection, hb *pb.Heartbeat) {
	s.logger.Debug("received heartbeat",
		"agent_id", conn.ID,
		"timestamp_ms", hb.GetTimestampMs(),
	)
}

// handleResponse routes a message response to the appropriate request handler
func (s *foldControlServer) handleResponse(conn *agent.Connection, resp *pb.MessageResponse) {
	s.logger.Debug("received response",
		"agent_id", conn.ID,
		"request_id", resp.GetRequestId(),
	)

	conn.HandleResponse(resp)
}

// handleExecutePackTool routes a pack tool execution request through the pack router
// and sends the result back to the agent.
func (s *foldControlServer) handleExecutePackTool(stream pb.FoldControl_AgentStreamServer, conn *agent.Connection, req *pb.ExecutePackTool) {
	s.logger.Debug("received pack tool execution request",
		"agent_id", conn.ID,
		"request_id", req.GetRequestId(),
		"tool_name", req.GetToolName(),
	)

	// Check if pack router is available
	if s.gateway.packRouter == nil {
		s.sendPackToolError(stream, req.GetRequestId(), "pack router not initialized")
		return
	}

	// Route the tool call (this blocks until the pack responds or timeout)
	resp, err := s.gateway.packRouter.RouteToolCall(
		stream.Context(),
		req.GetToolName(),
		req.GetInputJson(),
		req.GetRequestId(),
	)

	if err != nil {
		s.logger.Warn("pack tool execution failed",
			"agent_id", conn.ID,
			"request_id", req.GetRequestId(),
			"tool_name", req.GetToolName(),
			"error", err,
		)
		s.sendPackToolError(stream, req.GetRequestId(), err.Error())
		return
	}

	// Send result back to agent
	result := &pb.ServerMessage{
		Payload: &pb.ServerMessage_PackToolResult{
			PackToolResult: &pb.PackToolResult{
				RequestId: req.GetRequestId(),
			},
		},
	}

	// Set the result based on the pack response
	if errMsg := resp.GetError(); errMsg != "" {
		result.GetPackToolResult().Result = &pb.PackToolResult_Error{Error: errMsg}
	} else {
		result.GetPackToolResult().Result = &pb.PackToolResult_OutputJson{OutputJson: resp.GetOutputJson()}
	}

	if err := stream.Send(result); err != nil {
		s.logger.Error("failed to send pack tool result",
			"agent_id", conn.ID,
			"request_id", req.GetRequestId(),
			"error", err,
		)
	} else {
		s.logger.Debug("pack tool result sent",
			"agent_id", conn.ID,
			"request_id", req.GetRequestId(),
			"tool_name", req.GetToolName(),
		)
	}
}

// sendPackToolError sends an error result for a pack tool execution request
func (s *foldControlServer) sendPackToolError(stream pb.FoldControl_AgentStreamServer, requestID, errMsg string) {
	result := &pb.ServerMessage{
		Payload: &pb.ServerMessage_PackToolResult{
			PackToolResult: &pb.PackToolResult{
				RequestId: requestID,
				Result:    &pb.PackToolResult_Error{Error: errMsg},
			},
		},
	}
	if err := stream.Send(result); err != nil {
		s.logger.Error("failed to send pack tool error",
			"request_id", requestID,
			"error", err,
		)
	}
}

// maybeGrantLeaderRole grants the "leader" role to a principal if the agent
// has "leader" in its capabilities array. Errors are logged but don't fail
// registration.
func (s *foldControlServer) maybeGrantLeaderRole(ctx context.Context, principalID string, capabilities []string) {
	// Skip if no principal ID (unauthenticated connection)
	if principalID == "" {
		return
	}

	// Check if agent has leader capability
	if !slices.Contains(capabilities, "leader") {
		return
	}

	// Type assert to SQLiteStore to access AddRole
	sqlStore, ok := s.gateway.store.(*store.SQLiteStore)
	if !ok {
		s.logger.Error("cannot grant leader role: store is not SQLiteStore")
		return
	}

	// Add the leader role to the principal (idempotent operation)
	err := sqlStore.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleLeader)
	if err != nil {
		s.logger.Error("failed to grant leader role",
			"principal_id", principalID,
			"error", err,
		)
		return
	}

	s.logger.Info("granted leader role to principal",
		"principal_id", principalID,
	)
}
