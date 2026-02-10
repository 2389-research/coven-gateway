# Code Style & Conventions

## Logging
- Use `slog` (stdlib structured logging) - NOT third-party loggers
- Example: `logger.Info("message", "key", value)`

## Context
- Context propagation throughout for cancellation
- First parameter to functions that do I/O

## Interfaces
- Define interfaces for testability (Store, messageSender, etc.)
- Mock implementations in `*_mock.go` or `mock_*.go` files

## Error Handling
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Define sentinel errors: `var ErrNotFound = errors.New("not found")`
- Check with `errors.Is(err, ErrNotFound)`

## File Headers
- Use ABOUTME comments at top of files:
```go
// ABOUTME: Brief description of file purpose
// ABOUTME: Second line if needed
```

## Naming
- Standard Go conventions (CamelCase exports, camelCase private)
- Package names are lowercase, single word preferred
- Interface names don't use "I" prefix

## Package Organization
- `internal/` for private packages
- `cmd/` for main packages
- `proto/` for protobuf definitions
