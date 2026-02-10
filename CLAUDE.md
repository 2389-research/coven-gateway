# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

coven-gateway is the production control plane for coven agents. It manages coven-agent connections via gRPC, routes messages from frontends (HTTP clients, Matrix) to agents, and streams responses back in real-time.

The coven-agent is a separate Rust project at `../coven-agent` that connects to this gateway. The shared protobuf definition is at `../coven-agent/proto/coven.proto`.

## Getting Started

**Prerequisites:** Go 1.22+, `protoc` (only for proto regeneration)

**First-time setup:**
```bash
make proto-deps    # Install protoc plugins (one-time)
make               # Generate proto + build all binaries
```

**Running the gateway:**
```bash
cp config.example.yaml config.yaml    # Create config from template
./bin/coven-gateway serve              # Start (gRPC + HTTP)
```

**Key files to read first:**
- `internal/gateway/gateway.go` — Orchestrator: owns gRPC server, HTTP server, agent manager, and store
- `internal/agent/manager.go` — Agent lifecycle: tracks connections, routes messages
- `internal/gateway/api.go` — HTTP handlers: SSE streaming, message sending, bindings
- `internal/store/events.go` — Persistence: ledger events, threads, messages
- `cmd/coven-gateway/main.go` — Entry point: config loading, server startup

**Common patterns:**
- `slog` for structured logging (stdlib, not third-party)
- Context propagation for cancellation throughout
- Interface-driven testability (`Store`, `messageSender`)
- Error wrapping: `fmt.Errorf("context: %w", err)`

## Build Commands

```bash
make                    # Generate proto + build all binaries
make build              # Build all binaries (without proto regen)
make proto              # Regenerate protobuf from ../coven-agent/proto/coven.proto
make proto-deps         # Install protoc plugins (one-time)
```

Individual binaries:
```bash
go build -o bin/coven-gateway ./cmd/coven-gateway
go build -o bin/coven-tui ./cmd/coven-tui
go build -o bin/coven-admin ./cmd/coven-admin
```

## Testing

```bash
go test ./...                           # All tests
go test -race ./...                     # With race detection
go test ./internal/agent/...            # Single package
go test -v -run TestSendMessage ./...   # Single test
```

## Lint & Format

```bash
go fmt ./...
golangci-lint run
```

## Running

```bash
cp config.example.yaml config.yaml      # Copy config template
./bin/coven-gateway serve                # Start gateway (uses COVEN_CONFIG or ~/.config/coven/gateway.yaml)
./bin/coven-tui                          # Interactive TUI client
./bin/coven-admin -watch                 # Monitor status
```

## Architecture

```
                      ┌─────────────────────────────────────────┐
                      │              coven-gateway               │
                      │                                         │
  HTTP Clients ─────► │  api.go ──► Manager ──► Connection(s)   │ ◄──── coven-agent(s)
  (SSE streaming)     │             │                           │       (gRPC stream)
                      │             ▼                           │
                      │          Store (SQLite)                 │
                      └─────────────────────────────────────────┘
```

**Key internal packages:**

- `internal/gateway/` - Orchestrator: Gateway struct owns gRPC server, HTTP server, agent manager, and store
- `internal/agent/` - Manager tracks connected agents, Connection handles individual agent streams and pending request/response correlation
- `internal/store/` - SQLite persistence for threads, messages, and channel bindings

**Message flow:**

1. HTTP client POSTs to `/api/send` with JSON body
2. `api.go` looks up agent (via binding or direct ID)
3. Manager.SendMessage creates request, sends via Connection's gRPC stream
4. Connection correlates responses by request_id, forwards to response channel
5. `api.go` streams responses as SSE events to client

**Binaries:**

- `coven-gateway` - Main server (gRPC + HTTP)
- `coven-tui` - Interactive terminal client for testing
- `coven-admin` - CLI for monitoring gateway status

## gRPC Protocol

The `CovenControl` service uses bidirectional streaming. Agents send `AgentMessage` (register, heartbeat, response events), gateway sends `ServerMessage` (welcome, send_message, shutdown).

Response events flow: Thinking → Text chunks → ToolUse/ToolResult → Done/Error

## Testing Patterns

**Test organization:**

- `internal/gateway/proto_contract_test.go` - Proto serialization round-trip tests (Go↔Rust compatibility)
- `internal/store/store_test.go` - Store interface tests using real SQLite
- `internal/store/mock_store.go` - Thread-safe in-memory MockStore for unit tests
- `internal/gateway/api_test.go` - HTTP handler and extracted function tests

**MockStore usage:**

```go
import "github.com/2389/coven-gateway/internal/store"

func TestSomething(t *testing.T) {
    mockStore := store.NewMockStore()
    // mockStore implements store.Store interface
}
```

**Extracted functions in api.go:**

- `parseSendRequest(r io.Reader)` - Parse and validate JSON request body
- `bindingResolver.Resolve(ctx, frontend, channelID, threadID)` - Resolve bindings to thread IDs
- `formatSSEEvent(eventType, data string)` - Format SSE event strings

## Code Style

- Use `slog` for structured logging (stdlib, not third-party)
- Context propagation for cancellation throughout
- Interfaces for testability (Store, messageSender)
- Error wrapping: `fmt.Errorf("context: %w", err)`
