// ABOUTME: Base pack provides default tools for all agents: log, todo, bbs.
// ABOUTME: Requires the "base" capability.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// BasePack creates the base pack with log, todo, and bbs tools.
func BasePack(s store.BuiltinStore) *packs.BuiltinPack {
	b := &baseHandlers{store: s}
	return &packs.BuiltinPack{
		ID: "builtin:base",
		Tools: []*packs.BuiltinTool{
			// Log tools
			{
				Definition: &pb.ToolDefinition{
					Name:                 "log_entry",
					Description:          "Log an activity or event",
					InputSchemaJson:      `{"type":"object","properties":{"message":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["message"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.LogEntry,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "log_search",
					Description:          "Search past log entries",
					InputSchemaJson:      `{"type":"object","properties":{"query":{"type":"string"},"since":{"type":"string","format":"date-time"},"limit":{"type":"integer"}}}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.LogSearch,
			},
			// Todo tools
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_add",
					Description:          "Create a todo",
					InputSchemaJson:      `{"type":"object","properties":{"description":{"type":"string"},"priority":{"type":"string","enum":["low","medium","high"]},"due_date":{"type":"string","format":"date-time"},"notes":{"type":"string"}},"required":["description"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoAdd,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_list",
					Description:          "List todos",
					InputSchemaJson:      `{"type":"object","properties":{"status":{"type":"string","enum":["pending","in_progress","completed"]},"priority":{"type":"string","enum":["low","medium","high"]}}}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoList,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_update",
					Description:          "Update a todo",
					InputSchemaJson:      `{"type":"object","properties":{"id":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]},"priority":{"type":"string","enum":["low","medium","high"]},"notes":{"type":"string"},"due_date":{"type":"string","format":"date-time"}},"required":["id"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoUpdate,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "todo_delete",
					Description:          "Delete a todo",
					InputSchemaJson:      `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.TodoDelete,
			},
			// BBS tools
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_create_thread",
					Description:          "Create a new discussion thread",
					InputSchemaJson:      `{"type":"object","properties":{"subject":{"type":"string"},"content":{"type":"string"}},"required":["subject","content"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSCreateThread,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_reply",
					Description:          "Reply to a thread",
					InputSchemaJson:      `{"type":"object","properties":{"thread_id":{"type":"string"},"content":{"type":"string"}},"required":["thread_id","content"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSReply,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_list_threads",
					Description:          "List discussion threads",
					InputSchemaJson:      `{"type":"object","properties":{"limit":{"type":"integer"}}}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSListThreads,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "bbs_read_thread",
					Description:          "Read a thread with replies",
					InputSchemaJson:      `{"type":"object","properties":{"thread_id":{"type":"string"}},"required":["thread_id"]}`,
					RequiredCapabilities: []string{"base"},
				},
				Handler: b.BBSReadThread,
			},
		},
	}
}

type baseHandlers struct {
	store store.BuiltinStore
}

// Log handlers

type logEntryInput struct {
	Message string   `json:"message"`
	Tags    []string `json:"tags"`
}

func (b *baseHandlers) LogEntry(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in logEntryInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	entry := &store.LogEntry{
		AgentID: agentID,
		Message: in.Message,
		Tags:    in.Tags,
	}
	if err := b.store.CreateLogEntry(ctx, entry); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"id": entry.ID, "status": "logged"})
}

type logSearchInput struct {
	Query string `json:"query"`
	Since string `json:"since"`
	Limit int    `json:"limit"`
}

func (b *baseHandlers) LogSearch(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in logSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	var since *time.Time
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since date: %w", err)
		}
		since = &t
	}

	entries, err := b.store.SearchLogEntries(ctx, in.Query, since, in.Limit)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"entries": entries, "count": len(entries)})
}

// Todo handlers

type todoAddInput struct {
	Description string `json:"description"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date"`
	Notes       string `json:"notes"`
}

func (b *baseHandlers) TodoAdd(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoAddInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	todo := &store.Todo{
		AgentID:     agentID,
		Description: in.Description,
		Priority:    in.Priority,
		Notes:       in.Notes,
	}
	if in.DueDate != "" {
		t, err := time.Parse(time.RFC3339, in.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		todo.DueDate = &t
	}

	if err := b.store.CreateTodo(ctx, todo); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"id": todo.ID, "status": "created"})
}

type todoListInput struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

func (b *baseHandlers) TodoList(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoListInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	todos, err := b.store.ListTodos(ctx, agentID, in.Status, in.Priority)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"todos": todos, "count": len(todos)})
}

type todoUpdateInput struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Notes    string `json:"notes"`
	DueDate  string `json:"due_date"`
}

func (b *baseHandlers) TodoUpdate(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoUpdateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	todo, err := b.store.GetTodo(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	// Verify ownership - agents can only update their own todos
	if todo.AgentID != agentID {
		return nil, fmt.Errorf("todo not found")
	}

	// Only update fields that were provided
	if in.Status != "" {
		todo.Status = in.Status
	}
	if in.Priority != "" {
		todo.Priority = in.Priority
	}
	if in.Notes != "" {
		todo.Notes = in.Notes
	}
	if in.DueDate != "" {
		t, err := time.Parse(time.RFC3339, in.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		todo.DueDate = &t
	}

	if err := b.store.UpdateTodo(ctx, todo); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"status": "updated"})
}

type todoDeleteInput struct {
	ID string `json:"id"`
}

func (b *baseHandlers) TodoDelete(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in todoDeleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Fetch and verify ownership before deleting
	todo, err := b.store.GetTodo(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if todo.AgentID != agentID {
		return nil, fmt.Errorf("todo not found")
	}

	if err := b.store.DeleteTodo(ctx, in.ID); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"status": "deleted"})
}

// BBS handlers

type bbsCreateThreadInput struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
}

func (b *baseHandlers) BBSCreateThread(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsCreateThreadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	post := &store.BBSPost{
		AgentID: agentID,
		Subject: in.Subject,
		Content: in.Content,
	}
	if err := b.store.CreateBBSPost(ctx, post); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"thread_id": post.ID, "status": "created"})
}

type bbsReplyInput struct {
	ThreadID string `json:"thread_id"`
	Content  string `json:"content"`
}

func (b *baseHandlers) BBSReply(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsReplyInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	post := &store.BBSPost{
		AgentID:  agentID,
		ThreadID: in.ThreadID,
		Content:  in.Content,
	}
	if err := b.store.CreateBBSPost(ctx, post); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"post_id": post.ID, "status": "posted"})
}

type bbsListThreadsInput struct {
	Limit int `json:"limit"`
}

func (b *baseHandlers) BBSListThreads(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsListThreadsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	threads, err := b.store.ListBBSThreads(ctx, in.Limit)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"threads": threads, "count": len(threads)})
}

type bbsReadThreadInput struct {
	ThreadID string `json:"thread_id"`
}

func (b *baseHandlers) BBSReadThread(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in bbsReadThreadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	thread, err := b.store.GetBBSThread(ctx, in.ThreadID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(thread)
}
