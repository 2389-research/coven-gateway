# Affinity Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace round-robin routing with channel-level affinity bindings that route frontend channels to specific agents.

**Architecture:** Add `ChannelBinding` to store layer, binding CRUD endpoints to API, update `Manager.SendMessage` to require explicit agent selection (either via binding lookup or direct agent_id).

**Tech Stack:** Go, SQLite, HTTP/JSON API

---

## Task 1: Add ChannelBinding Type to Store Interface

**Files:**
- Modify: `internal/store/store.go`

**Step 1: Add ChannelBinding struct after Message struct (around line 32)**

```go
// ChannelBinding represents a sticky assignment of a frontend channel to an agent
type ChannelBinding struct {
	FrontendName string
	ChannelID    string
	AgentID      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
```

**Step 2: Add binding methods to Store interface (after GetAgentState method, around line 48)**

```go
	// Channel bindings
	CreateBinding(ctx context.Context, binding *ChannelBinding) error
	GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error)
	ListBindings(ctx context.Context) ([]*ChannelBinding, error)
	DeleteBinding(ctx context.Context, frontend, channelID string) error
```

**Step 3: Verify compilation**

Run: `go build ./internal/store/...`
Expected: FAIL - SQLiteStore doesn't implement new methods (this is correct, we'll implement next)

**Step 4: Commit interface changes**

```bash
git add internal/store/store.go
git commit -m "feat(store): add ChannelBinding type and interface methods"
```

---

## Task 2: Add Binding Schema to SQLite

**Files:**
- Modify: `internal/store/sqlite.go`

**Step 1: Add channel_bindings table to createSchema (after agent_state table, around line 101)**

```go
		CREATE TABLE IF NOT EXISTS channel_bindings (
			frontend TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (frontend, channel_id)
		);
```

**Step 2: Verify schema creation compiles**

Run: `go build ./internal/store/...`
Expected: FAIL - still missing method implementations

---

## Task 3: Implement CreateBinding

**Files:**
- Test: `internal/store/sqlite_test.go`
- Modify: `internal/store/sqlite.go`

**Step 1: Write failing test at end of sqlite_test.go**

```go
func TestCreateBinding(t *testing.T) {
	store := setupTestStore(t)

	binding := &ChannelBinding{
		FrontendName: "slack",
		ChannelID:    "C0123456789",
		AgentID:      "agent-uuid-123",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := store.CreateBinding(context.Background(), binding)
	if err != nil {
		t.Fatalf("CreateBinding failed: %v", err)
	}

	// Verify by reading back
	got, err := store.GetBinding(context.Background(), "slack", "C0123456789")
	if err != nil {
		t.Fatalf("GetBinding failed: %v", err)
	}

	if got.AgentID != "agent-uuid-123" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-uuid-123")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestCreateBinding -v`
Expected: FAIL - method not implemented

**Step 3: Implement CreateBinding in sqlite.go (after GetAgentState, around line 374)**

```go
// CreateBinding creates a new channel binding.
// Returns an error if a binding already exists for this frontend/channel combination.
func (s *SQLiteStore) CreateBinding(ctx context.Context, binding *ChannelBinding) error {
	query := `
		INSERT INTO channel_bindings (frontend, channel_id, agent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		binding.FrontendName,
		binding.ChannelID,
		binding.AgentID,
		binding.CreatedAt.UTC().Format(time.RFC3339),
		binding.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting binding: %w", err)
	}

	s.logger.Debug("created binding", "frontend", binding.FrontendName, "channel", binding.ChannelID, "agent", binding.AgentID)
	return nil
}
```

**Step 4: Implement GetBinding (needed for test)**

```go
// GetBinding retrieves a binding by frontend and channel ID.
// Returns ErrNotFound if no binding exists.
func (s *SQLiteStore) GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error) {
	query := `
		SELECT frontend, channel_id, agent_id, created_at, updated_at
		FROM channel_bindings
		WHERE frontend = ? AND channel_id = ?
	`

	var binding ChannelBinding
	var createdAtStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx, query, frontend, channelID).Scan(
		&binding.FrontendName,
		&binding.ChannelID,
		&binding.AgentID,
		&createdAtStr,
		&updatedAtStr,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying binding: %w", err)
	}

	binding.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	binding.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	return &binding, nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/store/... -run TestCreateBinding -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/store/sqlite.go internal/store/sqlite_test.go
