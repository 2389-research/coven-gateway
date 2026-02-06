# Admin Agent Messages Tool — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `admin_agent_history` with `admin_agent_messages` — a paginated, chronological tool that returns all events for an agent with token usage stats.

**Architecture:** The new tool uses `store.GetEvents` (keyed on `ConversationKey = agent_id`) for cursor-based pagination and `store.GetUsageStats` (filtered by `AgentID`) for aggregated token usage. This replaces the old thread-iteration approach with a single indexed query. The `AdminPack` signature gains a `store.UsageStore` parameter so it can access usage data.

**Tech Stack:** Go, SQLite (via existing store layer), protobuf tool definitions

---

### Task 1: Update AdminPack signature to accept UsageStore

**Files:**
- Modify: `internal/builtins/admin.go:17-21`
- Modify: `internal/gateway/gateway.go:189`

**Step 1: Write the failing test**

Add a compile-time interface check in `admin.go`. The test that will fail is the existing `TestAdminPackToolDefinitions` because we'll change the tool name from `admin_agent_history` to `admin_agent_messages`. But first, update the signature.

Open `internal/builtins/admin.go` and change:

```go
func AdminPack(mgr *agent.Manager, s store.Store) *packs.BuiltinPack {
	a := &adminHandlers{manager: mgr, store: s}
```

to:

```go
func AdminPack(mgr *agent.Manager, s store.Store, us store.UsageStore) *packs.BuiltinPack {
	a := &adminHandlers{manager: mgr, store: s, usageStore: us}
```

And update the struct:

```go
type adminHandlers struct {
	manager    *agent.Manager
	store      store.Store
	usageStore store.UsageStore
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && go build ./...`
Expected: Compilation errors because callers pass 2 args instead of 3.

**Step 3: Fix callers**

In `internal/gateway/gateway.go`, change line 189 from:

```go
if err := packRegistry.RegisterBuiltinPack(builtins.AdminPack(agentMgr, s)); err != nil {
```

to:

```go
if err := packRegistry.RegisterBuiltinPack(builtins.AdminPack(agentMgr, s, builtinStore)); err != nil {
```

`builtinStore` is already the `*store.SQLiteStore` (line 185) which implements `UsageStore`.

In all test files (`internal/builtins/admin_test.go`), update `AdminPack(mgr, s)` calls to `AdminPack(mgr, s, s)` since `*store.SQLiteStore` implements both interfaces.

**Step 4: Run tests to verify they pass**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && go build ./... && go test ./internal/builtins/...`
Expected: Build succeeds. Tests pass (tool behavior unchanged so far).

**Step 5: Commit**

```bash
git add internal/builtins/admin.go internal/gateway/gateway.go internal/builtins/admin_test.go
git commit -m "refactor: add UsageStore parameter to AdminPack"
```

---

### Task 2: Replace admin_agent_history with admin_agent_messages

**Files:**
- Modify: `internal/builtins/admin.go`
- Modify: `internal/builtins/admin_test.go`

**Step 1: Write the failing test**

In `admin_test.go`, replace the three `TestAdminAgentHistory_*` tests with tests for `admin_agent_messages`. Write these tests first:

```go
func TestAdminAgentMessages_RequiresAgentID(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	_, err := handler(context.Background(), "admin-agent", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error when agent_id is missing")
	}
}

