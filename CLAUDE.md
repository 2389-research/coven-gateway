# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

coven-gateway is the production control plane for coven agents. It manages coven-agent connections via gRPC, routes messages from frontends (HTTP clients, Matrix) to agents, and streams responses back in real-time.

The coven ecosystem spans multiple repositories:
- **coven-gateway** (this repo) - Go control plane server
- **[coven](https://github.com/2389-research/coven)** - Rust agent platform (`coven-agent`, `coven-tui`)
- **[coven-proto](https://github.com/2389-research/coven-proto)** - Shared protobuf definitions (git submodule at `proto/coven-proto`)

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
make proto              # Regenerate protobuf from proto/coven-proto/coven.proto
make proto-deps         # Install protoc plugins (one-time)
```

Individual binaries:
```bash
go build -o bin/coven-gateway ./cmd/coven-gateway
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
./bin/coven-admin status                 # Check gateway status
```

For the interactive TUI client, install from the [coven](https://github.com/2389-research/coven) Rust project.

## Architecture

```text
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

**Binaries (this repo):**

- `coven-gateway` - Main server (gRPC + HTTP)
- `coven-admin` - CLI for gateway administration

**Related binaries (from [coven](https://github.com/2389-research/coven) repo):**

- `coven-tui` - Interactive terminal client
- `coven-agent` - Agent that connects to this gateway

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

## Error Handling Guidelines

**Never ignore errors silently:**
- Always check and handle errors from `json.Unmarshal`, `time.Parse`, `json.NewEncoder().Encode()`
- Use `parseTimeWithWarning()` helper in store package for time parsing with logged warnings
- Log errors at minimum; return them when possible

**JSON encoding/decoding:**
- Check `json.NewEncoder(w).Encode()` errors and log them
- For JSON parsing from untrusted input, consider `DisallowUnknownFields()` for strict validation

## Concurrency Guidelines

**Channel safety:**
- Only the producer should close a channel
- Use `sync.Once` to guard channel close when multiple goroutines might attempt closure
- Buffered channels (size 1+) prevent blocking on single-message scenarios

**Timer management:**
- Never use `time.After()` in loops (creates new timer each iteration, memory leak)
- Use a reusable `time.Timer` with `Reset()` and proper `Stop()` + drain pattern:
  ```go
  timer := time.NewTimer(5 * time.Second)
  defer timer.Stop()
  for item := range items {
      if !timer.Stop() {
          select { case <-timer.C: default: }
      }
      timer.Reset(5 * time.Second)
      // use timer...
  }
  ```

**Nil checks:**
- Always validate that injected dependencies (logger, stream, store) are non-nil
- Provide sensible defaults: `if logger == nil { logger = slog.Default() }`

## ID Generation

**Use centralized ID generation:**
- Principal IDs: Use `generatePrincipalID()` in `internal/admin/principals.go`
- General UUIDs: Use `uuid.New().String()` from `github.com/google/uuid`
- Never create timestamp-only IDs (collision risk under concurrent load)

## Security-Critical Packages

The following packages handle authentication, authorization, and identity management.
They have **mandatory test coverage requirements** enforced by CI:

| Package | Min Coverage | Purpose |
|---------|--------------|---------|
| `internal/admin` | 70% | Principal CRUD, bindings, tokens |
| `internal/auth` | 60% | Authentication, JWT, SSH verification |

**When modifying these packages:**
- All new exported functions must have corresponding tests
- Test both success paths and error/validation paths
- Include audit log verification where applicable
- PR reviewers should verify test coverage before approving

## Frontend Redesign (Active Migration)

IMPORTANT: When working on anything in `web/`, any frontend redesign deliverable, any phase file, or any task referencing the frontend migration — you MUST first read `docs/plans/frontend-redesign/RUNBOOK.md` and follow its session patterns. The runbook defines how to manage context across sessions to prevent smearing. Do not freestyle.

**Plans directory:** `docs/plans/frontend-redesign/`
**Design doc:** `docs/plans/2026-02-17-frontend-redesign-design.md` (reference for tokens, component APIs, build pipeline)
**Current phase:** 2 (Design System Core — Build What Chat Needs)
**Runbook:** `docs/plans/frontend-redesign/RUNBOOK.md`

### Frontend Build Commands (from `web/` directory)
```bash
npm ci                              # Install deps
npm run dev                         # Vite dev server with HMR
npm run build                       # Production build to web/dist/
npx tsx scripts/build-tokens.ts     # Regenerate variables.css from tokens.json
make web                            # Build frontend (from repo root)
make web-dev                        # Start Vite dev server (from repo root)
```

### Frontend Testing (from `web/` directory)
```bash
npm test                            # Vitest unit tests
npx playwright test                 # E2E tests
npm run storybook                   # Component stories
```

### Session Rules
- One batch of deliverables (1-3) per session. Do not combine unrelated work.
- Use subagents for codebase investigation — preserve main context for implementation.
- Always read the phase file at the start of a session — it IS your context.
- After completing a batch, stop and report. Do not continue to the next batch.

### Phase Learnings

**Phase 1 (Foundation)** — Completed 2026-02-18, PR #47

- Tailwind v4 works via `@tailwindcss/vite` plugin + `@import 'tailwindcss'`. No `@theme` directive or `tailwind.config.js` needed.
- Vite uses base64url hashes (not hex). Hash regex: `[a-zA-Z0-9_-]{8,}`.
- HTMX events `beforeSwap`, `beforeCleanupElement`, `afterSwap`, `load` cover all mount/unmount cases. `beforeCleanupElement` (not `beforeCleanup`) catches individual element removals.
- WeakSet concurrent mount guard needed — `afterSwap` and `load` can fire for the same mutation.
- Bundle budget: 15KB realistic floor (Svelte runtime ~8KB gzip). 10KB was unrealistic.
- Storybook 8.6 is current stable with Svelte 5 support (not 9).
- Inter variable font: 352KB woff2. Fixed cost in binary.
- Makefile `web-deps` target ensures `npx tsx` uses pinned deps, not globally-installed versions.
- `X-Accel-Buffering: no` required on SSE endpoints for reverse proxy compatibility.
- CSS `@import` must come before any other rules (including `@font-face`) per spec, or imports are silently ignored.
- golangci-lint version drift (local vs CI) resolved via global gosec excludes in `.golangci.yml`.
