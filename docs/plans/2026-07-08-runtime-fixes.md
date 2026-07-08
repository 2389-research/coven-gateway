# Runtime Bug Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix four verified runtime defects: a goroutine leak in the agent manager, silent SQLite write loss under concurrency, an agent message loop that freezes during pack tool execution (plus an existing concurrent-`stream.Send` data race), and an `EventType` constant that violates the DB CHECK constraint.

**Architecture:** coven-gateway is a Go control plane: agents connect via a bidirectional gRPC stream (`internal/gateway/grpc.go` → `internal/agent`), HTTP/SSE clients consume responses (`internal/gateway/api.go`), everything persists to SQLite (`internal/store`). Pack tools route through `internal/packs`. All four fixes are small, surgical, and behavior-preserving for the happy path.

**Tech Stack:** Go 1.25.5, `modernc.org/sqlite` (pure-Go driver — pragmas via `db.Exec`, NOT mattn-style DSN params), gRPC-Go, stdlib testing.

**Verification evidence:** `.superpowers/sdd/runtime-investigation.md` (findings R-1..R-4, confirmed against current code 2026-07-08).

## Global Constraints

- Branch: `fix/runtime-bugs`, based on current `main` (1720233). Conventional commits, imperative present tense.
- TDD for every task: failing test first, watch it fail, minimal fix, watch it pass, commit.
- Logging: `slog` only. Error wrapping: `fmt.Errorf("context: %w", err)`.
- New files start with two `// ABOUTME:` comment lines.
- Match surrounding style; no whitespace-only changes; `gofmt` clean.
- NEVER touch `coven-gateway.db`, `coven-gateway.db.bak`, or the `proto/coven-proto` submodule pointer. `git add` only files you created/modified for the task — never `git add -A`.
- Never bypass git hooks (no `--no-verify`).
- gRPC-Go rule this plan enforces: it is NOT safe for two goroutines to call `Send` on the same server stream concurrently. After Task 4, `Connection.Send` is the ONLY place that calls `stream.Send`, guarded by a mutex.
- Existing tests must keep passing: `go test ./...` and `go test -race ./internal/agent/... ./internal/gateway/... ./internal/store/... ./internal/client/...`.

---

### Task 1: Fix goroutine leak in `transformResponses`

`Manager.transformResponses` has two unconditional blocking sends onto `outChan` (buffer 16). When the sole reader (the SSE handler) disconnects and the buffer is full, the goroutine blocks forever; its deferred `CloseRequest` never runs, so the connection's pending map also grows forever.

**Files:**
- Modify: `internal/agent/manager.go:152-175` (`transformResponses` loop body)
- Test: `internal/agent/manager_test.go` (append new test)

