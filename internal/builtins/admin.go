// ABOUTME: Admin pack provides administrative tools for agents.
// ABOUTME: Requires the "admin" capability.

package builtins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// AdminPack creates the admin pack with agent management tools.
func AdminPack(mgr *agent.Manager, s store.Store, us store.UsageStore) *packs.BuiltinPack {
	a := &adminHandlers{manager: mgr, store: s, usageStore: us}
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
					Name:                 "admin_agent_messages",
					Description:          "Read all messages for an agent in chronological order with token usage stats. Supports cursor-based pagination.",
					InputSchemaJson:      `{"type":"object","properties":{"agent_id":{"type":"string","description":"The agent to inspect"},"limit":{"type":"integer","description":"Events per page (default 50, max 500)"},"cursor":{"type":"string","description":"Pagination cursor from previous response"}},"required":["agent_id"]}`,
					RequiredCapabilities: []string{"admin"},
				},
				Handler: a.AgentMessages,
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
	manager    *agent.Manager
	store      store.Store
	usageStore store.UsageStore
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

type agentMessagesInput struct {
	AgentID string `json:"agent_id"`
	Limit   int    `json:"limit"`
	Cursor  string `json:"cursor"`
}

// clampLimit returns limit clamped to [1, 500] with default 50.
func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 500:
		return 500
	default:
		return limit
	}
}

// convertEventToMap converts a store ledger event to a JSON-friendly map.
func convertEventToMap(evt store.LedgerEvent) map[string]any {
	e := map[string]any{
		"id":        evt.ID,
		"direction": string(evt.Direction),
		"author":    evt.Author,
		"type":      string(evt.Type),
		"timestamp": evt.Timestamp.Format(time.RFC3339),
	}
	if evt.ThreadID != nil {
		e["thread_id"] = *evt.ThreadID
	}
	if evt.Text != nil {
		e["text"] = *evt.Text
	}
	return e
}

// usageToMap converts usage stats to a JSON-friendly map.
func usageToMap(stats *store.UsageStats) map[string]any {
	return map[string]any{
		"total_input":       stats.TotalInput,
		"total_output":      stats.TotalOutput,
		"total_cache_read":  stats.TotalCacheRead,
		"total_cache_write": stats.TotalCacheWrite,
		"total_thinking":    stats.TotalThinking,
		"total_tokens":      stats.TotalTokens,
		"request_count":     stats.RequestCount,
	}
}

// AgentMessages retrieves all events for an agent in chronological order with usage stats.
func (a *adminHandlers) AgentMessages(ctx context.Context, callerAgentID string, input json.RawMessage) (json.RawMessage, error) {
	var in agentMessagesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	if in.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}

	result, err := a.store.GetEvents(ctx, store.GetEventsParams{
		ConversationKey: in.AgentID,
		Limit:           clampLimit(in.Limit),
		Cursor:          in.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}

	events := make([]map[string]any, len(result.Events))
	for i, evt := range result.Events {
		events[i] = convertEventToMap(evt)
	}

	agentID := in.AgentID
	stats, err := a.usageStore.GetUsageStats(ctx, store.UsageFilter{AgentID: &agentID})
	if err != nil {
		return nil, fmt.Errorf("querying usage stats: %w", err)
	}

	resp := map[string]any{
		"agent_id": in.AgentID,
		"events":   events,
		"count":    len(events),
		"has_more": result.HasMore,
		"usage":    usageToMap(stats),
	}
	if result.NextCursor != "" {
		resp["next_cursor"] = result.NextCursor
	}
	return json.Marshal(resp)
}

type sendMessageInput struct {
	AgentID string `json:"agent_id"`
	Content string `json:"content"`
}

// collectResponses reads from the response channel and returns the final text and done state.
func collectResponses(respChan <-chan *agent.Response) (string, bool, error) {
	var textParts []string
	var fullResponse string
	var gotDone bool

	for resp := range respChan {
		switch resp.Event {
		case agent.EventText:
			textParts = append(textParts, resp.Text)
		case agent.EventDone:
			fullResponse = resp.Text
			gotDone = true
		case agent.EventError:
			return "", false, fmt.Errorf("agent error: %s", resp.Error)
		}
	}

	if fullResponse == "" {
		var fullResponseSb201 strings.Builder
		for _, part := range textParts {
			fullResponseSb201.WriteString(part)
		}
		fullResponse += fullResponseSb201.String()
	}
	return fullResponse, gotDone, nil
}

// SendMessage sends a message to another agent.
func (a *adminHandlers) SendMessage(ctx context.Context, callerAgentID string, input json.RawMessage) (json.RawMessage, error) {
	var in sendMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	if in.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}
	if in.Content == "" {
		return nil, errors.New("content is required")
	}

	req := &agent.SendRequest{
		AgentID: in.AgentID,
		Content: in.Content,
		Sender:  "admin:" + callerAgentID,
	}

	respChan, err := a.manager.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sending message: %w", err)
	}

	responseText, gotDone, err := collectResponses(respChan)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"status":   "sent",
		"agent_id": in.AgentID,
		"response": responseText,
		"done":     gotDone,
	})
}
