# Fold Test-First Foundation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Establish a test-first foundation with contract tests, interface extraction, and decomposition to prepare the codebase for multi-client/multi-agent orchestration.

**Architecture:** Strangler pattern for `handleSendMessage` decomposition, repository pattern for Store interface, contract tests ensuring Go↔Rust protocol compatibility.

**Tech Stack:** Go (gateway), Rust (agent), gRPC/protobuf, SQLite, testify

---

## Phase 1: Contract & Store Tests

### Task 1: Proto Contract Tests - Setup

**Files:**
- Create: `internal/gateway/proto_contract_test.go`

**Step 1: Write the test file with imports**

```go
package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/2389/fold-gateway/proto/fold"
)
```

**Step 2: Run test to verify imports work**

Run: `go test -c ./internal/gateway/`
Expected: Compiles without errors

**Step 3: Commit**

```bash
git add internal/gateway/proto_contract_test.go
git commit -m "test: add proto contract test file skeleton"
```

---

### Task 2: Contract Test - RegisterAgent Round-Trip

**Files:**
- Modify: `internal/gateway/proto_contract_test.go`

**Step 1: Write the failing test**

```go
// TestProtoContract_RegisterAgent verifies RegisterAgent message serializes
// and deserializes correctly, ensuring Go and Rust see identical bytes.
func TestProtoContract_RegisterAgent(t *testing.T) {
	original := &pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId:      "agent-123",
				Name:         "test-agent",
				Capabilities: []string{"code", "chat"},
			},
		},
	}

	// Serialize
	data, err := proto.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	decoded := &pb.AgentMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	reg := decoded.GetRegister()
	require.NotNil(t, reg)
	assert.Equal(t, "agent-123", reg.GetAgentId())
	assert.Equal(t, "test-agent", reg.GetName())
	assert.Equal(t, []string{"code", "chat"}, reg.GetCapabilities())
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run TestProtoContract_RegisterAgent ./internal/gateway/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gateway/proto_contract_test.go
git commit -m "test: add RegisterAgent proto contract test"
```

---

### Task 3: Contract Test - SendMessage Round-Trip

**Files:**
- Modify: `internal/gateway/proto_contract_test.go`

**Step 1: Write the test**

```go
// TestProtoContract_SendMessage verifies SendMessage serializes correctly
// including attachments.
func TestProtoContract_SendMessage(t *testing.T) {
	original := &pb.ServerMessage{
		Payload: &pb.ServerMessage_SendMessage{
			SendMessage: &pb.SendMessage{
				RequestId: "req-456",
				ThreadId:  "thread-789",
				Sender:    "user@example.com",
				Content:   "Hello, agent!",
				Attachments: []*pb.FileAttachment{
					{
						Filename: "test.txt",
						MimeType: "text/plain",
						Data:     []byte("file contents"),
					},
				},
			},
		},
	}

	data, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &pb.ServerMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	msg := decoded.GetSendMessage()
	require.NotNil(t, msg)
	assert.Equal(t, "req-456", msg.GetRequestId())
	assert.Equal(t, "thread-789", msg.GetThreadId())
	assert.Equal(t, "user@example.com", msg.GetSender())
	assert.Equal(t, "Hello, agent!", msg.GetContent())
	require.Len(t, msg.GetAttachments(), 1)
	assert.Equal(t, "test.txt", msg.GetAttachments()[0].GetFilename())
}
```

**Step 2: Run test**

Run: `go test -v -run TestProtoContract_SendMessage ./internal/gateway/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gateway/proto_contract_test.go
git commit -m "test: add SendMessage proto contract test"
```

---

### Task 4: Contract Test - MessageResponse Events

**Files:**
- Modify: `internal/gateway/proto_contract_test.go`

**Step 1: Write tests for all response event types**

