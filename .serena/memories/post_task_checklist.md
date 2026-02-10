# Post-Task Checklist

After completing any code changes, run these commands:

## 1. Format Code
```bash
go fmt ./...
```

## 2. Run Linter
```bash
golangci-lint run
```

## 3. Run Tests
```bash
go test ./...
```

## 4. Build Verification
```bash
make build
```

## For Proto Changes
If you modified `proto/coven.proto`:
```bash
make proto
```
Then rebuild and test.

## Before Committing
- Ensure all tests pass
- No linter warnings
- Code is formatted
- ABOUTME comments updated if file purpose changed
