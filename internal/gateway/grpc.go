// ABOUTME: CovenControl gRPC service implementation for agent communication
// ABOUTME: Handles bidirectional streaming for agent registration, heartbeats, and message routing

package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// covenControlServer implements the CovenControl gRPC service.
type covenControlServer struct {
	pb.UnimplementedCovenControlServer
	gateway *Gateway
	logger  *slog.Logger
}

// newCovenControlServer creates a new CovenControl service instance.
func newCovenControlServer(gw *Gateway, logger *slog.Logger) *covenControlServer {
	return &covenControlServer{
		gateway: gw,
		logger:  logger,
	}
}

// agentRegistrationInfo holds extracted registration data.
type agentRegistrationInfo struct {
	principalID string
	workspaces  []string
	workingDir  string
	backend     string
	instanceID  string
}

// extractRegistrationInfo extracts info from registration message and auth context.
func (s *covenControlServer) extractRegistrationInfo(ctx context.Context, reg *pb.RegisterAgent) agentRegistrationInfo {
	info := agentRegistrationInfo{
		instanceID: uuid.New().String()[:12],
	}
	if authCtx := auth.FromContext(ctx); authCtx != nil {
		info.principalID = authCtx.PrincipalID
	}
	if metadata := reg.GetMetadata(); metadata != nil {
		info.workspaces = metadata.GetWorkspaces()
		info.workingDir = metadata.GetWorkingDirectory()
		info.backend = metadata.GetBackend()
	}
	return info
}

// loadAgentSecrets resolves effective secrets for an agent.
func (s *covenControlServer) loadAgentSecrets(ctx context.Context, agentID string) map[string]string {
	sqlStore, ok := s.gateway.store.(*store.SQLiteStore)
	if !ok {
		return make(map[string]string)
	}
	secretsMap, err := sqlStore.GetEffectiveSecrets(ctx, agentID)
	if err != nil {
		s.logger.Warn("failed to load secrets for agent", "agent_id", agentID, "error", err)
		return make(map[string]string)
	}
	return secretsMap
}

// createMCPToken creates an MCP token for the agent if token store is available.
func (s *covenControlServer) createMCPToken(agentID string, capabilities []string) string {
	if s.gateway.mcpTokens == nil {
		return ""
	}
	token := s.gateway.mcpTokens.CreateToken(agentID, capabilities)
	s.logger.Debug("created MCP token for agent", "agent_id", agentID, "capabilities", capabilities)
	return token
}

// getAgentTools returns available pack tools filtered by agent's capabilities.
func (s *covenControlServer) getAgentTools(agentID string, capabilities []string) []*pb.ToolDefinition {
	if s.gateway.packRegistry == nil {
		return nil
	}
	tools := s.gateway.packRegistry.GetToolsForCapabilities(capabilities)
	s.logger.Debug("filtered pack tools for agent", "agent_id", agentID, "capabilities", capabilities, "tool_count", len(tools))
	return tools
}

// receiveRegistration waits for and validates the registration message.
// Returns (reg, cleanDisconnect, grpcError).
func (s *covenControlServer) receiveRegistration(stream pb.CovenControl_AgentStreamServer) (*pb.RegisterAgent, bool, error) {
	msg, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, true, nil
		}
		return nil, false, status.Errorf(codes.Internal, "receiving first message: %v", err)
	}
	reg := msg.GetRegister()
	if reg == nil {
		return nil, false, status.Error(codes.InvalidArgument, "first message must be RegisterAgent")
	}
	if reg.GetAgentId() == "" {
		return nil, false, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	return reg, false, nil
}

// registerAgent registers the agent and handles duplicate registration errors.
func (s *covenControlServer) registerAgent(conn *agent.Connection) error {
	if err := s.gateway.agentManager.Register(conn); err != nil {
		if errors.Is(err, agent.ErrAgentAlreadyRegistered) {
			return status.Errorf(codes.AlreadyExists, "agent %s already registered", conn.ID)
		}
		return status.Errorf(codes.Internal, "registering agent: %v", err)
	}
	return nil
}

