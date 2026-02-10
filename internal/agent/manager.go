// ABOUTME: Manages connected coven-agents, handles registration, and routes messages.
// ABOUTME: Central coordinator for agent connections and message dispatch.

package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// ErrAgentAlreadyRegistered indicates an agent with the same ID is already connected.
var ErrAgentAlreadyRegistered = errors.New("agent already registered")

// ErrAgentNotFound indicates the specified agent was not found.
var ErrAgentNotFound = errors.New("agent not found")

// Manager coordinates all connected agents and routes messages to them.
type Manager struct {
	agents map[string]*Connection
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewManager creates a new Manager instance.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		agents: make(map[string]*Connection),
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
	m.logger.Info("=== AGENT CONNECTED ===",
		"agent_id", agent.ID,
		"name", agent.Name,
		"capabilities", agent.Capabilities,
		"total_agents", len(m.agents),
	)
	return nil
}

// Unregister removes an agent from the manager and closes all pending request channels.
// This unblocks any goroutines waiting for responses from the disconnecting agent.
func (m *Manager) Unregister(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, exists := m.agents[agentID]; exists {
		// Close all pending request channels to unblock waiting goroutines
		agent.Close()
		delete(m.agents, agentID)
		m.logger.Info("=== AGENT DISCONNECTED ===",
			"agent_id", agentID,
			"name", agent.Name,
			"total_agents", len(m.agents),
		)
	}
}