```go
// TestProtoContract_MessageResponse_Text verifies text streaming events.
func TestProtoContract_MessageResponse_Text(t *testing.T) {
	original := &pb.MessageResponse{
		RequestId: "req-123",
		Event: &pb.MessageResponse_Text{
			Text: "Hello from agent",
		},
	}

	data, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &pb.MessageResponse{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, "req-123", decoded.GetRequestId())
	assert.Equal(t, "Hello from agent", decoded.GetText())
}

// TestProtoContract_MessageResponse_ToolUse verifies tool invocation events.
func TestProtoContract_MessageResponse_ToolUse(t *testing.T) {
	original := &pb.MessageResponse{
		RequestId: "req-123",
		Event: &pb.MessageResponse_ToolUse{
			ToolUse: &pb.ToolUse{
				Id:        "tool-1",
				Name:      "read_file",
				InputJson: `{"path": "/etc/hosts"}`,
			},
		},
	}

	data, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &pb.MessageResponse{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	tool := decoded.GetToolUse()
	require.NotNil(t, tool)
	assert.Equal(t, "tool-1", tool.GetId())
	assert.Equal(t, "read_file", tool.GetName())
	assert.Equal(t, `{"path": "/etc/hosts"}`, tool.GetInputJson())
}

// TestProtoContract_MessageResponse_Done verifies completion events.
func TestProtoContract_MessageResponse_Done(t *testing.T) {
	original := &pb.MessageResponse{
		RequestId: "req-123",
		Event: &pb.MessageResponse_Done{
			Done: &pb.Done{
				FullResponse: "Complete response text here",
			},
		},
	}

	data, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &pb.MessageResponse{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	done := decoded.GetDone()
	require.NotNil(t, done)
	assert.Equal(t, "Complete response text here", done.GetFullResponse())
}
```

**Step 2: Run tests**

Run: `go test -v -run TestProtoContract_MessageResponse ./internal/gateway/`
Expected: PASS (all 3 tests)

**Step 3: Commit**

```bash
git add internal/gateway/proto_contract_test.go
git commit -m "test: add MessageResponse proto contract tests"
```

---

### Task 5: Store Interface Tests - Setup

**Files:**
- Create: `internal/store/store_test.go`

**Step 1: Create test file with setup**

```go
package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		store.Close()
	})

	return store
}
```

**Step 2: Run test to verify setup compiles**

Run: `go test -c ./internal/store/`
Expected: Compiles without errors

**Step 3: Commit**

```bash
git add internal/store/store_test.go
git commit -m "test: add store test setup helper"
```

---

### Task 6: Store Test - Thread CRUD

**Files:**
- Modify: `internal/store/store_test.go`

**Step 1: Write thread tests**

```go
func TestStore_CreateThread(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	thread := &Thread{
		ID:        "thread-123",
		CreatedAt: time.Now(),
	}

	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetThread(ctx, "thread-123")
	require.NoError(t, err)
	assert.Equal(t, "thread-123", retrieved.ID)
}

func TestStore_CreateThread_Duplicate(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	thread := &Thread{
		ID:        "thread-123",
		CreatedAt: time.Now(),
	}

	err := store.CreateThread(ctx, thread)
	require.NoError(t, err)

	// Second create should fail
	err = store.CreateThread(ctx, thread)
	assert.Error(t, err, "duplicate thread creation should fail")
}

func TestStore_GetThread_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetThread(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}
```

**Step 2: Run tests**

Run: `go test -v -run TestStore_.*Thread ./internal/store/`
Expected: PASS (or FAIL if ErrNotFound isn't defined - see next step)

**Step 3: If ErrNotFound doesn't exist, add it to sqlite.go**

Check if `ErrNotFound` exists. If not, add to `internal/store/sqlite.go`:

```go
var ErrNotFound = errors.New("not found")
```

And update `GetThread` to return it when no rows found.

**Step 4: Run tests again**

Run: `go test -v -run TestStore_.*Thread ./internal/store/`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/
git commit -m "test: add thread CRUD tests with ErrNotFound"
```

---

### Task 7: Store Test - Message Operations

**Files:**
- Modify: `internal/store/store_test.go`

**Step 1: Write message tests**

```go
func TestStore_SaveMessage(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread first
	thread := &Thread{ID: "thread-123", CreatedAt: time.Now()}
	require.NoError(t, store.CreateThread(ctx, thread))

	msg := &Message{
		ID:        "msg-1",
		ThreadID:  "thread-123",
		Role:      "user",
		Content:   "Hello",
		CreatedAt: time.Now(),
	}

	err := store.SaveMessage(ctx, msg)
	require.NoError(t, err)

	// Retrieve messages
	messages, err := store.GetThreadMessages(ctx, "thread-123")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "Hello", messages[0].Content)
}