// checkRecvError handles receive errors, returning (shouldContinue, grpcError).
// Returns (true, nil) if stream should continue, (false, nil) for clean exit, (false, err) for error.
func (s *covenControlServer) checkRecvError(err error, agentID string) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, io.EOF) {
		s.logger.Debug("agent stream EOF", "agent_id", agentID)
		return false, nil
	}
	if status.Code(err) == codes.Canceled {
		s.logger.Debug("agent stream canceled", "agent_id", agentID)
		return false, nil
	}
	s.logger.Error("receiving message", "error", err, "agent_id", agentID)
	return false, status.Errorf(codes.Internal, "receiving message: %v", err)
}

// dispatchMessage routes an agent message to the appropriate handler.
func (s *covenControlServer) dispatchMessage(stream pb.CovenControl_AgentStreamServer, conn *agent.Connection, msg *pb.AgentMessage) {
	switch payload := msg.GetPayload().(type) {
	case *pb.AgentMessage_Heartbeat:
		s.handleHeartbeat(conn, payload.Heartbeat)
	case *pb.AgentMessage_Response:
		s.handleResponse(conn, payload.Response)
	case *pb.AgentMessage_Register:
		s.logger.Warn("received duplicate registration", "agent_id", conn.ID)
	case *pb.AgentMessage_ExecutePackTool:
		s.handleExecutePackTool(stream, conn, payload.ExecutePackTool)
	default:
		s.logger.Warn("received unknown message type", "agent_id", conn.ID)
	}
}

// runMessageLoop handles the main receive loop for an agent connection.
func (s *covenControlServer) runMessageLoop(stream pb.CovenControl_AgentStreamServer, conn *agent.Connection) error {
	for {
		msg, err := stream.Recv()
		if shouldContinue, grpcErr := s.checkRecvError(err, conn.ID); !shouldContinue {
			return grpcErr
		}
		s.dispatchMessage(stream, conn, msg)
	}
}

// AgentStream handles the bidirectional streaming connection with an agent.
// Protocol flow:
// 1. Agent sends RegisterAgent message
// 2. Server responds with Welcome message
// 3. Agent sends Heartbeat or MessageResponse messages
// 4. Server sends SendMessage or Shutdown messages.
func (s *covenControlServer) AgentStream(stream pb.CovenControl_AgentStreamServer) error {
	s.logger.Debug("AgentStream handler invoked, waiting for registration")

	// Send headers immediately to unblock tonic clients
	if err := stream.SendHeader(nil); err != nil {
		s.logger.Error("failed to send initial headers", "error", err)
	}

	// Wait for and validate registration message
	reg, cleanDisconnect, err := s.receiveRegistration(stream)
	if cleanDisconnect {
		return nil
	}
	if err != nil {
		return err
	}

	// Extract registration info and create connection
	info := s.extractRegistrationInfo(stream.Context(), reg)
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:           reg.GetAgentId(),
		Name:         reg.GetName(),
		Capabilities: reg.GetCapabilities(),
		PrincipalID:  info.principalID,
		Workspaces:   info.workspaces,
		WorkingDir:   info.workingDir,
		InstanceID:   info.instanceID,
		Backend:      info.backend,
		Stream:       stream,
		Logger:       s.logger.With("agent_id", reg.GetAgentId()),
	})

	// Register the agent with the manager
	if err := s.registerAgent(conn); err != nil {
		return err
	}

	// Auto-update bindings that match this agent's workspace name
	s.maybeUpdateBindingsForWorkspace(stream.Context(), conn.Name, conn.ID)

	// Generate MCP token for this agent's capabilities
	mcpToken := s.createMCPToken(conn.ID, reg.GetCapabilities())

	// Ensure we unregister on exit and invalidate MCP token
	defer func() {
		s.gateway.agentManager.Unregister(conn.ID)
		if s.gateway.mcpTokens != nil && mcpToken != "" {
			s.gateway.mcpTokens.InvalidateToken(mcpToken)
			s.logger.Debug("invalidated MCP token for agent", "agent_id", conn.ID)
		}
	}()

	// Get available pack tools and secrets for welcome message
	availableTools := s.getAgentTools(reg.GetAgentId(), reg.GetCapabilities())
	secretsMap := s.loadAgentSecrets(stream.Context(), reg.GetAgentId())

	// Send welcome message
	welcome := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Welcome{
			Welcome: &pb.Welcome{
				ServerId:       s.gateway.serverID,
				AgentId:        reg.GetAgentId(),
				InstanceId:     info.instanceID,
				PrincipalId:    info.principalID,
				AvailableTools: availableTools,
				McpToken:       mcpToken,
				McpEndpoint:    s.gateway.mcpEndpoint,
				Secrets:        secretsMap,
			},
		},
	}

	if err := stream.Send(welcome); err != nil {
		return status.Errorf(codes.Internal, "sending welcome: %v", err)
	}

	// Auto-grant leader role if agent has "leader" capability
	s.maybeGrantLeaderRole(stream.Context(), info.principalID, reg.GetCapabilities())

	return s.runMessageLoop(stream, conn)
}

