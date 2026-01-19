# Bindings Flow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable users to bind chat channels to agents via `/fold bind <instance-id>` commands.

**Architecture:** Add `working_dir` to bindings table, add agent lookup by instance_id to manager, create HTTP API endpoints for bind/unbind/status/agents, integrate commands into Matrix bridge.

**Tech Stack:** Go, SQLite, gRPC, Matrix (mautrix-go)

---

## Task 1: Add working_dir to bindings table

**Files:**
- Modify: `internal/store/sqlite.go` (schema migration)
- Modify: `internal/store/bindings.go` (model + methods)
- Test: `internal/store/bindings_test.go`

**Step 1: Write the failing test**

Add to `internal/store/bindings_test.go`:

```go
func TestCreateBindingV2_WithWorkingDir(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	// Create an agent principal first
	principal := &Principal{
		ID:          "agent-uuid",
		Type:        PrincipalTypeAgent,
		DisplayName: "test-agent",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}
	err := s.CreatePrincipal(context.Background(), principal)
	require.NoError(t, err)

	binding := &Binding{
		ID:         "binding-uuid",
		Frontend:   "matrix",
		ChannelID:  "!room:server",
		PrincipalID: "agent-uuid",
		WorkingDir: "/projects/website",
		CreatedAt:  time.Now().UTC(),
	}

	err = s.CreateBindingV2(context.Background(), binding)
	require.NoError(t, err)

	// Retrieve and verify
	got, err := s.GetBindingByChannel(context.Background(), "matrix", "!room:server")
	require.NoError(t, err)
	assert.Equal(t, "/projects/website", got.WorkingDir)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestCreateBindingV2_WithWorkingDir -v`
Expected: FAIL - `Binding` struct doesn't have `WorkingDir` field

**Step 3: Update schema and model**

In `internal/store/sqlite.go`, update the bindings table creation (find existing CREATE TABLE):

```go
CREATE TABLE IF NOT EXISTS bindings (
    binding_id TEXT PRIMARY KEY,
    frontend TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    working_dir TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT,
    UNIQUE(frontend, channel_id)
)
```

In `internal/store/bindings.go`, update the `Binding` struct:

```go
type Binding struct {
	ID          string
	Frontend    string
	ChannelID   string
	AgentID     string    // principal_id of agent (renamed for clarity but keep DB column)
	WorkingDir  string    // agent's working directory
	CreatedAt   time.Time
	CreatedBy   *string
}
```

Update `CreateBindingV2` to include working_dir:

```go
query := `
    INSERT INTO bindings (binding_id, frontend, channel_id, agent_id, working_dir, created_at, created_by)
    VALUES (?, ?, ?, ?, ?, ?, ?)
`

_, err := s.db.ExecContext(ctx, query,
    b.ID,
    b.Frontend,
    b.ChannelID,
    b.AgentID,
    b.WorkingDir,
    b.CreatedAt.UTC().Format(time.RFC3339),
    b.CreatedBy,
)
```

Update `scanBinding` and `scanBindingRow` to include working_dir:

```go
func (s *SQLiteStore) scanBinding(row *sql.Row) (*Binding, error) {
	var b Binding
	var createdAtStr string
	var createdBy *string
	var workingDir sql.NullString

	err := row.Scan(
		&b.ID,
		&b.Frontend,
		&b.ChannelID,
		&b.AgentID,
		&workingDir,
		&createdAtStr,
		&createdBy,
	)
	// ... handle errors ...

	if workingDir.Valid {
		b.WorkingDir = workingDir.String
	}
	// ...
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -run TestCreateBindingV2_WithWorkingDir -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/sqlite.go internal/store/bindings.go internal/store/bindings_test.go
git commit -m "feat(store): add working_dir to bindings table"
```

---

## Task 2: Add GetAgentByInstanceID to agent manager

**Files:**
- Modify: `internal/agent/manager.go`
- Test: `internal/agent/manager_test.go`

