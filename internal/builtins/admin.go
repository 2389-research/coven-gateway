// ABOUTME: Admin pack provides administrative tools for agents.
// ABOUTME: Requires the "admin" capability.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// AdminPack creates the admin pack with agent management tools.
func AdminPack(mgr *agent.Manager, s store.Store) *packs.BuiltinPack {
	a := &adminHandlers{manager: mgr, store: s}
	return &packs.BuiltinPack{
		ID: "builtin:admin",
		Tools: []*packs.BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:                 "admin_list_agents",
					Description:          "List connected agents",
					InputSchemaJson:      `{"type":"object","properties":{}}`,
					RequiredCapabilities: []string{"admin"},
				},
				Handler: a.ListAgents,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "admin_agent_history",
					Description:          "Read an agent's message history",
					InputSchemaJson:      `{"type":"object","properties":{"agent_id":{"type":"string"},"limit":{"type":"integer"}},"required":["agent_id"]}`,
					RequiredCapabilities: []string{"admin"},
				},
				Handler: a.AgentHistory,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "admin_send_message",
					Description:          "Send message to another agent",
					InputSchemaJson:      `{"type":"object","properties":{"agent_id":{"type":"string"},"content":{"type":"string"}},"required":["agent_id","content"]}`,
					RequiredCapabilities: []string{"admin"},
				},
				Handler: a.SendMessage,
			},
		},
	}
}

type adminHandlers struct {
	manager *agent.Manager
	store   store.Store
}

// ListAgents returns information about all connected agents.
func (a *adminHandlers) ListAgents(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	agents := a.manager.ListAgents()

	// Convert to a simpler representation for JSON output
	result := make([]map[string]any, len(agents))
	for i, ag := range agents {
		result[i] = map[string]any{
			"id":           ag.ID,
			"name":         ag.Name,
			"capabilities": ag.Capabilities,
			"workspaces":   ag.Workspaces,
			"working_dir":  ag.WorkingDir,
			"instance_id":  ag.InstanceID,
			"backend":      ag.Backend,
		}
	}

	return json.Marshal(map[string]any{
		"agents": result,
		"count":  len(result),
	})
}

type agentHistoryInput struct {
	AgentID string `json:"agent_id"`
	Limit   int    `json:"limit"`
}

// AgentHistory retrieves message history for a specific agent.
func (a *adminHandlers) AgentHistory(ctx context.Context, callerAgentID string, input json.RawMessage) (json.RawMessage, error) {
	var in agentHistoryInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if in.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	// Get all threads and filter by agent_id
	threads, err := a.store.ListThreads(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("listing threads: %w", err)
	}

	// Collect messages from threads belonging to this agent
	var allMessages []*store.Message
	for _, thread := range threads {
		if thread.AgentID != in.AgentID {
			continue
		}

		messages, err := a.store.GetThreadMessages(ctx, thread.ID, limit)
		if err != nil {
			continue // Skip threads we can't read
		}
		allMessages = append(allMessages, messages...)

		// Stop if we have enough messages
		if len(allMessages) >= limit {
			allMessages = allMessages[:limit]
			break
		}
	}

	return json.Marshal(map[string]any{
		"agent_id": in.AgentID,
		"messages": allMessages,
		"count":    len(allMessages),
	})
}

type sendMessageInput struct {
	AgentID string `json:"agent_id"`
	Content string `json:"content"`
}

// SendMessage sends a message to another agent.
func (a *adminHandlers) SendMessage(ctx context.Context, callerAgentID string, input json.RawMessage) (json.RawMessage, error) {
	var in sendMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if in.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if in.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// Create the send request
	req := &agent.SendRequest{
		AgentID: in.AgentID,
		Content: in.Content,
		Sender:  "admin:" + callerAgentID,
	}

	// Send the message
	respChan, err := a.manager.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sending message: %w", err)
	}

	// Collect responses until done
	var textParts []string
	var responseText string
	var gotDone bool

	for resp := range respChan {
		switch resp.Event {
		case agent.EventText:
			textParts = append(textParts, resp.Text)
		case agent.EventDone:
			responseText = resp.Text
			gotDone = true
		case agent.EventError:
			return nil, fmt.Errorf("agent error: %s", resp.Error)
		}
	}

	// Use full response if available, otherwise concatenate text parts
	if responseText == "" && len(textParts) > 0 {
		for _, part := range textParts {
			responseText += part
		}
	}

	return json.Marshal(map[string]any{
		"status":   "sent",
		"agent_id": in.AgentID,
		"response": responseText,
		"done":     gotDone,
	})
}