func TestStore_GetThreadMessages_Order(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	thread := &Thread{ID: "thread-123", CreatedAt: time.Now()}
	require.NoError(t, store.CreateThread(ctx, thread))

	// Save messages in order
	for i, content := range []string{"first", "second", "third"} {
		msg := &Message{
			ID:        fmt.Sprintf("msg-%d", i),
			ThreadID:  "thread-123",
			Role:      "user",
			Content:   content,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, store.SaveMessage(ctx, msg))
	}

	messages, err := store.GetThreadMessages(ctx, "thread-123")
	require.NoError(t, err)
	require.Len(t, messages, 3)

	// Should be in chronological order
	assert.Equal(t, "first", messages[0].Content)
	assert.Equal(t, "second", messages[1].Content)
	assert.Equal(t, "third", messages[2].Content)
}
```

**Step 2: Add fmt import if needed**

**Step 3: Run tests**

Run: `go test -v -run TestStore_.*Message ./internal/store/`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/store/store_test.go
git commit -m "test: add message save and ordering tests"
```

---

### Task 8: Store Test - Channel Bindings

**Files:**
- Modify: `internal/store/store_test.go`

**Step 1: Write binding tests**

```go
func TestStore_CreateBinding(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create thread first
	thread := &Thread{ID: "thread-123", CreatedAt: time.Now()}
	require.NoError(t, store.CreateThread(ctx, thread))

	binding := &ChannelBinding{
		Frontend:  "matrix",
		ChannelID: "!room:server.com",
		ThreadID:  "thread-123",
		AgentID:   "agent-1",
	}

	err := store.CreateBinding(ctx, binding)
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := store.GetBinding(ctx, "matrix", "!room:server.com")
	require.NoError(t, err)
	assert.Equal(t, "thread-123", retrieved.ThreadID)
	assert.Equal(t, "agent-1", retrieved.AgentID)
}

func TestStore_CreateBinding_Duplicate(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	thread := &Thread{ID: "thread-123", CreatedAt: time.Now()}
	require.NoError(t, store.CreateThread(ctx, thread))

	binding := &ChannelBinding{
		Frontend:  "matrix",
		ChannelID: "!room:server.com",
		ThreadID:  "thread-123",
		AgentID:   "agent-1",
	}

	err := store.CreateBinding(ctx, binding)
	require.NoError(t, err)

	// Duplicate should fail (PRIMARY KEY constraint)
	err = store.CreateBinding(ctx, binding)
	assert.Error(t, err, "duplicate binding should fail")
}

func TestStore_GetBinding_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetBinding(ctx, "matrix", "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}
```

**Step 2: Run tests**

Run: `go test -v -run TestStore_.*Binding ./internal/store/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/store/store_test.go
git commit -m "test: add channel binding tests with duplicate detection"
```

---

## Phase 2: Interface Extraction

### Task 9: Extract Store Interface

**Files:**
- Create: `internal/store/store.go`
- Modify: `internal/store/sqlite.go`

**Step 1: Create the Store interface file**

