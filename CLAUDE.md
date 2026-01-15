# fold-gateway

Production control plane for fold agents, written in Go.

## Project Context

This is the Go implementation of the fold control server. It replaces the test TUI server in `../fold/crates/fold-server` with a production-ready gateway that supports:

- Multiple fold-agent connections via GRPC
- Slack and Matrix frontend integrations
- Message routing with affinity (same thread -> same agent)
- Thread persistence in SQLite

## Related Projects

- `../fold` - Rust workspace containing:
  - `fold-core` - Core library for Claude interaction
  - `fold-agent` - GRPC client that connects to this gateway
  - `fold-server` - Test TUI server (reference implementation)
- `../fold/proto/fold.proto` - GRPC protocol definition (shared)

## Architecture

See SPEC.md for full details. Key components:

```
cmd/fold-gateway/main.go     - Entry point
internal/gateway/gateway.go  - Main orchestrator
internal/agent/manager.go    - Agent connection management
internal/agent/router.go     - Message routing
internal/frontend/           - Slack, Matrix, Web frontends
internal/store/              - SQLite persistence
internal/config/             - Configuration
```

## Development

```bash
# Generate protobuf
protoc --go_out=. --go-grpc_out=. ../fold/proto/fold.proto

# Run
go run ./cmd/fold-gateway serve --config config.yaml

# Test with fold-agent
cd ../fold && cargo run -p fold-agent -- --server http://127.0.0.1:50051
```

## Commands

```bash
# Format
go fmt ./...

# Lint
golangci-lint run

# Test
go test ./...

# Build
go build -o bin/fold-gateway ./cmd/fold-gateway
```

## Code Style

- Use `slog` for structured logging (stdlib)
- Context propagation for cancellation
- Prefer interfaces for testability
- Error wrapping with `fmt.Errorf("...: %w", err)`
