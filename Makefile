# ABOUTME: Build and development commands for coven-gateway
# ABOUTME: Handles proto generation, building, and testing

.PHONY: all build build-gateway build-admin build-tui proto update-proto clean test lint fmt run setup hooks

# Default target
all: proto build

# Build all binaries
build: build-gateway build-admin build-tui

build-gateway:
	go build -o bin/coven-gateway ./cmd/coven-gateway

build-admin:
	go build -o bin/coven-admin ./cmd/coven-admin

build-tui:
	go build -o bin/coven-tui ./cmd/coven-tui

# Generate protobuf code from shared proto submodule
proto:
	@mkdir -p proto/coven
	protoc \
		--go_out=. \
		--go_opt=module=github.com/2389/coven-gateway \
		--go_opt=Mcoven.proto=github.com/2389/coven-gateway/proto/coven \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/2389/coven-gateway \
		--go-grpc_opt=Mcoven.proto=github.com/2389/coven-gateway/proto/coven \
		-I proto/coven-proto \
		proto/coven-proto/coven.proto

# Update proto submodule and regenerate
update-proto:
	git submodule update --remote proto/coven-proto
	$(MAKE) proto
	@echo "Proto updated from coven-proto submodule"

# Install protoc plugins (run once)
proto-deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Run the server
run: build
	COVEN_CONFIG=config.yaml ./bin/coven-gateway serve

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
	rm -rf proto/coven/*.pb.go

# Development: watch and rebuild
dev:
	@echo "Run: go run ./cmd/coven-gateway serve --config config.yaml"

# Install git hooks (symlink so updates are automatic)
hooks:
	@echo "Installing pre-commit hook..."
	@ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	@echo "Pre-commit hook installed!"

# Setup development environment (run after cloning)
setup: proto-deps hooks
	@git submodule update --init --recursive
	@echo "Development environment ready!"
