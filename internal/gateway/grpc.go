// ABOUTME: FoldControl gRPC service implementation for agent communication
// ABOUTME: Handles bidirectional streaming for agent registration, heartbeats, and message routing

package gateway

import (
	"errors"
	"io"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/fold-gateway/internal/agent"
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

	// Create connection for this agent
	conn := agent.NewConnection(
		reg.GetAgentId(),
		reg.GetName(),
		reg.GetCapabilities(),
		stream,
		s.logger.With("agent_id", reg.GetAgentId()),
	)

	// Register the agent with the manager
	if err := s.gateway.agentManager.Register(conn); err != nil {
		if errors.Is(err, agent.ErrAgentAlreadyRegistered) {
			return status.Errorf(codes.AlreadyExists, "agent %s already registered", reg.GetAgentId())
		}
		return status.Errorf(codes.Internal, "registering agent: %v", err)
	}

	// Ensure we unregister on exit
	defer s.gateway.agentManager.Unregister(conn.ID)

	s.logger.Info("agent connected",
		"agent_id", reg.GetAgentId(),
		"name", reg.GetName(),
		"capabilities", reg.GetCapabilities(),
	)

	// Send welcome message
	welcome := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Welcome{
			Welcome: &pb.Welcome{
				ServerId: s.gateway.serverID,
				AgentId:  reg.GetAgentId(),
			},
		},
	}

	if err := stream.Send(welcome); err != nil {
		return status.Errorf(codes.Internal, "sending welcome: %v", err)
	}

	// Main receive loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				s.logger.Info("agent disconnected (EOF)", "agent_id", conn.ID)
				return nil
			}
			// Check for context cancellation
			if status.Code(err) == codes.Canceled {
				s.logger.Info("agent stream cancelled", "agent_id", conn.ID)
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
