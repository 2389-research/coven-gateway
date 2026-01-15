# ABOUTME: Build and development commands for fold-gateway
# ABOUTME: Handles proto generation, building, and testing

.PHONY: all build proto clean test lint fmt run

# Default target
all: proto build

# Build the binary
build:
	go build -o bin/fold-gateway ./cmd/fold-gateway

# Generate protobuf code from shared proto
proto:
	@mkdir -p proto/fold
	protoc \
		--go_out=. \
		--go_opt=module=github.com/2389/fold-gateway \
		--go_opt=Mfold.proto=github.com/2389/fold-gateway/proto/fold \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/2389/fold-gateway \
		--go-grpc_opt=Mfold.proto=github.com/2389/fold-gateway/proto/fold \
		-I ../fold/proto \
		../fold/proto/fold.proto

# Install protoc plugins (run once)
proto-deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Run the server
run: build
	./bin/fold-gateway serve --config config.yaml

# Run tests
test:
	go test -v ./...

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf proto/fold/*.pb.go

# Development: watch and rebuild
dev:
	@echo "Run: go run ./cmd/fold-gateway serve --config config.yaml"