func TestAdminAgentMessages_EmptyHistory(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	result, err := handler(context.Background(), "admin-agent", json.RawMessage(`{"agent_id": "unknown-agent"}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp["count"].(float64) != 0 {
		t.Errorf("expected count 0, got %v", resp["count"])
	}
	if resp["has_more"].(bool) != false {
		t.Error("expected has_more false")
	}
	// Should include usage stats
	usage := resp["usage"].(map[string]any)
	if usage["total_tokens"].(float64) != 0 {
		t.Errorf("expected total_tokens 0, got %v", usage["total_tokens"])
	}
}

func TestAdminAgentMessages_WithEvents(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	ctx := context.Background()
	agentID := "test-agent"

	// Save events with conversation_key = agent_id (matches how conversation service works)
	threadID := "thread-1"
	content1 := "Hello agent"
	content2 := "Agent response"
	now := time.Now()

	evt1 := &store.LedgerEvent{
		ID:              "evt-1",
		ConversationKey: agentID,
		ThreadID:        &threadID,
		Direction:       store.EventDirectionInbound,
		Author:          "user",
		Timestamp:       now,
		Type:            store.EventTypeMessage,
		Text:            &content1,
	}
	evt2 := &store.LedgerEvent{
		ID:              "evt-2",
		ConversationKey: agentID,
		ThreadID:        &threadID,
		Direction:       store.EventDirectionOutbound,
		Author:          "agent:" + agentID,
		Timestamp:       now.Add(time.Second),
		Type:            store.EventTypeMessage,
		Text:            &content2,
	}
	if err := s.SaveEvent(ctx, evt1); err != nil {
		t.Fatalf("save event 1: %v", err)
	}
	if err := s.SaveEvent(ctx, evt2); err != nil {
		t.Fatalf("save event 2: %v", err)
	}

	// Save usage data for the agent
	if err := s.SaveUsage(ctx, &store.TokenUsage{
		ID:          "usage-1",
		ThreadID:    threadID,
		RequestID:   "req-1",
		AgentID:     agentID,
		InputTokens: 100,
		OutputTokens: 50,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save usage: %v", err)
	}

	// Query messages for the agent
	result, err := handler(ctx, "admin-agent", json.RawMessage(`{"agent_id": "test-agent"}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}

	events := resp["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify chronological order
	first := events[0].(map[string]any)
	if first["id"].(string) != "evt-1" {
		t.Errorf("expected first event id 'evt-1', got %q", first["id"])
	}
	if first["direction"].(string) != "inbound_to_agent" {
		t.Errorf("expected inbound direction, got %q", first["direction"])
	}

	// Verify usage stats
	usage := resp["usage"].(map[string]any)
	if usage["total_input"].(float64) != 100 {
		t.Errorf("expected total_input 100, got %v", usage["total_input"])
	}
	if usage["total_output"].(float64) != 50 {
		t.Errorf("expected total_output 50, got %v", usage["total_output"])
	}
	if usage["request_count"].(float64) != 1 {
		t.Errorf("expected request_count 1, got %v", usage["request_count"])
	}
}

func TestAdminAgentMessages_Pagination(t *testing.T) {
	mgr := agent.NewManager(slog.Default())
	s := newTestStore(t)
	pack := AdminPack(mgr, s, s)

	handler := findHandler(pack, "admin_agent_messages")
	if handler == nil {
		t.Fatal("admin_agent_messages handler not found")
	}

	ctx := context.Background()
	agentID := "test-agent"
	threadID := "thread-1"
	now := time.Now()

	// Create 3 events
	for i := 0; i < 3; i++ {
		content := fmt.Sprintf("message %d", i)
		evt := &store.LedgerEvent{
			ID:              fmt.Sprintf("evt-%d", i),
			ConversationKey: agentID,
			ThreadID:        &threadID,
			Direction:       store.EventDirectionInbound,
			Author:          "user",
			Timestamp:       now.Add(time.Duration(i) * time.Second),
			Type:            store.EventTypeMessage,
			Text:            &content,
		}
		if err := s.SaveEvent(ctx, evt); err != nil {
			t.Fatalf("save event %d: %v", i, err)
		}
	}

	// Request with limit 2 — should get first 2 and has_more=true
	result, err := handler(ctx, "admin-agent", json.RawMessage(`{"agent_id": "test-agent", "limit": 2}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}
	if resp["has_more"].(bool) != true {
		t.Error("expected has_more true")
	}
	cursor := resp["next_cursor"].(string)
	if cursor == "" {
		t.Error("expected non-empty next_cursor")
	}

	// Request with cursor — should get the remaining event
	input := fmt.Sprintf(`{"agent_id": "test-agent", "limit": 2, "cursor": %q}`, cursor)
	result2, err := handler(ctx, "admin-agent", json.RawMessage(input))
	if err != nil {
		t.Fatalf("handler error on page 2: %v", err)
	}

	var resp2 map[string]any
	if err := json.Unmarshal(result2, &resp2); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if resp2["count"].(float64) != 1 {
		t.Errorf("expected count 1 on page 2, got %v", resp2["count"])
	}
	if resp2["has_more"].(bool) != false {
		t.Error("expected has_more false on page 2")
	}
}
```

Also update `TestAdminPackToolDefinitions` to expect `admin_agent_messages` instead of `admin_agent_history`.

**Step 2: Run tests to verify they fail**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && go test ./internal/builtins/... -run TestAdminAgent`
Expected: FAIL — `admin_agent_messages` handler not found

**Step 3: Implement admin_agent_messages handler**

In `admin.go`, replace the `admin_agent_history` tool definition and handler:

```go
// In AdminPack function, replace the admin_agent_history tool block:
{
    Definition: &pb.ToolDefinition{
        Name:                 "admin_agent_messages",
        Description:          "Read all messages for an agent in chronological order with token usage stats. Supports cursor-based pagination.",
        InputSchemaJson:      `{"type":"object","properties":{"agent_id":{"type":"string","description":"The agent to inspect"},"limit":{"type":"integer","description":"Events per page (default 50, max 500)"},"cursor":{"type":"string","description":"Pagination cursor from previous response"}},"required":["agent_id"]}`,
        RequiredCapabilities: []string{"admin"},
    },
    Handler: a.AgentMessages,
},
```

Replace `agentHistoryInput` and `AgentHistory` with:

```go
type agentMessagesInput struct {
	AgentID string `json:"agent_id"`
	Limit   int    `json:"limit"`
	Cursor  string `json:"cursor"`
}

// AgentMessages retrieves all events for an agent in chronological order with usage stats.
func (a *adminHandlers) AgentMessages(ctx context.Context, callerAgentID string, input json.RawMessage) (json.RawMessage, error) {
	var in agentMessagesInput
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

	// Get paginated events keyed by agent's conversation key
	result, err := a.store.GetEvents(ctx, store.GetEventsParams{
		ConversationKey: in.AgentID,
		Limit:           limit,
		Cursor:          in.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}

	// Convert events to a JSON-friendly format with timing
	events := make([]map[string]any, len(result.Events))
	for i, evt := range result.Events {
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
		events[i] = e
	}

	// Get aggregated usage stats for the agent
	agentID := in.AgentID
	stats, err := a.usageStore.GetUsageStats(ctx, store.UsageFilter{
		AgentID: &agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying usage stats: %w", err)
	}

	resp := map[string]any{
		"agent_id": in.AgentID,
		"events":   events,
		"count":    len(events),
		"has_more": result.HasMore,
		"usage": map[string]any{
			"total_input":       stats.TotalInput,
			"total_output":      stats.TotalOutput,
			"total_cache_read":  stats.TotalCacheRead,
			"total_cache_write": stats.TotalCacheWrite,
			"total_thinking":    stats.TotalThinking,
			"total_tokens":      stats.TotalTokens,
			"request_count":     stats.RequestCount,
		},
	}

	if result.NextCursor != "" {
		resp["next_cursor"] = result.NextCursor
	}

	return json.Marshal(resp)
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && go test ./internal/builtins/... -v`
Expected: All tests pass.

**Step 5: Run full test suite**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && go test ./...`
Expected: All tests pass (no other code references `admin_agent_history` as a handler).

**Step 6: Commit**

```bash
git add internal/builtins/admin.go internal/builtins/admin_test.go
git commit -m "feat: replace admin_agent_history with admin_agent_messages

Paginated chronological event stream per agent with token usage stats.
Supports cursor-based pagination via GetEvents store method."
```

---

### Task 3: Verify end-to-end build and test

**Files:** None (verification only)

**Step 1: Build all binaries**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && make build`
Expected: Clean build, no errors.

**Step 2: Run full test suite with race detection**

Run: `cd /Users/harper/Public/src/2389/fold-project/coven-gateway && go test -race ./...`
Expected: All tests pass, no races detected.

**Step 3: Commit (if any fixes needed)**

Only commit if fixes were required during verification.