// SendMessage routes a message to a specified agent and returns a channel for responses.
// The channel will receive Response events until a Done or Error event is sent.
// AgentID is required - the caller must specify which agent should receive the message.
func (m *Manager) SendMessage(ctx context.Context, req *SendRequest) (<-chan *Response, error) {
	if req.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}

	agent, ok := m.GetAgent(req.AgentID)
	if !ok {
		return nil, ErrAgentNotFound
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
				Error: "context canceled",
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
// Response builders for each event type.

func buildThinkingResponse(event *pb.MessageResponse_Thinking) *Response {
	return &Response{Event: EventThinking, Text: event.Thinking}
}

func buildTextResponse(event *pb.MessageResponse_Text) *Response {
	return &Response{Event: EventText, Text: event.Text}
}

func buildToolUseResponse(event *pb.MessageResponse_ToolUse) *Response {
	return &Response{
		Event: EventToolUse,
		ToolUse: &ToolUseEvent{
			ID:        event.ToolUse.GetId(),
			Name:      event.ToolUse.GetName(),
			InputJSON: event.ToolUse.GetInputJson(),
		},
	}
}

func buildToolResultResponse(event *pb.MessageResponse_ToolResult) *Response {
	return &Response{
		Event: EventToolResult,
		ToolResult: &ToolResultEvent{
			ID:      event.ToolResult.GetId(),
			Output:  event.ToolResult.GetOutput(),
			IsError: event.ToolResult.GetIsError(),
		},
	}
}

func buildFileResponse(event *pb.MessageResponse_File) *Response {
	return &Response{
		Event: EventFile,
		File: &FileEvent{
			Filename: event.File.GetFilename(),
			MimeType: event.File.GetMimeType(),
			Data:     event.File.GetData(),
		},
	}
}

func buildDoneResponse(event *pb.MessageResponse_Done) *Response {
	return &Response{Event: EventDone, Text: event.Done.GetFullResponse(), Done: true}
}

func buildErrorResponse(event *pb.MessageResponse_Error) *Response {
	return &Response{Event: EventError, Error: event.Error, Done: true}
}

func buildSessionInitResponse(event *pb.MessageResponse_SessionInit) *Response {
	return &Response{Event: EventSessionInit, SessionID: event.SessionInit.GetSessionId()}
}

func buildSessionOrphanedResponse(event *pb.MessageResponse_SessionOrphaned) *Response {
	return &Response{Event: EventSessionOrphaned, Error: event.SessionOrphaned.GetReason()}
}

func buildUsageResponse(event *pb.MessageResponse_Usage) *Response {
	return &Response{
		Event: EventUsage,
		Usage: &UsageEvent{
			InputTokens:      event.Usage.GetInputTokens(),
			OutputTokens:     event.Usage.GetOutputTokens(),
			CacheReadTokens:  event.Usage.GetCacheReadTokens(),
			CacheWriteTokens: event.Usage.GetCacheWriteTokens(),
			ThinkingTokens:   event.Usage.GetThinkingTokens(),
		},
	}
}

func buildToolStateResponse(event *pb.MessageResponse_ToolState) *Response {
	return &Response{
		Event: EventToolState,
		ToolState: &ToolStateEvent{
			ID:     event.ToolState.GetId(),
			State:  toolStateToString(event.ToolState.GetState()),
			Detail: event.ToolState.GetDetail(),
		},
	}
}

func buildCancelledResponse(event *pb.MessageResponse_Cancelled) *Response {
	return &Response{Event: EventCanceled, Error: event.Cancelled.GetReason(), Done: true}
}

func buildToolApprovalRequestResponse(event *pb.MessageResponse_ToolApprovalRequest, requestID string) *Response {
	return &Response{
		Event: EventToolApprovalRequest,
		ToolApprovalRequest: &ToolApprovalRequestEvent{
			ID:        event.ToolApprovalRequest.GetId(),
			Name:      event.ToolApprovalRequest.GetName(),
			InputJSON: event.ToolApprovalRequest.GetInputJson(),
			RequestID: requestID,
		},
	}
}

// convertContentEvent handles message content events (thinking, text, tool interactions, file).
func convertContentEvent(event any) *Response {
	switch e := event.(type) {
	case *pb.MessageResponse_Thinking:
		return buildThinkingResponse(e)
	case *pb.MessageResponse_Text:
		return buildTextResponse(e)
	case *pb.MessageResponse_ToolUse:
		return buildToolUseResponse(e)
	case *pb.MessageResponse_ToolResult:
		return buildToolResultResponse(e)
	case *pb.MessageResponse_File:
		return buildFileResponse(e)
	}
	return nil
}

// convertControlEvent handles control flow events (done, error, session, usage).
func convertControlEvent(event any) *Response {
	switch e := event.(type) {
	case *pb.MessageResponse_Done:
		return buildDoneResponse(e)
	case *pb.MessageResponse_Error:
		return buildErrorResponse(e)
	case *pb.MessageResponse_SessionInit:
		return buildSessionInitResponse(e)
	case *pb.MessageResponse_SessionOrphaned:
		return buildSessionOrphanedResponse(e)
	case *pb.MessageResponse_Usage:
		return buildUsageResponse(e)
	}
	return nil
}

// convertToolStateEvent handles tool state events.
func convertToolStateEvent(event any, requestID string) *Response {
	switch e := event.(type) {
	case *pb.MessageResponse_ToolState:
		return buildToolStateResponse(e)
	case *pb.MessageResponse_Cancelled:
		return buildCancelledResponse(e)
	case *pb.MessageResponse_ToolApprovalRequest:
		return buildToolApprovalRequestResponse(e, requestID)
	}
	return nil
}

// convertResponse converts a protobuf response to the internal Response type.
func (m *Manager) convertResponse(pbResp *pb.MessageResponse) *Response {
	event := pbResp.GetEvent()
	if resp := convertContentEvent(event); resp != nil {
		return resp
	}
	if resp := convertControlEvent(event); resp != nil {
		return resp
	}
	if resp := convertToolStateEvent(event, pbResp.GetRequestId()); resp != nil {
		return resp
	}
	return &Response{}
}

// toolStateToString converts a pb.ToolState enum to a string.
func toolStateToString(state pb.ToolState) string {
	switch state {
	case pb.ToolState_TOOL_STATE_PENDING:
		return "pending"
	case pb.ToolState_TOOL_STATE_AWAITING_APPROVAL:
		return "awaiting_approval"
	case pb.ToolState_TOOL_STATE_RUNNING:
		return "running"
	case pb.ToolState_TOOL_STATE_COMPLETED:
		return "completed"
	case pb.ToolState_TOOL_STATE_FAILED:
		return "failed"
	case pb.ToolState_TOOL_STATE_DENIED:
		return "denied"
	case pb.ToolState_TOOL_STATE_TIMEOUT:
		return "timeout"
	case pb.ToolState_TOOL_STATE_CANCELLED:
		return "canceled"
	default:
		return "unknown"
	}
}

// ListAgents returns information about all connected agents.
func (m *Manager) ListAgents() []*AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, &AgentInfo{
			ID:           agent.ID,
			PrincipalID:  agent.PrincipalID,
			Name:         agent.Name,
			Capabilities: agent.Capabilities,
			Workspaces:   agent.Workspaces,
			WorkingDir:   agent.WorkingDir,
			InstanceID:   agent.InstanceID,
			Backend:      agent.Backend,
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

// IsOnline checks whether an agent with the given ID is currently connected.
// Implements the gateway.AgentChecker interface for use with the Router.
func (m *Manager) IsOnline(agentID string) bool {
	_, ok := m.GetAgent(agentID)
	return ok
}

// GetByInstanceID returns the connection with the given instance ID, or nil if not found.
func (m *Manager) GetByInstanceID(instanceID string) *Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, agent := range m.agents {
		if agent.InstanceID == instanceID {
			return agent
		}
	}
	return nil
}

// GetByPrincipalAndWorkDir finds an online agent matching both principal and working dir.
// This is used for routing messages to bound channels, where a binding stores
// (principal_id, working_dir) and we need to find the specific agent instance.
func (m *Manager) GetByPrincipalAndWorkDir(principalID, workingDir string) *Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, agent := range m.agents {
		if agent.PrincipalID == principalID && agent.WorkingDir == workingDir {
			return agent
		}
	}
	return nil
}