**Step 1: Write the failing test**

Add to `internal/agent/manager_test.go`:

```go
func TestManager_GetByInstanceID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewManager(logger)

	// Register a mock connection
	conn := &Connection{
		ID:          "bob-projects-website",
		Name:        "bob",
		PrincipalID: "principal-uuid",
		WorkingDir:  "/projects/website",
		InstanceID:  "0fb8187d-c06",
	}
	m.connections["bob-projects-website"] = conn

	// Lookup by instance ID
	got := m.GetByInstanceID("0fb8187d-c06")
	require.NotNil(t, got)
	assert.Equal(t, "bob-projects-website", got.ID)
	assert.Equal(t, "/projects/website", got.WorkingDir)

	// Lookup non-existent
	got = m.GetByInstanceID("nonexistent")
	assert.Nil(t, got)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/... -run TestManager_GetByInstanceID -v`
Expected: FAIL - `GetByInstanceID` method doesn't exist

**Step 3: Implement GetByInstanceID**

Add to `internal/agent/manager.go`:

```go
// GetByInstanceID returns the connection with the given instance ID, or nil if not found.
func (m *Manager) GetByInstanceID(instanceID string) *Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, conn := range m.connections {
		if conn.InstanceID == instanceID {
			return conn
		}
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/... -run TestManager_GetByInstanceID -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/manager.go internal/agent/manager_test.go
git commit -m "feat(agent): add GetByInstanceID lookup method"
```

---

## Task 3: Add POST /api/bindings endpoint

**Files:**
- Modify: `internal/gateway/api.go`
- Test: `internal/gateway/api_test.go`

**Step 1: Write the failing test**

Add to `internal/gateway/api_test.go`:

