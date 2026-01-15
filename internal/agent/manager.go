// ABOUTME: Manages connected fold-agents, handles registration, and routes messages.
// ABOUTME: Central coordinator for agent connections and message dispatch.

package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	pb "github.com/2389/fold-gateway/proto/fold"
)

// ErrAgentAlreadyRegistered indicates an agent with the same ID is already connected.
var ErrAgentAlreadyRegistered = errors.New("agent already registered")

// Manager coordinates all connected agents and routes messages to them.
type Manager struct {
	agents map[string]*Connection
	mu     sync.RWMutex
	router *Router
	logger *slog.Logger
}

// NewManager creates a new Manager instance.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		agents: make(map[string]*Connection),
		router: NewRouter(),
		logger: logger,
	}
}

// Register adds a new agent connection to the manager.
// Returns ErrAgentAlreadyRegistered if an agent with the same ID exists.
func (m *Manager) Register(agent *Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[agent.ID]; exists {
		return ErrAgentAlreadyRegistered
	}

	m.agents[agent.ID] = agent
	m.logger.Info("agent registered",
		"agent_id", agent.ID,
		"name", agent.Name,
		"capabilities", agent.Capabilities,
	)
	return nil
}

// Unregister removes an agent from the manager.
func (m *Manager) Unregister(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, exists := m.agents[agentID]; exists {
		delete(m.agents, agentID)
		m.logger.Info("agent unregistered",
			"agent_id", agentID,
			"name", agent.Name,
		)
	}
}

// SendMessage routes a message to an available agent and returns a channel for responses.
// The channel will receive Response events until a Done or Error event is sent.
func (m *Manager) SendMessage(ctx context.Context, req *SendRequest) (<-chan *Response, error) {
	// Get snapshot of available agents
	m.mu.RLock()
	agents := make([]*Connection, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	m.mu.RUnlock()

	// Select an agent via the router
	agent, err := m.router.SelectAgent(agents)
	if err != nil {
		return nil, err
	}

	// Generate a unique request ID
	requestID := uuid.New().String()

	// Create the request channel on the connection
	respChan := agent.CreateRequest(requestID)

	// Build the protobuf message
	pbMsg := &pb.ServerMessage{
		Payload: &pb.ServerMessage_SendMessage{
			SendMessage: &pb.SendMessage{
				RequestId: requestID,
				ThreadId:  req.ThreadID,
				Sender:    req.Sender,
				Content:   req.Content,
			},
		},
	}

	// Add attachments if present
	if len(req.Attachments) > 0 {
		attachments := make([]*pb.FileAttachment, len(req.Attachments))
		for i, att := range req.Attachments {
			attachments[i] = &pb.FileAttachment{
				Filename: att.Filename,
				MimeType: att.MimeType,
				Data:     att.Data,
			}
		}
		pbMsg.GetSendMessage().Attachments = attachments
	}

	// Send the message
	if err := agent.Send(pbMsg); err != nil {
		agent.CloseRequest(requestID)
		return nil, err
	}

	m.logger.Debug("message sent to agent",
		"agent_id", agent.ID,
		"request_id", requestID,
		"thread_id", req.ThreadID,
	)

	// Create a channel to transform pb responses into Response types
	outChan := make(chan *Response, 16)

	// Start a goroutine to transform responses
	go m.transformResponses(ctx, agent, requestID, respChan, outChan)

	return outChan, nil
}

// transformResponses converts pb.MessageResponse events into Response events.
func (m *Manager) transformResponses(
	ctx context.Context,
	agent *Connection,
	requestID string,
	respChan <-chan *pb.MessageResponse,
	outChan chan<- *Response,
) {
	defer close(outChan)
	defer agent.CloseRequest(requestID)

	for {
		select {
		case <-ctx.Done():
			outChan <- &Response{
				Event: EventError,
				Error: "context cancelled",
				Done:  true,
			}
			return

		case pbResp, ok := <-respChan:
			if !ok {
				return
			}

			resp := m.convertResponse(pbResp)
			outChan <- resp

			if resp.Done {
				return
			}
		}
	}
}

// convertResponse transforms a pb.MessageResponse into a Response.
func (m *Manager) convertResponse(pbResp *pb.MessageResponse) *Response {
	resp := &Response{}

	switch event := pbResp.GetEvent().(type) {
	case *pb.MessageResponse_Thinking:
		resp.Event = EventThinking
		resp.Text = event.Thinking

	case *pb.MessageResponse_Text:
		resp.Event = EventText
		resp.Text = event.Text

	case *pb.MessageResponse_ToolUse:
		resp.Event = EventToolUse
		resp.ToolUse = &ToolUseEvent{
			ID:        event.ToolUse.GetId(),
			Name:      event.ToolUse.GetName(),
			InputJSON: event.ToolUse.GetInputJson(),
		}

	case *pb.MessageResponse_ToolResult:
		resp.Event = EventToolResult
		resp.ToolResult = &ToolResultEvent{
			ID:      event.ToolResult.GetId(),
			Output:  event.ToolResult.GetOutput(),
			IsError: event.ToolResult.GetIsError(),
		}

	case *pb.MessageResponse_File:
		resp.Event = EventFile
		resp.File = &FileEvent{
			Filename: event.File.GetFilename(),
			MimeType: event.File.GetMimeType(),
			Data:     event.File.GetData(),
		}

	case *pb.MessageResponse_Done:
		resp.Event = EventDone
		resp.Text = event.Done.GetFullResponse()
		resp.Done = true

	case *pb.MessageResponse_Error:
		resp.Event = EventError
		resp.Error = event.Error
		resp.Done = true
	}

	return resp
}

// ListAgents returns information about all connected agents.
func (m *Manager) ListAgents() []*AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, &AgentInfo{
			ID:           agent.ID,
			Name:         agent.Name,
			Capabilities: agent.Capabilities,
		})
	}
	return agents
}

// GetAgent retrieves a specific agent by ID.
func (m *Manager) GetAgent(id string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[id]
	return agent, ok
}

// SendRequest represents a request to send a message to an agent.
type SendRequest struct {
	ThreadID    string
	Sender      string
	Content     string
	Attachments []Attachment
}

// Attachment represents a file attached to a message.
type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// Response represents a response event from an agent.
type Response struct {
	Event      ResponseEvent
	Text       string
	ToolUse    *ToolUseEvent
	ToolResult *ToolResultEvent
	File       *FileEvent
	Error      string
	Done       bool
}

// ResponseEvent indicates the type of response event.
type ResponseEvent int

const (
	EventThinking ResponseEvent = iota
	EventText
	EventToolUse
	EventToolResult
	EventFile
	EventDone
	EventError
)

// ToolUseEvent represents a tool invocation by the agent.
type ToolUseEvent struct {
	ID        string
	Name      string
	InputJSON string
}

// ToolResultEvent represents the result of a tool invocation.
type ToolResultEvent struct {
	ID      string
	Output  string
	IsError bool
}

// FileEvent represents a file output from the agent.
type FileEvent struct {
	Filename string
	MimeType string
	Data     []byte
}

// AgentInfo contains public information about a connected agent.
type AgentInfo struct {
	ID           string
	Name         string
	Capabilities []string
}
