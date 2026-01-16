# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

fold-gateway is the production control plane for fold agents. It manages fold-agent connections via gRPC, routes messages from frontends (HTTP clients, Matrix) to agents, and streams responses back in real-time.

The fold-agent is a separate Rust project at `../fold-agent` that connects to this gateway. The shared protobuf definition is at `../fold-agent/proto/fold.proto`.

## Build Commands

```bash
make                    # Generate proto + build all binaries
make build              # Build all binaries (without proto regen)
make proto              # Regenerate protobuf from ../fold-agent/proto/fold.proto
make proto-deps         # Install protoc plugins (one-time)
```

Individual binaries:
```bash
go build -o bin/fold-gateway ./cmd/fold-gateway
go build -o bin/fold-tui ./cmd/fold-tui
go build -o bin/fold-admin ./cmd/fold-admin
go build -tags goolm -o bin/fold-matrix ./cmd/fold-matrix  # requires goolm tag
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
./bin/fold-gateway serve                # Start gateway (uses FOLD_CONFIG or ~/.config/fold/gateway.yaml)
./bin/fold-tui                          # Interactive TUI client
./bin/fold-admin -watch                 # Monitor status
```

## Architecture

```
                      ┌─────────────────────────────────────────┐
                      │              fold-gateway               │
                      │                                         │
  HTTP Clients ─────► │  api.go ──► Manager ──► Connection(s)   │ ◄──── fold-agent(s)
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

- `fold-gateway` - Main server (gRPC + HTTP)
- `fold-tui` - Interactive terminal client for testing
- `fold-admin` - CLI for monitoring gateway status
- `fold-matrix` - Standalone Matrix bridge (separate config: config.toml)

## gRPC Protocol

The `FoldControl` service uses bidirectional streaming. Agents send `AgentMessage` (register, heartbeat, response events), gateway sends `ServerMessage` (welcome, send_message, shutdown).

Response events flow: Thinking → Text chunks → ToolUse/ToolResult → Done/Error

## Code Style

- Use `slog` for structured logging (stdlib, not third-party)
- Context propagation for cancellation throughout
- Interfaces for testability (Store, messageSender)
- Error wrapping: `fmt.Errorf("context: %w", err)`
