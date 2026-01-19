# ABOUTME: Build and development commands for fold-gateway
# ABOUTME: Handles proto generation, building, and testing

.PHONY: all build build-gateway build-matrix build-admin build-tui proto update-proto clean test lint fmt run

# Default target
all: proto build

# Build all binaries
build: build-gateway build-matrix build-admin build-tui

build-gateway:
	go build -o bin/fold-gateway ./cmd/fold-gateway

build-matrix:
	go build -tags goolm -o bin/fold-matrix ./cmd/fold-matrix

build-admin:
	go build -o bin/fold-admin ./cmd/fold-admin

build-tui:
	go build -o bin/fold-tui ./cmd/fold-tui

# Generate protobuf code from shared proto submodule
proto:
	@mkdir -p proto/fold
	protoc \
		--go_out=. \
		--go_opt=module=github.com/2389/fold-gateway \
		--go_opt=Mfold.proto=github.com/2389/fold-gateway/proto/fold \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/2389/fold-gateway \
		--go-grpc_opt=Mfold.proto=github.com/2389/fold-gateway/proto/fold \
		-I proto/fold-proto \
		proto/fold-proto/fold.proto

# Update proto submodule and regenerate
update-proto:
	git submodule update --remote proto/fold-proto
	$(MAKE) proto
	@echo "Proto updated from fold-proto submodule"

# Install protoc plugins (run once)
proto-deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Run the server
run: build
	FOLD_CONFIG=config.yaml ./bin/fold-gateway serve

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
