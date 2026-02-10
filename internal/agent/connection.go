// ABOUTME: Represents a single connected agent and manages its bidirectional stream.
// ABOUTME: Handles sending messages and routing responses by request ID.

package agent

import (
	"log/slog"
	"sync"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// Connection represents a connected agent with its GRPC stream.
type Connection struct {
	ID           string
	Name         string
	Capabilities []string
	PrincipalID  string   // Authenticated principal UUID (for audit)
	Workspaces   []string // From registration metadata
	WorkingDir   string   // From registration metadata
	InstanceID   string   // Short code for binding commands
	Backend      string   // Backend type: "mux", "cli", "acp", "direct"

	stream  pb.CovenControl_AgentStreamServer
	pending map[string]chan *pb.MessageResponse
	mu      sync.RWMutex
	logger  *slog.Logger
}

// ConnectionParams contains the parameters needed to create a new Connection.
type ConnectionParams struct {
	ID           string
	Name         string
	Capabilities []string
	PrincipalID  string
	Workspaces   []string
	WorkingDir   string
	InstanceID   string
	Backend      string
	Stream       pb.CovenControl_AgentStreamServer
	Logger       *slog.Logger
}

// NewConnection creates a new Connection for a connected agent.
func NewConnection(params ConnectionParams) *Connection {
	return &Connection{
		ID:           params.ID,
		Name:         params.Name,
		Capabilities: params.Capabilities,
		PrincipalID:  params.PrincipalID,
		Workspaces:   params.Workspaces,
		WorkingDir:   params.WorkingDir,
		InstanceID:   params.InstanceID,
		Backend:      params.Backend,
		stream:       params.Stream,
		pending:      make(map[string]chan *pb.MessageResponse),
		logger:       params.Logger,
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

// Close closes all pending request channels and releases resources.
// This should be called when the connection is being terminated to unblock
// any goroutines waiting on response channels.
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for requestID, ch := range c.pending {
		close(ch)
		delete(c.pending, requestID)
	}
}

// HandleResponse routes a MessageResponse to the appropriate pending request channel.
// If no matching request is found, the response is logged and discarded.
func (c *Connection) HandleResponse(resp *pb.MessageResponse) {
	c.mu.RLock()
	ch, ok := c.pending[resp.GetRequestId()]
	if !ok {
		c.mu.RUnlock()
		c.logger.Warn("received response for unknown request",
			"request_id", resp.GetRequestId(),
			"agent_id", c.ID,
		)
		return
	}

	// Non-blocking send to avoid deadlock if channel is full.
	// Keep RLock held to prevent Close/CloseRequest from closing channel mid-send.
	select {
	case ch <- resp:
	default:
		c.logger.Warn("response channel full, dropping message",
			"request_id", resp.GetRequestId(),
			"agent_id", c.ID,
		)
	}
	c.mu.RUnlock()
}
