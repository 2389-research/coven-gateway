# Development Guide

This guide covers setting up a development environment and contributing to coven-gateway.

## Prerequisites

- Go 1.22+
- `protoc` (Protocol Buffers compiler) - only needed if modifying proto files
- SQLite (bundled, no separate install needed)
- Git with submodule support

## Quick Setup

```bash
# Clone with submodules
git clone --recursive https://github.com/2389-research/coven-gateway.git
cd coven-gateway

# Install protoc plugins (one-time)
make proto-deps

# Build everything
make

# Copy and edit config
cp config.example.yaml config.yaml
```

## Project Structure

```text
coven-gateway/
├── cmd/                      # Binary entry points
│   ├── coven-gateway/        # Main server
│   └── coven-admin/          # Admin CLI
├── internal/                 # Private packages
│   ├── gateway/              # Server orchestration, HTTP handlers
│   ├── agent/                # gRPC agent management
│   ├── store/                # SQLite persistence
│   ├── conversation/         # Conversation service
│   ├── packs/                # Tool pack system
│   ├── builtins/             # Built-in tool packs
│   ├── mcp/                  # MCP server
│   ├── webadmin/             # Web admin UI
│   ├── auth/                 # Authentication
│   ├── config/               # Configuration loading
│   ├── dedupe/               # Message deduplication
│   ├── contract/             # Protocol contract tests
│   ├── client/               # gRPC client
│   └── admin/                # Admin operations
├── proto/
│   └── coven-proto/          # Protobuf definitions (git submodule)
├── docs/                     # Documentation
└── internal/webadmin/
    ├── templates/            # HTML templates
    └── docs/                 # Help documentation
```

## Build Commands

```bash
make                    # Generate proto + build all binaries
make build              # Build binaries only (skip proto regen)
make proto              # Regenerate protobuf code
make proto-deps         # Install protoc plugins
make clean              # Remove build artifacts
```

## Running Locally

### Start the Gateway

```bash
./bin/coven-gateway serve

# With debug logging
./bin/coven-gateway serve --log-level debug
```

The gateway starts:
- gRPC server on `0.0.0.0:50051` (agent connections)
- HTTP server on `0.0.0.0:8080` (API, web admin)

### Connect a Test Agent

From the [coven](https://github.com/2389-research/coven) repository:

```bash
cargo run -p coven-agent -- --server http://127.0.0.1:50051 --name "test-agent"
```

### Access Web Admin

Open `http://localhost:8080` in your browser.

For initial setup:
```bash
./bin/coven-gateway bootstrap --name "Admin"
```

This creates an admin user and prints a setup URL.

## Testing

```bash
# All tests
go test ./...

# With race detection (recommended for CI)
go test -race ./...

# Single package
go test ./internal/gateway/...

# Single test with verbose output
go test -v -run TestSendMessage ./...

# With coverage
go test -cover ./...
```

### Test Patterns

- Unit tests use `store.NewMockStore()` for in-memory storage
- Integration tests use real SQLite with `:memory:` path
- Contract tests verify Go↔Rust proto compatibility

### Important Test Files

- `internal/gateway/api_test.go` - HTTP handler tests
- `internal/store/store_test.go` - Store interface tests
- `internal/gateway/proto_contract_test.go` - Protocol compatibility

## Code Style

### Formatting and Linting

```bash
go fmt ./...
golangci-lint run
```

### Patterns to Follow

**Structured logging:**
```go
logger := slog.Default()
logger.Info("message sent",
    "agent_id", agentID,
    "request_id", requestID,
)
```

**Error wrapping:**
```go
if err != nil {
    return fmt.Errorf("failed to send message: %w", err)
}
```

**Context propagation:**
```go
func DoSomething(ctx context.Context, ...) error {
    // Pass ctx to all downstream calls
    result, err := store.GetThread(ctx, id)
}
```

**Interface-driven testing:**
```go
type Store interface {
    GetThread(ctx context.Context, id string) (*Thread, error)
    // ...
}

// MockStore implements Store for testing
```

## Working with Protobufs

### Updating the Protocol

1. Edit `proto/coven-proto/coven.proto`
2. Regenerate Go code: `make proto`
3. Update the Rust side in the coven repository
4. Run contract tests: `go test ./internal/contract/...`

### Protocol Compatibility

The gateway and agents must agree on the protocol. Contract tests in `internal/contract/` verify that Go marshalling matches Rust.

## Debugging

### Enable Debug Logging

```yaml
# config.yaml
logging:
  level: "debug"
  format: "text"
```

### View Structured Logs

```bash
./bin/coven-gateway serve 2>&1 | jq 'select(.level == "ERROR")'
```

### Common Issues

**"address already in use":**
```bash
lsof -i :8080  # Find the process
kill <pid>     # Or change the port
```

**"database is locked":**
- Ensure only one gateway instance per database
- Don't use network filesystems for the database

**Agent won't connect:**
1. Check gateway is running: `./bin/coven-gateway health`
2. Verify gRPC port is accessible
3. Check agent registration mode in config

## Database Migrations

Migrations are embedded in the store package and run automatically on startup.

To add a new migration:
1. Create `internal/store/migrations/NNNN_description.sql`
2. The migration runs in order based on the numeric prefix
3. Test with a fresh database

## Contributing

1. Create a branch: `git checkout -b feature/my-feature`
2. Make changes with tests
3. Run `go fmt ./...` and `golangci-lint run`
4. Ensure tests pass: `go test -race ./...`
5. Commit with descriptive message
6. Open a pull request

### Commit Messages

Follow conventional commits:
- `feat: add new feature`
- `fix: resolve bug`
- `docs: update documentation`
- `refactor: restructure code`
- `test: add tests`