**Interfaces:**
- Consumes: existing `mockStream` test helper (`manager_test.go:20-52`), `Manager.SendMessage(ctx, *SendRequest) (<-chan *Response, error)`, `Connection.HandleResponse(*pb.MessageResponse)`.
- Produces: no signature changes. Behavior change: `transformResponses` exits when `ctx` is canceled even if `outChan` is full and unread.

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/manager_test.go` (package `agent`; add `"runtime"` and `"time"` to imports if not present — `context`, `slog`, `pb` already are):

```go
func TestTransformResponsesExitsWhenReaderGone(t *testing.T) {
	manager := NewManager(slog.Default())
	stream := newMockStream()
	conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})
	if err := manager.Register(conn); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	baseline := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	outChan, err := manager.SendMessage(ctx, &SendRequest{
		AgentID:  "agent-1",
		ThreadID: "thread-1",
		Sender:   "user",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Learn the request ID from the message the manager sent to the agent.
	sent := stream.getSentMessages()
	requestID := sent[len(sent)-1].GetSendMessage().GetRequestId()

	// Simulate a fast agent whose reader has gone away: nobody reads
	// outChan while the agent floods responses. respChan (cap 16) and
	// outChan (cap 16) both fill, then transformResponses blocks on its
	// send into outChan.
	for i := 0; i < 40; i++ {
		conn.HandleResponse(&pb.MessageResponse{
			RequestId: requestID,
			Event: &pb.MessageResponse_Text{
				Text: "chunk",
			},
		})
	}

	// Client disconnects.
	cancel()

	// The transform goroutine must exit even though outChan is full and
	// never read again. Poll goroutine count back down to baseline.
	deadline := time.After(2 * time.Second)
	for runtime.NumGoroutine() > baseline {
		select {
		case <-deadline:
			t.Fatalf("transformResponses leaked: %d goroutines, baseline %d",
				runtime.NumGoroutine(), baseline)
		case <-time.After(10 * time.Millisecond):
		}
	}
	_ = outChan // intentionally never read: simulates a disconnected SSE client
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestTransformResponsesExitsWhenReaderGone ./internal/agent/ -v`
Expected: FAIL with `transformResponses leaked: N goroutines, baseline M` (after the 2s poll window).

- [ ] **Step 3: Fix both blocking sends**

In `internal/agent/manager.go`, replace the body of the `for` loop in `transformResponses` (currently lines 152-175). Old code:

```go
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
```

New code:

```go
	for {
		select {
		case <-ctx.Done():
			// Best-effort final error: if the reader is gone and the
			// buffer is full, drop it rather than block forever.
			select {
			case outChan <- &Response{
				Event: EventError,
				Error: "context canceled",
				Done:  true,
			}:
			default:
			}
			return

		case pbResp, ok := <-respChan:
			if !ok {
				return
			}

			resp := m.convertResponse(pbResp)
			select {
			case outChan <- resp:
			case <-ctx.Done():
				// Reader gone mid-stream; exit rather than block forever.
				return
			}

			if resp.Done {
				return
			}
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/agent/ -v`
Expected: PASS, including `TestTransformResponsesExitsWhenReaderGone` and all pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/manager.go internal/agent/manager_test.go
git commit -m "fix(agent): stop transformResponses leaking when response reader disconnects"
```

---

### Task 2: Serialize SQLite access and add busy_timeout

`database/sql` defaults to an unlimited connection pool. With SQLite that means concurrent writers on separate connections, which fail instantly with `SQLITE_BUSY` (default `busy_timeout` is 0) — and those errors are swallowed by callers, silently losing conversation history. Bonus defect fixed for free: today `PRAGMA foreign_keys=ON` (and WAL) via `db.Exec` only applies to whichever pooled connection ran it; with a single connection the pragmas reliably govern all access.

**Files:**
- Modify: `internal/store/sqlite.go:38-53` (`NewSQLiteStore` open sequence)
- Test: `internal/store/sqlite_test.go` (append new test; uses existing `newTestStore(t)` helper at line 571)

**Interfaces:**
- Consumes: `newTestStore(t) *SQLiteStore`, `SaveEvent(ctx, *LedgerEvent) error`, `GetEvents(ctx, GetEventsParams) (*GetEventsResult, error)` (`.Events []LedgerEvent`).
- Produces: no signature changes. Behavior change: all DB access serialized through one connection; residual lock contention retries for up to 5s.

- [ ] **Step 1: Write the failing test**

Append to `internal/store/sqlite_test.go` (package `store`; ensure imports include `context`, `fmt`, `sync`, `time`):

```go
func TestConcurrentWritesDoNotFail(t *testing.T) {
	s := newTestStore(t)

	const goroutines = 8
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*eventsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := &LedgerEvent{
					ID:              fmt.Sprintf("evt-%d-%d", g, i),
					ConversationKey: "conv-concurrent",
					Direction:       EventDirectionInbound,
					Author:          "tester",
					Timestamp:       time.Now().UTC(),
					Type:            EventTypeMessage,
				}
				if err := s.SaveEvent(context.Background(), event); err != nil {
					errCh <- fmt.Errorf("goroutine %d event %d: %w", g, i, err)
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent SaveEvent failed: %v", err)
	}

	result, err := s.GetEvents(context.Background(), GetEventsParams{
		ConversationKey: "conv-concurrent",
		Limit:           500,
	})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if got, want := len(result.Events), goroutines*eventsPerGoroutine; got != want {
		t.Errorf("expected %d persisted events, got %d", want, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestConcurrentWritesDoNotFail ./internal/store/ -v`
Expected: FAIL — some `SaveEvent` calls error with `database is locked (SQLITE_BUSY)` and/or the persisted count is below 160. (The exact failure count varies run to run; any failure is RED.)

- [ ] **Step 3: Limit the pool and add busy_timeout**

In `internal/store/sqlite.go`, `NewSQLiteStore`. After the `sql.Open` error check (line 41) and BEFORE the WAL pragma, insert:

```go
	// SQLite allows one writer at a time; a pool of connections just turns
	// writer contention into immediate SQLITE_BUSY errors. A single
	// connection serializes all access — and guarantees the per-connection
	// pragmas below apply to every query.
	db.SetMaxOpenConns(1)
```

After the `PRAGMA foreign_keys=ON` block (line 50-53), insert:

```go
	// Retry window for residual lock contention (e.g. external tooling
	// holding the file). modernc.org/sqlite takes pragmas via Exec, not
	// mattn-style DSN parameters.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting busy timeout: %w", err)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/store/ -v`
Expected: PASS, including `TestConcurrentWritesDoNotFail` (all 160 events persisted, zero errors) and all pre-existing store tests.

- [ ] **Step 5: Commit**

```bash
git add internal/store/sqlite.go internal/store/sqlite_test.go
git commit -m "fix(store): serialize SQLite access and add busy_timeout to stop silent write loss"
```

---

### Task 3: Align `EventType` constants with the schema CHECK constraint

`store.EventTypeTextChunk` (`"text_chunk"`) is not in the `ledger_events` CHECK constraint (`sqlite.go:98`: `CHECK (type IN ('message', 'tool_call', 'tool_result', 'system', 'error'))`). It is only ever used as an ephemeral broadcast tag in `internal/client` — nothing persists it today — but its presence in the `store` package is a landmine: any future `SaveEvent` call with it fails at runtime, and that error gets swallowed. Move it to `internal/client` as an unexported constant so the store's `EventType` set corresponds 1:1 with the schema.

**Files:**
- Modify: `internal/store/events.go:33` (delete the constant)
- Modify: `internal/client/send_message.go` (define local constant; use at line 208)
- Modify: `internal/client/stream_events.go:269` (use local constant)
- Modify: `internal/client/send_message_test.go:518`, `internal/client/stream_events_test.go:211,233` (use local constant; both files are package `client`, so the unexported constant is visible)
- Test: `internal/store/sqlite_test.go` (append invariant test)

**Interfaces:**
- Consumes: `newTestStore(t)`, `SaveEvent`, the `EventType` constants.
- Produces: `internal/client` gains unexported `const eventTypeTextChunk store.EventType = "text_chunk"`. `store.EventTypeTextChunk` ceases to exist — full reference list (verified by grep, there are exactly these six): `store/events.go:33` (def), `client/send_message.go:208`, `client/stream_events.go:269`, `client/send_message_test.go:518`, `client/stream_events_test.go:211`, `client/stream_events_test.go:233`.

- [ ] **Step 1: Write the failing invariant test**

Append to `internal/store/sqlite_test.go`:

```go
// Every EventType constant the store package exports must be insertable —
// i.e. present in the ledger_events CHECK constraint. Guards against
// constants drifting from the schema, where the failure mode is a runtime
// constraint error that callers swallow.
func TestEveryEventTypeIsInsertable(t *testing.T) {
	s := newTestStore(t)

	types := []EventType{
		EventTypeMessage,
		EventTypeTextChunk,
		EventTypeToolCall,
		EventTypeToolResult,
		EventTypeSystem,
		EventTypeError,
	}

	for _, et := range types {
		event := &LedgerEvent{
			ID:              "evt-type-" + string(et),
			ConversationKey: "conv-types",
			Direction:       EventDirectionInbound,
			Author:          "tester",
			Timestamp:       time.Now().UTC(),
			Type:            et,
		}
		if err := s.SaveEvent(context.Background(), event); err != nil {
			t.Errorf("EventType %q is defined in the store package but not insertable: %v", et, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestEveryEventTypeIsInsertable ./internal/store/ -v`
Expected: FAIL with a CHECK constraint error for `EventType "text_chunk"` only.

- [ ] **Step 3: Move the constant out of store**

In `internal/store/events.go`, delete line 33 (`EventTypeTextChunk  EventType = "text_chunk"`) so the block reads:

```go
const (
	EventTypeMessage    EventType = "message"
	EventTypeToolCall   EventType = "tool_call"
	EventTypeToolResult EventType = "tool_result"
	EventTypeSystem     EventType = "system"
	EventTypeError      EventType = "error"
)
```

Add this comment line directly above the `const (` line (keep the existing `// EventType categorizes...` doc comment on the type):

```go
// These values correspond 1:1 with the ledger_events CHECK constraint in
// sqlite.go — adding one here requires a schema migration.
```

In `internal/client/send_message.go`, add near the top of the file (after the import block):

```go
// eventTypeTextChunk tags ephemeral streaming chunks broadcast to live
// watchers. Deliberately NOT part of store's EventType set: it is never
// persisted, and the ledger_events CHECK constraint rejects it.
const eventTypeTextChunk store.EventType = "text_chunk"
```

Then update the five use sites:
- `send_message.go:208`: `Type: store.EventTypeTextChunk,` → `Type: eventTypeTextChunk,`
- `stream_events.go:269`: `if e.Type == store.EventTypeTextChunk {` → `if e.Type == eventTypeTextChunk {`
- `send_message_test.go:518`: `assert.Equal(t, store.EventTypeTextChunk, broadcasts[0].Type)` → `assert.Equal(t, eventTypeTextChunk, broadcasts[0].Type)`
- `stream_events_test.go:211` and `:233`: `Type: store.EventTypeTextChunk,` → `Type: eventTypeTextChunk,`

Finally, remove `EventTypeTextChunk` from the new invariant test's `types` list in `internal/store/sqlite_test.go` (the store package now defines exactly the five persisted types, and the test enumerates all of them).

- [ ] **Step 4: Verify no references remain, run tests**

Run: `grep -rn "EventTypeTextChunk" --include="*.go" .` from the repo root.
Expected: zero matches outside `internal/client` (only the unexported `eventTypeTextChunk`).

Run: `go build ./... && go test ./internal/store/ ./internal/client/ -v`
Expected: PASS, including `TestEveryEventTypeIsInsertable` with the five-type list.

- [ ] **Step 5: Commit**

```bash
git add internal/store/events.go internal/store/sqlite_test.go internal/client/send_message.go internal/client/stream_events.go internal/client/send_message_test.go internal/client/stream_events_test.go
git commit -m "fix(store): keep EventType constants in lockstep with the ledger schema"
```

---

### Task 4: Serialize stream writes; stop pack tools blocking the agent message loop

Two coupled defects in the agent gRPC stream:

1. **Existing data race:** `Connection.Send` (`connection.go:71`) calls `c.stream.Send(msg)` with no lock. HTTP-handler goroutines send via `Connection.Send` while the message-loop goroutine calls `stream.Send` directly (welcome at `grpc.go:252`, pack tool results at `:338`, `:365`). gRPC-Go forbids concurrent `Send` on one stream.
2. **Loop freeze:** `handleExecutePackTool` runs synchronously inside `runMessageLoop` (`grpc.go:157` via `dispatchMessage`), so `stream.Recv()` is not called again until the tool finishes — up to 30s default, or arbitrarily long with `tool.Definition.GetTimeoutSeconds()`. Heartbeats and responses from the agent queue unprocessed; keepalives can sever the connection mid-call.

Fix: give `Connection` a send mutex and make `Connection.Send` the ONLY path to `stream.Send`; route grpc.go's direct sends through the conn; then dispatch `handleExecutePackTool` on its own goroutine (its existing per-call timeout in `packs.Router.RouteToolCall` bounds goroutine lifetime).

**Files:**
- Modify: `internal/agent/connection.go` (add `sendMu sync.Mutex` field; lock in `Send`)
- Modify: `internal/gateway/grpc.go` (welcome send at :252; `dispatchMessage` case at :156-157; `handleExecutePackTool` at :282; `sendPackToolError` at :356)
- Test: `internal/agent/manager_test.go` (append concurrency test)
- Create: `internal/gateway/grpc_test.go` (async dispatch test)

**Interfaces:**
- Consumes: `packs.NewRegistry(logger)`, `Registry.RegisterPack(packID, *pb.PackManifest)`, `packs.NewRouter(packs.RouterConfig{Registry, Logger, Timeout})`, `pb.PackManifest{PackId, Version, Tools}`, `pb.ToolDefinition{Name}`, `Gateway.packRouter` field (unexported, but tests are package `gateway`).
- Produces: `handleExecutePackTool(ctx context.Context, conn *agent.Connection, req *pb.ExecutePackTool)` (was `(stream, conn, req)`); `sendPackToolError(conn *agent.Connection, requestID, errMsg string)` (was `(stream, ...)`). `Connection.Send` is now goroutine-safe and is the single `stream.Send` path.

- [ ] **Step 1: Write the failing send-serialization test**

Append to `internal/agent/manager_test.go` (add `"sync"` and `"sync/atomic"` to imports if not present):

```go
// concurrencyDetectingStream fails the test if two goroutines are ever
// inside Send simultaneously — gRPC server streams forbid concurrent Send.
type concurrencyDetectingStream struct {
	mockStream
	inFlight atomic.Int32
	overlap  atomic.Bool
}

func (s *concurrencyDetectingStream) Send(msg *pb.ServerMessage) error {
	if s.inFlight.Add(1) > 1 {
		s.overlap.Store(true)
	}
	time.Sleep(time.Millisecond) // widen the race window
	s.inFlight.Add(-1)
	return nil
}

func TestConnectionSendSerializesStreamWrites(t *testing.T) {
	stream := &concurrencyDetectingStream{}
	conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Stream: stream, Logger: slog.Default()})

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if err := conn.Send(&pb.ServerMessage{}); err != nil {
					t.Errorf("Send failed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if stream.overlap.Load() {
		t.Fatal("concurrent stream.Send detected: Connection.Send must serialize writes")
	}
}
```

Note: `concurrencyDetectingStream` embeds `mockStream` by value to inherit the rest of the `pb.CovenControl_AgentStreamServer` interface; the `Send` override above shadows `mockStream.Send`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -run TestConnectionSendSerializesStreamWrites ./internal/agent/ -v`
Expected: FAIL with `concurrent stream.Send detected`.

- [ ] **Step 3: Add the send mutex**

In `internal/agent/connection.go`:

Add a field to the `Connection` struct (after the existing `mu sync.RWMutex` field at line 27):

```go
	sendMu  sync.Mutex // serializes stream.Send: gRPC forbids concurrent Send on one stream
```

Replace the body of `Send` (lines 71-76):

```go
func (c *Connection) Send(msg *pb.ServerMessage) error {
	if c.stream == nil {
		return ErrNilStream
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.stream.Send(msg)
}
```

(Do NOT reuse `c.mu` — that guards the pending map and must not be held across network I/O.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/agent/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit the agent half**

```bash
git add internal/agent/connection.go internal/agent/manager_test.go
git commit -m "fix(agent): serialize stream writes through Connection.Send"
```

- [ ] **Step 6: Write the failing async-dispatch test**

Create `internal/gateway/grpc_test.go`:

```go
// ABOUTME: Tests for the CovenControl gRPC service message dispatch.
// ABOUTME: Proves pack tool execution does not block the agent message loop.

package gateway

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/packs"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// fakeAgentStream implements pb.CovenControl_AgentStreamServer for tests.
// Only Context, Send, and Recv are exercised; the embedded nil
// grpc.ServerStream panics if anything else is called, which is what we want.
type fakeAgentStream struct {
	grpc.ServerStream
	ctx  context.Context
	mu   sync.Mutex
	sent []*pb.ServerMessage
}

func (f *fakeAgentStream) Context() context.Context { return f.ctx }

func (f *fakeAgentStream) Send(msg *pb.ServerMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeAgentStream) Recv() (*pb.AgentMessage, error) {
	<-f.ctx.Done()
	return nil, io.EOF
}

func (f *fakeAgentStream) sentMessages() []*pb.ServerMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*pb.ServerMessage(nil), f.sent...)
}

func TestExecutePackToolDoesNotBlockMessageLoop(t *testing.T) {
	logger := slog.Default()

	// A registered pack whose channel nobody serves: RouteToolCall blocks
	// until the router timeout, deterministically.
	registry := packs.NewRegistry(logger)
	manifest := &pb.PackManifest{
		PackId:  "slowpack",
		Version: "1.0.0",
		Tools:   []*pb.ToolDefinition{{Name: "slow_tool"}},
	}
	if err := registry.RegisterPack("slowpack", manifest); err != nil {
		t.Fatalf("RegisterPack failed: %v", err)
	}
	router := packs.NewRouter(packs.RouterConfig{
		Registry: registry,
		Logger:   logger,
		Timeout:  300 * time.Millisecond,
	})

	srv := &covenControlServer{
		gateway: &Gateway{packRouter: router},
		logger:  logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := &fakeAgentStream{ctx: ctx}
	conn := agent.NewConnection(agent.ConnectionParams{ID: "agent-1", Name: "Test Agent", Stream: stream, Logger: logger})

	start := time.Now()
	srv.dispatchMessage(stream, conn, &pb.AgentMessage{
		Payload: &pb.AgentMessage_ExecutePackTool{
			ExecutePackTool: &pb.ExecutePackTool{
				RequestId: "req-1",
				ToolName:  "slow_tool",
			},
		},
	})
	elapsed := time.Since(start)

	// The message loop must get control back immediately, not after the
	// tool's 300ms timeout.
	if elapsed > 100*time.Millisecond {
		t.Fatalf("dispatchMessage blocked %v on a pack tool; the message loop must not stall", elapsed)
	}

	// The (timeout-error) result must still reach the agent, eventually.
	deadline := time.After(2 * time.Second)
	for {
		var result *pb.PackToolResult
		for _, msg := range stream.sentMessages() {
			if r := msg.GetPackToolResult(); r != nil {
				result = r
				break
			}
		}
		if result != nil {
			if got := result.GetRequestId(); got != "req-1" {
				t.Fatalf("PackToolResult for request %q, want %q", got, "req-1")
			}
			if result.GetError() == "" {
				t.Fatal("expected a timeout error in PackToolResult")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("PackToolResult never sent to agent")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
```

- [ ] **Step 7: Run test to verify it fails**

Run: `go test -run TestExecutePackToolDoesNotBlockMessageLoop ./internal/gateway/ -v`
Expected: FAIL with `dispatchMessage blocked ~300ms on a pack tool` (the synchronous call waits out the router timeout).

- [ ] **Step 8: Dispatch pack tools asynchronously via conn.Send**

In `internal/gateway/grpc.go`, four changes:

(a) `dispatchMessage` (line 156-157) — run the handler on its own goroutine, passing the stream context:

```go
	case *pb.AgentMessage_ExecutePackTool:
		// Pack tools can run for up to their per-call timeout; never block
		// the receive loop on them. All stream writes go through conn.Send,
		// which serializes concurrent senders.
		go s.handleExecutePackTool(stream.Context(), conn, payload.ExecutePackTool)
```

(b) `handleExecutePackTool` (line 282) — drop the stream parameter, take a context, send via the connection:

```go
func (s *covenControlServer) handleExecutePackTool(ctx context.Context, conn *agent.Connection, req *pb.ExecutePackTool) {
```

Inside it: replace `stream.Context()` with `ctx` in the `RouteToolCall` call; replace `s.sendPackToolError(stream, ...)` with `s.sendPackToolError(conn, ...)` (both call sites); replace `stream.Send(result)` with `conn.Send(result)`. No other logic changes.

(c) `sendPackToolError` (line 356) — send via the connection:

```go
func (s *covenControlServer) sendPackToolError(conn *agent.Connection, requestID, errMsg string) {
	result := &pb.ServerMessage{
		Payload: &pb.ServerMessage_PackToolResult{
			PackToolResult: &pb.PackToolResult{
				RequestId: requestID,
				Result:    &pb.PackToolResult_Error{Error: errMsg},
			},
		},
	}
	if err := conn.Send(result); err != nil {
		s.logger.Error("failed to send pack tool error",
			"request_id", requestID,
			"error", err,
		)
	}
}
```

(d) The welcome send in `AgentStream` (line 252) — route through the connection so ALL writes share the send mutex:

```go
	if err := conn.Send(welcome); err != nil {
		return status.Errorf(codes.Internal, "sending welcome: %v", err)
	}
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test -race ./internal/gateway/ ./internal/agent/ ./internal/packs/ -v`
Expected: PASS, including `TestExecutePackToolDoesNotBlockMessageLoop` (dispatch returns in microseconds; timeout result arrives within ~300ms).

- [ ] **Step 10: Full suite and commit**

Run: `go build ./... && go test ./...`
Expected: PASS.

```bash
git add internal/gateway/grpc.go internal/gateway/grpc_test.go
git commit -m "fix(gateway): stop pack tool execution blocking the agent message loop"
```

---

## Out of scope (documented, deliberate)

- **`conversation/service.go` drain goroutine** (investigation §Additional-2): it drains until `outChan` closes; Task 1 guarantees `transformResponses` always returns and closes `outChan`, so the amplified leak dies with R-1. No separate change.
- **`migrateMessagesToEvents` not transactional** (§Additional-3): idempotent via `WHERE NOT EXISTS`; recovery is automatic on restart. Harden only if migrations grow.
- **MCP `/mcp` auth**: deliberately unauthenticated — tailnet is the auth boundary (Harper, 2026-07-08). Do not "fix".