```go
// ABOUTME: Store interface for persistence operations
// ABOUTME: Allows swapping implementations (SQLite, memory, etc.)

package store

import (
	"context"
	"errors"
)

// ErrNotFound indicates the requested resource was not found.
var ErrNotFound = errors.New("not found")

// Store defines the persistence interface for the gateway.
type Store interface {
	// Thread operations
	CreateThread(ctx context.Context, thread *Thread) error
	GetThread(ctx context.Context, id string) (*Thread, error)
	UpdateThread(ctx context.Context, thread *Thread) error

	// Message operations
	SaveMessage(ctx context.Context, msg *Message) error
	GetThreadMessages(ctx context.Context, threadID string) ([]*Message, error)

	// Agent state operations
	SaveAgentState(ctx context.Context, state *AgentState) error
	GetAgentState(ctx context.Context, agentID string) (*AgentState, error)

	// Channel binding operations
	CreateBinding(ctx context.Context, binding *ChannelBinding) error
	GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error)
	ListBindings(ctx context.Context) ([]*ChannelBinding, error)
	DeleteBinding(ctx context.Context, frontend, channelID string) error

	// Lifecycle
	Close() error
}
```

**Step 2: Remove ErrNotFound from sqlite.go if it was added there**

**Step 3: Run tests to verify interface matches implementation**

Run: `go test ./internal/store/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/store/
git commit -m "refactor: extract Store interface from SQLiteStore"
```

---

### Task 10: Update Gateway to Use Store Interface

**Files:**
- Modify: `internal/gateway/gateway.go`

**Step 1: Change store field type from `*store.SQLiteStore` to `store.Store`**

Find the Gateway struct and change:

```go
// Before
store *store.SQLiteStore

// After
store store.Store
```

**Step 2: Run tests**

Run: `go test ./internal/gateway/...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/gateway/gateway.go
git commit -m "refactor: gateway uses Store interface instead of concrete type"
```

---

### Task 11: Create Mock Store for Testing

**Files:**
- Create: `internal/store/mock_store.go`

**Step 1: Create mock implementation**

```go
// ABOUTME: Mock Store implementation for testing
// ABOUTME: Allows tests to run without SQLite

package store

import (
	"context"
	"sync"
)

// MockStore is an in-memory Store implementation for testing.
type MockStore struct {
	mu       sync.RWMutex
	threads  map[string]*Thread
	messages map[string][]*Message // keyed by threadID
	bindings map[string]*ChannelBinding // keyed by "frontend:channelID"
	agents   map[string]*AgentState
}

// NewMockStore creates a new MockStore.
func NewMockStore() *MockStore {
	return &MockStore{
		threads:  make(map[string]*Thread),
		messages: make(map[string][]*Message),
		bindings: make(map[string]*ChannelBinding),
		agents:   make(map[string]*AgentState),
	}
}

func (m *MockStore) CreateThread(ctx context.Context, thread *Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.threads[thread.ID]; exists {
		return errors.New("thread already exists")
	}
	m.threads[thread.ID] = thread
	return nil
}

func (m *MockStore) GetThread(ctx context.Context, id string) (*Thread, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.threads[id]; ok {
		return t, nil
	}
	return nil, ErrNotFound
}

func (m *MockStore) UpdateThread(ctx context.Context, thread *Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threads[thread.ID] = thread
	return nil
}

func (m *MockStore) SaveMessage(ctx context.Context, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ThreadID] = append(m.messages[msg.ThreadID], msg)
	return nil
}

func (m *MockStore) GetThreadMessages(ctx context.Context, threadID string) ([]*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.messages[threadID], nil
}

func (m *MockStore) SaveAgentState(ctx context.Context, state *AgentState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[state.AgentID] = state
	return nil
}

func (m *MockStore) GetAgentState(ctx context.Context, agentID string) (*AgentState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.agents[agentID]; ok {
		return s, nil
	}
	return nil, ErrNotFound
}

func bindingKey(frontend, channelID string) string {
	return frontend + ":" + channelID
}

func (m *MockStore) CreateBinding(ctx context.Context, binding *ChannelBinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := bindingKey(binding.Frontend, binding.ChannelID)
	if _, exists := m.bindings[key]; exists {
		return errors.New("binding already exists")
	}
	m.bindings[key] = binding
	return nil
}

func (m *MockStore) GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.bindings[bindingKey(frontend, channelID)]; ok {
		return b, nil
	}
	return nil, ErrNotFound
}

func (m *MockStore) ListBindings(ctx context.Context) ([]*ChannelBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*ChannelBinding, 0, len(m.bindings))
	for _, b := range m.bindings {
		result = append(result, b)
	}
	return result, nil
}

func (m *MockStore) DeleteBinding(ctx context.Context, frontend, channelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bindings, bindingKey(frontend, channelID))
	return nil
}

func (m *MockStore) Close() error {
	return nil
}
```

