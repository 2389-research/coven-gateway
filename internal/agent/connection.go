// ABOUTME: Represents a single connected agent and manages its bidirectional stream.
// ABOUTME: Handles sending messages and routing responses by request ID.

package agent

import (
	"log/slog"
	"sync"

	pb "github.com/2389/fold-gateway/proto/fold"
)

// Connection represents a connected agent with its GRPC stream.
type Connection struct {
	ID           string
	Name         string
	Capabilities []string

	stream  pb.FoldControl_AgentStreamServer
	pending map[string]chan *pb.MessageResponse
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewConnection creates a new Connection for a connected agent.
func NewConnection(id, name string, caps []string, stream pb.FoldControl_AgentStreamServer, logger *slog.Logger) *Connection {
	return &Connection{
		ID:           id,
		Name:         name,
		Capabilities: caps,
		stream:       stream,
		pending:      make(map[string]chan *pb.MessageResponse),
		logger:       logger,
	}
}

// Send transmits a ServerMessage to the agent via the GRPC stream.
func (c *Connection) Send(msg *pb.ServerMessage) error {
	return c.stream.Send(msg)
}

// CreateRequest registers a new pending request and returns a channel for responses.
// The caller is responsible for eventually calling CloseRequest to clean up.
func (c *Connection) CreateRequest(requestID string) <-chan *pb.MessageResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan *pb.MessageResponse, 16)
	c.pending[requestID] = ch
	return ch
}

// CloseRequest closes and removes the response channel for a request.
func (c *Connection) CloseRequest(requestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := c.pending[requestID]; ok {
		close(ch)
		delete(c.pending, requestID)
	}
}

// HandleResponse routes a MessageResponse to the appropriate pending request channel.
// If no matching request is found, the response is logged and discarded.
func (c *Connection) HandleResponse(resp *pb.MessageResponse) {
	c.mu.RLock()
	ch, ok := c.pending[resp.GetRequestId()]
	c.mu.RUnlock()

	if !ok {
		c.logger.Warn("received response for unknown request",
			"request_id", resp.GetRequestId(),
			"agent_id", c.ID,
		)
		return
	}

	// Non-blocking send to avoid deadlock if channel is full
	select {
	case ch <- resp:
	default:
		c.logger.Warn("response channel full, dropping message",
			"request_id", resp.GetRequestId(),
			"agent_id", c.ID,
		)
	}
}