git commit -m "feat(store): implement CreateBinding and GetBinding"
```

---

## Task 4: Implement ListBindings and DeleteBinding

**Files:**
- Test: `internal/store/sqlite_test.go`
- Modify: `internal/store/sqlite.go`

**Step 1: Write failing test for ListBindings**

```go
func TestListBindings(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create two bindings
	bindings := []*ChannelBinding{
		{FrontendName: "slack", ChannelID: "C001", AgentID: "agent-1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{FrontendName: "matrix", ChannelID: "!room:example.com", AgentID: "agent-2", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, b := range bindings {
		if err := store.CreateBinding(ctx, b); err != nil {
			t.Fatalf("CreateBinding failed: %v", err)
		}
	}

	got, err := store.ListBindings(ctx)
	if err != nil {
		t.Fatalf("ListBindings failed: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("got %d bindings, want 2", len(got))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestListBindings -v`
Expected: FAIL

**Step 3: Implement ListBindings**

```go
// ListBindings returns all channel bindings.
func (s *SQLiteStore) ListBindings(ctx context.Context) ([]*ChannelBinding, error) {
	query := `
		SELECT frontend, channel_id, agent_id, created_at, updated_at
		FROM channel_bindings
		ORDER BY frontend, channel_id
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying bindings: %w", err)
	}
	defer rows.Close()

	var bindings []*ChannelBinding
	for rows.Next() {
		var b ChannelBinding
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&b.FrontendName, &b.ChannelID, &b.AgentID, &createdAtStr, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("scanning binding: %w", err)
		}

		b.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		b.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		bindings = append(bindings, &b)
	}

	return bindings, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -run TestListBindings -v`
Expected: PASS

**Step 5: Write failing test for DeleteBinding**

```go
func TestDeleteBinding(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	binding := &ChannelBinding{
		FrontendName: "slack",
		ChannelID:    "C001",
		AgentID:      "agent-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := store.CreateBinding(ctx, binding); err != nil {
		t.Fatalf("CreateBinding failed: %v", err)
	}

	if err := store.DeleteBinding(ctx, "slack", "C001"); err != nil {
		t.Fatalf("DeleteBinding failed: %v", err)
	}

	_, err := store.GetBinding(ctx, "slack", "C001")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
```

**Step 6: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestDeleteBinding -v`
Expected: FAIL

**Step 7: Implement DeleteBinding**

```go
// DeleteBinding removes a channel binding.
// Returns ErrNotFound if the binding doesn't exist.
func (s *SQLiteStore) DeleteBinding(ctx context.Context, frontend, channelID string) error {
	query := `DELETE FROM channel_bindings WHERE frontend = ? AND channel_id = ?`

	result, err := s.db.ExecContext(ctx, query, frontend, channelID)
	if err != nil {
		return fmt.Errorf("deleting binding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	s.logger.Debug("deleted binding", "frontend", frontend, "channel", channelID)
	return nil
}
```

**Step 8: Run test to verify it passes**

Run: `go test ./internal/store/... -run TestDeleteBinding -v`
Expected: PASS

**Step 9: Run all store tests**

Run: `go test ./internal/store/... -v`
Expected: ALL PASS

**Step 10: Commit**

```bash
git add internal/store/sqlite.go internal/store/sqlite_test.go
git commit -m "feat(store): implement ListBindings and DeleteBinding"
```

---

## Task 5: Add Binding API Endpoints to Gateway

**Files:**
- Modify: `internal/gateway/gateway.go` (register routes)
- Modify: `internal/gateway/api.go` (add handlers)

**Step 1: Add request/response types to api.go (after AgentInfoResponse)**

```go
// CreateBindingRequest is the JSON request body for POST /api/bindings.
type CreateBindingRequest struct {
	Frontend  string `json:"frontend"`
	ChannelID string `json:"channel_id"`
	AgentID   string `json:"agent_id"`
}

// BindingResponse is the JSON response for binding operations.
type BindingResponse struct {
	Frontend    string `json:"frontend"`
	ChannelID   string `json:"channel_id"`
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name,omitempty"`
	AgentOnline bool   `json:"agent_online"`
	CreatedAt   string `json:"created_at"`
}

// ListBindingsResponse is the JSON response for GET /api/bindings.
type ListBindingsResponse struct {
	Bindings []BindingResponse `json:"bindings"`
}
```

**Step 2: Add route registrations in gateway.go New() function (after /api/send)**

```go
	mux.HandleFunc("/api/bindings", gw.handleBindings)
```

**Step 3: Add handleBindings dispatcher to api.go**

```go
// handleBindings routes binding requests by HTTP method.
func (g *Gateway) handleBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.handleListBindings(w, r)
	case http.MethodPost:
		g.handleCreateBinding(w, r)
	case http.MethodDelete:
		g.handleDeleteBinding(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
```

**Step 4: Implement handleListBindings**

```go
// handleListBindings handles GET /api/bindings.
func (g *Gateway) handleListBindings(w http.ResponseWriter, r *http.Request) {
	bindings, err := g.store.ListBindings(r.Context())
	if err != nil {
		g.sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := ListBindingsResponse{
		Bindings: make([]BindingResponse, len(bindings)),
	}

	for i, b := range bindings {
		agentOnline := false
		agentName := ""
		if agent, ok := g.agentManager.GetAgent(b.AgentID); ok {
			agentOnline = true
			agentName = agent.Name
		}

		response.Bindings[i] = BindingResponse{
			Frontend:    b.FrontendName,
			ChannelID:   b.ChannelID,
			AgentID:     b.AgentID,
			AgentName:   agentName,
			AgentOnline: agentOnline,
			CreatedAt:   b.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

**Step 5: Implement handleCreateBinding**

```go
// handleCreateBinding handles POST /api/bindings.
func (g *Gateway) handleCreateBinding(w http.ResponseWriter, r *http.Request) {
	var req CreateBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Frontend == "" || req.ChannelID == "" || req.AgentID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "frontend, channel_id, and agent_id are required")
		return
	}

	now := time.Now()
	binding := &store.ChannelBinding{
		FrontendName: req.Frontend,
		ChannelID:    req.ChannelID,
		AgentID:      req.AgentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := g.store.CreateBinding(r.Context(), binding); err != nil {
		// Check for duplicate
		if existing, _ := g.store.GetBinding(r.Context(), req.Frontend, req.ChannelID); existing != nil {
			g.sendJSONError(w, http.StatusConflict, "binding already exists")
			return
		}
		g.sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	agentOnline := false
	agentName := ""
	if agent, ok := g.agentManager.GetAgent(req.AgentID); ok {
		agentOnline = true
		agentName = agent.Name
	}

	response := BindingResponse{
		Frontend:    binding.FrontendName,
		ChannelID:   binding.ChannelID,
		AgentID:     binding.AgentID,
		AgentName:   agentName,
		AgentOnline: agentOnline,
		CreatedAt:   binding.CreatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
```

**Step 6: Implement handleDeleteBinding**

```go
// handleDeleteBinding handles DELETE /api/bindings?frontend=X&channel_id=Y.
func (g *Gateway) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	frontend := r.URL.Query().Get("frontend")
	channelID := r.URL.Query().Get("channel_id")

	if frontend == "" || channelID == "" {
		g.sendJSONError(w, http.StatusBadRequest, "frontend and channel_id query params required")
		return
	}

	err := g.store.DeleteBinding(r.Context(), frontend, channelID)
	if err == store.ErrNotFound {
		g.sendJSONError(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		g.sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

**Step 7: Add required imports to api.go**

```go
import (
	// ... existing imports
	"time"

	"github.com/2389/fold-gateway/internal/store"
)
```

**Step 8: Verify compilation**

Run: `go build ./...`
Expected: PASS

**Step 9: Commit**

```bash
git add internal/gateway/gateway.go internal/gateway/api.go
git commit -m "feat(api): add channel binding CRUD endpoints"
```

---

## Task 6: Add Binding API Tests

**Files:**
- Modify: `internal/gateway/api_test.go`

**Step 1: Add test for create and list bindings**

```go
func TestBindingsCRUD(t *testing.T) {
	gw := setupTestGateway(t)
	defer gw.Shutdown(context.Background())

	// Create a binding
	createReq := `{"frontend":"slack","channel_id":"C001","agent_id":"test-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bindings", strings.NewReader(createReq))
	w := httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("create binding: got status %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// List bindings
	req = httptest.NewRequest(http.MethodGet, "/api/bindings", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("list bindings: got status %d, want %d", w.Code, http.StatusOK)
	}

	var listResp ListBindingsResponse
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(listResp.Bindings) != 1 {
		t.Errorf("got %d bindings, want 1", len(listResp.Bindings))
	}

	// Delete binding
	req = httptest.NewRequest(http.MethodDelete, "/api/bindings?frontend=slack&channel_id=C001", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("delete binding: got status %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/bindings", nil)
	w = httptest.NewRecorder()
	gw.handleBindings(w, req)

	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(listResp.Bindings) != 0 {
		t.Errorf("got %d bindings after delete, want 0", len(listResp.Bindings))
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/gateway/... -run TestBindingsCRUD -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gateway/api_test.go
git commit -m "test(api): add binding CRUD tests"
```

---

## Task 7: Update SendMessage to Use Bindings

**Files:**
- Modify: `internal/gateway/api.go`

**Step 1: Update SendMessageRequest to include frontend context**

```go
// SendMessageRequest is the JSON request body for POST /api/send.
type SendMessageRequest struct {
	ThreadID  string `json:"thread_id,omitempty"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	AgentID   string `json:"agent_id,omitempty"`
	Frontend  string `json:"frontend,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
}
```

**Step 2: Update handleSendMessage to resolve agent from binding**

Replace the validation section (after decoding JSON) with:

```go
	if req.Content == "" {
		g.sendJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Resolve agent ID - either from direct specification or binding lookup
	agentID := req.AgentID
	if agentID == "" {
		// Must have frontend + channel_id for binding lookup
		if req.Frontend == "" || req.ChannelID == "" {
			g.sendJSONError(w, http.StatusBadRequest, "must specify agent_id or frontend+channel_id")
			return
		}

		binding, err := g.store.GetBinding(r.Context(), req.Frontend, req.ChannelID)
		if err == store.ErrNotFound {
			g.sendJSONError(w, http.StatusBadRequest, "channel not bound to agent")
			return
		}
		if err != nil {
			g.sendJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agentID = binding.AgentID
	}

	// Verify agent exists and is online
	if _, ok := g.agentManager.GetAgent(agentID); !ok {
		g.sendJSONError(w, http.StatusServiceUnavailable, "agent unavailable")
		return
	}
```

**Step 3: Update sendReq construction to use resolved agentID**

```go
	// Create the send request
	sendReq := &agent.SendRequest{
		ThreadID: req.ThreadID,
		Sender:   req.Sender,
		Content:  req.Content,
		AgentID:  agentID,
	}
```

**Step 4: Verify compilation**

Run: `go build ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/gateway/api.go
git commit -m "feat(api): resolve agent from binding in SendMessage"
```

---

## Task 8: Remove Round-Robin Router

**Files:**
- Delete: `internal/agent/router.go`
- Modify: `internal/agent/manager.go`

**Step 1: Delete router.go**

```bash
rm internal/agent/router.go
```

**Step 2: Remove router from Manager struct in manager.go**

Remove the `router` field from the Manager struct and `NewManager`:

```go
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
```

**Step 3: Update SendMessage to require AgentID**

Replace the agent selection logic in SendMessage with:

```go
func (m *Manager) SendMessage(ctx context.Context, req *SendRequest) (<-chan *Response, error) {
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	agent, ok := m.GetAgent(req.AgentID)
	if !ok {
		return nil, ErrAgentNotFound
	}

	// Generate a unique request ID
	requestID := uuid.New().String()
	// ... rest of the function stays the same
```

**Step 4: Remove ErrNoAgentsAvailable (no longer used)**

Delete this line from manager.go:
```go
var ErrNoAgentsAvailable = errors.New("no agents available")
```

**Step 5: Update api.go to remove ErrNoAgentsAvailable handling**

In handleSendMessage, remove:
```go
		if err == agent.ErrNoAgentsAvailable {
			g.sendJSONError(w, http.StatusServiceUnavailable, "no agents available")
			return
		}
```

**Step 6: Verify compilation**

Run: `go build ./...`
Expected: PASS

**Step 7: Run all tests**

Run: `go test ./... -v`
Expected: Some tests may fail (we'll fix in next task)

**Step 8: Commit**

```bash
git add -A
git commit -m "refactor: remove round-robin router, require explicit agent selection"
```

---

## Task 9: Fix Manager Tests

**Files:**
- Modify: `internal/agent/manager_test.go`

**Step 1: Update tests to always specify AgentID**

Find tests that call SendMessage without AgentID and update them to include the agent ID. The exact changes depend on existing test structure, but the pattern is:

```go
// Before
req := &SendRequest{Content: "hello"}

// After
req := &SendRequest{Content: "hello", AgentID: agent.ID}
```

**Step 2: Remove any tests that specifically test round-robin behavior**

Delete tests like `TestRouterRoundRobin` or `TestSendMessageRoutesToAgent` that test automatic agent selection.

**Step 3: Run tests**

Run: `go test ./internal/agent/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/agent/manager_test.go
git commit -m "test: update manager tests for explicit agent selection"
```

---

## Task 10: Remove Routing Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `config.example.yaml`

**Step 1: Remove RoutingConfig from Config struct**

Remove:
```go
	Routing   RoutingConfig   `yaml:"routing"`
```

And remove:
```go
// RoutingConfig holds routing strategy configuration
type RoutingConfig struct {
	Strategy string `yaml:"strategy"`
}
```

**Step 2: Remove routing validation from Validate()**

Remove:
```go
	if c.Routing.Strategy == "" {
		return fmt.Errorf("routing.strategy is required")
	}

	if !validRoutingStrategies[c.Routing.Strategy] {
		return fmt.Errorf("routing.strategy %q is invalid; must be one of: round_robin, affinity, capability, random", c.Routing.Strategy)
	}
```

And remove:
```go
var validRoutingStrategies = map[string]bool{
	"round_robin": true,
	"affinity":    true,
	"capability":  true,
	"random":      true,
}
```

**Step 3: Update config.example.yaml**

Remove the routing section:
```yaml
routing:
  strategy: "round_robin"
```

**Step 4: Update config tests to remove routing references**

Update test configs to not include Routing field.

**Step 5: Run tests**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go config.example.yaml
git commit -m "refactor: remove routing config (no longer used)"
```

---

## Task 11: Update Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/CLIENT_PROTOCOL.md`

**Step 1: Update README.md features section**

Replace "Agent Routing: Round-robin routing across connected agents" with:
"Channel Bindings: Sticky routing from frontend channels to specific agents"

**Step 2: Add binding API section to README.md**

```markdown
### Channel Bindings

```bash
# Create a binding (channel → agent)
curl -X POST http://localhost:8080/api/bindings \
  -H "Content-Type: application/json" \
  -d '{"frontend":"slack","channel_id":"C0123456789","agent_id":"agent-uuid"}'

# List all bindings
curl http://localhost:8080/api/bindings

# Delete a binding
curl -X DELETE "http://localhost:8080/api/bindings?frontend=slack&channel_id=C0123456789"
```
```

**Step 3: Update docs/CLIENT_PROTOCOL.md**

Add section documenting the binding endpoints and update /api/send to show frontend/channel_id fields.

**Step 4: Commit**

```bash
git add README.md docs/CLIENT_PROTOCOL.md
git commit -m "docs: update for channel binding routing"
```

---

## Task 12: Final Verification

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 2: Build binary**

Run: `go build -o bin/fold-gateway ./cmd/fold-gateway`
Expected: SUCCESS

**Step 3: Verify binary starts**

Run: `./bin/fold-gateway serve --config config.example.yaml` (Ctrl+C after startup)
Expected: Starts without errors

**Step 4: Final commit if any fixes needed**

**Step 5: Push all changes**

```bash
git push
```