**Step 2: Add errors import**

**Step 3: Verify it compiles**

Run: `go build ./internal/store/...`
Expected: Compiles

**Step 4: Commit**

```bash
git add internal/store/mock_store.go
git commit -m "feat: add MockStore for testing without SQLite"
```

---

## Phase 3: handleSendMessage Decomposition

### Task 12: Identify handleSendMessage Location

**Files:**
- Read: `internal/gateway/api.go`

**Step 1: Find handleSendMessage and note its line numbers**

Search for `handleSendMessage` in `api.go` and document:
- Start line
- End line
- Major responsibilities

**Step 2: Document responsibilities in a comment block**

Add a comment above the function listing what it does:
1. Parse JSON body
2. Validate required fields
3. Look up or create binding
4. Get or select agent
5. Send message via manager
6. Stream responses as SSE

**Step 3: Commit the documentation**

```bash
git add internal/gateway/api.go
git commit -m "docs: document handleSendMessage responsibilities"
```

---

### Task 13: Extract Request Parsing

**Files:**
- Modify: `internal/gateway/api.go`

**Step 1: Write test for request parsing**

Create or add to `internal/gateway/api_test.go`:

```go
func TestParseSendRequest_Valid(t *testing.T) {
	body := `{"content": "hello", "sender": "user@test.com"}`
	req, err := parseSendRequest(strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, "hello", req.Content)
	assert.Equal(t, "user@test.com", req.Sender)
}

func TestParseSendRequest_MissingContent(t *testing.T) {
	body := `{"sender": "user@test.com"}`
	_, err := parseSendRequest(strings.NewReader(body))
	assert.Error(t, err)
}
```

**Step 2: Run test - should fail**

Run: `go test -v -run TestParseSendRequest ./internal/gateway/`
Expected: FAIL (function doesn't exist)

**Step 3: Extract parseSendRequest function**

```go
// sendRequestBody represents the JSON body for /api/send
type sendRequestBody struct {
	Content     string       `json:"content"`
	Sender      string       `json:"sender"`
	ThreadID    string       `json:"thread_id,omitempty"`
	AgentID     string       `json:"agent_id,omitempty"`
	ChannelID   string       `json:"channel_id,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
}

// parseSendRequest parses and validates a send request body.
func parseSendRequest(r io.Reader) (*sendRequestBody, error) {
	var body sendRequestBody
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if body.Content == "" {
		return nil, errors.New("content is required")
	}
	return &body, nil
}
```

**Step 4: Run test - should pass**

Run: `go test -v -run TestParseSendRequest ./internal/gateway/`
Expected: PASS

**Step 5: Update handleSendMessage to use parseSendRequest**

Replace the inline JSON parsing with a call to `parseSendRequest`.

**Step 6: Run all tests**

Run: `go test ./internal/gateway/...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/gateway/
git commit -m "refactor: extract parseSendRequest from handleSendMessage"
```

---

### Task 14: Extract Binding Resolution

**Files:**
- Modify: `internal/gateway/api.go`

**Step 1: Write test for binding resolution**

```go
func TestResolveBinding_ExistingBinding(t *testing.T) {
	store := store.NewMockStore()
	ctx := context.Background()

	// Setup existing binding
	store.CreateThread(ctx, &store.Thread{ID: "existing-thread"})
	store.CreateBinding(ctx, &store.ChannelBinding{
		Frontend:  "test",
		ChannelID: "channel-1",
		ThreadID:  "existing-thread",
		AgentID:   "agent-1",
	})

	resolver := &bindingResolver{store: store}
	result, err := resolver.Resolve(ctx, "test", "channel-1", "")

	require.NoError(t, err)
	assert.Equal(t, "existing-thread", result.ThreadID)
	assert.Equal(t, "agent-1", result.AgentID)
}
```

**Step 2: Run test - should fail**

Run: `go test -v -run TestResolveBinding ./internal/gateway/`
Expected: FAIL

**Step 3: Extract bindingResolver**

```go
type bindingResolver struct {
	store store.Store
}

