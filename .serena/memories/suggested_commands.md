# Suggested Commands

## Build Commands
```bash
make                    # Generate proto + build all binaries
make build              # Build all binaries (without proto regen)
make proto              # Regenerate protobuf from proto/coven.proto
make proto-deps         # Install protoc plugins (one-time)
```

## Individual Binaries
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
./bin/coven-gateway serve               # Start gateway
./bin/coven-tui                         # Interactive TUI client
./bin/coven-admin -watch                # Monitor status
```

## Environment Variables
- `COVEN_CONFIG` - Path to config file (default: ~/.config/coven/gateway.yaml)