// SendToolApproval sends a tool approval response to an agent.
// toolID must match the ToolApprovalRequest.id from the agent.
func (m *Manager) SendToolApproval(agentID, toolID string, approved, approveAll bool) error {
	agent, ok := m.GetAgent(agentID)
	if !ok {
		return ErrAgentNotFound
	}

	msg := &pb.ServerMessage{
		Payload: &pb.ServerMessage_ToolApproval{
			ToolApproval: &pb.ToolApprovalResponse{
				Id:         toolID,
				Approved:   approved,
				ApproveAll: approveAll,
			},
		},
	}

	if err := agent.Send(msg); err != nil {
		return err
	}

	m.logger.Info("tool approval sent",
		"agent_id", agentID,
		"tool_id", toolID,
		"approved", approved,
		"approve_all", approveAll,
	)

	return nil
}

// SendRequest represents a request to send a message to an agent.
type SendRequest struct {
	ThreadID    string
	Sender      string
	Content     string
	Attachments []Attachment
	AgentID     string // Required: specifies which agent should handle this request
}

// Attachment represents a file attached to a message.
type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// Response represents a response event from an agent.
type Response struct {
	Event               ResponseEvent
	Text                string
	ToolUse             *ToolUseEvent
	ToolResult          *ToolResultEvent
	File                *FileEvent
	Error               string
	Done                bool
	SessionID           string                    // For EventSessionInit
	Usage               *UsageEvent               // For EventUsage
	ToolState           *ToolStateEvent           // For EventToolState
	ToolApprovalRequest *ToolApprovalRequestEvent // For EventToolApprovalRequest
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
	EventSessionInit
	EventSessionOrphaned
	EventUsage               // Token usage update
	EventToolState           // Tool lifecycle state change
	EventCanceled            // Request was canceled
	EventToolApprovalRequest // Tool needs approval before execution
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

// UsageEvent represents token consumption from an LLM call.
type UsageEvent struct {
	InputTokens      int32
	OutputTokens     int32
	CacheReadTokens  int32
	CacheWriteTokens int32
	ThinkingTokens   int32
}

// ToolStateEvent represents a tool lifecycle state change.
type ToolStateEvent struct {
	ID     string
	State  string // "pending", "awaiting_approval", "running", "completed", "failed", "denied", "timeout", "canceled"
	Detail string
}

// ToolApprovalRequestEvent represents a tool awaiting approval before execution.
type ToolApprovalRequestEvent struct {
	ID        string // Tool invocation ID
	Name      string // Tool name
	InputJSON string // Tool input for display
	RequestID string // Request ID for correlation
}

// AgentInfo contains public information about a connected agent.
type AgentInfo struct {
	ID           string
	PrincipalID  string
	Name         string
	Capabilities []string
	Workspaces   []string
	WorkingDir   string
	InstanceID   string
	Backend      string
}