type resolvedBinding struct {
	ThreadID string
	AgentID  string
	IsNew    bool
}

func (r *bindingResolver) Resolve(ctx context.Context, frontend, channelID, preferredAgent string) (*resolvedBinding, error) {
	// Try existing binding first
	existing, err := r.store.GetBinding(ctx, frontend, channelID)
	if err == nil {
		return &resolvedBinding{
			ThreadID: existing.ThreadID,
			AgentID:  existing.AgentID,
			IsNew:    false,
		}, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// Create new thread and binding
	threadID := uuid.New().String()
	thread := &store.Thread{
		ID:        threadID,
		CreatedAt: time.Now(),
	}
	if err := r.store.CreateThread(ctx, thread); err != nil {
		return nil, err
	}

	binding := &store.ChannelBinding{
		Frontend:  frontend,
		ChannelID: channelID,
		ThreadID:  threadID,
		AgentID:   preferredAgent,
	}
	if err := r.store.CreateBinding(ctx, binding); err != nil {
		return nil, err
	}

	return &resolvedBinding{
		ThreadID: threadID,
		AgentID:  preferredAgent,
		IsNew:    true,
	}, nil
}
```

**Step 4: Run test - should pass**

Run: `go test -v -run TestResolveBinding ./internal/gateway/`
Expected: PASS

**Step 5: Update handleSendMessage to use bindingResolver**

**Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/gateway/
git commit -m "refactor: extract bindingResolver from handleSendMessage"
```

---

### Task 15: Extract SSE Response Streaming

**Files:**
- Modify: `internal/gateway/api.go`

**Step 1: Write test for SSE formatting**

```go
func TestFormatSSEEvent(t *testing.T) {
	event := formatSSEEvent("text", `{"content": "hello"}`)
	assert.Equal(t, "event: text\ndata: {\"content\": \"hello\"}\n\n", event)
}
```

**Step 2: Run test - should fail**

**Step 3: Extract formatSSEEvent function**

```go
func formatSSEEvent(eventType, data string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
}
```

**Step 4: Run test - should pass**

**Step 5: Commit**

```bash
git add internal/gateway/
git commit -m "refactor: extract formatSSEEvent helper"
```

---

## Phase 4: Final Verification

### Task 16: Run Full Test Suite

**Files:** None (verification only)

**Step 1: Run all tests with race detection**

Run: `go test -race ./...`
Expected: All PASS

**Step 2: Run linter**

Run: `golangci-lint run`
Expected: No errors (or acceptable warnings)

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "fix: address linter warnings"
```

---

### Task 17: Update CLAUDE.md with New Structure

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Add documentation about new abstractions**

Add section:

```markdown
## Testing

The codebase uses interface-based design for testability:

- `store.Store` interface with `SQLiteStore` and `MockStore` implementations
- Proto contract tests ensure Go↔Rust serialization compatibility
- Integration tests in `gateway_test.go` test full message round-trips
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with testing patterns"
```

---

## Summary

This plan establishes a test-first foundation:

1. **Contract tests** ensure Go↔Rust protocol compatibility
2. **Store interface tests** validate persistence layer
3. **Store interface extraction** enables mock testing
4. **handleSendMessage decomposition** improves maintainability

Each task is bite-sized (one action) with exact file paths and commands.