// handleHeartbeat processes a heartbeat message from an agent.
func (s *covenControlServer) handleHeartbeat(conn *agent.Connection, hb *pb.Heartbeat) {
	s.logger.Debug("received heartbeat",
		"agent_id", conn.ID,
		"timestamp_ms", hb.GetTimestampMs(),
	)
}

// handleResponse routes a message response to the appropriate request handler.
func (s *covenControlServer) handleResponse(conn *agent.Connection, resp *pb.MessageResponse) {
	s.logger.Debug("received response",
		"agent_id", conn.ID,
		"request_id", resp.GetRequestId(),
	)

	conn.HandleResponse(resp)
}

// handleExecutePackTool routes a pack tool execution request through the pack router
// and sends the result back to the agent.
func (s *covenControlServer) handleExecutePackTool(stream pb.CovenControl_AgentStreamServer, conn *agent.Connection, req *pb.ExecutePackTool) {
	started := time.Now()

	s.logger.Info("→ pack tool request",
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
		conn.ID,
	)

	elapsed := time.Since(started)

	if err != nil {
		s.logger.Warn("✗ pack tool failed",
			"agent_id", conn.ID,
			"request_id", req.GetRequestId(),
			"tool_name", req.GetToolName(),
			"duration_ms", elapsed.Milliseconds(),
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
	status := "success"
	if errMsg := resp.GetError(); errMsg != "" {
		result.GetPackToolResult().Result = &pb.PackToolResult_Error{Error: errMsg}
		status = "error"
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
		s.logger.Info("← pack tool result",
			"agent_id", conn.ID,
			"request_id", req.GetRequestId(),
			"tool_name", req.GetToolName(),
			"status", status,
			"duration_ms", elapsed.Milliseconds(),
		)
	}
}

// sendPackToolError sends an error result for a pack tool execution request.
func (s *covenControlServer) sendPackToolError(stream pb.CovenControl_AgentStreamServer, requestID, errMsg string) {
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
func (s *covenControlServer) maybeGrantLeaderRole(ctx context.Context, principalID string, capabilities []string) {
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

// maybeUpdateBindingsForWorkspace updates bindings that match an agent's workspace
// name to point to the newly registered agent. This handles device prefix changes
// (e.g., when "m_notes" reconnects as "magic_notes", bindings update automatically).
// Errors are logged but don't fail registration.
func (s *covenControlServer) maybeUpdateBindingsForWorkspace(ctx context.Context, workspace, agentID string) {
	// Skip if no workspace name
	if workspace == "" {
		return
	}

	// Type assert to SQLiteStore to access UpdateBindingsByWorkspace
	sqlStore, ok := s.gateway.store.(*store.SQLiteStore)
	if !ok {
		s.logger.Error("cannot update bindings: store is not SQLiteStore")
		return
	}

	// Update bindings that match this workspace pattern
	count, err := sqlStore.UpdateBindingsByWorkspace(ctx, workspace, agentID)
	if err != nil {
		s.logger.Error("failed to update bindings for workspace",
			"workspace", workspace,
			"agent_id", agentID,
			"error", err,
		)
		return
	}

	if count > 0 {
		s.logger.Info("auto-updated bindings for reconnected agent",
			"workspace", workspace,
			"agent_id", agentID,
			"bindings_updated", count,
		)
	}
}