```go
func TestCreateBindingByInstanceID(t *testing.T) {
	// Setup gateway with mock store and agent manager
	gw := setupTestGateway(t)

	// Register a mock agent
	conn := &agent.Connection{
		ID:          "bob-projects-website",
		Name:        "bob",
		PrincipalID: "principal-uuid",
		WorkingDir:  "/projects/website",
		InstanceID:  "0fb8187d-c06",
	}
	gw.agentManager.Register(conn)

	// Create binding request
	body := `{"frontend":"matrix","channel_id":"!room:server","instance_id":"0fb8187d-c06"}`
	req := httptest.NewRequest("POST", "/api/bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Add auth context (mock authenticated user)
	ctx := auth.WithAuth(req.Context(), &auth.AuthContext{PrincipalID: "user-uuid"})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	gw.handleCreateBinding(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "bob", resp["agent_name"])
	assert.Equal(t, "/projects/website", resp["working_dir"])
}

func TestCreateBindingByInstanceID_NotFound(t *testing.T) {
	gw := setupTestGateway(t)

	body := `{"frontend":"matrix","channel_id":"!room:server","instance_id":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithAuth(req.Context(), &auth.AuthContext{PrincipalID: "user-uuid"})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	gw.handleCreateBinding(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/... -run TestCreateBindingByInstanceID -v`
Expected: FAIL - `handleCreateBinding` doesn't exist

**Step 3: Implement handleCreateBinding**

Add to `internal/gateway/api.go`:

```go
type createBindingRequest struct {
	Frontend   string `json:"frontend"`
	ChannelID  string `json:"channel_id"`
	InstanceID string `json:"instance_id"`
}

type createBindingResponse struct {
	BindingID   string  `json:"binding_id"`
	AgentName   string  `json:"agent_name"`
	WorkingDir  string  `json:"working_dir"`
	ReboundFrom *string `json:"rebound_from,omitempty"`
}

func (g *Gateway) handleCreateBinding(w http.ResponseWriter, r *http.Request) {
	var req createBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Lookup agent by instance ID
	conn := g.agentManager.GetByInstanceID(req.InstanceID)
	if conn == nil {
		http.Error(w, fmt.Sprintf(`{"error":"no agent online with instance_id '%s'"}`, req.InstanceID), http.StatusNotFound)
		return
	}

	// Get auth context for created_by
	authCtx := auth.FromContext(r.Context())
	var createdBy *string
	if authCtx != nil {
		createdBy = &authCtx.PrincipalID
	}

	// Check if binding already exists
	existing, err := g.store.GetBindingByChannel(r.Context(), req.Frontend, req.ChannelID)
	var reboundFrom *string
	if err == nil && existing != nil {
		// Already bound - check if same agent
		if existing.AgentID == conn.PrincipalID && existing.WorkingDir == conn.WorkingDir {
			// Same agent, return success
			json.NewEncoder(w).Encode(createBindingResponse{
				BindingID:  existing.ID,
				AgentName:  conn.Name,
				WorkingDir: conn.WorkingDir,
			})
			return
		}
		// Different agent - delete old binding first
		oldName := existing.AgentID // TODO: lookup name
		reboundFrom = &oldName
		g.store.DeleteBindingByID(r.Context(), existing.ID)
	}

	// Create new binding
	binding := &store.Binding{
		ID:         uuid.New().String(),
		Frontend:   req.Frontend,
		ChannelID:  req.ChannelID,
		AgentID:    conn.PrincipalID,
		WorkingDir: conn.WorkingDir,
		CreatedAt:  time.Now().UTC(),
		CreatedBy:  createdBy,
	}

	if err := g.store.CreateBindingV2(r.Context(), binding); err != nil {
		g.logger.Error("failed to create binding", "error", err)
		http.Error(w, `{"error":"failed to create binding"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createBindingResponse{
		BindingID:   binding.ID,
		AgentName:   conn.Name,
		WorkingDir:  conn.WorkingDir,
		ReboundFrom: reboundFrom,
	})
}
```

Register the route (find where routes are registered):

```go
mux.HandleFunc("POST /api/bindings", g.handleCreateBinding)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway/... -run TestCreateBindingByInstanceID -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/gateway/api.go internal/gateway/api_test.go
git commit -m "feat(api): add POST /api/bindings endpoint"
```

---

## Task 4: Add DELETE and GET /api/bindings endpoints

**Files:**
- Modify: `internal/gateway/api.go`
- Test: `internal/gateway/api_test.go`

**Step 1: Write the failing tests**

Add to `internal/gateway/api_test.go`:

```go
func TestDeleteBinding(t *testing.T) {
	gw := setupTestGateway(t)

	// Create a binding first
	binding := &store.Binding{
		ID:         "binding-uuid",
		Frontend:   "matrix",
		ChannelID:  "!room:server",
		AgentID:    "agent-uuid",
		WorkingDir: "/projects/website",
		CreatedAt:  time.Now().UTC(),
	}
	gw.store.CreateBindingV2(context.Background(), binding)

	req := httptest.NewRequest("DELETE", "/api/bindings?frontend=matrix&channel_id=!room:server", nil)
	w := httptest.NewRecorder()
	gw.handleDeleteBinding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetBindingStatus(t *testing.T) {
	gw := setupTestGateway(t)

	// Create a binding
	binding := &store.Binding{
		ID:         "binding-uuid",
		Frontend:   "matrix",
		ChannelID:  "!room:server",
		AgentID:    "agent-uuid",
		WorkingDir: "/projects/website",
		CreatedAt:  time.Now().UTC(),
	}
	gw.store.CreateBindingV2(context.Background(), binding)

	// Register agent so it shows as online
	conn := &agent.Connection{
		ID:          "bob-projects-website",
		PrincipalID: "agent-uuid",
		Name:        "bob",
		WorkingDir:  "/projects/website",
	}
	gw.agentManager.Register(conn)

	req := httptest.NewRequest("GET", "/api/bindings?frontend=matrix&channel_id=!room:server", nil)
	w := httptest.NewRecorder()
	gw.handleGetBinding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "/projects/website", resp["working_dir"])
	assert.Equal(t, true, resp["online"])
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/gateway/... -run "TestDeleteBinding|TestGetBindingStatus" -v`
Expected: FAIL

**Step 3: Implement handlers**

Add to `internal/gateway/api.go`:

```go
func (g *Gateway) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	frontend := r.URL.Query().Get("frontend")
	channelID := r.URL.Query().Get("channel_id")

	if frontend == "" || channelID == "" {
		http.Error(w, `{"error":"frontend and channel_id required"}`, http.StatusBadRequest)
		return
	}

	binding, err := g.store.GetBindingByChannel(r.Context(), frontend, channelID)
	if err != nil {
		http.Error(w, `{"error":"no binding for this channel"}`, http.StatusNotFound)
		return
	}

	if err := g.store.DeleteBindingByID(r.Context(), binding.ID); err != nil {
		http.Error(w, `{"error":"failed to delete binding"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

type getBindingResponse struct {
	BindingID  string `json:"binding_id"`
	AgentName  string `json:"agent_name"`
	WorkingDir string `json:"working_dir"`
	Online     bool   `json:"online"`
}

func (g *Gateway) handleGetBinding(w http.ResponseWriter, r *http.Request) {
	frontend := r.URL.Query().Get("frontend")
	channelID := r.URL.Query().Get("channel_id")

	if frontend == "" || channelID == "" {
		http.Error(w, `{"error":"frontend and channel_id required"}`, http.StatusBadRequest)
		return
	}

	binding, err := g.store.GetBindingByChannel(r.Context(), frontend, channelID)
	if err != nil {
		http.Error(w, `{"error":"no binding for this channel"}`, http.StatusNotFound)
		return
	}

	// Check if agent is online
	online := g.agentManager.GetByPrincipalAndWorkDir(binding.AgentID, binding.WorkingDir) != nil

	// TODO: lookup agent name from principal
	agentName := "agent"

	json.NewEncoder(w).Encode(getBindingResponse{
		BindingID:  binding.ID,
		AgentName:  agentName,
		WorkingDir: binding.WorkingDir,
		Online:     online,
	})
}
```

Register routes:

```go
mux.HandleFunc("DELETE /api/bindings", g.handleDeleteBinding)
mux.HandleFunc("GET /api/bindings", g.handleGetBinding)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/gateway/... -run "TestDeleteBinding|TestGetBindingStatus" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/gateway/api.go internal/gateway/api_test.go
git commit -m "feat(api): add DELETE and GET /api/bindings endpoints"
```

---

## Task 5: Add GetByPrincipalAndWorkDir to agent manager

**Files:**
- Modify: `internal/agent/manager.go`
- Test: `internal/agent/manager_test.go`

**Step 1: Write the failing test**

```go
func TestManager_GetByPrincipalAndWorkDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewManager(logger)

	conn := &Connection{
		ID:          "bob-projects-website",
		PrincipalID: "principal-uuid",
		WorkingDir:  "/projects/website",
	}
	m.connections["bob-projects-website"] = conn

	// Exact match
	got := m.GetByPrincipalAndWorkDir("principal-uuid", "/projects/website")
	require.NotNil(t, got)

	// Wrong workdir
	got = m.GetByPrincipalAndWorkDir("principal-uuid", "/other/dir")
	assert.Nil(t, got)

	// Wrong principal
	got = m.GetByPrincipalAndWorkDir("other-uuid", "/projects/website")
	assert.Nil(t, got)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/... -run TestManager_GetByPrincipalAndWorkDir -v`
Expected: FAIL

**Step 3: Implement**

```go
// GetByPrincipalAndWorkDir finds an online agent matching both principal and working dir.
func (m *Manager) GetByPrincipalAndWorkDir(principalID, workingDir string) *Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, conn := range m.connections {
		if conn.PrincipalID == principalID && conn.WorkingDir == workingDir {
			return conn
		}
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/... -run TestManager_GetByPrincipalAndWorkDir -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/manager.go internal/agent/manager_test.go
git commit -m "feat(agent): add GetByPrincipalAndWorkDir lookup"
```

---

## Task 6: Add GET /api/agents endpoint

**Files:**
- Modify: `internal/gateway/api.go`
- Test: `internal/gateway/api_test.go`

**Step 1: Write the failing test**

```go
func TestListAgents(t *testing.T) {
	gw := setupTestGateway(t)

	// Register agents
	gw.agentManager.Register(&agent.Connection{
		ID:          "bob-projects-website",
		Name:        "bob",
		PrincipalID: "uuid-1",
		WorkingDir:  "/projects/website",
		InstanceID:  "0fb8187d-c06",
		Workspaces:  []string{"Code"},
	})
	gw.agentManager.Register(&agent.Connection{
		ID:          "alice-work",
		Name:        "alice",
		PrincipalID: "uuid-2",
		WorkingDir:  "/work",
		InstanceID:  "a1b2c3d4e5f6",
		Workspaces:  []string{"Work"},
	})

	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	gw.handleListAgents(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	agents := resp["agents"].([]interface{})
	assert.Len(t, agents, 2)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/... -run TestListAgents -v`
Expected: FAIL

**Step 3: Implement**

```go
type agentInfo struct {
	InstanceID  string   `json:"instance_id"`
	Name        string   `json:"name"`
	WorkingDir  string   `json:"working_dir"`
	Workspaces  []string `json:"workspaces"`
}

func (g *Gateway) handleListAgents(w http.ResponseWriter, r *http.Request) {
	conns := g.agentManager.List()

	agents := make([]agentInfo, 0, len(conns))
	for _, conn := range conns {
		agents = append(agents, agentInfo{
			InstanceID:  conn.InstanceID,
			Name:        conn.Name,
			WorkingDir:  conn.WorkingDir,
			Workspaces:  conn.Workspaces,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
	})
}
```

Register route:

```go
mux.HandleFunc("GET /api/agents", g.handleListAgents)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway/... -run TestListAgents -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/gateway/api.go internal/gateway/api_test.go
git commit -m "feat(api): add GET /api/agents endpoint"
```

---

## Task 7: Integrate /fold commands into Matrix bridge

**Files:**
- Modify: `cmd/fold-matrix/main.go` or bridge handler file
- Note: This requires understanding fold-matrix structure

**Step 1: Find the message handler**

Look for where incoming Matrix messages are processed. Search for `HandleMessage` or similar.

**Step 2: Add command detection**

```go
func (b *Bridge) handleMessage(ctx context.Context, evt *event.Event) {
	content := evt.Content.AsMessage()
	body := content.Body

	// Check for /fold commands
	if strings.HasPrefix(body, "/fold ") {
		b.handleFoldCommand(ctx, evt, strings.TrimPrefix(body, "/fold "))
		return
	}

	// Normal message handling...
}

func (b *Bridge) handleFoldCommand(ctx context.Context, evt *event.Event, cmd string) {
	roomID := evt.RoomID.String()
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		b.sendMessage(ctx, evt.RoomID, "Usage: /fold <bind|unbind|status|agents>")
		return
	}

	switch parts[0] {
	case "bind":
		if len(parts) < 2 {
			b.sendMessage(ctx, evt.RoomID, "Usage: /fold bind <instance-id>")
			return
		}
		b.handleBind(ctx, evt.RoomID, parts[1])
	case "unbind":
		b.handleUnbind(ctx, evt.RoomID)
	case "status":
		b.handleStatus(ctx, evt.RoomID)
	case "agents":
		b.handleListAgents(ctx, evt.RoomID)
	default:
		b.sendMessage(ctx, evt.RoomID, "Unknown command. Use: bind, unbind, status, agents")
	}
}
```

**Step 3: Implement each command handler**

```go
func (b *Bridge) handleBind(ctx context.Context, roomID id.RoomID, instanceID string) {
	resp, err := b.gatewayClient.CreateBinding(ctx, &CreateBindingRequest{
		Frontend:   "matrix",
		ChannelID:  roomID.String(),
		InstanceID: instanceID,
	})
	if err != nil {
		b.sendMessage(ctx, roomID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	msg := fmt.Sprintf("✓ Bound to agent **%s** at `%s`", resp.AgentName, resp.WorkingDir)
	if resp.ReboundFrom != nil {
		msg = fmt.Sprintf("✓ Rebound from **%s** to **%s** at `%s`", *resp.ReboundFrom, resp.AgentName, resp.WorkingDir)
	}
	b.sendMessage(ctx, roomID, msg)
}

func (b *Bridge) handleUnbind(ctx context.Context, roomID id.RoomID) {
	err := b.gatewayClient.DeleteBinding(ctx, "matrix", roomID.String())
	if err != nil {
		b.sendMessage(ctx, roomID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}
	b.sendMessage(ctx, roomID, "✓ Unbound from agent")
}

func (b *Bridge) handleStatus(ctx context.Context, roomID id.RoomID) {
	binding, err := b.gatewayClient.GetBinding(ctx, "matrix", roomID.String())
	if err != nil {
		b.sendMessage(ctx, roomID, "This channel is not bound to any agent")
		return
	}

	status := "offline"
	if binding.Online {
		status = "online"
	}
	b.sendMessage(ctx, roomID, fmt.Sprintf("Bound to **%s** at `%s` (%s)", binding.AgentName, binding.WorkingDir, status))
}

func (b *Bridge) handleListAgents(ctx context.Context, roomID id.RoomID) {
	agents, err := b.gatewayClient.ListAgents(ctx)
	if err != nil {
		b.sendMessage(ctx, roomID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	if len(agents) == 0 {
		b.sendMessage(ctx, roomID, "No agents currently online")
		return
	}

	var sb strings.Builder
	sb.WriteString("Online agents:\n")
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("• **%s** at `%s` (`%s`)\n", a.Name, a.WorkingDir, a.InstanceID))
	}
	b.sendMessage(ctx, roomID, sb.String())
}
```

**Step 4: Test manually**

Start gateway and Matrix bridge, send `/fold agents` in a room.

**Step 5: Commit**

```bash
git add cmd/fold-matrix/
git commit -m "feat(matrix): add /fold bind/unbind/status/agents commands"
```

---

## Task 8: End-to-end scenario test

**Files:**
- Create: `.scratch/test-bindings-flow.sh`

**Step 1: Write scenario test**

```bash
#!/bin/bash
# Test the full bindings flow

set -e

# Start gateway with approved auto-registration
# Start agent
# Start matrix bridge (or mock it)
# Create binding via API
# Verify binding works
# Unbind
# Verify unbind works
```

**Step 2: Run and verify all scenarios pass**

**Step 3: Document in scenarios.jsonl**

```json
{"name": "bind-via-instance-id", "description": "Bind channel to agent using instance ID", "given": "Gateway running, agent online with instance_id", "when": "POST /api/bindings with instance_id", "then": "Binding created with principal_id + working_dir", "validates": ["instance lookup", "binding creation"]}
```

---

## Summary

| Task | Description |
|------|-------------|
| 1 | Add working_dir to bindings table |
| 2 | Add GetAgentByInstanceID to manager |
| 3 | Add POST /api/bindings endpoint |
| 4 | Add DELETE and GET /api/bindings endpoints |
| 5 | Add GetByPrincipalAndWorkDir to manager |
| 6 | Add GET /api/agents endpoint |
| 7 | Integrate /fold commands into Matrix bridge |
| 8 | End-to-end scenario test |
